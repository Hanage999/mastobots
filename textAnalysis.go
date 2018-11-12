package mastobots

import (
	"bytes"
	"errors"
	"golang.org/x/net/html"
	"log"
	"os/exec"
	"strings"
)

// parseResultはテキストの形態素解析結果を格納する。
type parseResult struct {
	Nodes [][]string
}

// textContentsは、htmlからテキストを抽出する。
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

// parseは、Juman++で文字列を形態素解析して結果を返す。
func parse(text string) (result parseResult, err error) {
	// Juman++で形態素解析
	cmd := exec.Command("jumanpp", "-F")
	cmd.Stdin = strings.NewReader(text)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err = cmd.Run(); err != nil {
		log.Printf("info: 形態素解析器が正常に起動できませんでした。：%s\n", err)
		return
	}

	// 解析結果をスライスに整理（スペースとアンダースコアは除外）
	nodeStrs := strings.Fields(out.String())
	nodes := make([][]string, 0)
	for _, s := range nodeStrs {
		if strings.HasPrefix(s, "_") {
			continue
		}
		node := strings.Split(s, "_")
		if len(node) < 7 {
			log.Println("info: 異常なjumanpp解析結果", node)
			if node[0] == "#" {
				err = errors.New("jumanppでエラーが発生しました。")
				log.Printf("info: %s", err)
				return
			}
			continue
		}
		nodes = append(nodes, node)
	}
	result = parseResult{nodes}

	return
}

// containは、形態素解析結果（基本形）に特定の単語が存在するかを調べる。
func (result parseResult) contain(str string) bool {
	for _, node := range result.Nodes {
		// 3番目の要素が基本形
		if node[2] == str {
			return true
		}
	}
	return false
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
