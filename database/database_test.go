package database

import (
	"path/filepath"
	"testing"
)

func TestDetect(t *testing.T) {
	cases := []struct {
		dsn, sqlitePath string
		wantType        Type
		wantDSN         string
	}{
		{"", "one-api.db", SQLite, "one-api.db?_pragma=busy_timeout(5000)"},
		{"local", "data/x.db?_pragma=journal_mode(WAL)", SQLite, "data/x.db?_pragma=journal_mode(WAL)"},
		{"postgres://u:p@localhost:5432/openapi", "", PostgreSQL, "postgres://u:p@localhost:5432/openapi"},
		{"postgresql://u:p@localhost/openapi?sslmode=disable", "", PostgreSQL, "postgresql://u:p@localhost/openapi?sslmode=disable"},
		{"u:p@tcp(127.0.0.1:3306)/openapi", "", MySQL, "u:p@tcp(127.0.0.1:3306)/openapi?parseTime=true"},
		{"u:p@tcp(127.0.0.1:3306)/openapi?charset=utf8mb4", "", MySQL, "u:p@tcp(127.0.0.1:3306)/openapi?charset=utf8mb4&parseTime=true"},
		{"u:p@tcp(db)/openapi?parseTime=false", "", MySQL, "u:p@tcp(db)/openapi?parseTime=false"},
	}
	for _, tc := range cases {
		typ, dsn := Detect(tc.dsn, tc.sqlitePath)
		if typ != tc.wantType || dsn != tc.wantDSN {
			t.Errorf("Detect(%q, %q) = %s %q, want %s %q", tc.dsn, tc.sqlitePath, typ, dsn, tc.wantType, tc.wantDSN)
		}
	}
}

func TestOpenSQLiteWithSeparateLogDB(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		SQLitePath:   filepath.Join(dir, "main.db"),
		LogDSN:       "local", // 日志库也走 SQLite：同一路径，验证能独立打开
		MaxIdleConns: 2, MaxOpenConns: 4,
	}
	db, logDB, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer Close(db)
	defer Close(logDB)
	if db.Dialector.Name() != "sqlite" || logDB == nil {
		t.Fatalf("dialector=%s logDB=%v", db.Dialector.Name(), logDB)
	}

	cfg.LogDSN = ""
	db2, logDB2, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer Close(db2)
	if logDB2 != nil {
		t.Fatal("logDB should be nil when LOG_SQL_DSN is empty")
	}
}
