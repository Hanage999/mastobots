# mastobots

[![en](https://img.shields.io/badge/lang-en-red.svg)](https://github.com/Hanage999/mastobots/blob/master/README.md)
[![ja](https://img.shields.io/badge/lang-ja-green.svg)](https://github.com/Hanage999/mastobots/blob/master/README.ja.md)

指定したキーワードを含むRSSフィードのアイテムを取得し、日本語または英語で解析した後、日本語のコメントをつけてMastodonに定期的にポストするボットです。また、メンションへの反応や天気情報の提供も行います。

RSSアイテムはMySQLデータベースに保存され、日本語アイテムはJuman++、英語アイテムはProseを用いて形態素解析されます。ボットが関心を持つキーワードを解析結果と照合し、自動的にコメント付きでポストします。

RSSアイテムをデータベースに取り込むには、[feedAggregator](https://blog.crazynewworld.net/2018/10/29/323/) など別のツールを使用してください。

設定ファイル (`config.yml`) を編集することで、複数のボットを並行運用できます。

## 必要なソフトウェア

事前に以下をインストールしてください。

- MySQL
- [Juman++ 2.0.0-rc3](http://nlp.ist.i.kyoto-u.ac.jp/index.php?JUMAN++)

## 主な機能

- 日本語・英語のRSSフィードに対応（解析後のポストは日本語）。
- ポスト間隔、コメント、キーワード等を細かく設定可能。
- 「いい（＋ボット固有の語尾）」とメンションすると肯定または否定の反応を返します。
- 「フォロー」を含むメンションでユーザーを自動的にフォロー。
- 場所と時間（今、今日、明日、明後日）を含めて天気を尋ねると、[OpenWeatherMap](https://openweathermap.org) から取得した天気情報を返答。「体感」を含めると体感温度で回答。
- 就寝・起床時間を設定可能。活動しない時間帯を設定できます。同一時刻に設定すると24時間稼働します。
- 設定で `LivesWithSun` を `true` にすると、緯度経度に基づく日の出・日の入り時刻に連動して寝起きします（[Yahoo! YOLP API](https://developer.yahoo.co.jp/webapi/map/) を使用）。
- 設定で `RandomFrequency` を設定すると、ランダムにポスト可能（`RandomToots` にメッセージを記述）。
- `-p <整数>` オプション付きで起動すると、指定分数のみ稼働。

## セットアップ方法

1. `database_tables.sql` をMySQLデータベースにインポートし、feedAggregator等でRSSアイテムを定期取得。
2. `cmd/mastobots` 内で `go build` し、`mastobots` 実行ファイルを作成。
3. `config.yml.example` を `config.yml` にコピー・編集。
4. `./mastobots` でボットを起動。systemdやscreenでバックグラウンド稼働を推奨。

## クレジット

- Webサービス提供：Yahoo! JAPAN ([Yahoo! YOLP API](https://developer.yahoo.co.jp/sitemap/))
- 天気情報提供：[OpenWeatherMap](https://openweathermap.org)