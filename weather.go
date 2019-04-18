package mastobots

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/html"
	"gopkg.in/ugjka/go-tz.v2/tz"
)

// WeatherData は、livedoor天気予報のAPIが返してくるjsonデータを保持する
type WeatherData struct {
	Forecasts []Forecast
	Location  Location
}

// Forecast は、livedoor天気予報のAPIが返してくるjsonデータを保持する。
type Forecast struct {
	DateLabel   string `json:"dateLabel"`
	Telop       string `json:"telop"`
	Date        string `json:"date"`
	Temperature struct {
		Min struct {
			Celsius    string `json:"celsius"`
			Fahrenheit string `json:"fahrenheit"`
		}
		Max struct {
			Celsius    string `json:"celsius"`
			Fahrenheit string `json:"fahrenheit"`
		}
	}
	Image struct {
		Width  int    `json:"width"`
		URL    string `json:"url"`
		Title  string `json:"title"`
		Height int    `json:"height"`
	}
}

// Location は、livedoor天気予報のAPIが返してくるjsonデータを保持する
type Location struct {
	City       string `json:"city"`
	Area       string `json:"area"`
	Prefecture string `json:"prefecture"`
}

// SunInfo は、日の入りと日の出時刻を格納する
type SunInfo struct {
	Results struct {
		Rise string `json:"civil_twilight_begin"`
		Set  string `json:"civil_twilight_end"`
	} `json:"results"`
}

