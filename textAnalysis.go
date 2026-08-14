package mastobots

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"os/exec"
	"strconv"
	"strings"
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
		if node.surface == "" || node.reading == "" || !node.isNoun() || node.isNumericNoun() || node.isJumanFormalNoun() {
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
		if node.dictionaryForm == str || node.normalizedForm == str {
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

// parseJapanese は、日本語のテキストをSudachiで形態素解析して結果を返す。
func parseJapanese(settings sudachiSettings, text string) (result sudachiResult, err error) {
	cmd := exec.Command("java", "-jar", settings.jarPath, "-r", settings.configPath, "-a")
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	cmd.Stdin = strings.NewReader(text)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail != "" {
			err = fmt.Errorf("Sudachiの実行に失敗しました：%w（%s）", err, detail)
		}
		log.Printf("info: 形態素解析器が正常に起動できませんでした：%s", err)
		return
	}

	return parseSudachiOutput(out)
}

// parseSudachiOutput は、Sudachiの -a 出力をアプリ内の解析結果へ変換する。
// 列は表層形、品詞6階層、正規化形、辞書形、読み、辞書ID、同義語グループID、OOVフラグの順。
func parseSudachiOutput(out []byte) (result sudachiResult, err error) {
	nodes := make([]sudachiMorpheme, 0)
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if line == "" || line == "EOS" {
			continue
		}

		columns := strings.Split(line, "\t")
		if len(columns) < 8 {
			return sudachiResult{nodes}, fmt.Errorf("異常なSudachi解析結果：%q", line)
		}

		parts := strings.Split(columns[1], ",")
		if len(parts) != 6 {
			return sudachiResult{nodes}, fmt.Errorf("異常なSudachi品詞情報：%q", columns[1])
		}
		var partOfSpeech [6]string
		copy(partOfSpeech[:], parts)

		dictionaryID, conversionErr := strconv.Atoi(columns[5])
		if conversionErr != nil {
			return sudachiResult{nodes}, fmt.Errorf("異常なSudachi辞書ID：%q", columns[5])
		}
		synonymGroupIDs, conversionErr := parseSudachiIDList(columns[6])
		if conversionErr != nil {
			return sudachiResult{nodes}, fmt.Errorf("異常なSudachi同義語グループID：%q", columns[6])
		}

		reading := columns[4]
		if reading == "" || reading == "*" {
			reading = columns[0]
		} else {
			reading = katakanaToHiragana(reading)
		}

		nodes = append(nodes, sudachiMorpheme{
			surface:         columns[0],
			partOfSpeech:    partOfSpeech,
			normalizedForm:  columns[2],
			dictionaryForm:  columns[3],
			reading:         reading,
			dictionaryID:    dictionaryID,
			synonymGroupIDs: synonymGroupIDs,
			isOOV:           dictionaryID < 0 || columns[7] == "(OOV)",
		})
	}

	if len(nodes) == 0 {
		return sudachiResult{}, fmt.Errorf("Sudachiの解析結果に形態素がありません（出力: %q）", truncateText(strings.TrimSpace(string(out)), 200))
	}

	return sudachiResult{nodes}, nil
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

func truncateText(text string, maxRunes int) string {
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[:maxRunes]) + "…"
}

func parseSudachiIDList(text string) ([]int, error) {
	text = strings.TrimSpace(text)
	if len(text) < 2 || text[0] != '[' || text[len(text)-1] != ']' {
		return nil, fmt.Errorf("角括弧で囲まれていません")
	}
	text = strings.TrimSpace(text[1 : len(text)-1])
	if text == "" {
		return []int{}, nil
	}

	values := strings.Split(text, ",")
	ids := make([]int, 0, len(values))
	for _, value := range values {
		id, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
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
