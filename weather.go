package mastobots

import (
	"encoding/json"
	"fmt"
	"golang.org/x/net/html"
	"log"
	"math/rand"
	"net/http"
	"regexp"
	"strings"
)

// WeatherDataは、livedoor天気予報のAPIが返してくるjsonデータを保持する
type WeatherData struct {
	Forecasts []Forecast
	Location  Location
}

// Forcastは、livedoor天気予報のAPIが返してくるjsonデータを保持する。
type Forecast struct {
	DateLabel   string `json: "dateLabel"`
	Telop       string `json: "telop"`
	Date        string `json: "date"`
	Temperature struct {
		Min struct {
			Celsius    string `json: "celsius"`
			Fahrenheit string `json: "fahrenheit"`
		}
		Max struct {
			Celsius    string `json: "celsius"`
			Fahrenheit string `json: "fahrenheit"`
		}
	}
	Image struct {
		Width  int    `json: "width"`
		URL    string `json: "url"`
		Title  string `json: "title"`
		Height int    `json: "height"`
	}
}

// Locationは、livedoor天気予報のAPIが返してくるjsonデータを保持する
type Location struct {
	City       string `json: "city"`
	Area       string `json: "area"`
	Prefecture string `json: "prefecture"`
}

// getLocationCodesは、livedoor天気予報の地域コードを取得する
func getLocationCodes() (results map[string]interface{}, err error) {
	url := "http://weather.livedoor.com/forecast/rss/primary_area.xml"

	results = make(map[string]interface{})

	res, err := http.Get(url)
	if err != nil {
		log.Printf("%s へのリクエストに失敗しました。：%s\n", url, err)
		return
	}
	defer res.Body.Close()

	if code := res.StatusCode; code >= 400 {
		err = fmt.Errorf("%s への接続エラーです(%d)。", url, code)
		log.Printf("info: %s\n", err)
		return
	}

	doc, err := html.Parse(res.Body)
	if err != nil {
		log.Printf("%s のパースに失敗しました。：%s", url, err)
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

// getRandomWeatherは、livedoor天気予報でランダムな地域の天気を取得する。
// when: 0は今日、1は明日、2は明後日
func GetRandomWeather(when int) (data WeatherData, err error) {
	_, code, err := RandomMap(locationCodes)
	if err != nil {
		log.Printf("info: %s\n", err)
		return
	}

	codeStr, _ := code.(string)
	url := "http://weather.livedoor.com/forecast/webservice/json/v1?city=" + codeStr

	res, err := http.Get(url)
	if err != nil {
		log.Printf("天気予報サイトへのリクエストに失敗しました。%s\n", err)
		return
	}

	if code := res.StatusCode; code >= 400 {
		err = fmt.Errorf("天気予報サイトへの接続エラーです(%d)。", code)
		log.Printf("info: %s\n", err)
		return
	}
	defer res.Body.Close()

	var response WeatherData

	if err = json.NewDecoder(res.Body).Decode(&response); err != nil {
		log.Printf("info: 予報データがデコードできませんでした。：%s", err)
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

// EmojifyWeatherは、天気を絵文字で表現する。
func emojifyWeather(telop string) (emojiStr string, err error) {
	if telop == "" {
		err = fmt.Errorf("info: 天気テキストが空です。")
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
	emojiStr = strings.Replace(emojiStr, "雪", "⛄️", -1)
	emojiStr = strings.Replace(emojiStr, "時々", "／", -1)
	emojiStr = strings.Replace(emojiStr, "のち", "→", -1)

	return
}

// forecastMessageは、天気予報を告げるメッセージを返す。
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

	msg = data.Forecasts[0].DateLabel + "の" + data.Location.City + "は " + data.Forecasts[0].Telop + cm + maxT + sep + minT + spc + "みたい" + assertion + "ね"

	return
}

// randomMapは、ランダムにmapを選び、そのキーと値を返す。
func RandomMap(m map[string]interface{}) (loc string, code interface{}, err error) {
	l := len(m)
	if l == 0 {
		err = fmt.Errorf("mapに要素がありません。\n")
		return
	}

	i := 0

	index := rand.Intn(l)

	for k, v := range m {
		if index == i {
			loc = k
			code = v
			break
		} else {
			i++
		}
	}

	return
}

// judgeWeatherRequestは、天気の要望の内容を判断する
func judgeWeatherRequest(txt string) (lc string, dt int, err error) {
	result, err := parse(txt)
	if err != nil {
		return
	}

	lc = result.getWeatherQueryLocation()
	dt = result.getWeatherQueryDate()

	return
}

// getWeatherQueryLocationは、天気情報の要望トゥートの形態素解析結果に地名が存在すればそれを返す。
func (result parseResult) getWeatherQueryLocation() (loc string) {
	for _, node := range result.Nodes {
		// 5番目の要素は品詞詳細、11番目の要素は諸情報
		if node[5] == "地名" || node[5] == "人名" || strings.Contains(node[11], "地名") || strings.Contains(node[11], "場所") {
			loc = node[0]
			return
		}
	}

	return
}

// getWeatherQueryDateは、天気情報の要望トゥートの形態素解析結果に日の指定があればそれを返す。
func (result parseResult) getWeatherQueryDate() (date int) {
	for _, node := range result.Nodes {
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
