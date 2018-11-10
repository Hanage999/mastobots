package mastobots

import (
	"context"
	"github.com/mattn/go-mastodon"
	"log"
	"math/rand"
	"regexp"
	"strings"
	"time"
)

// moitorは、websocketでタイムラインを監視して反応する。
func (bot *Persona) monitor(ctx context.Context) {
	log.Printf("trace: %s がタイムライン監視を開始しました。", bot.Name)
	newCtx, cancel := context.WithCancel(ctx)
	evch, err := bot.openStreaming(newCtx)
	if err != nil {
		log.Printf("info: %s がストリーミングを受信開始できませんでした：%s\n", bot.Name, err)
		return
	}

LOOP:
	for {
		select {
		case ev := <-evch:
			switch t := ev.(type) {
			case *mastodon.UpdateEvent:
				if err := bot.respondToUpdate(newCtx, t); err != nil {
					log.Printf("info: %s がトゥートに反応できませんでした。\n")
				}
			case *mastodon.NotificationEvent:
				if bot.respondToNotification(newCtx, t); err != nil {
					log.Printf("info: %s が通知に反応できませんでした。\n")
				}
			case *mastodon.ErrorEvent:
				cancel()
				itv := rand.Intn(5000) + 1
				log.Printf("info: %s の接続が切れました。%dミリ秒後に再接続します：%s\n", bot.Name, itv, t.Error())
				time.Sleep(time.Duration(itv) * time.Millisecond)
				go bot.monitor(ctx)
				break LOOP
			}
		case <-ctx.Done():
			log.Printf("trace: %s が今日のタイムライン監視を終了しました", bot.Name)
			break LOOP
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
		break
	}

	return
}

// respondToUpdateは、statusに反応する。
func (bot *Persona) respondToUpdate(ctx context.Context, ev *mastodon.UpdateEvent) (err error) {
	// メンションは無視
	if len(ev.Status.Mentions) != 0 {
		return
	}

	// 自分のトゥートは無視
	uid := ev.Status.Account.ID
	if uid == bot.MyID {
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
				log.Printf("info: %s がふぁぼを諦めました：%s\n", bot.Name, err)
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
			log.Printf("info: %s がメンションに反応できませんでした。\n")
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

	msg := ""

	switch {
	case strings.Contains(txt, "フォロー頼む"+bot.Assertion):
		rel, err := bot.relationWith(ctx, account.ID)
		if err != nil {
			log.Printf("info: %s が関係取得に失敗しました。\n")
			return err
		}
		if (*rel[0]).Following == true {
			msg = "@" + account.Acct + " " + name + "さんはもうフォローしてるから大丈夫" + bot.Assertion + "よー"
		} else {
			if err = bot.follow(ctx, account.ID); err != nil {
				log.Printf("info: %s がフォローに失敗しました。\n")
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
	}

	if msg != "" {
		toot := mastodon.Toot{Status: msg, Visibility: status.Visibility, InReplyToID: status.ID}
		if err = bot.post(ctx, toot); err != nil {
			log.Printf("info: %s がリプライに失敗しました。\n")
			return
		}
	}

	return
}
