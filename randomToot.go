package mastobots

import (
	"context"
	"log"
	"math/rand"
	"time"

	mastodon "github.com/hanage999/go-mastodon"
)

// randomTootは、ランダムにトゥートする。
func (bot *Persona) randomToot(ctx context.Context) {
	bt := 24 * 60 / bot.RandomFrequency
	ft := bt - bt*2/3 + rand.Intn(bt*4/3)
	itvl := time.Duration(ft) * time.Minute

	t := time.NewTimer(itvl)

	select {
	case <-t.C:
		idx := rand.Intn(len(bot.RandomToots))
		msg := bot.RandomToots[idx]
		if msg != "" {
			msg = msg + nuance()
			toot := mastodon.Toot{Status: msg}
			if err := bot.post(ctx, toot); err != nil {
				log.Printf("info: %s がランダムな呟きに失敗しました", bot.Name)
			}
		}

		bot.randomToot(ctx)
	case <-ctx.Done():
		if !t.Stop() {
			<-t.C
		}
	}
}

// nuance は、投稿にニュアンスを添えたり添えなかったりする。
func nuance() (s string) {
	gb := [...]string{"", "？", "?!", "!?", "！", "！！", "！！！", "！！！！", "！！！！！", "…", "……", "………", "w", "www", "…？", "…！", "…?!", "…?!", "…w", "……w", "………w"}
	s = gb[rand.Intn(len(gb))]
	return
}
