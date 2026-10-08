package controller

import (
	"context"
	"encoding/json"
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
	gin "github.com/king54346/gin-tiny"
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

// mockSoraUpstream 模拟 Sora 查询接口：vid_ok 已完成，vid_fail 失败，其余处理中。
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
		Key: "k", Name: "sora", Group: "default", Models: "sora-2", BaseURL: &baseURL}
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

	updateUnfinishedTasks(context.Background())

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
	// 只查询 3 个有效且未超时的任务：已完成、无 ID、超时、渠道已删除的都不应请求上游
	if n := calls.Load(); n != 3 {
		t.Errorf("expected 3 upstream calls, got %d", n)
	}

	// 第二轮只剩仍在处理中的任务
	calls.Store(0)
	updateUnfinishedTasks(context.Background())
	if n := calls.Load(); n != 1 {
		t.Errorf("second round: expected 1 upstream call, got %d", n)
	}
}

func TestTaskListAPI(t *testing.T) {
	setupTaskDB(t)
	now := time.Now().Unix()
	for i, spec := range []struct {
		user    int
		channel int
		status  model.TaskStatus
	}{
		{7, 1, model.TaskStatusSuccess},
		{7, 2, model.TaskStatusFailure},
		{8, 1, model.TaskStatusSuccess},
	} {
		task := &model.Task{TaskID: "t" + strconv.Itoa(i), Platform: "55", UserId: spec.user,
			ChannelId: spec.channel, Status: spec.status, SubmitTime: now - int64(i)}
		if err := task.Insert(); err != nil {
			t.Fatal(err)
		}
	}

	call := func(handler gin.HandlerFunc, userID int, query string) (int64, []model.Task) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.SetRequest(httptest.NewRequest(http.MethodGet, "/api/task/?"+query, nil))
		c.Set("id", userID)
		handler(c)
		var resp struct {
			Success bool `json:"success"`
			Data    struct {
				Items []model.Task `json:"items"`
				Total int64        `json:"total"`
			} `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil || !resp.Success {
			t.Fatalf("%s: %s", query, w.Body.String())
		}
		return resp.Data.Total, resp.Data.Items
	}

	if total, _ := call(GetAllTask, 1, ""); total != 3 {
		t.Errorf("admin all: %d", total)
	}
	if total, _ := call(GetAllTask, 1, "channel_id=1&status=SUCCESS"); total != 2 {
		t.Errorf("admin filtered: %d", total)
	}
	if total, items := call(GetAllTask, 1, "p=1&page_size=1"); total != 3 || len(items) != 1 {
		t.Errorf("admin paged: total=%d items=%d", total, len(items))
	}
	if total, items := call(GetUserTask, 7, ""); total != 2 || items[0].UserId != 7 {
		t.Errorf("user 7: %d", total)
	}
	// 用户接口忽略 channel_id，且只能看到自己的任务
	if total, _ := call(GetUserTask, 8, "channel_id=2"); total != 1 {
		t.Errorf("user 8: %d", total)
	}
	if total, _ := call(GetUserTask, 7, "start_timestamp="+strconv.FormatInt(now, 10)); total != 1 {
		t.Errorf("user 7 time range: %d", total)
	}
}
