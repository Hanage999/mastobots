# mastobots

[![en](https://img.shields.io/badge/lang-en-red.svg)](https://github.com/Hanage999/mastobots/blob/master/README.md)
[![ja](https://img.shields.io/badge/lang-ja-green.svg)](https://github.com/Hanage999/mastobots/blob/master/README.ja.md)

A customizable Mastodon bot that periodically retrieves RSS feed items containing specified keywords, analyzes them in Japanese (using Juman++) or English (using Prose), and then posts automatically-generated Japanese comments. It also responds to mentions and provides weather information.

RSS feed items are stored in a MySQL database and analyzed for relevant keywords. Japanese items are parsed with Juman++, and English items are processed using Prose, but all posts are generated in Japanese.

Populate RSS items into the database beforehand using tools such as [feedAggregator](https://blog.crazynewworld.net/2018/10/29/323/).

Configure multiple bots simultaneously via the `config.yml` file.

## Dependencies

Install the following before running:

- MySQL
- [Juman++ 2.0.0-rc3](http://nlp.ist.i.kyoto-u.ac.jp/index.php?JUMAN++)

## Features

- Supports Japanese and English RSS feed items (all posts in Japanese).
- Highly customizable posting intervals, comments, and keywords.
- Responds positively or negatively to mentions containing the keyword "いい" plus a bot-specific suffix.
- Automatically follows users who mention it with the word "フォロー" (follow).
- Provides weather forecasts for requested location and time (current, today, tomorrow, day after tomorrow) using [OpenWeatherMap](https://openweathermap.org). Mention "体感" (feels-like) to get perceived temperature.
- Configurable sleeping/waking hours. The bot is inactive during sleep hours. Set identical times to stay active continuously.
- With `LivesWithSun` set to `true`, sleep cycles synchronize to local sunrise/sunset based on latitude/longitude ([Yahoo! YOLP API](https://developer.yahoo.co.jp/webapi/map/) required).
- Randomly timed posts enabled via `RandomFrequency` and defined in `RandomToots`.
- Run the bot for a limited time using the `-p <minutes>` option.

## Usage

1. Import the schema (`database_tables.sql`) into your MySQL database and periodically populate RSS items (e.g., using feedAggregator).
2. In `cmd/mastobots`, run `go build` to compile the `mastobots` binary.
3. Copy `config.yml.example` to `config.yml` and edit accordingly.
4. Launch the bot with `./mastobots`. Using systemd or screen for background execution is recommended.

## Credits

- Web services: Yahoo! JAPAN ([Yahoo! YOLP API](https://developer.yahoo.co.jp/sitemap/))
- Weather data: [OpenWeatherMap](https://openweathermap.org)