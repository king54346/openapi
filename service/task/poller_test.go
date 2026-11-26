package task

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"openapi/common"
	"openapi/constant"
	"openapi/model"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupTaskDB(t *testing.T) {
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
	old := constant.TaskTimeoutMinutes
	constant.TaskTimeoutMinutes = 60
	t.Cleanup(func() { constant.TaskTimeoutMinutes = old })
}

func mockSoraUpstream(t *testing.T, calls *atomic.Int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		id := strings.TrimPrefix(r.URL.Path, "/v1/videos/")
		w.Header().Set("Content-Type", "application/json")
		switch id {
		case "vid_ok":
			_, _ = w.Write([]byte(`{"id":"vid_ok","status":"completed","progress":100}`))
		case "vid_fail":
			_, _ = w.Write([]byte(`{"id":"vid_fail","status":"failed","error":{"message":"content policy violation"}}`))
		default:
			_, _ = w.Write([]byte(`{"id":"` + id + `","status":"in_progress","progress":42}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestUpdateUnfinishedTasks(t *testing.T) {
	setupTaskDB(t)
	var calls atomic.Int32
	upstream := mockSoraUpstream(t, &calls)

	baseURL := upstream.URL
	ch := &model.Channel{Id: 1, Type: constant.ChannelTypeSora, Status: common.ChannelStatusEnabled,
		Key: "k", Name: "sora", Models: "sora-2", BaseURL: &baseURL}
	if err := ch.Insert(); err != nil {
		t.Fatal(err)
	}

	platform := constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeSora))
	now := time.Now().Unix()
	newTask := func(taskID string, channelID int, status model.TaskStatus, submit int64) *model.Task {
		task := &model.Task{TaskID: taskID, Platform: platform, UserId: 7, ChannelId: channelID,
			Status: status, SubmitTime: submit, Progress: "10%", Action: "generate"}
		if err := task.Insert(); err != nil {
			t.Fatal(err)
		}
		return task
	}
	ok := newTask("vid_ok", 1, model.TaskStatusInProgress, now)
	failed := newTask("vid_fail", 1, model.TaskStatusSubmitted, now)
	running := newTask("vid_running", 1, model.TaskStatusQueued, now)
	noID := newTask("", 1, model.TaskStatusSubmitted, now)
	timedOut := newTask("vid_old", 1, model.TaskStatusInProgress, now-2*3600)
	orphan := newTask("vid_orphan", 99, model.TaskStatusInProgress, now)
	done := newTask("vid_done", 1, model.TaskStatusSuccess, now)

	UpdateUnfinishedTasks(context.Background())

	get := func(task *model.Task) *model.Task {
		var got model.Task
		if err := model.DB.First(&got, task.ID).Error; err != nil {
			t.Fatal(err)
		}
		return &got
	}
	if got := get(ok); got.Status != model.TaskStatusSuccess || got.Progress != "100%" || got.FinishTime == 0 {
		t.Errorf("vid_ok: %+v", got)
	}
	if got := get(failed); got.Status != model.TaskStatusFailure || got.FailReason != "content policy violation" {
		t.Errorf("vid_fail: status=%s reason=%q", got.Status, got.FailReason)
	}
	if got := get(running); got.Status != model.TaskStatusInProgress || got.Progress != "42%" {
		t.Errorf("vid_running: status=%s progress=%s", got.Status, got.Progress)
	}
	for name, task := range map[string]*model.Task{"no task id": noID, "timeout": timedOut, "orphan": orphan} {
		if got := get(task); got.Status != model.TaskStatusFailure || got.Progress != "100%" || got.FailReason == "" {
			t.Errorf("%s: status=%s progress=%s reason=%q", name, got.Status, got.Progress, got.FailReason)
		}
	}
	if got := get(done); got.Status != model.TaskStatusSuccess {
		t.Errorf("finished task must not change: %+v", got)
	}
	if n := calls.Load(); n != 3 {
		t.Errorf("expected 3 upstream calls, got %d", n)
	}

	calls.Store(0)
	UpdateUnfinishedTasks(context.Background())
	if n := calls.Load(); n != 1 {
		t.Errorf("second round: expected 1 upstream call, got %d", n)
	}
}
