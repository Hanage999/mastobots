package mastobots

import (
	"bufio"
	"encoding/json"
	"errors"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"log"
	"net/http"
	"os"

	"github.com/goml/gobrain"
	"github.com/hanage999/go-mastodon"
	"github.com/muesli/smartcrop"
	"github.com/muesli/smartcrop/nfnt"
	"github.com/nfnt/resize"
)

var (
	model  *gobrain.FeedForward
	labels []string
)

func dec(d []float64) int {
	n := 0
	for i, v := range d {
		log.Printf("info: おしり度：%f", v)
		if v > 0.9 {
			n += 1 << uint(i)
		}
	}
	return n
}

func cropImage(img image.Image) image.Image {
	analyzer := smartcrop.NewAnalyzer(nfnt.NewDefaultResizer())
	topCrop, err := analyzer.FindBestCrop(img, 75, 75)
	if err == nil {
		type SubImager interface {
			SubImage(r image.Rectangle) image.Image
		}
		img = img.(SubImager).SubImage(topCrop)
	}
	return resize.Resize(75, 75, img, resize.Lanczos3)
}

func decodeImage(url string) ([]float64, error) {
	res, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	src, _, err := image.Decode(res.Body)
	if err != nil {
		return nil, err
	}

	src = cropImage(src)
	bounds := src.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w < h {
		w = h
	} else {
		h = w
	}
	bb := make([]float64, w*h*3)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, b, _ := src.At(x, y).RGBA()
			bb[y*w*3+x*3] = float64(r) / 255.0
			bb[y*w*3+x*3+1] = float64(g) / 255.0
			bb[y*w*3+x*3+2] = float64(b) / 255.0
		}
	}
	return bb, nil
}

func loadModel() (*gobrain.FeedForward, []string, error) {
	f, err := os.Open("labels.txt")
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()

	labels := []string{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		labels = append(labels, scanner.Text())
	}
	if scanner.Err() != nil {
		return nil, nil, err
	}

	if len(labels) == 0 {
		return nil, nil, errors.New("画像認識ラベルが見つかりませんでした。")
	}

	f, err = os.Open("model.json")
	if err != nil {
		return nil, labels, nil
	}
	defer f.Close()

	ff := &gobrain.FeedForward{}
	err = json.NewDecoder(f).Decode(ff)
	if err != nil {
		log.Printf("info: 画像認識モデルが読み込めませんでした。：%s", err)
		return nil, labels, err
	}
	return ff, labels, nil
}

func detectImage(status *mastodon.Status) (itis bool) {
	if len(status.MediaAttachments) == 0 {
		return
	}

	if status.MediaAttachments[0].Type != "image" {
		return
	}

	for _, atc := range status.MediaAttachments {
		input, err := decodeImage(atc.PreviewURL)
		if err != nil {
			log.Printf("info: 画像 %s が開けませんでした。：%s", atc.PreviewURL, err)
			break
		}
		result := model.Update(input)
		log.Printf("info: %s 判定：%f", atc.PreviewURL, result[0])
		if result[0] > 0.9 {
			itis = true
			break
		}
	}

	return
}
