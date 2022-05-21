package mastobots

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// OCResult は、OpenCageからのデータを格納する
type OCResult struct {
	Annotations struct {
		Flag     string `json:"flag"`
		Timezone struct {
			Name string `json:"name"`
		} `json:"timezone"`
	} `json:"annotations"`
	Components map[string]interface{} `json:"components"`
	Geometry   struct {
		Lat float64 `json:"lat"`
		Lng float64 `json:"lng"`
	} `json:"geometry"`
}

// OCResults は、OpenCageからのデータを格納する
type OCResults struct {
	Results []OCResult `json:"results"`
}

// SunInfo は、日の入りと日の出時刻を格納する
type SunInfo struct {
	Results struct {
		Rise string `json:"civil_twilight_begin"`
		Set  string `json:"civil_twilight_end"`
		Noon string `json:"solar_noon"`
	} `json:"results"`
}

// getLocDatafromCoordinates は、botの座標から所在地情報を取得して格納する
func getLocDataFromCoordinates(key string, lat, lng float64) (result OCResult, err error) {
	query := "https://api.opencagedata.com/geocode/v1/json?q=" + fmt.Sprint(lat) + "%2C" + fmt.Sprint(lng) + "&key=" + key + "&language=ja&pretty=1"

	res, err := http.Get(query)
	if err != nil {
		log.Printf("info: OpenCageへのリクエストに失敗しました：%s", err)
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

// getLocDataFromString は、OpenCageDataから地名に該当する座標データを返す
func getLocDataFromString(key string, loc []string) (data OCResult, err error) {
	area := strings.Join(loc, " ")
	areaq := url.QueryEscape(area)
	query := "https://api.opencagedata.com/geocode/v1/json?q=" + areaq + "&key=" + key + "&language=ja&pretty=1"

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

	if len(oc.Results) > 0 {
		data = oc.Results[0]
	} else {
		err = fmt.Errorf("そんな地名おまへんがな")
	}

	return
}

// getLocString は、現在地の文字列を返す
func getLocString(data OCResult, simple bool) (str string) {

	mp := make(map[string]string)

	for k, v := range data.Components {
		if txt, ok := v.(string); ok {
			mp[k] = txt
		} else {
			mp[k] = ""
		}
	}

	tp := mp["_type"]
	str = mp[tp]

	country := mp["country"] + data.Annotations.Flag
	state := mp["state"]
	stateDistrict := mp["state_district"]
	county := mp["county"]
	city := mp["city"]
	cityDistrict := mp["city_district"]
	suburb := mp["suburb"]
	town := mp["town"]
	neighborhood := mp["neighborhood"]
	unknown := mp["unknown"]

	names := [...]string{unknown, neighborhood, town, suburb, cityDistrict, city}
	for _, name := range names {
		if str != "" {
			break
		}
		str = name
	}

	if simple {
		return
	}

	if country == "" {
		country = "国ではないどこか"
	}

	countryKakko := ""
	if strings.Contains(country, "日本") {
		country = ""
	} else {
		countryKakko = "（" + country + "）"
	}

	nameadrs := [...]*string{&stateDistrict, &county, &city, &cityDistrict, &suburb, &town, &neighborhood}
	for _, name := range nameadrs {
		if str == *name {
			*name = ""
		}
	}

	if town == city {
		town = ""
	}

	if str == state {
		if country != "" {
			str = country + "の" + str
		}
	} else {
		details := state + stateDistrict + county + city + cityDistrict + suburb + town + neighborhood + countryKakko
		if details != "" {
			str = details + "の" + str
		}
	}

	return
}

// getDayCycleBySunMovement は、太陽の出入り時刻と現在時刻に応じて寝起きの時刻を返す
func getDayCycleBySunMovement(zone string, lat, lng float64) (sleep, active time.Duration, cond string, err error) {
	wt, st, err := getSleepWakeTimeBySunMovement(zone, lat, lng)
	if err != nil {
		return
	}

	if wt.IsZero() {
		sleep = time.Until(st)
		active = 0
		cond = "極夜"
		return
	}

	if st.IsZero() {
		sleep = 0
		active = time.Until(wt)
		cond = "白夜"
		return
	}

	sleep = time.Until(wt)
	tillSleep := time.Until(st)
	active = st.Sub(wt)
	if active < 0 {
		active += 24 * time.Hour
	}
	if active > tillSleep {
		sleep = 0
		active = tillSleep
	}

	return
}

// getSleepWakeTimeBySunMovement は、太陽の出入り時刻と現在時刻に応じて寝起きの時刻を返す
func getSleepWakeTimeBySunMovement(zone string, lat, lng float64) (wt, st time.Time, err error) {
	var loc *time.Location
	if strings.Contains(zone, "GMT") {
		offset, _ := strconv.Atoi(strings.Replace(zone, "GMT", "", -1))
		loc = time.FixedZone(zone, offset*60*60)
		log.Printf("info: GMTからのオフセット：%d", offset)
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
	noon := make(map[string]time.Time, len(days))

	format := "2006-01-02T15:04:05-07:00"

	for i, day := range days {
		y, m, d := now.Add(time.Duration(i) * 24 * time.Hour).Date()
		dst := fmt.Sprintf("%d-%d-%d", y, int(m), d)
		url := "https://api.sunrise-sunset.org/json?" + "lat=" + fmt.Sprint(lat) + "&lng=" + fmt.Sprint(lng) + "&date=" + dst + "&formatted=0"

		res, err := http.Get(url)
		if err != nil {
			log.Printf("info: 日の出日没時刻サイトへのリクエストに失敗しました：%s", err)
			return wt, st, err
		}
		if code := res.StatusCode; code >= 400 {
			err = fmt.Errorf("info 日の出日没時刻サイトへの接続エラーです(%d)", code)
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
		noon[day], _ = time.Parse(format, sun.Results.Noon)

	}

	for _, day := range days {
		y, _, _ := sunrise[day].Date()
		if y == 1970 {
			wt, st = getExtremeCycle(noon, loc, lat)
			return wt, st, nil
		}
	}

	wt = sunrise[today]
	st = sunset[today]

	now = time.Now()
	if sunrise[today].Before(now) {
		wt = sunrise[tomorrow]
	}
	if sunset[today].Before(now) {
		st = sunset[tomorrow]
	}

	return
}

// getExtremeCycle は、白夜あるいは極夜の生活サイクルを返す
func getExtremeCycle(noon map[string]time.Time, loc *time.Location, lat float64) (wt, st time.Time) {
	now := time.Now()
	today, tomorrow := "today", "tomorrow"

	isWhite := whiteOrBlack(loc, lat)

	if isWhite {
		// wtを１日で最も暗い時刻に設定
		wt = noon[today].Add(12 * time.Hour)
		if wt.Before(now) {
			wt = noon[tomorrow].Add(12 * time.Hour)
		}
		st = time.Time{}
	} else {
		// stを１日で最も明るい時刻に設定
		wt = time.Time{}
		st = noon[today]
		if st.Before(now) {
			st = noon[tomorrow]
		}
	}

	return
}

// whiteOrBlack は、現在その地点が白夜か極夜かを返す
func whiteOrBlack(loc *time.Location, lat float64) bool {
	now := time.Now().In(loc)
	_, m, _ := now.Date()

	if lat > 0 {
		if 3 < int(m) && int(m) < 10 {
			// 白夜
			return true
		}
		// 極夜
		return false
	}

	if 3 < int(m) && int(m) < 10 {
		// 極夜
		return false
	}
	// 白夜
	return true
}
