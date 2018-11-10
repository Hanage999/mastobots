package main

import (
	"flag"
	"github.com/hanage999/mastobots"
	"log"
	"math/rand"
	"os"
	"time"
)

func main() {
	os.Exit(run())
}

func run() (exitCode int) {
	exitCode = 0

	// 初期化

	// ランダムのシード値を設定
	rand.Seed(time.Now().UnixNano())

	// フラグ読み込み
	var p = flag.Int("p", 0, "実行終了までの時間（分）")
	flag.Parse()

	// botsの準備
	bots, db, err := mastobots.Initialize()
	if err != nil {
		log.Printf("alert: 初期化に失敗しました。理由：%s\n", err)
		exitCode = 1
		return
	}
	defer db.Close()

	// 活動開始
	if err = mastobots.ActivateBots(bots, db, *p); err != nil {
		log.Printf("alert: 停止しました。理由：%s\n", err)
		exitCode = 1
		return
	}

	return
}
