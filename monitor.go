package mastobots

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"regexp"
	"strings"
	"time"

	mastodon "github.com/hanage999/go-mastodon"
)

// moitorは、websocketでホームタイムラインを監視して反応する。
func (bot *Persona) monitor(ctx context.Context) {
	log.Printf("info: %s がタイムライン監視を開始しました", bot.Name)
	newCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	evch, err := bot.openStreaming(newCtx)
	if err != nil {
		log.Printf("info: %s がストリーミングを受信開始できませんでした", bot.Name)
		return
	}

	ers := ""
	for ev := range evch {
		switch t := ev.(type) {
		case *mastodon.UpdateEvent:
			go func() {
				if err := bot.respondToUpdate(newCtx, t); err != nil {
					log.Printf("info: %s がトゥートに反応できませんでした", bot.Name)
				}
			}()
		case *mastodon.NotificationEvent:
			go func() {
				if err := bot.respondToNotification(newCtx, t); err != nil {
					log.Printf("info: %s が通知に反応できませんでした", bot.Name)
				}
			}()
		case *mastodon.ErrorEvent:
			ers = t.Error()
			log.Printf("info: %s がエラーイベントを受信しました：%s", bot.Name, ers)
		}
	}

	if ctx.Err() != nil {
		log.Printf("info: %s が今日のタイムライン監視を終了しました：%s", bot.Name, ctx.Err())
	} else {
		itvl := rand.Intn(4000) + 1000
		log.Printf("info: %s の接続が切れました。%dミリ秒後に再接続します：%s", bot.Name, itvl, ers)
		time.Sleep(time.Duration(itvl) * time.Millisecond)
		go bot.monitor(ctx)
	}
}

// moitorFederationは、websocketで連合タイムラインを監視して反応する。
func (bot *Persona) monitorFederation(ctx context.Context) {
	log.Printf("info: %s が連合タイムライン監視を開始しました", bot.Name)
	newCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	evch, err := bot.openFederatedStreaming(newCtx)
	if err != nil {
		log.Printf("info: %s が連合ストリーミングを受信開始できませんでした", bot.Name)
		return
	}

	ers := ""
	for ev := range evch {
		switch t := ev.(type) {
		case *mastodon.UpdateEvent:
			go func() {
				if err := bot.respondToImages(newCtx, t); err != nil {
					log.Printf("info: %s が連合のトゥートに反応できませんでした。\n", bot.Name)
				}
			}()
		case *mastodon.ErrorEvent:
			ers = t.Error()
			log.Printf("info: %s が連合でエラーイベントを受信しました：%s", bot.Name, ers)
		}
	}

	if ctx.Err() != nil {
		log.Printf("info: %s が今日のタイムライン監視を終了しました：%s", bot.Name, ctx.Err())
	} else {
		itvl := rand.Intn(4000) + 1000
		log.Printf("info: %s の接続が切れました。%dミリ秒後に再接続します：%s", bot.Name, itvl, ers)
		time.Sleep(time.Duration(itvl) * time.Millisecond)
		go bot.monitorFederation(ctx)
	}
}

// openStreamingは、HTLのストリーミング接続を開始する。失敗したらmaxRetryを上限に再試行する。
func (bot *Persona) openStreaming(ctx context.Context) (evch chan mastodon.Event, err error) {
	wsc := bot.Client.NewWSClient()
	for i := 0; i < maxRetry; i++ {
		evch, err = wsc.StreamingWSUser(ctx)
		if err != nil {
			time.Sleep(retryInterval)
			log.Printf("info: %s のストリーミング受信開始をリトライします：%s", bot.Name, err)
			continue
		}
		log.Printf("trace: %s のストリーミング受信に成功しました", bot.Name)
		return
	}
	log.Printf("info: %s のストリーミング受信開始に失敗しました：%s", bot.Name, err)
	return
}

