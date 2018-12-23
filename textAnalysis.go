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

// parseは、Juman++で文字列を形態素解析して結果を返す。
func parse(text string) (result parseResult, err error) {
	if text == "" {
		err = errors.New("解析する文字列が空です。")
		log.Printf("info: %s", err)
		return
	}

	// 改行のない長文はJumanppに食わせるとエラーになるので、句点で強制改行
	safeStr := strings.Replace(text, "。\n", "。", -1)
	safeStr = strings.Replace(safeStr, "。", "。\n", -1)

	// Juman++で形態素解析
	cmd := exec.Command("jumanpp")
	cmd.Stdin = strings.NewReader(safeStr)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err = cmd.Run(); err != nil {
		log.Printf("info: 形態素解析器が正常に起動できませんでした。：%s\n", err)
		return
	}

	// 解析結果をスライスに整理（半角スペース等は除外）
	nodeStrs := strings.Split(out.String(), "\n")
	nodes := make([][]string, 0)
	strange := false
	for _, s := range nodeStrs {
		if strings.HasPrefix(s, " ") || strings.HasPrefix(s, "@") || strings.HasPrefix(s, "EOS") || s == "" {
			continue
		}
		node := strings.SplitN(s, " ", 12)
		if len(node) < 12 {
			strange = true
			log.Println("info: 異常なjumanpp解析結果：", node)
			if node[0] == "#" {
				err = errors.New("jumanppでエラーが発生しました。")
				log.Printf("info: %s", err)
				break
			}
			continue
		}
		nodes = append(nodes, node)
	}
	result = parseResult{nodes}

	if strange {
		log.Printf("trace: 解析異常が出たテキスト：%s", safeStr)
	}

	return
}

// containは、形態素解析結果（基本形）に特定の単語が存在するかを調べる。
func (result parseResult) contain(str string) bool {
	for _, node := range result.Nodes {
		// 3番目の要素が基本形
		if node[2] == str {
			log.Printf("trace: 一致した単語：%s", str)
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
