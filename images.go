package mastobots

import (
	"bytes"
	"io"
	"io/ioutil"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/hanage999/go-mastodon"
)

func label(status *mastodon.Status, label string) (itis bool, foundLabel string) {
	if len(status.MediaAttachments) == 0 {
		return
	}

	if status.MediaAttachments[0].Type != "image" {
		return
	}

	for _, atc := range status.MediaAttachments {
		detectedLabel, err := checkImage(atc.PreviewURL)
		if err != nil {
			log.Printf("info: 画像 %s が開けませんでした。：%s", atc.PreviewURL, err)
			continue
		}
		if strings.Contains(detectedLabel, label) {
			itis = true
			foundLabel = detectedLabel
			return
		}
	}

	return
}

func checkImage(imgurl string) (label string, err error) {
	// 添付画像にアクセス
	res, err := http.Get(imgurl)
	if err != nil {
		log.Printf("info: 画像のURL %s が開けませんでした。：%s", imgurl, err)
		return
	}
	defer res.Body.Close()

	// ファイル名を取得
	myurl, err := url.Parse(imgurl)
	if err != nil {
		log.Printf("info: URL %s がパースできませんでした。：%s", imgurl, err)
		return
	}
	fileName := path.Base(myurl.Path)

	// 画像データをフォームに読み込むためのバッファ作成
	bodyBuf := &bytes.Buffer{}
	bodyWriter := multipart.NewWriter(bodyBuf)
	fileWriter, err := bodyWriter.CreateFormFile("uploadfile", fileName)
	if err != nil {
		log.Printf("info: フォームファイルを作成できませんでした。：%s", err)
		return
	}

	// バッファに画像データを読み込む
	_, err = io.Copy(fileWriter, res.Body)
	if err != nil {
		log.Printf("info: フォームファイルに画像データを読み込めませんでした。：%s", err)
		return
	}

	contentType := bodyWriter.FormDataContentType()
	bodyWriter.Close()

	// フォームをポスト
	resp, err := http.Post(imageDetector, contentType, bodyBuf)
	if err != nil {
		log.Printf("info: 画像データをPOSTできませんでした。：%s", err)
		return
	}
	defer resp.Body.Close()

	// レスポンスデータの確保
	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Printf("info: レスポンスが返ってきませんでした。：%s", err)
		return
	}

	log.Println("info: " + string(respBody))
	return string(respBody), nil
}
