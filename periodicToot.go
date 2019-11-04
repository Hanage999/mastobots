package mastobots

import (
	"context"
	"log"
	"math/rand"
	"regexp"
	"strings"
	"time"

	mastodon "github.com/mattn/go-mastodon"
)

// periodicTootは、指定された時刻（分）を皮切りに一定時間ごとにトゥートする。
func (bot *Persona) periodicToot(ctx context.Context, db DB) {
	itvl := time.Duration(bot.Interval) * time.Minute

	// 起動後最初のトゥートまでの待機時間を、Intervalより短くする
	delay := until(-1, bot.FirstFire, 0)
	for i := 1; delay > itvl; i++ {
		m := bot.FirstFire + bot.Interval*i
		if m >= 60 {
			m -= 60
		}
		delay = until(-1, m, 0)
	}

	tc := tickAfterWait(ctx, delay, itvl)
	log.Printf("info: %s が今日の定期トゥートを開始しました", bot.Name)

LOOP:
	for {
		select {
		case _, ok := <-tc:
			if !ok {
				log.Printf("info: %s が今日の定期トゥートを終了しました", bot.Name)
				break LOOP
			}
			go func() {
				if err := db.deleteOldCandidates(bot); err != nil {
					log.Printf("info :%s が古いトゥート候補の削除に失敗しました", bot.Name)
					return
				}
				if err := db.stockItems(bot); err != nil {
					log.Printf("info: %s がアイテムの収集に失敗しました", bot.Name)
					return
				}
				toot, item, err := bot.createNewsToot(db)
				if err != nil {
					log.Printf("info :%s がトゥートの作成に失敗しました", bot.Name)
					return
				}
				if item.Title != "" {
					if err := bot.post(ctx, toot); err != nil {
						log.Printf("info: %s がトゥートできませんでした。今回は諦めます……", bot.Name)
					} else {
						if err := db.deleteItem(bot, item); err != nil {
							log.Printf("info: %s がトゥート済みアイテムの削除に失敗しました", bot.Name)
						}
					}
				}
			}()
		}
	}
}

// createNewsTootはトゥートする内容を作成する。
func (bot *Persona) createNewsToot(db DB) (toot mastodon.Toot, item Item, err error) {
	// たまった候補からランダムに一つ選ぶ
	item, err = db.pickItem(bot)
	if err != nil {
		log.Printf("info: %s が投稿アイテムを選択できませんでした", bot.Name)
	}
	if item.Title == "" {
		return
	}

	// 投稿トゥート作成
	msg, err := bot.messageFromItem(item)
	if err != nil {
		log.Printf("info: %s がアイテムid %d から投稿文の作成に失敗しました：%s", bot.Name, item.ID, err)
		return
	}

	if msg != "" {
		toot = mastodon.Toot{Status: msg}
	}
	return
}

// messageFromItemは、itemの内容から投稿文を作成する。
func (bot *Persona) messageFromItem(item Item) (msg string, err error) {
	txt := item.Title
	if !strings.HasPrefix(item.Content, txt) {
		txt = txt + "\n" + item.Content
	}
	log.Printf("trace: 素のcontent：%s", txt)

	// 2ちゃんねるヘッダー除去
	rep := regexp.MustCompile(`\d+[:：]?.*\d{4}\/\d{2}\/\d{2}\(.\) *\d{2}:\d{2}:\d{2}(\.\d+)?( ID:[ -~｡-ﾟ]+)?`)
	txt = rep.ReplaceAllString(txt, " ")

	// url除去
	rep = regexp.MustCompile(`(http(s)?:\/\/)?([\w\-]+\.)+[\w-]+(\/[\w\- .\/?%&=]*)?`)
	txt = rep.ReplaceAllString(txt, " ")

	log.Printf("trace: id %d 形態素解析に食わせるcontent：%s", item.ID, txt)

	result, err := parse(txt)
	if err != nil {
		log.Printf("info: %s がトゥート時のサマリーのパースに失敗しました", bot.Name)
		return
	}

	// トゥートに使う単語の選定
	cds := result.candidates()
	best, err := bestCandidate(cds)
	if err != nil {
		log.Printf("info: 単語選定に失敗した本文：%s", txt)
		return
	}

	// ハッシュタグ生成
	var hashtagStr string
	for _, t := range bot.Hashtags {
		hashtagStr += `#` + t + " "
	}
	hashtagStr = strings.TrimSpace(hashtagStr)

	// コメントの生成
	idx := 0
	if len(bot.Comments) > 1 {
		idx = rand.Intn(len(bot.Comments))
	}
	msg = bot.Comments[idx]
	msg = strings.Replace(msg, "_keyword1_", best.surface, -1)
	msg = strings.Replace(msg, "_topkana1_", best.firstKana, -1)

	// リンクとハッシュタグを追加
	msg += "\n\n" + item.Title + " " + item.URL + "\n\n" + hashtagStr
	log.Printf("trace: %s のトゥート内容：\n\n%s", bot.Name, msg)
	return
}
