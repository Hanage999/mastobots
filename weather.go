package mastobots

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
)

// OWForcasts は、OpenWeatherMapからの天気予報データを格納する
type OWForcasts struct {
	Current struct {
		Dt       int64   `json:"dt"`
		Temp     float64 `json:"TEMP"`
		Humidity int     `json:"humidity"`
		Weather  []struct {
			ID          int    `json:"id"`
			Main        string `json:"main"`
			Description string `json:"description"`
			Icon        string `json:"icon"`
		} `json:"weather"`
	}
	Daily []OWForcast `json:"daily"`
}

// OWForcast は、OpenWeatherMapからの天気予報データを格納する
type OWForcast struct {
	Dt   int64 `json:"dt"`
	Temp struct {
		Morn  float64 `json:"morn"`
		Day   float64 `json:"day"`
		Eve   float64 `json:"eve"`
		Night float64 `json:"night"`
		Min   float64 `json:"min"`
		Max   float64 `json:"max"`
	} `json:"temp"`
	Humidity int `json:"humidity"`
	Weather  []struct {
		ID          int    `json:"id"`
		Main        string `json:"main"`
		Description string `json:"description"`
		Icon        string `json:"icon"`
	} `json:"weather"`
}

// isWeatherRelated は、文字列が天気関係の話かどうかを調べる。
func (result jumanResult) isWeatherRelated() bool {
	kws := [...]string{"天気", "気温", "湿度", "暖", "暑", "雨", "晴", "曇", "雪", "風", "嵐", "雹", "湿", "乾", "冷える", "蒸す", "熱帯夜"}
	for _, node := range result.Nodes {
		for _, w := range kws {
			if strings.Contains(node[11], w) {
				return true
			}
		}
	}
	return false
}

// judgeWeatherRequest は、天気の要望の内容を判断する
func (result jumanResult) judgeWeatherRequest() (lc []string, dt int, err error) {
	lc = result.getWeatherQueryLocation()
	dt = result.getWeatherQueryDate()
	return
}

// getWeatherQueryLocation は、天気情報の要望トゥートの形態素解析結果に地名が存在すればそれを返す。
func (result jumanResult) getWeatherQueryLocation() (loc []string) {
	for _, node := range result.Nodes {
		// 5番目の要素は品詞詳細、11番目の要素は諸情報
		if node[5] == "地名" || node[5] == "人名" || strings.Contains(node[11], "地名") || strings.Contains(node[11], "場所") {
			loc = append(loc, node[0])
		}
	}
	return
}

// getWeatherQueryDate は、天気情報の要望トゥートの形態素解析結果に日の指定があればそれを返す。
func (result jumanResult) getWeatherQueryDate() (date int) {
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

// GetLocationWeather は、指定された座標の天気をOpenWeatherMapで取得する。
// when: 0は今日、1は明日、2は明後日
func GetLocationWeather(lat, lng float64, when int) (data OWForcast, err error) {
	query := "https://api.openweathermap.org/data/2.5/onecall?lat=" + fmt.Sprintf("%f", lat) + "&lon=" + fmt.Sprintf("%f", lng) + "&units=metric&lang=ja&exclude=hourly,minutely&appid=" + weatherKey

	res, err := http.Get(query)
	if err != nil {
		log.Printf("OpenWeatherMapへのリクエストに失敗しました：%s", err)
		return
	}
	if code := res.StatusCode; code >= 400 {
		err = fmt.Errorf("OpenWeatherMapへの接続エラーです(%d)", code)
		log.Printf("info: %s", err)
		return
	}
	var ow OWForcasts
	if err = json.NewDecoder(res.Body).Decode(&ow); err != nil {
		log.Printf("info: OpenWeatherMapからのレスポンスがデコードできませんでした：%s", err)
		res.Body.Close()
		return
	}
	res.Body.Close()

	if len(ow.Daily) > 0 {
		data = ow.Daily[when]
	}

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
func forecastMessage(ldata OCResult, wdata OWForcast, when int, assertion string) (msg string) {
	maxT := ""
	t := wdata.Temp.Max
	maxT = "最高 " + fmt.Sprintf("%.1f", t) + "℃"

	minT := ""
	t = wdata.Temp.Min
	minT = "最低 " + fmt.Sprintf("%.1f", t) + "℃"

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

	whenstr := ""
	switch when {
	case 0:
		whenstr = "今日"
	case 1:
		whenstr = "明日"
	case 2:
		whenstr = "明後日"
	}

	msg = whenstr + "の" + getLocString(ldata, false) + "は" + wdata.Weather[0].Description + cm + maxT + sep + minT + "、湿度 " + fmt.Sprintf("%d", wdata.Humidity) + "%" + spc + "みたい" + assertion + "ね"

	return
}

// forecastMorningMessage は、起きがけの天気予報メッセージを返す。
func forecastMorningMessage(wdata OWForcast, when int, assertion string) (msg string) {
	maxT := ""
	t := wdata.Temp.Max
	maxT = "最高 " + fmt.Sprintf("%f", t) + "℃"

	minT := ""
	t = wdata.Temp.Min
	minT = "最低 " + fmt.Sprintf("%f", t) + "℃"

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

	whenstr := ""
	switch when {
	case 0:
		whenstr = "今日"
	case 1:
		whenstr = "明日"
	case 2:
		whenstr = "明後日"
	}

	msg = whenstr + "の" + "このあたりは" + wdata.Weather[0].Description + cm + maxT + sep + minT + "、湿度 " + fmt.Sprintf("%d", wdata.Humidity) + "%" + spc + "みたい" + assertion + "ね"

	return
}
