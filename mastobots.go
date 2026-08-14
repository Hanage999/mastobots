package mastobots

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
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
	sudachi       sudachiSettings
}

type sudachiSettings struct {
	jarPath    string
	configPath string
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
	for _, cmd := range []string{"java", "mysql"} {
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
	cmn.sudachi, err = loadSudachiSettings(conf.GetString("SudachiHome"))
	if err != nil {
		log.Printf("alert: Sudachiの設定を読み込めませんでした：%s", err)
		return nil, db, err
	}
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

func loadSudachiSettings(home string) (sudachiSettings, error) {
	if home == "" {
		return sudachiSettings{}, fmt.Errorf("設定項目 SudachiHome が設定されていません")
	}

	settings := sudachiSettings{
		jarPath:    filepath.Join(home, "sudachi-0.8.0.jar"),
		configPath: filepath.Join(home, "sudachi.json"),
	}
	for _, path := range []string{settings.jarPath, settings.configPath} {
		info, err := os.Stat(path)
		if err != nil {
			return sudachiSettings{}, fmt.Errorf("Sudachiのファイル %q を確認できません：%w", path, err)
		}
		if info.IsDir() {
			return sudachiSettings{}, fmt.Errorf("Sudachiのファイル %q がディレクトリです", path)
		}
	}

	return settings, nil
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
