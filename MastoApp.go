package mastobots

import (
	"context"
	"errors"
	"log"

	mastodon "github.com/mattn/go-mastodon"
)

// MastoApp は、Mastodonクライアントの情報を格納する。
type MastoApp struct {
	Server       string
	ClientID     string
	ClientSecret string
}

// initMastoApp は、新たに登録すべきマストドンクライアントアプリケーション登録し、
// 新旧のアプリを全て含んだスライスを返す。
func initMastoApps(apps []*MastoApp, appName, instance string) (updatedApps []*MastoApp, err error) {
	for _, a := range apps {
		if a.Server == instance && a.ClientID != "" && a.ClientSecret != "" {
			return
		}
	}

	app, err := newMastoApp(appName, instance)
	if err != nil {
		log.Printf("alert: %s へのアプリケーション登録に失敗しました", instance)
		return
	}
	updatedApps = append(apps, &app)

	return
}

// newMastoApp は、インスタンスに新たにMastoAppを登録し、それを返す。
func newMastoApp(name, instance string) (app MastoApp, err error) {
	newApp, err := mastodon.RegisterApp(context.Background(), &mastodon.AppConfig{
		Server:     instance,
		ClientName: name,
		Scopes:     "read write follow",
		Website:    "https://github.com/hanage999/mastobots",
	})
	if err != nil {
		log.Printf("alert: マストドンアプリケーションが新規登録できませんでした：%s", err)
		return
	}

	app.Server = instance
	app.ClientID = newApp.ClientID
	app.ClientSecret = newApp.ClientSecret

	return
}

// getApp は、インスタンスのためのMastoAppを取得する。
func getApp(instance string, apps []*MastoApp) (app *MastoApp, err error) {
	for _, a := range apps {
		if a.Server == instance && a.ClientID != "" && a.ClientSecret != "" {
			app = a
			return
		}
	}

	err = errors.New(instance + "のためのアプリが取得できませんでした")
	log.Printf("alert: %s", err)
	return
}