// openFederatedStreamingは、FTLのストリーミング接続を開始する。失敗したらmaxRetryを上限に再試行する。
func (bot *Persona) openFederatedStreaming(ctx context.Context) (evch chan mastodon.Event, err error) {
	wsc := bot.Client.NewWSClient()
	for i := 0; i < maxRetry; i++ {
		evch, err = wsc.StreamingWSPublic(ctx, false)
		if err != nil {
			time.Sleep(retryInterval)
			log.Printf("info: %s の連合ストリーミング受信開始をリトライします：%s\n", bot.Name, err)
			continue
		}
		log.Printf("trace: %s の連合ストリーミング受信に成功しました。\n", bot.Name)
		return
	}
	log.Printf("info: %s の連合ストリーミング受信開始に失敗しました。：%s\n", bot.Name, err)
	return
}

// respondToUpdateは、statusに反応する。
func (bot *Persona) respondToUpdate(ctx context.Context, ev *mastodon.UpdateEvent) (err error) {
	orig := ev.Status
	rebl := false
	if orig.Reblog != nil {
		orig = orig.Reblog
		rebl = true
	}

	// メンションは無視（ブーストされたのものは見る）
	if len(ev.Status.Mentions) != 0 && !rebl {
		return
	}

	// 自分のトゥートは無視
	if orig.Account.ID == bot.MyID {
		return
	}

	// トゥートを形態素解析
	text := textContent(orig.Content)
	if text == "" {
		return
	}
	result, err := parse(bot.JobPool, text)
	if err != nil {
		return
	}

	// キーワードを検知したらふぁぼる。同じ鯖のbotならブースト＋引用コメントする
	for _, w := range bot.Keywords {
		if result.contain(w) {
			if err = bot.fav(ctx, ev.Status.ID); err != nil {
				log.Printf("info: %s がふぁぼを諦めました", bot.Name)
			}
			if !strings.Contains(ev.Status.Account.Acct, "@") && ev.Status.Account.Bot {
				if err = bot.boost(ctx, ev.Status.ID); err != nil {
					log.Printf("info: %s がブーストを諦めました", bot.Name)
				}
				if err = bot.quoteComment(ctx, result, orig.URL); err != nil {
					log.Printf("info: %s が引用＋コメントを諦めました", bot.Name)
				}
			}
			break
		}
	}
	return
}

// quoteCommentは、トゥートを引用コメントする
func (bot *Persona) quoteComment(ctx context.Context, result parseResult, url string) (err error) {
	msg, err := bot.messageFromParseResult(result, url)
	if err != nil || msg == "" {
		log.Printf("info: %s が引用コメントを作成できませんでした", bot.Name)
		return
	}

	toot := mastodon.Toot{Status: msg}
	if err := bot.post(ctx, toot); err != nil {
		log.Printf("info: %s が引用コメントできませんでした。今回は諦めます……", bot.Name)
	}

	return
}

// messageFromParseResultは、パース結果とURLから投稿文を作成する。
func (bot *Persona) messageFromParseResult(result parseResult, url string) (msg string, err error) {
	// トゥートに使う単語の選定
	cds := result.candidates()
	best, err := bestCandidate(cds)
	if err != nil {
		log.Printf("info: %s が引用コメントの単語選定に失敗しました", bot.Name)
		return
	}

	// コメントの生成
	idx := 0
	if len(bot.Comments) > 1 {
		idx = rand.Intn(len(bot.Comments))
	}
	msg = bot.Comments[idx]
	msg = strings.Replace(msg, "_keyword1_", best.surface, -1)
	msg = strings.Replace(msg, "_topkana1_", best.firstKana, -1)

	// リンクを追加
	msg += "\n\n" + url
	log.Printf("trace: %s のトゥート内容：\n\n%s", bot.Name, msg)
	return
}

