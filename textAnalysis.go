package mastobots

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"

	"golang.org/x/net/html"
	"gopkg.in/jdkato/prose.v2"
)

// parseResultはテキストの形態素解析結果のインターフェースを提供する。
type parseResult interface {
	length() int
	candidates() []candidate
	contain(str string) bool
}

// candidateはbotがあげつらう単語の候補。
type candidate struct {
	surface   string
	firstKana string
	priority  int
}

// Sudachi/UniDicの品詞体系には「形式名詞」がない。
// JumanDICで形式名詞として定義される8語のうち、Sudachiが名詞として出力する6語を辞書形で互換判定する。
// 残りの「の」と「ん」はSudachiでは助詞-準体助詞となるため、名詞候補から自動的に除外される。
var jumanFormalNounDictionaryForms = map[string]struct{}{
	"こと":  {},
	"はず":  {},
	"わけ":  {},
	"つもり": {},
	"もの":  {},
	"もん":  {},
}

const (
	sudachiSplitMode           = "B"
	defaultSudachiAPITimeout   = 20 * time.Second
	maxSudachiAPIResponseBytes = 16 << 20
)

type sudachiClient struct {
	endpoint   string
	httpClient *http.Client
}

type sudachiAPIRequest struct {
	Text string `json:"text"`
	Mode string `json:"mode"`
}

type sudachiAPIResponse struct {
	Tokens []sudachiAPIToken `json:"tokens"`
	Count  int               `json:"count"`
	Mode   string            `json:"mode"`
}

type sudachiAPIToken struct {
	Surface         string   `json:"surface"`
	PartOfSpeech    []string `json:"part_of_speech"`
	NormalizedForm  string   `json:"normalized_form"`
	DictionaryForm  *string  `json:"dictionary_form"`
	ReadingForm     *string  `json:"reading_form"`
	DictionaryID    *int     `json:"dictionary_id"`
	SynonymGroupIDs *[]int   `json:"synonym_group_ids"`
	OOV             *bool    `json:"oov"`
}

type sudachiAPIErrorResponse struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// sudachiMorpheme は、Sudachiで解析した一つの形態素を格納する。
type sudachiMorpheme struct {
	surface         string
	partOfSpeech    [6]string
	normalizedForm  string
	dictionaryForm  string
	reading         string
	dictionaryID    int
	synonymGroupIDs []int
	isOOV           bool
}

// sudachiResult は、テキストをSudachiで形態素解析した結果を格納する。
type sudachiResult struct {
	Nodes []sudachiMorpheme
}

// proseResult は、テキストをproseで形態素解析した結果を格納する
type proseResult struct {
	Nodes    []prose.Token
	Entities []prose.Entity
}

func (result sudachiResult) length() int {
	return len(result.Nodes)
}

func (result proseResult) length() int {
	return len(result.Nodes)
}

func (result sudachiResult) candidates() (cds []candidate) {
	cds = make([]candidate, 0)
	for _, node := range result.Nodes {
		if node.surface == "" || !node.isNoun() || node.isNumericNoun() || node.isJumanFormalNoun() {
			continue
		}
		cd := candidate{node.surface, string(getRuneAt(node.reading, 0)), rand.Intn(2000)}
		if node.isProperNoun() {
			cd.priority = 700 + rand.Intn(2000)
		}
		cds = append(cds, cd)
	}
	if len(cds) == 0 && len(result.Nodes) > 0 {
		log.Printf("info: Sudachi解析結果に名詞候補がありません（形態素数: %d、先頭の解析結果: %s）", len(result.Nodes), result.summary(10))
	}
	return
}

func (result proseResult) candidates() (cds []candidate) {
	cds = make([]candidate, 0)

	for _, node := range result.Nodes {
		if !strings.Contains(node.Tag, "NN") || node.Text == "\"" || node.Text == "." {
			continue
		}
		cd := candidate{node.Text, string(getRuneAt(node.Text, 0)), rand.Intn(2000)}
		cds = append(cds, cd)
		log.Printf("trace: %s, %s\n", node.Text, node.Tag)
	}

	for _, node := range result.Entities {
		if strings.Contains(node.Text, "\"") {
			continue
		}
		cd := candidate{node.Text, string(getRuneAt(node.Text, 0)), 700 + rand.Intn(2000)}
		cds = append(cds, cd)
		log.Printf("trace: %s, %s\n", node.Text, node.Label)
	}

	return
}

func (result sudachiResult) contain(str string) bool {
	for _, node := range result.Nodes {
		if node.dictionaryForm == str || node.normalizedForm == str || node.surface == str {
			log.Printf("trace: 一致した単語：%s", str)
			return true
		}
	}
	return false
}

func (result proseResult) contain(str string) bool {
	for _, node := range result.Nodes {
		if node.Text == str {
			log.Printf("trace: 一致した単語：%s", str)
			return true
		}
	}
	return false
}

