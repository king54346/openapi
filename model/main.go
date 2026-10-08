package model

import (
	"errors"
	"fmt"

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

// InitDB 注入数据库连接并自动迁移表结构。logDB 为 nil 时日志与业务数据共用 db。
// 数据库驱动由调用方选择（gorm.io/driver/mysql、postgres、sqlite 等），这里不做绑定。
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

	if err := DB.AutoMigrate(&Channel{}, &Task{}); err != nil {
		return err
	}
	// users / tokens 表由管理端维护且列更多，已存在时不迁移，避免改动其结构；仅新库时建最小表
	for _, table := range []any{&User{}, &Token{}} {
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
