package model

import (
	"context"
	"errors"
	"fmt"
	"time"

	"openapi/common"

	"gorm.io/gorm"
)

// DB 存放渠道、任务等业务数据；LOG_DB 存放请求日志，可以与 DB 是同一个连接
var (
	DB     *gorm.DB
	LOG_DB *gorm.DB
)

var (
	usingMySQL      bool
	usingPostgreSQL bool

	// 不同数据库对保留字列名的转义方式不同
	commonGroupCol string
	commonKeyCol   string
	logGroupCol    string
)

// InitDB 注入数据库连接并自动迁移表结构（仅 master 节点迁移）。logDB 为 nil 时日志与业务数据共用 db。
// 连接由 database 包按 SQL_DSN 打开，这里不绑定具体驱动。
func InitDB(db *gorm.DB, logDB *gorm.DB) error {
	if db == nil {
		return errors.New("db is nil")
	}
	if logDB == nil {
		logDB = db
	}
	DB = db
	LOG_DB = logDB

	usingMySQL, usingPostgreSQL = dialectOf(db)
	commonGroupCol = quoteCol("group", usingPostgreSQL)
	commonKeyCol = quoteCol("key", usingPostgreSQL)
	_, logUsingPostgreSQL := dialectOf(logDB)
	logGroupCol = quoteCol("group", logUsingPostgreSQL)

	// 多实例同时启动且配置了 Redis 时，通过分布式锁串行化表结构迁移，避免并发 DDL 锁表冲突
	releaseMigrateLock, acquired := common.AcquireLockWait(context.Background(), "db:migrate", 60*time.Second, 60*time.Second)
	if acquired {
		defer releaseMigrateLock()
	}
	if err := DB.AutoMigrate(&Channel{}, &Task{}, &CronJob{}, &CronJobRun{}); err != nil {
		return err
	}
	// 分组功能已移除：清掉历史 group 列与分组配置项（幂等，失败只记日志）
	DropGroupColumns()
	// users / tokens / models 等表由管理端维护且列更多，已存在时不迁移，避免改动其结构；仅新库时建表
	for _, table := range []any{
		&User{}, &Token{}, &Model{},
		&Option{}, &Setup{},
		&Vendor{}, &PrefillGroup{}, &TokenAccessTemplate{}, &ChannelTestRecord{},
	} {
		if !DB.Migrator().HasTable(table) {
			if err := DB.AutoMigrate(table); err != nil {
				return err
			}
		}
	}
	return LOG_DB.AutoMigrate(&Log{})
}

func dialectOf(db *gorm.DB) (mysql bool, postgres bool) {
	switch db.Dialector.Name() {
	case "mysql":
		return true, false
	case "postgres":
		return false, true
	}
	return false, false
}

func quoteCol(name string, postgres bool) string {
	if postgres {
		return `"` + name + `"`
	}
	return "`" + name + "`"
}

// scanJSONBytes 取出 JSON 列的原始内容：SQLite 中可能是 BLOB（[]byte）或 TEXT（string），NULL 返回 nil。
func scanJSONBytes(value any) ([]byte, error) {
	switch v := value.(type) {
	case nil:
		return nil, nil
	case []byte:
		return v, nil
	case string:
		return []byte(v), nil
	default:
		return nil, fmt.Errorf("unsupported JSON column type %T", value)
	}
}