// parseは、テキストを形態素解析した結果を返す。
func parse(settings *commonSettings, text string) (result parseResult, err error) {
	if text == "" {
		err = errors.New("解析する文字列が空です")
		log.Printf("info: %s", err)
		return
	}

	settings.langJobPool <- 0
	defer func() { <-settings.langJobPool }()

	if isJap(text) {
		result, err = parseJapanese(settings.sudachi, text)
	} else {
		result, err = parseEnglish(text)
	}

	return
}

func isJap(text string) bool {
	for _, r := range text {
		if unicode.In(r, unicode.Hiragana, unicode.Katakana) {
			return true
		}
	}
	return false
}

// parseEnglish は、英語のテキストをproseで形態素解析して結果を返す。
func parseEnglish(text string) (proseResult, error) {
	var tks []prose.Token
	var etts []prose.Entity

	{
		doc, err := prose.NewDocument(text, prose.WithSegmentation(false), prose.WithTokenization(false))
		if err != nil {
			log.Printf("info: 形態素解析器が正常に起動できませんでした：%s", err)
			return proseResult{tks, etts}, err
		}

		tks = doc.Tokens()
		etts = doc.Entities()
	}

	return proseResult{tks, etts}, nil
}

func newSudachiClient(endpoint string, timeout time.Duration) (*sudachiClient, error) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return nil, fmt.Errorf("設定項目 SudachiAPIURL が設定されていません")
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, fmt.Errorf("SudachiAPIURL が正しいHTTP URLではありません：%q", endpoint)
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("SudachiAPIURL にユーザー情報、クエリ、フラグメントは指定できません")
	}
	if !strings.HasSuffix(strings.TrimRight(parsed.Path, "/"), "/v1/analyze") {
		return nil, fmt.Errorf("SudachiAPIURL に /v1/analyze エンドポイントを指定してください")
	}
	if timeout <= 0 {
		timeout = defaultSudachiAPITimeout
	}
	return &sudachiClient{
		endpoint:   strings.TrimRight(endpoint, "/"),
		httpClient: &http.Client{Timeout: timeout},
	}, nil
}

