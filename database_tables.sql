CREATE TABLE `bots` (
  `id` int(11) unsigned NOT NULL AUTO_INCREMENT,
  `name` varchar(191) NOT NULL DEFAULT '',
  `checked_until` int(11) unsigned NOT NULL DEFAULT '0',
  `created_at` datetime DEFAULT NULL,
  `updated_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `bot_name` (`name`)
) ENGINE=InnoDB AUTO_INCREMENT=5 DEFAULT CHARSET=utf8;

CREATE TABLE `candidates` (
  `id` int(11) unsigned NOT NULL AUTO_INCREMENT,
  `bot_id` int(11) unsigned NOT NULL,
  `item_id` int(11) unsigned NOT NULL,
  `created_at` datetime DEFAULT NULL,
  `updated_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `item_per_bot` (`bot_id`,`item_id`)
) ENGINE=InnoDB AUTO_INCREMENT=716 DEFAULT CHARSET=utf8;

CREATE TABLE `items` (
  `id` int(11) unsigned NOT NULL AUTO_INCREMENT,
  `title` varchar(250) NOT NULL DEFAULT '',
  `url` varchar(191) NOT NULL,
  `created_at` datetime DEFAULT NULL,
  `updated_at` datetime DEFAULT NULL,
  `content` text NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `url` (`url`)
) ENGINE=InnoDB AUTO_INCREMENT=264293 DEFAULT CHARSET=utf8;

CREATE TABLE `rss_feeds` (
  `id` int(11) unsigned NOT NULL AUTO_INCREMENT,
  `url` varchar(191) NOT NULL,
  `last_updated` datetime NOT NULL,
  `created_at` datetime DEFAULT NULL,
  `updated_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `url` (`url`)
) ENGINE=InnoDB AUTO_INCREMENT=154 DEFAULT CHARSET=utf8;
