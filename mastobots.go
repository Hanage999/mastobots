package mastobots

import (
	"context"
	"log"
	"os/exec"
	"strconv"
	"time"

	"github.com/comail/colog"
	"github.com/ringsaturn/tzf"
	"github.com/spf13/viper"
)

var (
	version = "1"
	f       tzf.F
)

type commonSettings struct {
	maxRetry      int
	retryInterval time.Duration
	yahooClientID string
	weatherKey    string
	langJobPool   chan int
}

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
	conf.UnmarshalKey("Personae", &bots)
	var cmn commonSettings
	cmn.maxRetry = 5
	cmn.retryInterval = time.Duration(5) * time.Second
	cmn.yahooClientID = conf.GetString("YahooClientID")
	cmn.weatherKey = conf.GetString("OpenWeatherMapKey")
	nOfJobs := conf.GetInt("NumConcurrentLangJobs")
	if nOfJobs <= 0 {
		nOfJobs = 1
	} else if nOfJobs > 10 {
		nOfJobs = 10
	}
	cmn.langJobPool = make(chan int, nOfJobs)
	for _, bot := range bots {
		bot.commonSettings = &cmn
	}
	cr = conf.GetStringMapString("DBCredentials")

	// botをMastodonサーバに接続し、アカウントIDを取得
	for _, bot := range bots {
		if err := bot.getMastoID(); err != nil {
			log.Printf("alert: %s のMastodonアカウントIDができませんでした。終了します", bot.Name)
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

	// TZF（グローバル変数に設定）を初期化
	f, err = tzf.NewDefaultFinder()
	if err != nil {
		log.Printf("info: %s", err)
		return nil, db, err
	}

	// botの住処を登録
	for _, bot := range bots {
		if bot.LivesWithSun {
			log.Printf("info: %s の所在地を設定しています……", bot.Name)
			time.Sleep(1001 * time.Millisecond)
			bot.PlaceName, bot.TimeZone, err = getLocDataFromCoordinates(bot.commonSettings.yahooClientID, bot.Latitude, bot.Longitude)
			if err != nil {
				log.Printf("alert: %s の所在地情報の設定に失敗しました：%s", bot.Name, err)
				return nil, db, err
			}
		}
	}

	f = nil

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
		go bot.spawn(ctx, db, true, false)
	}

	<-ctx.Done()
	log.Printf("info: %d分経ったのでシャットダウンします", p)
	return
}
