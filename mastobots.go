package mastobots

import (
	"context"
	"log"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/comail/colog"
	"github.com/spf13/viper"
)

var (
	version       = "1"
	revision      = "0"
	maxRetry      = 5
	retryInterval = time.Duration(5) * time.Second
	locationCodes map[string]interface{}
	openCageKey   = ""
)

// Initialize は、config.ymlに従ってbotとデータベース接続を初期化する。
func Initialize() (bots []*Persona, db DB, err error) {
	// colog 設定
	if version == "" {
		colog.SetDefaultLevel(colog.LDebug)
		colog.SetMinLevel(colog.LTrace)
		colog.SetFormatter(&colog.StdFormatter{
			Colors: true,
			Flag:   log.Ldate | log.Ltime | log.Lshortfile,
		})
	} else {
		colog.SetDefaultLevel(colog.LDebug)
		colog.SetMinLevel(colog.LInfo)
		colog.SetFormatter(&colog.StdFormatter{
			Colors: true,
			Flag:   log.Ldate | log.Ltime,
		})
	}
	colog.Register()

	// 依存アプリの存在確認
	for _, cmd := range []string{"jumanpp", "mysql"} {
		_, err := exec.LookPath(cmd)
		if err != nil {
			log.Printf("alert: %s がインストールされていません！", cmd)
			return nil, db, err
		}
	}

	// 天気予報の地域コード読み込み
	locationCodes, err = getLocationCodes()
	if err != nil {
		return
	}

	var appName string
	var apps []*MastoApp
	var cr map[string]string

	// bot設定ファイル読み込み
	conf := viper.New()
	conf.SetConfigName("config")
	conf.AddConfigPath(".")
	conf.SetConfigType("yaml")
	if err := conf.ReadInConfig(); err != nil {
		log.Printf("alert: 設定ファイルが読み込めませんでした")
		return nil, db, err
	}
	appName = conf.GetString("MastoAppName")
	openCageKey = conf.GetString("OpenCageKey")
	conf.UnmarshalKey("Personae", &bots)
	cr = conf.GetStringMapString("DBCredentials")

	// マストドンアプリ設定ファイル読み込み
	file, err := os.OpenFile("apps.yml", os.O_CREATE, 0666)
	if err != nil {
		log.Printf("alert: アプリ設定ファイルが作成できませんでした")
		return nil, db, err
	}
	file.Close()
	appConf := viper.New()
	appConf.AddConfigPath(".")
	appConf.SetConfigName("apps")
	appConf.SetConfigType("yaml")
	if err := appConf.ReadInConfig(); err != nil {
		log.Printf("alert: アプリ設定ファイルが読み込めませんでした")
		return nil, db, err
	}
	appConf.UnmarshalKey("MastoApps", &apps)

	// Mastodonクライアントの登録
	dirtyConfig := false
	for _, bot := range bots {
		updatedApps, err := initMastoApps(apps, appName, bot.Instance)
		if err != nil {
			log.Printf("alert: %s のためのアプリを登録できませんでした", bot.Instance)
			return nil, db, err
		}
		if len(updatedApps) > 0 {
			apps = updatedApps
			dirtyConfig = true
		}
	}
	if dirtyConfig {
		appConf.Set("MastoApps", apps)
		if err := appConf.WriteConfig(); err != nil {
			log.Printf("alert: アプリ設定ファイルが書き込めませんでした：%s", err)
			return nil, db, err
		}
		log.Printf("info: 設定ファイルを更新しました")
	}

	// botの初期化（複数設定可）
	for _, bot := range bots {
		if err := initPersona(apps, bot); err != nil {
			log.Printf("alert: %s を初期化できませんでした", bot.Name)
			return nil, db, err
		}
	}

	// データベースへの接続
	db, err = newDB(cr)
	if err != nil {
		log.Printf("alert: データベースへの接続が確保できませんでした")
		return nil, db, err
	}

	// botがまだデータベースに登録されていなかったら登録
	if err = db.addNewBots(bots); err != nil {
		log.Printf("alert: データベースにbotが登録できませんでした")
		return nil, db, err
	}

	// botのデータベース上のIDを取得
	for _, bot := range bots {
		id, err := db.botID(bot)
		if err != nil {
			log.Printf("alert: botのデータベース上のIDが取得できませんでした")
			return nil, db, err
		}
		bot.DBID = id
	}

	return
}

// ActivateBots は、botたちを活動させる。
func ActivateBots(bots []*Persona, db DB, p int) (err error) {
	// 全てをシャットダウンするタイムアウトの設定
	ctx := context.Background()
	var cancel context.CancelFunc
	msg := "mastobots、時間無制限でスタートです！"
	if p > 0 {
		msg = "mastobots、" + strconv.Itoa(p) + "分間動きます！"
		dur := time.Duration(p) * time.Minute
		ctx, cancel = context.WithTimeout(ctx, dur)
		defer cancel()
	}
	log.Printf("info: " + msg)

	// 行ってらっしゃい
	for _, bot := range bots {
		if bot.LivesWithSun {
			time.Sleep(1001 * time.Millisecond)
			bot.LocInfo, err = getLocInfo(openCageKey, bot.Latitude, bot.Longitude)
			if err != nil {
				log.Printf("alert: 所在地情報の設定に失敗しました：%s", err)
				return
			}
		}
		go bot.spawn(ctx, db, true, false)
	}

	<-ctx.Done()
	log.Printf("info: %d分経ったのでシャットダウンします", p)
	return
}
