package mastobots

import (
	"database/sql"
	"log"
	"math/rand"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql" // for sql library
)

// DB は、データベース接続を格納する。
type DB struct {
	*sql.DB
}

// Item は、itemsテーブルの行データを格納する
type Item struct {
	ID      int
	Title   string
	URL     string
	Content string
	Summary string
	Keyword string
}

// newDBは、新たなデータベース接続を作成する。
func newDB(cr map[string]string) (db *DB, err error) {
	dbase, err := sql.Open("mysql", cr["user"]+":"+
		cr["password"]+
		"@tcp("+cr["Server"]+")/"+
		cr["database"]+
		"?parseTime=true&loc=Asia%2FTokyo")
	if err != nil {
		log.Printf("alert: データベースがOpenできませんでした：%s", err)
		return db, err
	}

	// 接続確認
	if err := dbase.Ping(); err != nil {
		log.Printf("alert: データベースに接続できませんでした：%s", err)
		return db, err
	}

	db = &DB{dbase}
	return
}

// addNewBotsは、もし新しいbotがいたらデータベースに登録する。
func (db *DB) addNewBots(bots []*Persona) (err error) {
	vsts := make([]string, 0)
	params := make([]interface{}, 0)
	now := time.Now()
	for _, bot := range bots {
		vsts = append(vsts, "(?, ?, ?)")
		params = append(params, bot.Name, now, now)
	}
	vst := strings.Join(vsts, ", ")
	_, err = db.Exec(`
		INSERT IGNORE INTO
			bots (name, created_at, updated_at)
		VALUES `+vst,
		params...,
	)
	if err != nil {
		log.Printf("info: botテーブルが更新できませんでした：%s", err)
		return
	}

	// auto_incrementの値を調整
	_, err = db.Exec(`
		ALTER TABLE bots
		AUTO_INCREMENT = 1
	`)
	if err != nil {
		log.Printf("info: itemsテーブルの自動採番値が調整できませんでした：%s", err)
		return
	}

	return
}

// deleteOldCandidates は、多すぎるトゥート候補を古いものから削除する
func (db *DB) deleteOldCandidates(bot *Persona, cap int) (err error) {
	_, err = db.Exec(`
		DELETE FROM candidates
		WHERE
			bot_id = ? AND id not in (
				SELECT * FROM (
					SELECT id FROM candidates
					ORDER BY id DESC limit ?
				) v
			)`,
		bot.DBID,
		cap,
	)
	if err != nil {
		log.Printf("alert: %s のDBエラーです：%s", bot.Name, err)
	}
	return
}

