package mastobots

import (
	"context"
	"log"
	"math/rand"
	"strings"
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
	if rand.Intn(2) == 0 {
		s = strings.Repeat("…", rand.Intn(4))
	} else {
		s = strings.Repeat("！", rand.Intn(6))
	}
	return
}
