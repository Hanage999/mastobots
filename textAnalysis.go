package mastobots

import (
	"bytes"
	"errors"
	"log"
	"math/rand"
	"os/exec"
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

// jumanResult は、テキストをjumanppで形態素解析した結果を格納する
type jumanResult struct {
	Nodes [][]string
}

// proseResult は、テキストをproseで形態素解析した結果を格納する
type proseResult struct {
	Nodes    []prose.Token
	Entities []prose.Entity
}

func (result jumanResult) length() int {
	return len(result.Nodes)
}

func (result proseResult) length() int {
	return len(result.Nodes)
}

func (result jumanResult) candidates() (cds []candidate) {
	cds = make([]candidate, 0)
	for _, node := range result.Nodes {
		if node[3] != "名詞" || node[5] == "数詞" || node[5] == "形式名詞" {
			continue
		}
		cd := candidate{node[0], string(getRuneAt(node[1], 0)), rand.Intn(2000)}
		if node[5] == "組織名" || node[5] == "人名" || node[5] == "地名" {
			cd.priority = 700 + rand.Intn(2000)
		}
		cds = append(cds, cd)
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

func (result jumanResult) contain(str string) bool {
	for _, node := range result.Nodes {
		// 3番目の要素が基本形
		if node[2] == str {
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
func parse(jpl chan int, text string) (result parseResult, err error) {
	if text == "" {
		err = errors.New("解析する文字列が空です")
		log.Printf("info: %s", err)
		return
	}

	if isJap(text) {
		result, err = parseJapanese(text)
	} else {
		jpl <- 0
		result, err = parseEnglish(text)
		<-jpl
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

// parseJapanese は、日本語のテキストをJuman++で形態素解析して結果を返す。
func parseJapanese(text string) (result jumanResult, err error) {
	// 改行のない長文はJumanppに食わせるとエラーになるので、句点で強制改行
	safeStr := strings.Replace(text, "。\n", "。", -1)
	safeStr = strings.Replace(safeStr, "。", "。\n", -1)

	// Juman++で形態素解析
	cmd := exec.Command("jumanpp")
	cmd.Stdin = strings.NewReader(safeStr)
	out, err := cmd.Output()
	if err != nil {
		log.Printf("info: 形態素解析器が正常に起動できませんでした：%s", err)
		return
	}

	// 解析結果をスライスに整理（半角スペース等は除外）
	nodeStrs := strings.Split(string(out), "\n")
	nodes := make([][]string, 0)
	strange := false
	for _, s := range nodeStrs {
		if strings.HasPrefix(s, "#") || strings.HasPrefix(s, " ") || strings.HasPrefix(s, "@") || strings.HasPrefix(s, "EOS") || s == "" {
			continue
		}
		node := strings.SplitN(s, " ", 12)
		if len(node) < 12 {
			strange = true
			log.Println("info: 異常なjumanpp解析結果：", node)
			continue
		}
		nodes = append(nodes, node)
	}
	result = jumanResult{nodes}

	if strange {
		log.Printf("info: 解析異常が出たテキスト：%s", safeStr)
	}

	return
}

// getRuneAtは、文字列の中のn番目の文字を返す。
// https://pinzolo.github.io/2016/05/31/golang-get-rune-from-string.html
func getRuneAt(s string, i int) rune {
	rs := []rune(s)
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
