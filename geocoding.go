package mastobots

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// loadLocInfo は、botの座標から所在地情報を取得して格納する
func getLocInfo(key string, lat, lng float64) (result OCResult, err error) {
	query := "https://api.opencagedata.com/geocode/v1/json?q=" + fmt.Sprint(lat) + "%2C" + fmt.Sprint(lng) + "&key=" + key + "&language=ja&pretty=1"

	res, err := http.Get(query)
	if err != nil {
		log.Printf("OpenCageへのリクエストに失敗しました：%s", err)
		return
	}
	if code := res.StatusCode; code >= 400 {
		err = fmt.Errorf("OpenCageへの接続エラーです(%d)", code)
		log.Printf("info: %s", err)
		return
	}
	var oc OCResults
	if err = json.NewDecoder(res.Body).Decode(&oc); err != nil {
		log.Printf("info: OpenCageからのレスポンスがデコードできませんでした：%s", err)
		res.Body.Close()
		return
	}
	res.Body.Close()

	result = oc.Results[0]

	return
}

// getDayCycleBySunMovement は、太陽の出入り時刻と現在時刻に応じて寝起きの時刻を返す
func getDayCycleBySunMovement(zone string, lat, lng float64) (wt, st time.Time, err error) {
	var loc *time.Location
	if strings.Contains(zone, "GMT") {
		offset, _ := strconv.Atoi(strings.Replace(zone, "GMT", "", -1))
		loc = time.FixedZone(zone, offset*60*60)
		fmt.Println(offset)
	} else {
		loc, err = time.LoadLocation(zone)
	}
	if err != nil {
		loc = time.Local
	}
	now := time.Now().In(loc)

	today, tomorrow := "today", "tomorrow"
	days := [...]string{today, tomorrow}

	sunrise := make(map[string]time.Time, len(days))
	sunset := make(map[string]time.Time, len(days))

	format := "2006-01-02T15:04:05-07:00"

	for i, day := range days {
		y, m, d := now.Add(time.Duration(i) * 24 * time.Hour).Date()
		dst := fmt.Sprintf("%d-%d-%d", y, int(m), d)
		url := "https://api.sunrise-sunset.org/json?" + "lat=" + fmt.Sprint(lat) + "&lng=" + fmt.Sprint(lng) + "&date=" + dst + "&formatted=0"

		res, err := http.Get(url)
		if err != nil {
			log.Printf("日の出日没時刻サイトへのリクエストに失敗しました：%s", err)
			return wt, st, err
		}
		if code := res.StatusCode; code >= 400 {
			err = fmt.Errorf("日の出日没時刻サイトへの接続エラーです(%d)", code)
			log.Printf("info: %s", err)
			return wt, st, err
		}
		var sun SunInfo
		if err = json.NewDecoder(res.Body).Decode(&sun); err != nil {
			log.Printf("info: %s の太陽の出入り時刻がデコードできませんでした：%s", day, err)
			res.Body.Close()
			return wt, st, err
		}
		res.Body.Close()

		sunrise[day], _ = time.Parse(format, sun.Results.Rise)
		sunset[day], _ = time.Parse(format, sun.Results.Set)

		y, _, _ = sunrise[day].Date()
		if y == 1970 {
			return wt, st, fmt.Errorf("白夜か黒夜です")
		}
	}

	wt = sunrise[today]
	st = sunset[today]

	now = time.Now().In(loc)

	if sunrise[today].Before(now) {
		wt = sunrise[tomorrow]
	}
	if sunset[today].Before(now) {
		st = sunset[tomorrow]
	}

	return
}
