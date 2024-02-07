package mastobots

import (
	"context"
	"log"
	"math/rand"
	"runtime"
	"sort"
	"strconv"
	"time"

	mastodon "github.com/hanage999/go-mastodon"
)

// Persona は、botの属性を格納する。
type Persona struct {
	Name            string
	Instance        string
	MyApp           *MastoApp
	Email           string
	Password        string
	Client          *mastodon.Client
	MyID            mastodon.ID
	Title           string
	Starter         string
	Assertion       string
	FirstFire       int
	Interval        int
	ItemPool        int
	Hashtags        []string
	Keywords        []string
	Comments        []string
	DBID            int
	WakeHour        int
	WakeMin         int
	SleepHour       int
	SleepMin        int
	LivesWithSun    bool
	Latitude        float64
	Longitude       float64
	PlaceName       string
	TimeZone        string
	RandomToots     []string
	RandomFrequency int
	Awake           time.Duration
	*commonSettings
}

// connectPersonaは、botとMastodonサーバの接続を確立する。
func connectPersona(apps []*MastoApp, bot *Persona) (err error) {
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

	for i := 0; i < bot.commonSettings.maxRetry+45; i++ {
		err = bot.Client.Authenticate(ctx, bot.Email, bot.Password)
		if err != nil {
			time.Sleep(bot.commonSettings.retryInterval)
			log.Printf("alert: %s がアクセストークンの取得に失敗しました。リトライします：%s", bot.Name, err)
			continue
		}
		break
	}
	if err != nil {
		log.Printf("alert: %s がアクセストークンの取得に失敗しました。終了します：%s", bot.Name, err)
		return
	}

	acc, err := bot.Client.GetAccountCurrentUser(ctx)
	if err != nil {
		log.Printf("alert: %s のアカウントIDが取得できませんでした：%s", bot.Name, err)
		return
	}
	bot.MyID = acc.ID

	return
}

// spawn は、botの活動を開始する
func (bot *Persona) spawn(ctx context.Context, db DB, firstLaunch bool, nextDayOfPolarNight bool) {
	sleep, active := getDayCycle(bot.WakeHour, bot.WakeMin, bot.SleepHour, bot.SleepMin)
	bot.Awake = active

	if bot.LivesWithSun {
		sl, ac, cond, err := getDayCycleBySunMovement(bot.TimeZone, bot.Latitude, bot.Longitude)
		if err == nil {
			sleep, active = sl, ac
			bot.Awake = ac
			switch cond {
			case "白夜":
				log.Printf("info: %s がいる %s は今、白夜です", bot.Name, bot.PlaceName)
				if !firstLaunch {
					go func() {
						toot := mastodon.Toot{Status: bot.PlaceName + "は、いま１日でいちばん暗い時間" + bot.Assertion + "。でも白夜だから寝ないの" + bot.Assertion + "よ"}
						if err := bot.post(ctx, toot); err != nil {
							log.Printf("info: %s がトゥートできませんでした。今回は諦めます……", bot.Name)
						}
					}()
				}
			case "極夜":
				log.Printf("info: %s がいる %s は今、極夜です", bot.Name, bot.PlaceName)
				if !firstLaunch && nextDayOfPolarNight {
					go func() {
						toot := mastodon.Toot{Status: bot.PlaceName + "は、いま１日でいちばん明るい時間" + bot.Assertion + "。でも極夜だから起きないの" + bot.Assertion + "よ💤……"}
						if err := bot.post(ctx, toot); err != nil {
							log.Printf("info: %s がトゥートできませんでした。今回は諦めます……", bot.Name)
						}
					}()
				}
			default:
				log.Printf("info: %s の所在地、起床までの時間、起床後の活動時間：", bot.Name)
				log.Printf("info: 　%s、%s、%s", bot.PlaceName, sleep, active)
			}
		} else {
			log.Printf("info: %s の生活サイクルが太陽の出没から決められませんでした。デフォルトの起居時刻を使います：%s", bot.Name, err)
		}
	}

	go bot.daylife(ctx, db, sleep, active, firstLaunch, nextDayOfPolarNight)
}

