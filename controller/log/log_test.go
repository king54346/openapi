package log_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"openapi/common"
	"openapi/controller/common/testhelper"
	"openapi/model"
)

func TestLogAPI(t *testing.T) {
	s := testhelper.NewTestServer(t)
	admin := s.CreateUser("admin", common.RoleAdminUser)
	alice := s.CreateUser("alice", common.RoleCommonUser)
	token := &model.Token{UserId: alice.ID, Name: "t", ExpiredTime: -1, UnlimitedQuota: true}
	if err := token.Insert(); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	for i, l := range []model.Log{
		{UserId: alice.ID, Username: "alice", Type: model.LogTypeConsume, ModelName: "gpt-4o", TokenId: token.Id, TokenName: "t", Quota: 10, RequestId: "r1"},
		{UserId: alice.ID, Username: "alice", Type: model.LogTypeError, ModelName: "gpt-4o", TokenId: token.Id, TokenName: "t", RequestId: "r2"},
		{UserId: admin.ID, Username: "admin", Type: model.LogTypeConsume, ModelName: "deepseek", Quota: 5},
	} {
		l.CreatedAt = now - int64(i)
		if err := model.LOG_DB.Create(&l).Error; err != nil {
			t.Fatal(err)
		}
	}

	all := testhelper.Decode[testhelper.Page[model.Log]](t, s.OK(admin, http.MethodGet, "/api/log/?p=1&page_size=2", nil))
	if all.Total != 3 || len(all.Items) != 2 {
		t.Fatalf("all logs: total=%d items=%d", all.Total, len(all.Items))
	}
	if consume := testhelper.Decode[testhelper.Page[model.Log]](t, s.OK(admin, http.MethodGet, fmt.Sprintf("/api/log/?type=%d", model.LogTypeConsume), nil)); consume.Total != 2 {
		t.Fatalf("consume logs: %d", consume.Total)
	}
	s.Fail(alice, http.MethodGet, "/api/log/", nil)
	if self := testhelper.Decode[testhelper.Page[model.Log]](t, s.OK(alice, http.MethodGet, "/api/log/self", nil)); self.Total != 2 {
		t.Fatalf("self logs: %d", self.Total)
	}
	stat := testhelper.Decode[map[string]int](t, s.OK(admin, http.MethodGet, fmt.Sprintf("/api/log/stat?type=%d", model.LogTypeConsume), nil))
	if stat["quota"] != 15 {
		t.Fatalf("stat: %v", stat)
	}

	byKey := testhelper.Decode[testhelper.Page[model.Log]](t, s.OK(testhelper.Actor{}, http.MethodGet, "/api/log/token?key=sk-"+token.Key+"&request_id=r2", nil))
	if byKey.Total != 1 || byKey.Items[0].RequestId != "r2" {
		t.Fatalf("logs by key: %+v", byKey)
	}
	if byKey = testhelper.Decode[testhelper.Page[model.Log]](t, s.OK(testhelper.Actor{}, http.MethodGet, "/api/log/token?key="+token.Key+"&p=2&page_size=1", nil)); byKey.Total != 2 || len(byKey.Items) != 1 || byKey.Items[0].RequestId != "r1" {
		t.Fatalf("logs by key page 2: %+v", byKey)
	}

	if r := testhelper.Decode[testhelper.Page[model.Log]](t, s.OK(admin, http.MethodGet, fmt.Sprintf("/api/log/search?keyword=%d&page_size=1", model.LogTypeConsume), nil)); r.Total != 2 || len(r.Items) != 1 {
		t.Fatalf("search all logs: %+v", r)
	}
	if r := testhelper.Decode[testhelper.Page[model.Log]](t, s.OK(alice, http.MethodGet, fmt.Sprintf("/api/log/self/search?keyword=%d", model.LogTypeConsume), nil)); r.Total != 1 || r.Items[0].Username != "alice" {
		t.Fatalf("search self logs: %+v", r)
	}
	s.Fail(testhelper.Actor{}, http.MethodGet, "/api/log/token?key=sk-nope", nil)

	s.Fail(admin, http.MethodDelete, "/api/log/", nil)
	if n := testhelper.Decode[int](t, s.OK(admin, http.MethodDelete, fmt.Sprintf("/api/log/?target_timestamp=%d", now), nil)); n != 2 {
		t.Fatalf("deleted logs: %d", n)
	}
}
