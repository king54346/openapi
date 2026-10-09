// Package database 按配置打开数据库连接，支持 SQLite、MySQL、PostgreSQL。
//
// 选择规则与 new-api 一致：
//   - SQL_DSN 为空或 "local"：SQLite，文件路径取 SQLITE_PATH
//   - SQL_DSN 以 postgres:// 或 postgresql:// 开头：PostgreSQL
//   - 其他：MySQL（如 user:pass@tcp(127.0.0.1:3306)/openapi）
//
// LOG_SQL_DSN 非空时日志单独存库（规则同上），否则与业务数据共用连接。
package database

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"openapi/common"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// Type 数据库类型
type Type string

const (
	SQLite     Type = "sqlite"
	MySQL      Type = "mysql"
	PostgreSQL Type = "postgres"
)

// sqliteBusyTimeout SQLite 未指定参数时默认附加的忙等待（毫秒），避免并发写入时立即报 database is locked
const sqliteBusyTimeout = 5000

// Config 数据库配置
type Config struct {
	DSN        string // SQL_DSN：业务库
	LogDSN     string // LOG_SQL_DSN：日志库，空则共用业务库
	SQLitePath string // SQLITE_PATH：SQLite 文件路径，可带 DSN 参数

	MaxIdleConns int           // SQL_MAX_IDLE_CONNS
	MaxOpenConns int           // SQL_MAX_OPEN_CONNS
	MaxLifetime  time.Duration // SQL_MAX_LIFETIME（秒）

	SlowThreshold time.Duration // SQL_SLOW_THRESHOLD_MS：慢查询日志阈值
	Debug         bool          // 输出全部 SQL（DEBUG=true）
}

// ConfigFromEnv 从环境变量读取数据库配置。
func ConfigFromEnv() Config {
	return Config{
		DSN:           os.Getenv("SQL_DSN"),
		LogDSN:        os.Getenv("LOG_SQL_DSN"),
		SQLitePath:    common.GetEnvOrDefaultString("SQLITE_PATH", "one-api.db"),
		MaxIdleConns:  common.GetEnvOrDefault("SQL_MAX_IDLE_CONNS", 100),
		MaxOpenConns:  common.GetEnvOrDefault("SQL_MAX_OPEN_CONNS", 1000),
		MaxLifetime:   time.Duration(common.GetEnvOrDefault("SQL_MAX_LIFETIME", 60)) * time.Second,
		SlowThreshold: time.Duration(common.GetEnvOrDefault("SQL_SLOW_THRESHOLD_MS", 500)) * time.Millisecond,
		Debug:         common.DebugEnabled,
	}
}

// Detect 根据 DSN 判断数据库类型，返回实际传给驱动的 DSN。
func Detect(dsn, sqlitePath string) (Type, string) {
	switch {
	case dsn == "" || dsn == "local":
		if !strings.Contains(sqlitePath, "?") {
			sqlitePath += fmt.Sprintf("?_pragma=busy_timeout(%d)", sqliteBusyTimeout)
		}
		return SQLite, sqlitePath
	case strings.HasPrefix(dsn, "postgres://"), strings.HasPrefix(dsn, "postgresql://"):
		return PostgreSQL, dsn
	default:
		// 时间列需要 parseTime 才能扫描到 time.Time
		if !strings.Contains(dsn, "parseTime") {
			if strings.Contains(dsn, "?") {
				dsn += "&parseTime=true"
			} else {
				dsn += "?parseTime=true"
			}
		}
		return MySQL, dsn
	}
}

// Open 打开业务库与日志库；未配置 LOG_SQL_DSN 时 logDB 为 nil（与业务库共用）。
func Open(cfg Config) (db *gorm.DB, logDB *gorm.DB, err error) {
	if db, err = open(cfg, cfg.DSN, "main"); err != nil {
		return nil, nil, err
	}
	if cfg.LogDSN == "" {
		return db, nil, nil
	}
	if logDB, err = open(cfg, cfg.LogDSN, "log"); err != nil {
		Close(db)
		return nil, nil, err
	}
	return db, logDB, nil
}

func open(cfg Config, dsn, name string) (*gorm.DB, error) {
	typ, dsn := Detect(dsn, cfg.SQLitePath)
	var dialector gorm.Dialector
	switch typ {
	case SQLite:
		common.SysLog(fmt.Sprintf("%s database: SQLite (%s)", name, dsn))
		dialector = sqlite.Open(dsn)
	case PostgreSQL:
		common.SysLog(name + " database: PostgreSQL")
		dialector = postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true})
	default:
		common.SysLog(name + " database: MySQL")
		dialector = mysql.Open(dsn)
	}

	logLevel := gormlogger.Warn
	if cfg.Debug {
		logLevel = gormlogger.Info
	}
	db, err := gorm.Open(dialector, &gorm.Config{
		PrepareStmt: true,
		Logger: gormlogger.New(log.New(os.Stdout, "\r\n", log.LstdFlags), gormlogger.Config{
			SlowThreshold: cfg.SlowThreshold,
			LogLevel:      logLevel,
			// 令牌/用户查不到是正常的鉴权失败，不当作错误输出
			IgnoreRecordNotFoundError: true,
		}),
	})
	if err != nil {
		return nil, fmt.Errorf("open %s database (%s): %w", name, typ, err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetConnMaxLifetime(cfg.MaxLifetime)
	if err := sqlDB.Ping(); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("ping %s database (%s): %w", name, typ, err)
	}
	return db, nil
}

// Close 关闭连接，nil 时忽略。
func Close(db *gorm.DB) {
	if db == nil {
		return
	}
	sqlDB, err := db.DB()
	if err != nil {
		common.SysError("failed to get sql.DB: " + err.Error())
		return
	}
	if err := sqlDB.Close(); err != nil {
		common.SysError("failed to close database: " + err.Error())
	}
}
