package main

import (
	"flag"
	"log"
	"os"

	"github.com/hanage999/mastobots"
)

func main() {
	os.Exit(run())
}

func run() (exitCode int) {
	exitCode = 0

	// 初期化

	// フラグ読み込み
	var p = flag.Int("p", 0, "実行終了までの時間（分）")
	flag.Parse()

	// もろもろ準備
	bots, db, err := mastobots.Initialize()
	if err != nil {
		log.Printf("alert: 初期化に失敗しました：%s", err)
		exitCode = 1
		return
	}
	defer db.Close()

	// 活動開始
	if err = mastobots.ActivateBots(bots, db, *p); err != nil {
		log.Printf("alert: 停止しました：%s", err)
		exitCode = 1
		return
	}

	return
}