// getLocationCodes は、livedoor天気予報の地域コードを取得する
func getLocationCodes() (results map[string]interface{}, err error) {
	url := "http://weather.livedoor.com/forecast/rss/primary_area.xml"

	results = make(map[string]interface{})

	res, err := http.Get(url)
	if err != nil {
		log.Printf("%s へのリクエストに失敗しました：%s", url, err)
		return
	}
	defer res.Body.Close()

	if code := res.StatusCode; code >= 400 {
		err = fmt.Errorf("%s への接続エラーです(%d)", url, code)
		log.Printf("info: %s\n", err)
		return
	}

	doc, err := html.Parse(res.Body)
	if err != nil {
		log.Printf("%s のパースに失敗しました：%s", url, err)
		return
	}

	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "city" {
			var ttl, code string
			for _, a := range n.Attr {
				if a.Key == "title" {
					ttl = a.Val
				}
				if a.Key == "id" {
					code = a.Val
				}
			}
			results[ttl] = code
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(doc)
	return
}

// GetRandomWeather は、livedoor天気予報でランダムな地域の天気を取得する。
// when: 0は今日、1は明日、2は明後日
func GetRandomWeather(when int) (data WeatherData, err error) {
	// mapのrangeは順番がランダム化されるらしいので
	var code interface{}
	for _, code = range locationCodes {
		break
	}

	codeStr, _ := code.(string)
	url := "http://weather.livedoor.com/forecast/webservice/json/v1?city=" + codeStr

	res, err := http.Get(url)
	if err != nil {
		log.Printf("天気予報サイトへのリクエストに失敗しました：%s", err)
		return
	}

	if code := res.StatusCode; code >= 400 {
		err = fmt.Errorf("天気予報サイトへの接続エラーです(%d)", code)
		log.Printf("info: %s", err)
		return
	}
	defer res.Body.Close()

	var response WeatherData

	if err = json.NewDecoder(res.Body).Decode(&response); err != nil {
		log.Printf("info: 予報データがデコードできませんでした：%s", err)
		return
	}

	response.Forecasts[when].Telop, err = emojifyWeather(response.Forecasts[when].Telop)
	if err != nil {
		return
	}

	data.Forecasts = []Forecast{response.Forecasts[when]}
	data.Location = response.Location

	return
}

// EmojifyWeather は、天気を絵文字で表現する。
func emojifyWeather(telop string) (emojiStr string, err error) {
	if telop == "" {
		err = fmt.Errorf("info: 天気テキストが空です")
		return
	}

	rep := regexp.MustCompile(`晴れ?`)
	emojiStr = rep.ReplaceAllString(telop, "☀️")
	rep = regexp.MustCompile(`(止む)?\(?曇り?\)?`)
	emojiStr = rep.ReplaceAllString(emojiStr, "☁️")
	rep = regexp.MustCompile(`雨で暴風を伴う|暴風雨`)
	emojiStr = rep.ReplaceAllString(emojiStr, "🌀☔️")
	rep = regexp.MustCompile(`雪で暴風を伴う|暴風雪`)
	emojiStr = rep.ReplaceAllString(emojiStr, "🌀☃️")
	emojiStr = strings.Replace(emojiStr, "雨", "☂️", -1)
	emojiStr = strings.Replace(emojiStr, "雪", "⛄", -1)
	emojiStr = strings.Replace(emojiStr, "時々", "／", -1)
	emojiStr = strings.Replace(emojiStr, "のち", "→", -1)

	return
}

// forecastMessage は、天気予報を告げるメッセージを返す。
func forecastMessage(data WeatherData, assertion string) (msg string) {
	maxT := ""
	if t := data.Forecasts[0].Temperature.Max.Celsius; t != "" {
		maxT = "最高 " + t + "℃"
	}

	minT := ""
	if t := data.Forecasts[0].Temperature.Min.Celsius; t != "" {
		minT = "最低 " + t + "℃"
	}

	cm := " "
	spc := ""
	if maxT != "" || minT != "" {
		cm = "、"
		spc = " "
	}

	sep := ""
	if maxT != "" && minT != "" {
		sep = "・"
	}

	msg = data.Forecasts[0].DateLabel + "の" + data.Location.Prefecture + data.Location.City + "は " + data.Forecasts[0].Telop + cm + maxT + sep + minT + spc + "みたい" + assertion + "ね"

	return
}

// judgeWeatherRequest は、天気の要望の内容を判断する
func (result jumanResult) judgeWeatherRequest() (lc string, dt int, err error) {
	lc = result.getWeatherQueryLocation()
	dt = result.getWeatherQueryDate()
	return
}

// getWeatherQueryLocation は、天気情報の要望トゥートの形態素解析結果に地名が存在すればそれを返す。
func (result jumanResult) getWeatherQueryLocation() (loc string) {
	for _, node := range *result.Nodes {
		// 5番目の要素は品詞詳細、11番目の要素は諸情報
		if node[5] == "地名" || node[5] == "人名" || strings.Contains(node[11], "地名") || strings.Contains(node[11], "場所") {
			loc = node[0]
			return
		}
	}
	return
}

// getWeatherQueryDate は、天気情報の要望トゥートの形態素解析結果に日の指定があればそれを返す。
func (result jumanResult) getWeatherQueryDate() (date int) {
	for _, node := range *result.Nodes {
		switch node[1] {
		case "あす", "あした", "みょうにち":
			date = 1
			return
		case "あさって", "みょうごにち":
			date = 2
			return
		}
	}
	return
}

// isWeatherRelated は、文字列が天気関係の話かどうかを調べる。
func (result jumanResult) isWeatherRelated() bool {
	kws := [...]string{"天気", "気温", "暖", "暑", "雨", "晴", "曇", "雪", "風", "嵐", "雹", "湿", "乾", "冷える", "蒸す", "熱帯夜"}
	for _, node := range *result.Nodes {
		for _, w := range kws {
			if strings.Contains(node[11], w) {
				return true
			}
		}
	}
	return false
}

// getDayCycleBySunMovement は、太陽の出入り時刻と現在時刻に応じて寝起きの時刻を返す
func getDayCycleBySunMovement(lat, lng float64) (wt, st time.Time, zone string, err error) {
	var loc *time.Location
	zn, err := tz.GetZone(tz.Point{
		Lon: lng, Lat: lat,
	})
	if err != nil {
		loc, _ = time.LoadLocation("Local")
	} else {
		zone = zn[0]
		loc, _ = time.LoadLocation(zone)
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
			return wt, st, "", err
		}
		if code := res.StatusCode; code >= 400 {
			err = fmt.Errorf("日の出日没時刻サイトへの接続エラーです(%d)", code)
			log.Printf("info: %s", err)
			return wt, st, "", err
		}
		var sun SunInfo
		if err = json.NewDecoder(res.Body).Decode(&sun); err != nil {
			log.Printf("info: %s の太陽の出入り時刻がデコードできませんでした：%s", day, err)
			res.Body.Close()
			return wt, st, "", err
		}
		res.Body.Close()

		sunrise[day], _ = time.Parse(format, sun.Results.Rise)
		sunset[day], _ = time.Parse(format, sun.Results.Set)
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
