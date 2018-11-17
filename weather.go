package mastobots

import (
	"log"
	"net/http"
	"encoding/json"
	"math/rand"
	"fmt"
	"strings"
	"regexp"
)

// Forcastは、livedoor天気予報のAPIが返してくるjsonデータを保持する。
type Forecast struct {
	DateLabel	string	`json: "dateLabel"`
	Telop		string	`json: "telop"`
	Date		string	`json: "date"`
	Temperature	struct {
		Min struct {
			Celsius	string	`json: "celsius"`
			Fahrenheit string `json: "fahrenheit"`
		}
		Max struct {
			Celsius string	`json: "celsius"`
			Fahrenheit string `json: "fahrenheit"`
		}
	}
	Image	struct {
		Width	int	`json: "width"`
		URL	string	`json: "url"`
		Title	string	`json: "title"`
		Height	int	`json: "height"`
	}
}

// getRandomWeatherは、livedoor天気予報でランダムな地域の天気を取得する。
// when: 0は今日、1は明日、2は明後日
func GetRandomWeather(when int) (loc string, forecast Forecast, err error) {
	var code interface{}
	loc, code, err = RandomMap(Cities)
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

	var result struct {
		Forecasts	[]Forecast
	}

	if err = json.NewDecoder(res.Body).Decode(&result); err != nil {
		log.Printf("info: 予報データがデコードできませんでした。：%s", err)
		return
	}

	result.Forecasts[when].Telop, err = emojifyWeather(result.Forecasts[when].Telop)
	if err != nil {
		return
	}

	forecast = result.Forecasts[when]

	return
}

// EmojifyWeatherは、天気を絵文字で表現する。
func emojifyWeather (telop string) (emojiStr string, err error) {
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
func judgeWeatherRequest(txt string) (lc string, dt int, err error){
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
		// 5番目の要素が品詞詳細
		if node[5] == "地名" || node[5] == "人名" {
			loc = node[0]
			return
		}
	}

	return
}

// getWeatherQueryDateは、天気情報の要望トゥートの形態素解析結果に日の指定があればそれを返す。
func (result parseResult) getWeatherQueryDate() (date int){
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

// Citiesは、livedoor天気で使われている地域コードを保持する
var Cities map[string]interface{} = map[string]interface{}{"高山": "210020", "相川": "150040", "大津": "250010", "中津": "440020", "室蘭": "015010", "宮古": "030020", "石垣島": "474010", "網走": "013010", "米沢": "060020", "松江": "320010", "新居浜": "380020", "清水": "390030", "旭川": "012010", "高田": "150030", "萩": "350040", "横手": "050020", "阿蘇乙姫": "430020", "和歌山": "300010", "伊万里": "410020", "秋田": "050010", "新庄": "060040", "東京": "130010", "松本": "200020", "岐阜": "210010", "大阪": "270000", "潮岬": "300020", "米子": "310020", "倶知安": "016030", "むつ": "020020", "新潟": "150010", "広島": "340010", "飯塚": "400030", "福江": "420040", "厳原": "420030", "人吉": "430040", "鹿屋": "460020", "前橋": "100010", "伏木": "160020", "佐賀": "410010", "高松": "370000", "白石": "040020", "津": "240010", "京都": "260010", "福島": "070010", "徳島": "360010", "佐世保": "420020", "日田": "440030", "福井": "180010", "庄原": "340020", "山口": "350020", "岩見沢": "016020", "飯田": "200030", "静岡": "220010", "名古屋": "230010", "柳井": "350030", "稚内": "011000", "北見": "013020", "浦河": "015020", "高知": "390010", "釧路": "014020", "小田原": "140020", "熊本": "430010", "浜田": "320020", "岡山": "330010", "館山": "120030", "長野": "200010", "彦根": "250020", "福岡": "400010", "久米島": "471030", "銚子": "120020", "尾鷲": "240020", "宇和島": "380030", "奈良": "290010", "鳥取": "310010", "那覇": "471010", "宮古島": "473000", "みなかみ": "100020", "秩父": "110030", "八丈島": "130030", "豊橋": "230020", "舞鶴": "260020", "松山": "380010", "宇都宮": "090010", "大島": "130020", "敦賀": "180020", "土浦": "080020", "甲府": "190010", "さいたま": "110010", "宮崎": "450010", "豊岡": "280020", "江差": "017020", "酒田": "060030", "熊谷": "110020", "三島": "220030", "西郷": "320030", "牛深": "430030", "延岡": "450020", "鹿児島": "460010", "紋別": "013030", "小名浜": "070020", "若松": "070030", "名瀬": "460040", "南大東": "472000", "札幌": "016010", "大船渡": "030030", "長岡": "150020", "高千穂": "450040", "留萌": "012020", "富山": "160010", "大分": "440010", "金沢": "170010", "日和佐": "360020", "水戸": "080010", "千葉": "120010", "父島": "130040", "室戸岬": "390020", "与那国島": "474020", "八戸": "020030", "津山": "330020", "下関": "350010", "風屋": "290020", "佐伯": "440040", "大田原": "090020", "輪島": "170020", "神戸": "280010", "根室": "014010", "盛岡": "030010", "種子島": "460030", "仙台": "040010", "横浜": "140010", "浜松": "220040", "帯広": "014030", "函館": "017010", "青森": "020010", "名護": "471020", "山形": "060010", "八幡": "400020", "久留米": "400040", "都城": "450030", "河口湖": "190020", "網代": "220020", "長崎": "420010"}
