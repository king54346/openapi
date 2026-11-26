package task_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"openapi/common"
	"openapi/constant"
	"openapi/controller/common/testhelper"
	taskctrl "openapi/controller/task"
	"openapi/model"

	gin "github.com/king54346/gin-tiny"
)

var videoTestPlatform = constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeSora))

func TestTaskListAPI(t *testing.T) {
	testhelper.NewTestServer(t)
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

	if total, _ := call(taskctrl.GetAllTask, 1, ""); total != 3 {
		t.Errorf("admin all: %d", total)
	}
	if total, _ := call(taskctrl.GetAllTask, 1, "channel_id=1&status=SUCCESS"); total != 2 {
		t.Errorf("admin filtered: %d", total)
	}
	if total, items := call(taskctrl.GetAllTask, 1, "p=1&page_size=1"); total != 3 || len(items) != 1 {
		t.Errorf("admin paged: total=%d items=%d", total, len(items))
	}
	if total, items := call(taskctrl.GetUserTask, 7, ""); total != 2 || items[0].UserId != 7 {
		t.Errorf("user 7: %d", total)
	}
	if total, _ := call(taskctrl.GetUserTask, 8, "channel_id=2"); total != 1 {
		t.Errorf("user 8: %d", total)
	}
	if total, _ := call(taskctrl.GetUserTask, 7, "start_timestamp="+strconv.FormatInt(now, 10)); total != 1 {
		t.Errorf("user 7 time range: %d", total)
	}
}

func TestVideoProxyAPI(t *testing.T) {
	s := testhelper.NewTestServer(t)
	alice := s.CreateUser("alice", common.RoleCommonUser)
	bob := s.CreateUser("bob", common.RoleCommonUser)

	videoBytes := strings.Repeat("v", 2048)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/video.mp4" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "video/mp4")
		if r.Header.Get("Range") == "bytes=0-9" {
			w.Header().Set("Content-Range", "bytes 0-9/2048")
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write([]byte(videoBytes[:10]))
			return
		}
		_, _ = w.Write([]byte(videoBytes))
	}))
	t.Cleanup(upstream.Close)

	ch := testhelper.InsertTestChannel(t, "ch", upstream.URL, "gpt-4o", common.ChannelStatusEnabled)
	task := &model.Task{
		TaskID: "task_ok", UserId: alice.ID, Platform: videoTestPlatform,
		Status: model.TaskStatusSuccess, ChannelId: ch.Id,
		PrivateData: model.TaskPrivateData{ResultURL: upstream.URL + "/video.mp4"},
	}
	if err := task.Insert(); err != nil {
		t.Fatal(err)
	}
	pending := &model.Task{
		TaskID: "task_pending", UserId: alice.ID, Platform: videoTestPlatform,
		Status: model.TaskStatusInProgress, ChannelId: ch.Id,
	}
	if err := pending.Insert(); err != nil {
		t.Fatal(err)
	}

	newProxyEngine := func(uid int) *gin.Engine {
		e := gin.New()
		e.GET("/v1/videos/:task_id/content", func(c gin.Context) {
			c.Set("id", uid)
			taskctrl.VideoProxy(c)
		})
		return e
	}
	aliceEngine := newProxyEngine(alice.ID)
	bobEngine := newProxyEngine(bob.ID)

	serve := func(e *gin.Engine, req *http.Request) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		e.ServeHTTP(w, req)
		return w
	}

	w := serve(aliceEngine, httptest.NewRequest(http.MethodGet, "/v1/videos/task_ok/content", nil))
	if w.Code != http.StatusOK || w.Body.String() != videoBytes {
		t.Fatalf("video proxy: %d, %d bytes", w.Code, w.Body.Len())
	}

	rangeReq := httptest.NewRequest(http.MethodGet, "/v1/videos/task_ok/content", nil)
	rangeReq.Header.Set("Range", "bytes=0-9")
	w = serve(aliceEngine, rangeReq)
	if w.Code != http.StatusPartialContent || w.Header().Get("Content-Range") == "" {
		t.Fatalf("range request: %d %v", w.Code, w.Header())
	}

	for _, tc := range []struct {
		engine *gin.Engine
		path   string
		want   int
	}{
		{aliceEngine, "/v1/videos/task_pending/content", http.StatusBadRequest},
		{aliceEngine, "/v1/videos/no_such_task/content", http.StatusNotFound},
		{bobEngine, "/v1/videos/task_ok/content", http.StatusNotFound},
	} {
		if w := serve(tc.engine, httptest.NewRequest(http.MethodGet, tc.path, nil)); w.Code != tc.want {
			t.Fatalf("%s: got %d, want %d", tc.path, w.Code, tc.want)
		}
	}
}
