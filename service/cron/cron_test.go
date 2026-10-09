package cron

import (
	"sync/atomic"
	"testing"
	"time"

	"openapi/model"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func at(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04", s, time.Local)
	if err != nil {
		panic(err)
	}
	return t
}

func TestParseAndMatch(t *testing.T) {
	// 2026-10-09 是周五
	cases := []struct {
		expr string
		t    string
		want bool
	}{
		{"* * * * *", "2026-10-09 13:27", true},
		{"*/5 * * * *", "2026-10-09 13:25", true},
		{"*/5 * * * *", "2026-10-09 13:27", false},
		{"0 9 * * *", "2026-10-09 09:00", true},
		{"0 9 * * *", "2026-10-09 09:01", false},
		{"0 9-18/3 * * *", "2026-10-09 15:00", true},
		{"0 9-18/3 * * *", "2026-10-09 16:00", false},
		{"30 2 1,15 * *", "2026-10-15 02:30", true},
		{"0 0 * * 5", "2026-10-09 00:00", true},    // 周五
		{"0 0 * * 0", "2026-10-11 00:00", true},    // 周日写 0
		{"0 0 * * 7", "2026-10-11 00:00", true},    // 周日写 7
		{"0 0 * * 1-5", "2026-10-11 00:00", false}, // 周日不在工作日
		{"0 0 1 * 5", "2026-10-09 00:00", true},    // 日与周都指定时满足其一即可（周五）
		{"0 0 1 * 5", "2026-10-01 00:00", true},    // （1 号）
		{"0 0 1 * 5", "2026-10-08 00:00", false},
		{"5/20 * * * *", "2026-10-09 13:45", true},
		{"0 0 * 2 *", "2026-10-09 00:00", false},
	}
	for _, tc := range cases {
		s, err := Parse(tc.expr)
		if err != nil {
			t.Fatalf("%q: %v", tc.expr, err)
		}
		if got := s.Match(at(tc.t)); got != tc.want {
			t.Errorf("%q at %s: got %v, want %v", tc.expr, tc.t, got, tc.want)
		}
	}
}

func TestParseInvalid(t *testing.T) {
	for _, expr := range []string{"", "* * * *", "* * * * * *", "60 * * * *", "* 24 * * *", "* * 0 * *", "* * * 13 *",
		"* * * * 8", "*/0 * * * *", "5-1 * * * *", "a * * * *", "1,,2 * * * *"} {
		if ValidateCron(expr) {
			t.Errorf("%q should be invalid", expr)
		}
	}
}

func setupDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := model.InitDB(db, nil); err != nil {
		t.Fatal(err)
	}
}

// waitRuns 等待任务累计运行 n 次（最多 2 秒）。
func waitRuns(t *testing.T, id int64, n int64) *model.CronJob {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		job, err := model.GetCronJobById(id)
		if err != nil {
			t.Fatal(err)
		}
		if job.RunCount >= n || time.Now().After(deadline) {
			return job
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestRunTick(t *testing.T) {
	setupDB(t)
	var calls atomic.Int32
	Register("test_ok", func(job *model.CronJob) (string, bool) {
		calls.Add(1)
		return "done: " + job.Payload, true
	})
	Register("test_panic", func(*model.CronJob) (string, bool) { panic("boom") })

	jobs := []*model.CronJob{
		{Name: "every minute", Type: "test_ok", Cron: "* * * * *", Payload: `{"a":1}`, Enabled: true},
		{Name: "never now", Type: "test_ok", Cron: "0 0 1 1 *", Enabled: true},
		{Name: "disabled", Type: "test_ok", Cron: "* * * * *", Enabled: false},
		{Name: "unknown type", Type: "account_test", Cron: "* * * * *", Enabled: true},
		{Name: "panics", Type: "test_panic", Cron: "* * * * *", Enabled: true},
	}
	for _, j := range jobs {
		if err := j.Insert(); err != nil {
			t.Fatal(err)
		}
	}

	RunTick(at("2026-10-09 13:27"))

	ok := waitRuns(t, jobs[0].Id, 1)
	if ok.RunCount != 1 || ok.LastStatus != model.CronJobStatusSuccess || ok.LastMessage != `done: {"a":1}` {
		t.Fatalf("matched job: %+v", ok)
	}
	panicked := waitRuns(t, jobs[4].Id, 1)
	if panicked.LastStatus != model.CronJobStatusFailed || panicked.FailCount != 1 {
		t.Fatalf("panicking job should be recorded as failed: %+v", panicked)
	}
	time.Sleep(50 * time.Millisecond)
	for _, j := range jobs[1:4] {
		got, _ := model.GetCronJobById(j.Id)
		if got.RunCount != 0 {
			t.Errorf("job %q should not run, run_count=%d", j.Name, got.RunCount)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("executor calls = %d", calls.Load())
	}
	if types := RegisteredTypes(); len(types) < 2 || types[0] > types[len(types)-1] {
		t.Fatalf("registered types should be sorted: %v", types)
	}
}

func TestExecuteSkipsWhileRunning(t *testing.T) {
	setupDB(t)
	release := make(chan struct{})
	Register("test_slow", func(*model.CronJob) (string, bool) {
		<-release
		return "slow done", true
	})
	job := &model.CronJob{Name: "slow", Type: "test_slow", Cron: "* * * * *", Enabled: true}
	if err := job.Insert(); err != nil {
		t.Fatal(err)
	}
	TriggerJobAsync(job)
	deadline := time.Now().Add(time.Second)
	for !IsJobRunning(job.Id) && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if msg, ok := RunJobNow(job); ok || msg == "" {
		t.Fatalf("second run should be skipped, got %q %v", msg, ok)
	}
	close(release)
	if got := waitRuns(t, job.Id, 1); got.LastMessage != "slow done" {
		t.Fatalf("first run result: %+v", got)
	}
}
