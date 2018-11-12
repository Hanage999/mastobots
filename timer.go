package mastobots

import (
	"context"
	"time"
)

// tickAfterWaitは、最初は指定時間後に送信し、あとは別に指定する間隔ごとに送信するチャンネルを返す
func tickAfterWait(ctx context.Context, wait time.Duration, itvl time.Duration) (ch chan string) {
	ch = make(chan string)

	go func() {
		t := time.NewTimer(wait)
		select {
		case <-t.C:
			ch <- "first tick"
		case <-ctx.Done():
			t.Stop()
			return
		}

		tk := time.NewTicker(itvl)
	TICKER:
		for {
			select {
			case <-tk.C:
				ch <- "routine tick"
			case <-ctx.Done():
				tk.Stop()
				break TICKER
			}
		}

	}()

	return
}

// untilは、指定された時刻までのDurationを返す。hourが負数の時は、分だけが指定されたとみなす。
func until(hour, min int) (dur time.Duration) {
	now := time.Now()
	var add time.Duration
	var t time.Time

	if hour < 0 {
		t = time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), min, 0, 0, now.Location())
		add = 60 * time.Minute
	} else {
		t = time.Date(now.Year(), now.Month(), now.Day(), hour, min, 0, 0, now.Location())
		add = 24 * time.Hour
	}

	if t.Before(now) {
		t = t.Add(add)
	}

	dur = t.Sub(now)

	return
}