// respondToImagesは、画像が含まれているstatusに反応する。
func (bot *Persona) respondToImages(ctx context.Context, ev *mastodon.UpdateEvent) (err error) {
	// メンションは無視
	if len(ev.Status.Mentions) != 0 {
		return
	}

	// 自分のトゥートは無視
	if ev.Status.Account.ID == bot.MyID {
		return
	}

	// お気に入り画像を含んでたらhanage999に報告
	oshiri, label := label(ev.Status, "oshiri")
	if oshiri {
		msg := "おしり発見！\n"
		if strings.Contains(label, "naked") {
			msg = "裸の" + msg
		}

		/*		msg = "@hanage999 " + msg + ev.Status.URI
				toot := mastodon.Toot{Status: msg, Visibility: "direct"}
				if err = bot.post(ctx, toot); err != nil {
					log.Printf("info: %s がトゥートに失敗しました。\n", bot.Name)
					return
				}
		*/
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
	res, err := parse(bot.JobPool, txt)
	if err != nil {
		return
	}

	if bot.DBID == 2 {
		// 画像を含んでたらhanage999に報告
		oshiri, label := label(status, "oshiri")
		if oshiri {
			msg := "おしり発見！\n"
			if strings.Contains(label, "naked") {
				msg = "裸の" + msg
			}

			/*			msg = "@hanage999 " + msg + status.URI
						toot := mastodon.Toot{Status: msg, Visibility: "direct"}
						if err = bot.post(ctx, toot); err != nil {
							log.Printf("info: %s がトゥートに失敗しました。\n", bot.Name)
							return
						}
			*/
		}
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
			log.Printf("info: %s が関係取得に失敗しました", bot.Name)
			return err
		}
		if (*rel[0]).Following == true {
			msg = "@" + account.Acct + " " + name + "さんはもうフォローしてるから大丈夫" + bot.Assertion + "よー"
		} else {
			if err = bot.follow(ctx, account.ID); err != nil {
				log.Printf("info: %s がフォローに失敗しました", bot.Name)
				return err
			}
			msg = "@" + account.Acct + " わーい、お友達" + bot.Assertion + "ね！これからは、" + name + "さんのトゥートを生温かく見守っていく" + bot.Assertion + "よー"
		}
		if msg != "" {
			toot := mastodon.Toot{Status: msg, Visibility: status.Visibility, InReplyToID: status.ID}
			if err = bot.post(ctx, toot); err != nil {
				log.Printf("info: %s がリプライに失敗しました", bot.Name)
				return err
			}
		}
	case strings.Contains(txt, "いい"+bot.Assertion):
		yon := "だめ" + bot.Assertion + "よ"
		if rand.Intn(2) == 1 {
			yon = "いい" + bot.Assertion + "よ"
		}
		msg = "@" + account.Acct + " " + bot.Starter + name + bot.Title + "。" + yon
		if msg != "" {
			toot := mastodon.Toot{Status: msg, Visibility: status.Visibility, InReplyToID: status.ID}
			if err = bot.post(ctx, toot); err != nil {
				log.Printf("info: %s がリプライに失敗しました", bot.Name)
				return err
			}
		}
	}

	if jm.isWeatherRelated() {
		lc, dt, err := jm.judgeWeatherRequest()
		if err != nil {
			return err
		}
		data, err := GetRandomWeather(dt)
		if err != nil {
			log.Printf("info: %s が天気の取得に失敗しました", bot.Name)
			return err
		}
		ignoreStr := ""
		if lc != "" && lc != data.Location.City {
			ignoreStr = lc + "はともかく、"
		}
		msg = "@" + account.Acct + " " + ignoreStr + forecastMessage(data, bot.Assertion)
		if msg != "" {
			toot := mastodon.Toot{Status: msg, Visibility: status.Visibility, InReplyToID: status.ID}
			if err = bot.post(ctx, toot); err != nil {
				log.Printf("info: %s がリプライに失敗しました", bot.Name)
				return err
			}
		}
	}

	return
}
