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
func (bot *Persona) periodicToot(ctx context.Context, db *DB) {
	log.Printf("info: %s が今日の定期トゥートを開始しました。", bot.Name)

	tc := tickAfterWait(ctx, until(-1, bot.FirstFire), time.Duration(bot.Interval)*time.Minute)
LOOP:
	for {
		select {
		case <-tc:
			go func() {
				toot, item, err := bot.createNewsToot(db)
				if err != nil {
					log.Printf("info :%s がトゥートの作成に失敗しました。\n", bot.Name)
					return
				}
				if item.Title != "" {
					if err := bot.post(ctx, toot); err != nil {
						log.Printf("info: %s がトゥートできませんでした。今回は諦めます……\n", bot.Name)
					} else {
						if err := db.deleteItem(bot, item); err != nil {
							log.Printf("info: %s がトゥート済みアイテムの削除に失敗しました。\n", bot.Name)
						}
					}
				}
			}()
		case <-ctx.Done():
			log.Printf("info: %s が今日の定期トゥートを終了しました。", bot.Name)
			break LOOP
		}
	}

}

// createNewsTootはトゥートする内容を作成する。
func (bot *Persona) createNewsToot(db *DB) (toot mastodon.Toot, item Item, err error) {
	// データベースから新規itemを物色してデータベースに登録
	if err != db.stockItems(bot) {
		log.Printf("info: %s がアイテムの収集に失敗しました。\n", bot.Name)
		return
	}

	// たまった候補からランダムに一つ選ぶ
	item, err = db.pickItem(bot)
	if err != nil {
		log.Printf("info: %s が投稿アイテムを選択できませんでした。\n", bot.Name)
	}
	if item.Title == "" {
		return
	}

	// 投稿トゥート作成
	msg, err := bot.messageFromItem(item)
	if err != nil {
		log.Printf("info: %s がアイテムid %d から投稿文の作成に失敗しました。\n", bot.Name, item.ID)
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
		txt = txt + "。" + item.Content
	}
	log.Printf("trace: 素のcontent：%s\n", txt)

	// 2ちゃんねるヘッダー除去
	rep := regexp.MustCompile(`\d+[:：]?.*\d{4}\/\d{2}\/\d{2}\(.\) *\d{2}:\d{2}:\d{2}(\.\d+)?( ID:[ -~｡-ﾟ]+)?`)
	txt = rep.ReplaceAllString(txt, "　")

	// url除去
	rep = regexp.MustCompile(`(http(s)?:\/\/)?([\w\-]+\.)+[\w-]+(\/[\w\- .\/?%&=]*)?`)
	txt = rep.ReplaceAllString(txt, "　")

	log.Printf("trace: id %d Jumanに食わせるcontent：%s\n\n", item.ID, txt)

	result, err := parse(txt)
	if err != nil {
		log.Printf("info: %s がトゥート時のサマリーのパースに失敗しました。\n", bot.Name)
		return
	}

	// トゥートに使う単語の選定
	candidates := make([]candidate, 0)
	for _, node := range result.Nodes {
		if node[5] != "普通名詞" && node[5] != "組織名" && node[5] != "人名" && node[5] != "地名" {
			continue
		}
		cd := candidate{node[0], string(getRuneAt(node[1], 0)), 700 + rand.Intn(2000)}
		if node[5] == "普通名詞" {
			cd.priority = rand.Intn(2000)
		}
		candidates = append(candidates, cd)
	}
	best := bestCandidate(candidates)

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
	msg += "\n\n【" + item.Title + "】 " + item.URL + "\n\n" + hashtagStr

	log.Printf("trace: %s のトゥート内容：\n\n%s", bot.Name, msg)

	return
}

// candidateはbotがあげつらう単語の候補。
type candidate struct {
	surface   string
	firstKana string
	priority  int
}

// bestCandidateは、candidateのスライスのうち優先度が最も高いものを返す。
func bestCandidate(items []candidate) (max candidate) {
	max = items[0]

	if len(items) == 1 {
		return
	}

	for i := 1; i < len(items); i++ {
		if items[i].priority > max.priority {
			max = items[i]
		}
	}

	return
}
