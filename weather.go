package mastobots

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
)

// OWForcast は、OpenWeatherMapからの天気予報データを格納する
type OWForcast struct {
	Dt        int64       `json:"dt"`
	Temp      interface{} `json:"temp"`
	FeelsLike interface{} `json:"feels_like"`
	Pressure  int         `json:"pressure"`
	Humidity  int         `json:"humidity"`
	WindSpeed float64     `json:"wind_speed"`
	WindDeg   int         `json:"wind_deg"`
	Weather   []struct {
		ID          int    `json:"id"`
		Main        string `json:"main"`
		Description string `json:"description"`
		Icon        string `json:"icon"`
	} `json:"weather"`
}

// OWForcasts は、OpenWeatherMapからの天気予報データを格納する
type OWForcasts struct {
	Current OWForcast   `json:"current"`
	Daily   []OWForcast `json:"daily"`
}

// isWeatherRelated は、文字列が天気関係の話かどうかを調べる。
func (result sudachiResult) isWeatherRelated() bool {
	kws := [...]string{"天気", "気温", "気圧", "雷", "嵐", "暖", "暑", "雨", "晴", "曇", "雪", "風", "嵐", "雹", "湿", "乾", "冷える", "蒸す", "熱帯夜", "何度"}
	for _, node := range result.Nodes {
		for _, w := range kws {
			if strings.Contains(node.surface, w) || strings.Contains(node.normalizedForm, w) || strings.Contains(node.dictionaryForm, w) {
				return true
			}
		}
	}
	return false
}

// judgeWeatherRequest は、天気の要望の内容を判断する
func (result sudachiResult) judgeWeatherRequest() (lc []string, dt int, fl bool, err error) {
	lc = result.getWeatherQueryLocation()
	dt = result.getWeatherQueryDate()
	fl = result.getWeatherQueryTempType()
	return
}

// getWeatherQueryLocation は、天気情報の要望トゥートの形態素解析結果に地名が存在すればそれを返す。
func (result sudachiResult) getWeatherQueryLocation() (loc []string) {
	for _, node := range result.Nodes {
		if node.isPlaceName() {
			if node.surface != "周辺" && node.surface != "場所" && node.surface != "公園" && node.reading != "ところ" && node.reading != "あたり" && node.reading != "へん" && node.surface != "地域" && node.surface != "地区" && node.surface != "県" && node.surface != "市" && node.surface != "町" && node.surface != "村" && node.surface != "府" && node.surface != "州" && node.surface != "郡" && node.surface != "地方" && node.surface != "どうなん" {
				loc = append(loc, node.surface)
			}
		}
	}
	return
}

// getWeatherQueryDate は、天気情報の要望トゥートの形態素解析結果に日の指定があればそれを返す。
func (result sudachiResult) getWeatherQueryDate() (date int) {
	for _, node := range result.Nodes {
		switch node.reading {
		case "あす", "あした", "みょうにち":
			date = 1
			return
		case "あさって", "みょうごにち":
			date = 2
			return
		case "いま", "げんざい":
			date = -1
			return
		}
	}
	return
}

// getWeatherQueryTempType は、天気情報の要望トゥートの形態素解析結果に体感温度表示の指定があればそれを返す。
func (result sudachiResult) getWeatherQueryTempType() (fl bool) {
	for _, node := range result.Nodes {
		if node.surface == "体感" {
			fl = true
			return
		}
	}
	return
}

// GetLocationWeather は、指定された座標の天気をOpenWeatherMapで取得する。
// when: -1は今、0は今日、1は明日、2は明後日
func GetLocationWeather(weatherKey string, lat, lng float64, when int) (data OWForcast, err error) {
	weatherKey = url.QueryEscape(weatherKey)
	query := "https://api.openweathermap.org/data/3.0/onecall?lat=" + fmt.Sprintf("%f", lat) + "&lon=" + fmt.Sprintf("%f", lng) + "&units=metric&lang=ja&exclude=hourly,minutely&appid=" + weatherKey

	res, err := http.Get(query)
	if err != nil {
		log.Printf("info: OpenWeatherMapへのリクエストに失敗しました：%s", err)
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

	if when == -1 {
		data = ow.Current
	} else if len(ow.Daily) > 0 {
		data = ow.Daily[when]
	} else {
		err = fmt.Errorf("OpenWeatherMapからのデータに天気の情報が含まれていません")
		log.Printf("info: %s", err)
	}

	return
}

// forecastMessage は、天気予報を告げるメッセージを返す。
func forecastMessage(locString string, wdata OWForcast, when int, assertion string, botLoc bool, fl bool) (msg string) {
	whenstr := ""
	switch when {
	case -1:
		whenstr = "今現在"
	case 0:
		whenstr = "今日"
	case 1:
		whenstr = "明日"
	case 2:
		whenstr = "明後日"
	}

	locStr := "このあたりは"
	if !botLoc {
		locStr = locString + "は"
	}

	description := strings.Replace(wdata.Weather[0].Description, "適度な", "", -1) + "、"

	tempstr := ""
	if when == -1 {
		if fl {
			feelslike, _ := wdata.FeelsLike.(float64)
			tempstr = fmt.Sprintf("体感 %.1f℃、", feelslike)
		} else {
			temp, _ := wdata.Temp.(float64)
			tempstr = fmt.Sprintf("気温 %.1f℃、", temp)
		}
	} else {
		mornT := ""

		var temp map[string]interface{}
		if fl {
			temp, _ = wdata.FeelsLike.(map[string]interface{})
			mornT = "体感で"
		} else {
			temp, _ = wdata.Temp.(map[string]interface{})
		}

		t, _ := temp["morn"].(float64)
		mornT = mornT + fmt.Sprintf("朝 %.1f℃・", t)

		dayT := ""
		t = temp["day"].(float64)
		dayT = fmt.Sprintf("日中 %.1f℃・", t)

		eveT := ""
		t = temp["eve"].(float64)
		eveT = fmt.Sprintf("夕方 %.1f℃・", t)

		nightT := ""
		t = temp["night"].(float64)
		nightT = fmt.Sprintf("夜 %.1f℃、", t)

		tempstr = mornT + dayT + eveT + nightT
	}

	hmdstr := fmt.Sprintf("湿度 %d%%、", wdata.Humidity)

	windstr := ""
	winddeg := wdata.WindDeg
	switch {
	case winddeg >= 338 || winddeg < 23:
		windstr = "北の風 "
	case winddeg >= 293:
		windstr = "北西の風 "
	case winddeg >= 248:
		windstr = "西の風 "
	case winddeg >= 203:
		windstr = "南西の風 "
	case winddeg >= 158:
		windstr = "南の風 "
	case winddeg >= 113:
		windstr = "南東の風 "
	case winddeg >= 68:
		windstr = "東の風 "
	case winddeg >= 23:
		windstr = "北東の風 "
	}
	windstr = windstr + fmt.Sprintf("%.1fm/s、", wdata.WindSpeed)

	pressurestr := fmt.Sprintf("気圧は %dhPa", wdata.Pressure)

	msg = whenstr + "の" + locStr + description + tempstr + hmdstr + windstr + pressurestr + "みたい" + assertion + "ね"

	return
}
