package model

import (
	"errors"
	"testing"
)

func TestRecordCronJobResult(t *testing.T) {
	setupTestDB(t)
	job := &CronJob{Name: "j", Type: "channel_test", Cron: "* * * * *", Enabled: true}
	if err := job.Insert(); err != nil {
		t.Fatal(err)
	}
	if err := RecordCronJobResult(job.Id, true, "ok", 12); err != nil {
		t.Fatal(err)
	}
	if err := RecordCronJobResult(job.Id, false, "bad", 34); err != nil {
		t.Fatal(err)
	}
	got, err := GetCronJobById(job.Id)
	if err != nil {
		t.Fatal(err)
	}
	if got.RunCount != 2 || got.FailCount != 1 || got.LastStatus != CronJobStatusFailed ||
		got.LastMessage != "bad" || got.LastLatency != 34 || got.LastRunAt == 0 {
		t.Fatalf("job stats: %+v", got)
	}
	// 修改定义不影响运行统计
	got.Name, got.Cron = "renamed", "0 * * * *"
	if err := got.Update(); err != nil {
		t.Fatal(err)
	}
	if again, _ := GetCronJobById(job.Id); again.Name != "renamed" || again.RunCount != 2 {
		t.Fatalf("after update: %+v", again)
	}
}

func TestCronJobRunHistoryTrimmed(t *testing.T) {
	setupTestDB(t)
	job := &CronJob{Name: "j", Type: "t", Cron: "* * * * *"}
	if err := job.Insert(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < CronJobRunHistoryLimit+5; i++ {
		if err := RecordCronJobResult(job.Id, i%2 == 0, "run", int64(i)); err != nil {
			t.Fatal(err)
		}
	}
	runs, total, err := GetCronJobRuns(job.Id, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if total != CronJobRunHistoryLimit || len(runs) != 10 || runs[0].LatencyMs != int64(CronJobRunHistoryLimit+4) {
		t.Fatalf("history: total=%d len=%d first=%+v", total, len(runs), runs[0])
	}
}

func TestCronJobNotFound(t *testing.T) {
	setupTestDB(t)
	if _, err := GetCronJobById(99); !errors.Is(err, ErrCronJobNotFound) {
		t.Fatalf("get: %v", err)
	}
	if err := SetCronJobEnabled(99, true); !errors.Is(err, ErrCronJobNotFound) {
		t.Fatalf("toggle: %v", err)
	}
	job := &CronJob{Name: "j", Type: "t", Cron: "* * * * *"}
	_ = job.Insert()
	_ = RecordCronJobResult(job.Id, true, "ok", 1)
	if err := DeleteCronJobById(job.Id); err != nil {
		t.Fatal(err)
	}
	if _, total, _ := GetCronJobRuns(job.Id, 0, 10); total != 0 {
		t.Fatal("runs should be deleted with the job")
	}
}
