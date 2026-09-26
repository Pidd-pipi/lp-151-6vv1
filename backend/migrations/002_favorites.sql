-- 收藏功能：表结构实际由 GORM AutoMigrate 创建，这里给出等价 DDL 供参考与手动部署。
-- 每个匿名身份对每个帖子仅保留一条收藏记录；不同身份相互独立。
CREATE TABLE IF NOT EXISTS `favorites` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `identity_id` BIGINT UNSIGNED NOT NULL,
  `post_id` BIGINT UNSIGNED NOT NULL,
  `created_at` DATETIME(3) NULL,
  PRIMARY KEY (`id`),
  UNIQUE INDEX `uniq_favorite_identity_post` (`identity_id`, `post_id`),
  INDEX `idx_favorites_created_at` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 帖子表新增收藏计数（与点赞计数同源维护，保证与收藏关系一致）。
ALTER TABLE `posts` ADD COLUMN `favorite_count` BIGINT NOT NULL DEFAULT 0 AFTER `like_count`;
