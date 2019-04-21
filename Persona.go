package mastobots

import (
	"context"
	"log"
	"math/rand"
	"strings"
	"time"

	mastodon "github.com/mattn/go-mastodon"
)

// Persona は、botの属性を格納する。
type Persona struct {
	Name         string
	Instance     string
	MyApp        *MastoApp
	Email        string
	Password     string
	Client       *mastodon.Client
	MyID         mastodon.ID
	Title        string
	Starter      string
	Assertion    string
	FirstFire    int
	Interval     int
	ItemPool     int
	Hashtags     []string
	Keywords     []string
	Comments     []string
	DBID         int
	WakeHour     int
	WakeMin      int
	SleepHour    int
	SleepMin     int
	LivesWithSun bool
	Latitude     float64
	Longitude    float64
	LocInfo      OCResult
}

// OCResult は、OpenCageからのデータを格納する
type OCResult struct {
	Annotations struct {
		Flag     string `json:"flag"`
		Timezone struct {
			Name string `json:"name"`
		} `json:"timezone"`
	} `json:"annotations"`
	Components map[string]string `json:"components"`
	Formatted  string            `json:"formatted"`
}

// OCResults は、OpenCageからのデータを格納する
type OCResults struct {
	Results []OCResult
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
	tillWake := until(bot.WakeHour, bot.WakeMin, 0)
	tillSleep := until(bot.SleepHour, bot.SleepMin, 0)
	awake := tillSleep - tillWake

	if awake < time.Second && awake > -1*time.Second {
		bot.activities(ctx, db)
		return
	}

	if awake < 0 {
		awake += 24 * time.Hour
	}

	if awake > tillSleep {
		tillWake = 0
		awake = tillSleep
	}

	// あとは任せた
	go bot.daylife(ctx, db, tillWake, awake)
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
		bot.spawn(ctx, db)
	case <-ctx.Done():
	}
}

// spawnWithSun は、太陽とともに生きるbotの活動を開始する
func (bot *Persona) spawnWithSun(ctx context.Context, db *DB) {
	tillWake := 8 * time.Hour
	tillSleep := 24 * time.Hour
	awake := tillSleep - tillWake

	wt, st, err := getDayCycleBySunMovement(bot.LocInfo.Annotations.Timezone.Name, bot.Latitude, bot.Longitude)
	if err == nil {
		tillWake = time.Until(wt)
		tillSleep = time.Until(st)
		awake = st.Sub(wt)
		if awake < 0 {
			awake += 24 * time.Hour
		}
		if awake > tillSleep {
			tillWake = 0
			awake = tillSleep
		}
		log.Printf("info: %s にいる %s の起床時刻：%s", bot.getLocStr(false), bot.Name, wt.Local())
		log.Printf("info: %s にいる %s の就寝時刻：%s", bot.getLocStr(false), bot.Name, st.Local())
	} else {
		if strings.Index(err.Error(), "白夜か黒夜") != -1 {
			loc, _ := time.LoadLocation(bot.LocInfo.Annotations.Timezone.Name)
			now := time.Now().In(loc)
			_, m, _ := now.Date()
			if bot.Latitude > 0 {
				if 3 < int(m) && int(m) < 10 {
					log.Printf("info: %s がいる %s は今、白夜です", bot.Name, bot.getLocStr(false))
					tillWake = 0
					awake = 24 * time.Hour
				} else {
					log.Printf("info: %s がいる %s は今、極夜です", bot.Name, bot.getLocStr(false))
					tillWake = 24 * time.Hour
					awake = 0
				}
			} else {
				if 3 < int(m) && int(m) < 10 {
					log.Printf("info: %s がいる %s は今、極夜です", bot.Name, bot.getLocStr(false))
					tillWake = 24 * time.Hour
					awake = 0
				} else {
					log.Printf("info: %s がいる %s は今、白夜です", bot.Name, bot.getLocStr(false))
					tillWake = 0
					awake = 24 * time.Hour
				}
			}
		}
	}

	// あとは任せた
	go bot.daylifeWithSun(ctx, db, tillWake, awake)
}

// daylife は、太陽とともに生きるbotの活動サイクルを作る
func (bot *Persona) daylifeWithSun(ctx context.Context, db *DB, sleep time.Duration, active time.Duration) {
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

	if active > 0 {
		bot.activities(newCtx, db)
	}

	if asleep {
		go func() {
			weatherStr := ""
			data, err := GetRandomWeather(0)
			if err != nil {
				log.Printf("info: %s が天気予報を取ってこれませんでした", bot.Name)
			} else {
				weatherStr = "。" + forecastMessage(data, bot.Assertion)
			}
			withSun := "そろそろ明るくなってきた" + bot.Assertion + "ね。" + bot.getLocStr(false) + "から"
			toot := mastodon.Toot{Status: withSun + "おはようございます" + bot.Assertion + weatherStr}
			if err := bot.post(newCtx, toot); err != nil {
				log.Printf("info: %s がトゥートできませんでした。今回は諦めます……", bot.Name)
			}
		}()
	}

	select {
	case <-newCtx.Done():
		if 0 < active && active < 24*time.Hour {
			withSun := bot.getLocStr(true) + "のあたりはもうすっかり暗くなった" + bot.Assertion + "ね。では、"
			toot := mastodon.Toot{Status: withSun + "おやすみなさい" + bot.Assertion + "💤……"}
			if err := bot.post(ctx, toot); err != nil {
				log.Printf("info: %s がトゥートできませんでした。今回は諦めます……", bot.Name)
			}
		}
		bot.spawnWithSun(ctx, db)
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

func (bot *Persona) getLocStr(simple bool) (str string) {
	info := bot.LocInfo

	tp := info.Components["_type"]
	str = info.Components[tp]

	country := info.Components["country"] + info.Annotations.Flag
	state := info.Components["state"]
	stateDistrict := info.Components["state_district"]
	county := info.Components["county"]
	city := info.Components["city"]
	suburb := info.Components["suburb"]
	town := info.Components["town"]

	names := [...]string{town, suburb, city}
	for _, name := range names {
		if str != "" {
			break
		}
		str = name
	}

	if simple {
		return
	}

	if country == "" {
		country = "国ではないどこか"
	}
	if city == "" {
		city = "名もない町"
	}

	if city != str {
		str = state + stateDistrict + county + city + "（" + country + "）" + "の" + str
	} else {
		if county != str {
			str = state + stateDistrict + county + "（" + country + "）" + "の" + str
		} else {
			str = state + stateDistrict + "（" + country + "）" + "の" + str
		}
	}

	return
}