// daylife は、botの活動サイクルを作る
func (bot *Persona) daylife(ctx context.Context, db DB, sleep time.Duration, active time.Duration, firstLaunch bool, nextDayOfPolarNight bool) {
	wakeWithSun, sleepWithSun := "", ""
	if bot.LivesWithSun {
		wakeWithSun = "そろそろ明るくなってきた" + bot.Assertion + "ね。" + bot.PlaceName + "から"
		sleepWithSun = bot.PlaceName + "のあたりはもうすっかり暗くなった" + bot.Assertion + "ね。では、"
	}

	if sleep > 0 {
		t := time.NewTimer(sleep)
		if !firstLaunch && !nextDayOfPolarNight {
			go func() {
				toot := mastodon.Toot{Status: sleepWithSun + "おやすみなさい" + bot.Assertion + "💤……"}
				if err := bot.post(ctx, toot); err != nil {
					log.Printf("info: %s がトゥートできませんでした。今回は諦めます……", bot.Name)
				}
			}()
		}
	LOOP:
		for {
			select {
			case <-t.C:
				break LOOP
			case <-ctx.Done():
				if !t.Stop() {
					<-t.C
				}
				return
			}
		}
	}

	newCtx, cancel := context.WithTimeout(ctx, active)
	defer cancel()

	if active > 0 {
		log.Printf("info: %s が起きたところ", bot.Name)
		log.Printf("trace: Goroutines: %d", runtime.NumGoroutine())
		nextDayOfPolarNight = false
		bot.activities(newCtx, db)
		if err := bot.checkNotifications(newCtx); err != nil {
			log.Printf("info: %s が通知を遡れませんでした。今回は諦めます……", bot.Name)
		}
		if sleep > 0 {
			go func() {
				weatherStr := ""
				data, err := GetLocationWeather(bot.commonSettings.weatherKey, bot.Latitude, bot.Longitude, 0)
				if err != nil {
					log.Printf("info: %s が天気予報を取ってこれませんでした", bot.Name)
				} else {
					weatherStr = "。" + forecastMessage(bot.PlaceName, data, 0, bot.Assertion, true, false)
				}
				toot := mastodon.Toot{Status: wakeWithSun + "おはようございます" + bot.Assertion + weatherStr}
				if err := bot.post(newCtx, toot); err != nil {
					log.Printf("info: %s がトゥートできませんでした。今回は諦めます……", bot.Name)
				}
			}()
		}
	} else {
		nextDayOfPolarNight = true
	}

	<-newCtx.Done()
	log.Printf("info: %s が寝たところ", bot.Name)
	log.Printf("trace: Goroutines: %d", runtime.NumGoroutine())
	if ctx.Err() == nil {
		bot.spawn(ctx, db, false, nextDayOfPolarNight)
	}
}

// activities は、botの活動の全てを実行する
func (bot *Persona) activities(ctx context.Context, db DB) {
	go bot.periodicActivity(ctx, db)
	go bot.monitor(ctx)
	if len(bot.RandomToots) > 0 && bot.RandomFrequency > 0 {
		go bot.randomToot(ctx)
	}
}

func (bot *Persona) checkNotifications(ctx context.Context) (err error) {
	ns, err := bot.notifications(ctx)
	if err != nil {
		log.Printf("info: %s が通知一覧を取得できませんでした：%s", bot.Name, err)
		return
	}

	sort.Sort(ns)

	for _, n := range ns {
		switch n.Type {
		case "mention":
			if err = bot.respondToMention(ctx, n.Account, n.Status); err != nil {
				log.Printf("info: %s がメンションに反応できませんでした：%s", bot.Name, err)
				return
			}
		case "reblog":
			// TODO
		case "favourite":
			// TODO
		case "follow":
			// TODO
		}
		if err = bot.dismissNotification(ctx, n.ID); err != nil {
			log.Printf("info: %s が id:%s の通知を削除できませんでした：%s", bot.Name, string(n.ID), err)
			return
		}
	}

	return
}

type Notifications []*mastodon.Notification

func (ns Notifications) Len() int {
	return len(ns)
}

func (ns Notifications) Swap(i, j int) {
	ns[i], ns[j] = ns[j], ns[i]
}

func (ns Notifications) Less(i, j int) bool {
	iv, _ := strconv.Atoi(string(ns[i].ID))
	jv, _ := strconv.Atoi(string(ns[j].ID))
	return iv < jv
}

