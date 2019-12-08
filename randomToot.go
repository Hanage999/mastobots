package mastobots

import (
	"context"
	"log"
	"math/rand"
	"time"

	mastodon "github.com/mattn/go-mastodon"
)

// randomTootは、ランダムにトゥートする。
func (bot *Persona) randomToot(ctx context.Context) {
	bt := 24 * 60 / bot.RandomFrequency
	ft := bt - bt*2/3 + rand.Intn(bt*4/3)
	itvl := time.Duration(ft) * time.Minute

	newCtx, cancel := context.WithTimeout(ctx, itvl)
	defer cancel()

	select {
	case <-newCtx.Done():
		idx := rand.Intn(len(bot.RandomToots))
		msg := bot.RandomToots[idx]
		if msg != "" {
			msg = msg + nuance()
			toot := mastodon.Toot{Status: msg}
			if err := bot.post(ctx, toot); err != nil {
				log.Printf("info: %s がランダムな呟きに失敗しました", bot.Name)
				return
			}
		}

		bot.randomToot(ctx)
	case <-ctx.Done():
	}
}

// nuance は、投稿にニュアンスを添えたり添えなかったりする。
func nuance() (s string) {
	gb := [...]string{"", "？", "?!", "!?", "！", "！！", "！！！", "！！！！", "！！！！！", "…", "……", "………", "w", "www", "…？", "…！", "…?!", "…?!", "…w", "……w", "………w"}
	s = gb[rand.Intn(len(gb))]
	return
}
