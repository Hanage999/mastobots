# mastobots

お好みの単語が含まれるRSSアイテムを探し、コメントをつけて定期的にトゥートするbotです。メンションに反応するなどの機能もあります。

MySQLデータベースに保存された日本語のRSSアイテムをJuman++で形態素解析し、好みの単語をその結果と照合します。

データベースには別途、[feedAggregator](https://blog.crazynewworld.net/2018/10/29/323/)などを使ってRSSアイテムを読み込んでおく必要があります。

設定ファイル（config.yml）にbotごとの情報を記入すれば、何匹でもbotを駆動することができます。

## 動作環境
+ Ubuntu 18.04.02
+ Go 1.14.1
+ MySQL 5.7.25
+ Juman++ 2.0.0-rc2

## 依存ソフトウェア
以下があらかじめインストールされていないと起動しません。
+ MySQL
+ [Juman++ 2.0.0-rc2](http://nlp.ist.i.kyoto-u.ac.jp/index.php?JUMAN++)

## 機能
+ トゥートの間隔、コメント、好みの単語などをカスタマイズ可能。
+ 「いい（＋bot固有の語尾）」とメンションすると、背中を押したり押さなかったりしてくれる。
+ botに「フォロー」を含んだメンションをすると、botがフォローしてくる。
+ botに天気を尋ねると、[livedoor天気予報](http://weather.livedoor.com/weather_hacks/webservice)の情報を教えてくれる。特定の場所を指定することもできるが、指定にはほとんど従わない。
+ 寝る。寝ている間はトゥートも反応もしない。就寝時刻と起床時刻は自由に設定可。二つを同時刻に設定すれば、寝ない。
+ 設定ファイルでLivesWithSunをtrueに設定すると、LatitudeとLongitudeで指定した地点での太陽の出入り時刻に応じて寝起きする。逆ジオコーディングデータは[OpenCage Geocoder](https://opencagedata.com/api)から、時刻は[Sunrise Sunset](https://sunrise-sunset.org/api)からそれぞれ取得。
+ 設定ファイルでRandomFrequencyをゼロ以上にし、かつRandomTootsに１つ以上の文字列を指定すると、不定期に指定文字列のいずれかを呟く。（この機能を使わない場合は、RandomFrequencyはゼロに設定し、かつRandomTootsには空の配列要素を１つ設定してください）
+ -p <整数> オプション付きで起動すると、<整数>分限定で起動する。

## 使い方
0. 下準備：database_tables.sql の記載に従って、MySQLデータベースにテーブルを作成する。定期的に[feedAggregator](https://blog.crazynewworld.net/2018/10/29/323/)などを使ってRSSアイテムを収集しておく。
1. cmd/mastobots フォルダで go get、go build すると、フォルダに mastobots コマンドができる。
2. config.yml.example を config.yml にリネームまたはコピーし、自分の環境に応じて書き換えるあるいは追記する。
3. ./mastobots で起動。screenなどと併用するか、systemdでサービス化してください。
