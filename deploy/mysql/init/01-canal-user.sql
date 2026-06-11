-- canal 用户：给 webook-migrator 的 GoMySQLCanalClient 拉 binlog 用
--
-- 触发时机：webook-mysql 容器首次启动时（mysql-data volume 为空），
-- entrypoint 自动执行 /docker-entrypoint-initdb.d/*.sql。volume 已 init 后此脚本不再跑。
--
-- 权限说明：
--   REPLICATION SLAVE         拉 binlog stream
--   REPLICATION CLIENT        SHOW MASTER STATUS / SHOW BINARY LOGS
--   SELECT *.*                canal 读 information_schema 拿表结构
-- 不给 INSERT/UPDATE/DELETE，避免 canal 误写

-- 多 host 全建:容器内 socket 走 localhost,host 机 TCP 走 127.0.0.1 / %,各场景都能登
-- 实测踩坑:只建 'canal'@'%' 时 host 机 go-sql-driver TCP 连会被解析为 localhost(MySQL 优先级匹配),
-- 因为没有 'canal'@'localhost' 记录而 Access denied
CREATE USER IF NOT EXISTS 'canal'@'%' IDENTIFIED WITH mysql_native_password BY 'canal';
CREATE USER IF NOT EXISTS 'canal'@'localhost' IDENTIFIED WITH mysql_native_password BY 'canal';
CREATE USER IF NOT EXISTS 'canal'@'127.0.0.1' IDENTIFIED WITH mysql_native_password BY 'canal';
GRANT REPLICATION SLAVE, REPLICATION CLIENT, SELECT ON *.* TO 'canal'@'%';
GRANT REPLICATION SLAVE, REPLICATION CLIENT, SELECT ON *.* TO 'canal'@'localhost';
GRANT REPLICATION SLAVE, REPLICATION CLIENT, SELECT ON *.* TO 'canal'@'127.0.0.1';
FLUSH PRIVILEGES;

-- 验证：
--   SHOW GRANTS FOR 'canal'@'%';
--   SHOW MASTER STATUS;        -- 应有 File / Position（binlog 已开）
--   SHOW VARIABLES LIKE 'binlog_format';     -- ROW
--   SHOW VARIABLES LIKE 'binlog_row_image';  -- FULL