// parseJapanese は、日本語のテキストをSudachi HTTP APIで形態素解析して結果を返す。
func parseJapanese(client *sudachiClient, text string) (result sudachiResult, err error) {
	requestBody, err := json.Marshal(sudachiAPIRequest{Text: text, Mode: sudachiSplitMode})
	if err != nil {
		return sudachiResult{}, fmt.Errorf("Sudachi APIリクエストを作成できません：%w", err)
	}
	request, err := http.NewRequest(http.MethodPost, client.endpoint, bytes.NewReader(requestBody))
	if err != nil {
		return sudachiResult{}, fmt.Errorf("Sudachi APIリクエストを作成できません：%w", err)
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := client.httpClient.Do(request)
	if err != nil {
		return sudachiResult{}, fmt.Errorf("Sudachi APIへの接続に失敗しました：%w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, maxSudachiAPIResponseBytes+1))
	if err != nil {
		return sudachiResult{}, fmt.Errorf("Sudachi APIレスポンスを読み込めません：%w", err)
	}
	if len(body) > maxSudachiAPIResponseBytes {
		return sudachiResult{}, fmt.Errorf("Sudachi APIレスポンスが大きすぎます")
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return sudachiResult{}, parseSudachiAPIError(response.StatusCode, body)
	}

	var apiResponse sudachiAPIResponse
	if err := json.Unmarshal(body, &apiResponse); err != nil {
		return sudachiResult{}, fmt.Errorf("Sudachi APIレスポンスが不正です：%w", err)
	}
	if apiResponse.Mode != sudachiSplitMode {
		return sudachiResult{}, fmt.Errorf("Sudachi APIレスポンスの分割モードが不正です：%q", apiResponse.Mode)
	}
	if apiResponse.Count != len(apiResponse.Tokens) {
		return sudachiResult{}, fmt.Errorf("Sudachi APIレスポンスの形態素数が一致しません：count=%d tokens=%d", apiResponse.Count, len(apiResponse.Tokens))
	}
	if len(apiResponse.Tokens) == 0 {
		return sudachiResult{}, fmt.Errorf("Sudachi APIの解析結果に形態素がありません")
	}

	nodes := make([]sudachiMorpheme, 0, len(apiResponse.Tokens))
	for i, token := range apiResponse.Tokens {
		if token.Surface == "" {
			return sudachiResult{Nodes: nodes}, fmt.Errorf("Sudachi APIの%d個目の形態素が不正です：表層形が空です", i+1)
		}
		if token.NormalizedForm == "" {
			return sudachiResult{Nodes: nodes}, fmt.Errorf("Sudachi APIの%d個目の形態素が不正です：正規化形が空です", i+1)
		}
		if len(token.PartOfSpeech) != 6 {
			return sudachiResult{Nodes: nodes}, fmt.Errorf("Sudachi APIの%d個目の形態素が不正です：品詞階層数が%dです", i+1, len(token.PartOfSpeech))
		}
		if token.DictionaryForm == nil || token.ReadingForm == nil || token.DictionaryID == nil || token.SynonymGroupIDs == nil || token.OOV == nil {
			return sudachiResult{Nodes: nodes}, fmt.Errorf("Sudachi APIの%d個目の形態素が不正です：-aの追加フィールドが不足しています", i+1)
		}
		var partOfSpeech [6]string
		copy(partOfSpeech[:], token.PartOfSpeech)
		dictionaryForm := *token.DictionaryForm
		if dictionaryForm == "" || dictionaryForm == "*" {
			dictionaryForm = token.NormalizedForm
		}
		reading := *token.ReadingForm
		if reading == "" || reading == "*" {
			reading = token.Surface
		} else {
			reading = katakanaToHiragana(reading)
		}
		nodes = append(nodes, sudachiMorpheme{
			surface:         token.Surface,
			partOfSpeech:    partOfSpeech,
			normalizedForm:  token.NormalizedForm,
			dictionaryForm:  dictionaryForm,
			reading:         reading,
			dictionaryID:    *token.DictionaryID,
			synonymGroupIDs: *token.SynonymGroupIDs,
			isOOV:           *token.OOV,
		})
	}
	return sudachiResult{Nodes: nodes}, nil
}

func parseSudachiAPIError(statusCode int, body []byte) error {
	var apiError sudachiAPIErrorResponse
	if err := json.Unmarshal(body, &apiError); err == nil && apiError.Error.Code != "" {
		return fmt.Errorf("Sudachi APIがエラーを返しました（HTTP %d, %s）：%s", statusCode, apiError.Error.Code, apiError.Error.Message)
	}
	return fmt.Errorf("Sudachi APIがエラーを返しました（HTTP %d）", statusCode)
}

func (result sudachiResult) summary(limit int) string {
	if limit > len(result.Nodes) {
		limit = len(result.Nodes)
	}
	items := make([]string, 0, limit)
	for _, node := range result.Nodes[:limit] {
		items = append(items, fmt.Sprintf("%s[%s]", node.surface, strings.Join(node.partOfSpeech[:], ",")))
	}
	return strings.Join(items, " ")
}

func (m sudachiMorpheme) isNoun() bool {
	return m.partOfSpeech[0] == "名詞"
}

func (m sudachiMorpheme) isProperNoun() bool {
	return m.isNoun() && m.partOfSpeech[1] == "固有名詞"
}

func (m sudachiMorpheme) isNumericNoun() bool {
	return m.isNoun() && m.partOfSpeech[1] == "数詞"
}

func (m sudachiMorpheme) isJumanFormalNoun() bool {
	if !m.isNoun() {
		return false
	}
	_, found := jumanFormalNounDictionaryForms[m.dictionaryForm]
	return found
}

func (m sudachiMorpheme) isPlaceName() bool {
	return m.isProperNoun() && m.partOfSpeech[2] == "地名"
}

func katakanaToHiragana(text string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'ァ' && r <= 'ヶ' {
			return r - 0x60
		}
		return r
	}, text)
}

// getRuneAtは、文字列の中のn番目の文字を返す。
// https://pinzolo.github.io/2016/05/31/golang-get-rune-from-string.html
func getRuneAt(s string, i int) rune {
	rs := []rune(s)
	if len(rs) == 0 {
		return 0
	}
	if i < 0 {
		i = 0
	}
	if len(rs) < i+1 {
		i = len(rs) - 1
	}
	return rs[i]
}

// textContentは、htmlからテキストを抽出する。
// https://github.com/mattn/go-mastodon/blob/master/cmd/mstdn/main.go より拝借
func textContent(s string) string {
	doc, err := html.Parse(strings.NewReader(s))
	if err != nil {
		return s
	}
	var buf bytes.Buffer

	var extractText func(node *html.Node, w *bytes.Buffer)
	extractText = func(node *html.Node, w *bytes.Buffer) {
		if node.Type == html.TextNode {
			data := strings.Trim(node.Data, "\r\n")
			if data != "" {
				w.WriteString(data)
			}
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			extractText(c, w)
		}
		if node.Type == html.ElementNode {
			name := strings.ToLower(node.Data)
			if name == "br" {
				w.WriteString("\n")
			}
		}
	}
	extractText(doc, &buf)

	return buf.String()
}

// bestCandidateは、candidateのスライスのうち優先度が最も高いものを返す。
func bestCandidate(items []candidate) (max candidate, err error) {
	if len(items) < 1 {
		err = errors.New("キーワード候補が見つかりませんでした")
		return
	}

	max = items[0]

	if len(items) == 1 {
		return
	}

	for i := 1; i < len(items); i++ {
		if items[i].priority > max.priority {
			max = items[i]
		}
	}

	return
}
