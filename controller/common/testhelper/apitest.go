package testhelper

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"strconv"
	"testing"

	"openapi/common"
	"openapi/constant"
	"openapi/model"
	"openapi/router"
	"openapi/setting"

	"github.com/glebarez/sqlite"
	gin "github.com/king54346/gin-tiny"
	"github.com/king54346/gin-tiny/middleware/sessions"
	"github.com/king54346/gin-tiny/middleware/sessions/cookie"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type APIResp struct {
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type Page[T any] struct {
	Items []T   `json:"items"`
	Total int64 `json:"total"`
}

type Actor struct {
	ID          int
	AccessToken string
	Cookie      string
}

type TestServer struct {
	T      *testing.T
	Engine *gin.Engine
}

func NewTestServer(t *testing.T) *TestServer {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	if err := model.InitDB(db, nil); err != nil {
		t.Fatal(err)
	}
	if err := model.InitChannelCache(); err != nil {
		t.Fatal(err)
	}
	common.SyncFrequency = 60
	model.ClearAuthCache()
	t.Cleanup(func() { _ = sqlDB.Close() })

	engine := gin.New()
	store := cookie.NewStore([]byte("test-secret"))
	engine.Use(sessions.Sessions("session", store))
	router.SetApiRouter(engine)
	return &TestServer{T: t, Engine: engine}
}

func (s *TestServer) CreateUser(username string, role int) Actor {
	s.T.Helper()
	u := &model.User{Username: username, Role: role}
	if err := u.Insert("password123"); err != nil {
		s.T.Fatalf("create user %s: %v", username, err)
	}
	token, err := model.GenerateUserAccessToken(u.Id)
	if err != nil {
		s.T.Fatal(err)
	}
	return Actor{ID: u.Id, AccessToken: token}
}

func (s *TestServer) Do(a Actor, method, path string, body any) (*httptest.ResponseRecorder, APIResp) {
	s.T.Helper()
	var reader *bytes.Reader
	if body != nil {
		b, _ := common.Marshal(body)
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	if a.ID > 0 {
		req.Header.Set("Open-Api-User", strconv.Itoa(a.ID))
	}
	if a.AccessToken != "" {
		req.Header.Set("Authorization", a.AccessToken)
	}
	if a.Cookie != "" {
		req.Header.Set("Cookie", a.Cookie)
	}
	w := httptest.NewRecorder()
	s.Engine.ServeHTTP(w, req)
	var resp APIResp
	_ = common.Unmarshal(w.Body.Bytes(), &resp)
	return w, resp
}

func (s *TestServer) OK(a Actor, method, path string, body any) json.RawMessage {
	s.T.Helper()
	w, resp := s.Do(a, method, path, body)
	if !resp.Success {
		s.T.Fatalf("%s %s: expected success, got %d %s", method, path, w.Code, w.Body.String())
	}
	return resp.Data
}

func (s *TestServer) Fail(a Actor, method, path string, body any) string {
	s.T.Helper()
	w, resp := s.Do(a, method, path, body)
	if resp.Success {
		s.T.Fatalf("%s %s: expected failure, got %d %s", method, path, w.Code, w.Body.String())
	}
	return resp.Message
}

func Decode[T any](t *testing.T, raw json.RawMessage) T {
	t.Helper()
	var v T
	if err := common.Unmarshal(raw, &v); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
	return v
}

func InsertTestChannel(t *testing.T, name, baseURL, models string, status int) *model.Channel {
	t.Helper()
	autoBan := 1
	ch := &model.Channel{
		Type:    constant.ChannelTypeOpenAI,
		Key:     "k-" + name,
		Name:    name,
		Status:  status,
		Models:  models,
		BaseURL: &baseURL,
		AutoBan: &autoBan,
	}
	if err := ch.Insert(); err != nil {
		t.Fatal(err)
	}
	return ch
}

func ResetGlobalSettings(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		setting.UpdateGlobalSettings(func(s *setting.GlobalSettings) {
			s.Notice, s.About, s.HomePageContent, s.UserAgreement, s.PrivacyPolicy, s.DocsLink = "", "", "", "", "", ""
			s.TurnstileCheckEnabled, s.TurnstileSiteKey, s.TurnstileSecretKey = false, "", ""
		})
	})
}