// stockItemsは、新規RSSアイテムの中からbotが興味を持ったitemをストックする。
func (db *DB) stockItems(bot *Persona) (err error) {
	// botの情報を取得
	var checkedUntil int
	if err := db.QueryRow(`
		SELECT
			checked_until
		FROM
			bots
		WHERE
			name = ?`,
		bot.Name,
	).Scan(&checkedUntil); err != nil {
		log.Printf("info: botsテーブルから %s の情報取得に失敗しました：%s", bot.Name, err)
		return err
	}

	// itemsテーブルから新規itemを取得
	rows, err := db.Query(`
		SELECT
			id, title, url, summary
		FROM
			items
		WHERE
			id > ?
		ORDER BY
			id DESC`,
		checkedUntil,
	)
	if err != nil {
		log.Printf("info: itemsテーブルから %s の趣味を集め損ねました：%s", bot.Name, err)
		return
	}
	defer rows.Close()

	// 結果を保存
	items := make([]Item, 0)
	for rows.Next() {
		var id int
		var title, url, summary string
		if err := rows.Scan(&id, &title, &url, &summary); err != nil {
			log.Printf("info: itemsテーブルから一行の情報取得に失敗しました：%s", err)
			continue
		}
		items = append(items, Item{ID: id, Title: title, URL: url, Summary: summary})
	}
	err = rows.Err()
	if err != nil {
		log.Printf("info: itemテーブルへの接続に結局失敗しました：%s", err)
		return
	}
	rows.Close()

	// 結果から、興味のある物件を収集
	myItems := make([]Item, 0)
	for _, item := range items {
		sumStr := item.Title
		if item.Summary != item.Title {
			sumStr = item.Title + "。\n" + textContent(item.Summary)
		}
		result, err := parse(sumStr)
		if err != nil {
			log.Printf("info: id: %d のサマリーのパースに失敗しました", item.ID)
			continue
		}

		if result.length() == 0 {
			continue
		}

		for _, w := range bot.Keywords {
			if result.contain(w) {
				item.Keyword = w
				myItems = append(myItems, item)
				log.Printf("trace: 収集されたitem_id: %d、 サマリー：%s", item.ID, sumStr)
				break
			}
		}
	}

	// 新規物件があったらcandidatesに登録
	if len(myItems) > 0 {
		vsts := make([]string, 0)
		params := make([]interface{}, 0)
		now := time.Now()
		for _, item := range myItems {
			vsts = append(vsts, "(?, ?, ?, ?, ?)")
			params = append(params, bot.DBID, item.ID, now, now, item.Keyword)
		}
		vst := strings.Join(vsts, ", ")
		_, err = db.Exec(`
			INSERT IGNORE INTO
				candidates (bot_id, item_id, created_at, updated_at, keyword)
			VALUES `+vst,
			params...,
		)
		if err != nil {
			log.Printf("info: candidatesテーブルが更新できませんでした：%s", err)
			return
		}
	}

	// botsテーブルのchecked_untilを更新
	if len(items) == 0 {
		return
	}
	_, err = db.Exec(`
		UPDATE bots
		SET checked_until = ?, updated_at = ?
		WHERE id = ?`,
		items[0].ID,
		time.Now(),
		bot.DBID,
	)
	if err != nil {
		log.Printf("info: %s のchecked_untilが更新できませんでした：%s", bot.Name, err)
	}

	return
}

// pickItemは、candidateから一件のitemをランダムで選択する。
func (db *DB) pickItem(bot *Persona) (item Item, err error) {
	// candidates, itemsテーブルから新規itemを取得
	rows, err := db.Query(`
		SELECT
			candidates.item_id, items.title, items.url, items.content
		FROM
			candidates
		INNER JOIN
			items
		ON
			candidates.item_id = items.id
		WHERE
			candidates.bot_id = ?`,
		bot.DBID,
	)
	if err != nil {
		log.Printf("info: %s の投稿候補を集め損ねました：%s", bot.Name, err)
		return
	}
	defer rows.Close()

	// 結果を保存
	items := make([]Item, 0)
	for rows.Next() {
		var id int
		var title, url, content string
		if err := rows.Scan(&id, &title, &url, &content); err != nil {
			log.Printf("info: itemsテーブルから一行の情報取得に失敗しました：%s", err)
			continue
		}
		items = append(items, Item{ID: id, Title: title, URL: url, Content: content})
	}
	err = rows.Err()
	if err != nil {
		log.Printf("info: itemテーブルの行読み込みに結局失敗しました：%s", err)
		return
	}
	rows.Close()
	if len(items) == 0 {
		return
	}

	// 一つランダムに選んで戻す
	n := len(items)
	if n > 0 {
		idx := 0
		if n > 1 {
			idx = rand.Intn(n)
		}

		item = items[idx]
	}

	return
}

// botIDは、botのデータベース上のIDを取得する。
func (db *DB) botID(bot *Persona) (id int, err error) {
	if err = db.QueryRow(`
		SELECT
			id
		FROM
			bots
		WHERE
			name = ?`,
		bot.Name,
	).Scan(&id); err != nil {
		log.Printf("info: botsテーブルから %s のID取得に失敗しました：%s", bot.Name, err)
		return
	}
	return
}

// deleteItemは、candidatesから一件を削除する。
func (db *DB) deleteItem(bot *Persona, item Item) (err error) {
	_, err = db.Exec(`
		DELETE FROM
			candidates
		WHERE
			item_id = ? AND bot_id = ?`,
		item.ID,
		bot.DBID,
	)
	if err != nil {
		log.Printf("alert: %s がcandidatesから%dの削除に失敗しました：%s", bot.Name, item.ID, err)
	}
	return
}
