package mastobots

import (
	"context"
	"log"
	"math/rand"
	"time"

	mastodon "github.com/mattn/go-mastodon"
)

// Persona は、botの属性を格納する。
type Persona struct {
	Name      string
	Instance  string
	MyApp     *MastoApp
	Email     string
	Password  string
	Client    *mastodon.Client
	MyID      mastodon.ID
	Title     string
	Starter   string
	Assertion string
	FirstFire int
	Interval  int
	Hashtags  []string
	Keywords  []string
	Comments  []string
	DBID      int
	WakeHour  int
	WakeMin   int
	SleepHour int
	SleepMin  int
	Awake     time.Duration
}

// initPersonaは、botとインスタンスの接続を確立する。
func initPersona(apps []*MastoApp, bot *Persona) (err error) {
	ctx := context.Background()

	bot.MyApp, err = getApp(bot.Instance, apps)
	if err != nil {
		log.Printf("alert: %s のためのアプリが取得できませんでした：%s", bot.Name, err)
		return
	}

	bot.Client = mastodon.NewClient(&mastodon.Config{
		Server:       bot.Instance,
		ClientID:     bot.MyApp.ClientID,
		ClientSecret: bot.MyApp.ClientSecret,
	})

	err = bot.Client.Authenticate(ctx, bot.Email, bot.Password)
	if err != nil {
		log.Printf("%s がアクセストークンの取得に失敗しました：%s", bot.Name, err)
		return
	}

	acc, err := bot.Client.GetAccountCurrentUser(ctx)
	if err != nil {
		log.Printf("%s のアカウントIDが取得できませんでした：%s", bot.Name, err)
		return
	}
	bot.MyID = acc.ID

	return
}

// spawn は、botの活動を開始する
func (bot *Persona) spawn(ctx context.Context, db *DB) {
	tillWake := until(bot.WakeHour, bot.WakeMin)
	tillSleep := until(bot.SleepHour, bot.SleepMin)
	bot.Awake = tillSleep - tillWake

	if bot.Awake < time.Second && bot.Awake > -1*time.Second {
		bot.activities(ctx, db)
		return
	}

	if bot.Awake < 0 {
		bot.Awake += 24 * time.Hour
	}

	s := tillWake
	w := bot.Awake
	if tillSleep < bot.Awake {
		s, _ = time.ParseDuration("0s")
		w = tillSleep
	}

	// あとは任せた
	go bot.daylife(ctx, db, s, w)
}

// daylife は、botの活動サイクルを作る
func (bot *Persona) daylife(ctx context.Context, db *DB, sleep time.Duration, active time.Duration) {
	asleep := false

	if sleep.Seconds() > 1 {
		asleep = true
		t := time.NewTimer(sleep)
		defer t.Stop()
	LOOP:
		for {
			select {
			case <-t.C:
				break LOOP
			case <-ctx.Done():
				return
			}
		}
	}

	newCtx, cancel := context.WithTimeout(ctx, active)
	defer cancel()

	bot.activities(newCtx, db)
	if asleep {
		go func() {
			weatherStr := ""
			data, err := GetRandomWeather(0)
			if err != nil {
				log.Printf("info: %s が天気予報を取ってこれませんでした", bot.Name)
			} else {
				weatherStr = "。" + forecastMessage(data, bot.Assertion)
			}
			toot := mastodon.Toot{Status: "おはようございます" + bot.Assertion + weatherStr}
			if err := bot.post(newCtx, toot); err != nil {
				log.Printf("info: %s がトゥートできませんでした。今回は諦めます……", bot.Name)
			}
		}()
	}

	select {
	case <-newCtx.Done():
		toot := mastodon.Toot{Status: "おやすみなさい" + bot.Assertion + "💤……"}
		if err := bot.post(ctx, toot); err != nil {
			log.Printf("info: %s がトゥートできませんでした。今回は諦めます……", bot.Name)
		}
		s := until(bot.WakeHour, bot.WakeMin)
		go bot.daylife(ctx, db, s, bot.Awake)
	case <-ctx.Done():
	}
}

// activities は、botの活動の全てを実行する
func (bot *Persona) activities(ctx context.Context, db *DB) {
	go bot.periodicToot(ctx, db)
	go bot.monitor(ctx)
}

// postはトゥートを投稿する。失敗したらmaxRetryを上限に再試行する。
func (bot *Persona) post(ctx context.Context, toot mastodon.Toot) (err error) {
	time.Sleep(time.Duration(rand.Intn(5000)+3000) * time.Millisecond)
	for i := 0; i < maxRetry; i++ {
		_, err = bot.Client.PostStatus(ctx, &toot)
		if err != nil {
			time.Sleep(retryInterval)
			log.Printf("info: %s がトゥートできませんでした。リトライします：%s", bot.Name, err)
			continue
		}
		break
	}
	return
}

// favは、ステータスをふぁぼる。失敗したらmaxRetryを上限に再試行する。
func (bot *Persona) fav(ctx context.Context, id mastodon.ID) (err error) {
	time.Sleep(time.Duration(rand.Intn(2000)+1000) * time.Millisecond)
	for i := 0; i < maxRetry; i++ {
		_, err = bot.Client.Favourite(ctx, id)
		if err != nil {
			time.Sleep(retryInterval)
			log.Printf("info: %s がふぁぼれませんでした。リトライします：%s", bot.Name, err)
			continue
		}
		break
	}
	return
}

// followは、アカウントをフォローする。失敗したらmaxRetryを上限に再試行する。
func (bot *Persona) follow(ctx context.Context, id mastodon.ID) (err error) {
	time.Sleep(time.Duration(rand.Intn(2000)+1000) * time.Millisecond)
	for i := 0; i < maxRetry; i++ {
		_, err = bot.Client.AccountFollow(ctx, id)
		if err != nil {
			time.Sleep(retryInterval)
			log.Printf("info: %s がフォローできませんでした。リトライします：%s", bot.Name, err)
			continue
		}
		break
	}
	return
}

// relationWithは、アカウントと自分との関係を取得する。失敗したらmaxRetryを上限に再実行する。
func (bot *Persona) relationWith(ctx context.Context, id mastodon.ID) (rel []*mastodon.Relationship, err error) {
	for i := 0; i < maxRetry; i++ {
		rel, err = bot.Client.GetAccountRelationships(ctx, []string{string(id)})
		if err != nil {
			time.Sleep(retryInterval)
			log.Printf("info: %s と id:%s の関係が取得できませんでした。リトライします：%s", bot.Name, string(id), err)
			continue
		}
		break
	}
	return
}