// postはトゥートを投稿する。失敗したらmaxRetryを上限に再試行する。
func (bot *Persona) post(ctx context.Context, toot mastodon.Toot) (err error) {
	time.Sleep(time.Duration(rand.Intn(5000)+3000) * time.Millisecond)
	for i := 0; i < bot.commonSettings.maxRetry; i++ {
		_, err = bot.Client.PostStatus(ctx, &toot)
		if err != nil {
			time.Sleep(bot.commonSettings.retryInterval)
			log.Printf("info: %s がトゥートできませんでした。リトライします：%s\n %s", bot.Name, toot.Status, err)
			continue
		}
		break
	}
	return
}

// favは、ステータスをふぁぼる。失敗したらmaxRetryを上限に再試行する。
func (bot *Persona) fav(ctx context.Context, id mastodon.ID) (err error) {
	time.Sleep(time.Duration(rand.Intn(2000)+1000) * time.Millisecond)
	for i := 0; i < bot.commonSettings.maxRetry; i++ {
		_, err = bot.Client.Favourite(ctx, id)
		if err != nil {
			time.Sleep(bot.commonSettings.retryInterval)
			log.Printf("info: %s がふぁぼれませんでした。リトライします：%s", bot.Name, err)
			continue
		}
		break
	}
	return
}

// boostは、ステータスをブーストする。失敗したらmaxRetryを上限に再試行する。
func (bot *Persona) boost(ctx context.Context, id mastodon.ID) (err error) {
	time.Sleep(time.Duration(rand.Intn(5000)+3000) * time.Millisecond)
	for i := 0; i < bot.commonSettings.maxRetry; i++ {
		_, err = bot.Client.Reblog(ctx, id)
		if err != nil {
			time.Sleep(bot.commonSettings.retryInterval)
			log.Printf("info: %s がブーストできませんでした。リトライします。：%s\n", bot.Name, err)
			continue
		}
		break
	}

	return
}

// followは、アカウントをフォローする。失敗したらmaxRetryを上限に再試行する。
func (bot *Persona) follow(ctx context.Context, id mastodon.ID) (err error) {
	time.Sleep(time.Duration(rand.Intn(2000)+1000) * time.Millisecond)
	for i := 0; i < bot.commonSettings.maxRetry; i++ {
		_, err = bot.Client.AccountFollow(ctx, id)
		if err != nil {
			time.Sleep(bot.commonSettings.retryInterval)
			log.Printf("info: %s がフォローできませんでした。リトライします：%s", bot.Name, err)
			continue
		}
		break
	}
	return
}

// relationWithは、アカウントと自分との関係を取得する。失敗したらmaxRetryを上限に再実行する。
func (bot *Persona) relationWith(ctx context.Context, id mastodon.ID) (rel []*mastodon.Relationship, err error) {
	for i := 0; i < bot.commonSettings.maxRetry; i++ {
		rel, err = bot.Client.GetAccountRelationships(ctx, []string{string(id)})
		if err != nil {
			time.Sleep(bot.commonSettings.retryInterval)
			log.Printf("info: %s と id:%s の関係が取得できませんでした。リトライします：%s", bot.Name, string(id), err)
			continue
		}
		break
	}
	return
}

func (bot *Persona) notifications(ctx context.Context) (ns Notifications, err error) {
	var pg mastodon.Pagination
	for i := 0; i < bot.commonSettings.maxRetry; i++ {
		ns, err = bot.Client.GetNotifications(ctx, &pg)
		if err != nil {
			time.Sleep(bot.commonSettings.retryInterval)
			log.Printf("info: %s が通知一覧を取得できませんでした。リトライします：%s", bot.Name, err)
			continue
		}
		break
	}
	return
}

func (bot *Persona) dismissNotification(ctx context.Context, id mastodon.ID) (err error) {
	for i := 0; i < bot.commonSettings.maxRetry; i++ {
		err = bot.Client.DismissNotification(ctx, id)
		if err != nil {
			time.Sleep(bot.commonSettings.retryInterval)
			log.Printf("info: %s が id:%s の通知を削除できませんでした。リトライします：%s", bot.Name, string(id), err)
			continue
		}
		break
	}
	return
}
