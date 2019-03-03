package mastobots

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"regexp"
	"strings"
	"time"

	mastodon "github.com/mattn/go-mastodon"
)

// moitorは、websocketでタイムラインを監視して反応する。
func (bot *Persona) monitor(ctx context.Context) {
	log.Printf("info: %s がタイムライン監視を開始しました。", bot.Name)
	newCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	evch, err := bot.openStreaming(newCtx)
	if err != nil {
		log.Printf("info: %s がストリーミングを受信開始できませんでした。\n", bot.Name)
		return
	}

	for ev := range evch {
		switch t := ev.(type) {
		case *mastodon.UpdateEvent:
			go func() {
				if err := bot.respondToUpdate(newCtx, t); err != nil {
					log.Printf("info: %s がトゥートに反応できませんでした。\n", bot.Name)
				}
			}()
		case *mastodon.NotificationEvent:
			go func() {
				if err := bot.respondToNotification(newCtx, t); err != nil {
					log.Printf("info: %s が通知に反応できませんでした。\n", bot.Name)
				}
			}()
		case *mastodon.ErrorEvent:
			if ctx.Err() != nil {
				log.Printf("info: %s が今日のタイムライン監視を終了しました：%s", bot.Name, ctx.Err())
				return
			}

			itvl := rand.Intn(4000) + 1000
			log.Printf("info: %s の接続が切れました。%dミリ秒後に再接続します：%s\n", bot.Name, itvl, t.Error())
			time.Sleep(time.Duration(itvl) * time.Millisecond)
			go bot.monitor(ctx)
			return
		}
	}
}

// openStreamingは、HTLのストリーミング接続を開始する。失敗したらmaxRetryを上限に再試行する。
func (bot *Persona) openStreaming(ctx context.Context) (evch chan mastodon.Event, err error) {
	wsc := bot.Client.NewWSClient()
	for i := 0; i < maxRetry; i++ {
		evch, err = wsc.StreamingWSUser(ctx)
		if err != nil {
			time.Sleep(retryInterval)
			log.Printf("info: %s のストリーミング受信開始をリトライします：%s\n", bot.Name, err)
			continue
		}
		log.Printf("trace: %s のストリーミング受信に成功しました。\n", bot.Name)
		return
	}
	log.Printf("info: %s のストリーミング受信開始に失敗しました。：%s\n", bot.Name, err)
	return
}

// respondToUpdateは、statusに反応する。
func (bot *Persona) respondToUpdate(ctx context.Context, ev *mastodon.UpdateEvent) (err error) {
	// メンションは無視
	if len(ev.Status.Mentions) != 0 {
		return
	}

	// 自分のトゥートは無視
	if ev.Status.Account.ID == bot.MyID {
		return
	}

	// トゥートを形態素解析
	text := textContent(ev.Status.Content)
	if text == "" {
		return
	}
	result, err := parse(text)
	if err != nil {
		return
	}

	// キーワードを検知したらふぁぼる
	for _, w := range bot.Keywords {
		if result.contain(w) {
			if err = bot.fav(ctx, ev.Status.ID); err != nil {
				log.Printf("info: %s がふぁぼを諦めました。\n", bot.Name)
				return
			}
			break
		}
	}
	return
}

// respondToNotificationは、通知に反応する。
func (bot *Persona) respondToNotification(ctx context.Context, ev *mastodon.NotificationEvent) (err error) {
	switch ev.Notification.Type {
	case "mention":
		if err = bot.respondToMention(ctx, ev.Notification.Account, ev.Notification.Status); err != nil {
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
	return
}

// respondToMentionは、メンションに反応する。
func (bot *Persona) respondToMention(ctx context.Context, account mastodon.Account, status *mastodon.Status) (err error) {
	r := regexp.MustCompile(`:.*:\z`)
	name := account.DisplayName
	if r.MatchString(name) {
		name = name + " "
	}
	txt := textContent(status.Content)
	res, err := parse(txt)
	if err != nil {
		return
	}

	var jm jumanResult
	var ok bool
	if jm, ok = res.(jumanResult); !ok {
		err = fmt.Errorf("%sに送られたメッセージは日本語ではありません", bot.Name)
		return
	}

	msg := ""

	switch {
	case strings.Contains(txt, "フォロー"):
		rel, err := bot.relationWith(ctx, account.ID)
		if err != nil {
			log.Printf("info: %s が関係取得に失敗しました。\n", bot.Name)
			return err
		}
		if (*rel[0]).Following == true {
			msg = "@" + account.Acct + " " + name + "さんはもうフォローしてるから大丈夫" + bot.Assertion + "よー"
		} else {
			if err = bot.follow(ctx, account.ID); err != nil {
				log.Printf("info: %s がフォローに失敗しました。\n", bot.Name)
				return err
			}
			msg = "@" + account.Acct + " わーい、お友達" + bot.Assertion + "ね！これからは、" + name + "さんのトゥートを生温かく見守っていく" + bot.Assertion + "よー"
		}
	case strings.Contains(txt, "いい"+bot.Assertion):
		yon := "だめ" + bot.Assertion + "よ"
		if rand.Intn(2) == 1 {
			yon = "いい" + bot.Assertion + "よ"
		}
		msg = "@" + account.Acct + " " + bot.Starter + name + bot.Title + "。" + yon
	case jm.isWeatherRelated():
		lc, dt, err := jm.judgeWeatherRequest()
		if err != nil {
			return err
		}
		data, err := GetRandomWeather(dt)
		if err != nil {
			log.Printf("info: %s が天気の取得に失敗しました。", bot.Name)
			return err
		}
		ignoreStr := ""
		if lc != "" && lc != data.Location.City {
			ignoreStr = lc + "はともかく、"
		}
		msg = "@" + account.Acct + " " + ignoreStr + forecastMessage(data, bot.Assertion)
	}

	if msg != "" {
		toot := mastodon.Toot{Status: msg, Visibility: status.Visibility, InReplyToID: status.ID}
		if err = bot.post(ctx, toot); err != nil {
			log.Printf("info: %s がリプライに失敗しました。\n", bot.Name)
			return err
		}
	}
	return
}
