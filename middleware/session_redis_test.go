package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"openapi/common"

	"github.com/alicebob/miniredis/v2"
	gin "github.com/king54346/gin-tiny"
	"github.com/king54346/gin-tiny/middleware/sessions"
	"github.com/redis/go-redis/v9"
)

func setupMiniredisGlobal(t *testing.T) *miniredis.Miniredis {
	t.Helper()
	mr := miniredis.RunT(t)
	oldRDB, oldEnabled := common.RDB, common.RedisEnabled
	common.RDB = redis.NewClient(&redis.Options{Addr: mr.Addr()})
	common.RedisEnabled = true
	t.Cleanup(func() {
		_ = common.RDB.Close()
		common.RDB = oldRDB
		common.RedisEnabled = oldEnabled
	})
	return mr
}

func TestRedisSessionStoreLoginLogoutAndRevoke(t *testing.T) {
	setupMiniredisGlobal(t)
	gin.SetMode(gin.TestMode)

	store := NewSessionStore([]byte("test-redis-session-secret"), sessions.Options{
		Path:     "/",
		MaxAge:   3600,
		HttpOnly: true,
	})
	if _, ok := store.(*RedisSessionStore); !ok {
		t.Fatalf("expected *RedisSessionStore when RedisEnabled=true, got %T", store)
	}

	engine := gin.New()
	engine.Use(sessions.Sessions("session", store))
	engine.POST("/login", func(c gin.Context) {
		s := sessions.Default(c)
		s.Set("id", 42)
		s.Set("username", "alice")
		s.Set("role", 1)
		s.Set("status", 1)
		if err := s.Save(); err != nil {
			t.Fatalf("save session: %v", err)
		}
		c.String(http.StatusOK, "ok")
	})
	engine.GET("/me", func(c gin.Context) {
		s := sessions.Default(c)
		uid := s.Get("id")
		if uid == nil {
			c.String(http.StatusUnauthorized, "unauthorized")
			return
		}
		idInt, ok := uid.(int)
		if !ok || idInt != 42 {
			t.Fatalf("expected int id=42, got %T(%v)", uid, uid)
		}
		c.String(http.StatusOK, s.Get("username").(string))
	})
	engine.POST("/logout", func(c gin.Context) {
		s := sessions.Default(c)
		s.Clear()
		_ = s.Save()
		c.String(http.StatusOK, "logged_out")
	})

	// 1. 登录后携带 Cookie 访问 /me 成功
	w1 := httptest.NewRecorder()
	engine.ServeHTTP(w1, httptest.NewRequest(http.MethodPost, "/login", nil))
	cookie := w1.Header().Get("Set-Cookie")
	if cookie == "" {
		t.Fatal("expected Set-Cookie header after login")
	}

	reqMe := httptest.NewRequest(http.MethodGet, "/me", nil)
	reqMe.Header.Set("Cookie", cookie)
	w2 := httptest.NewRecorder()
	engine.ServeHTTP(w2, reqMe)
	if w2.Code != http.StatusOK || w2.Body.String() != "alice" {
		t.Fatalf("expected 200 alice, got %d %s", w2.Code, w2.Body.String())
	}

	// 2. 管理员调用 RevokeUserSessions(42) 强制踢下线后，原 Cookie 立即失效
	common.RevokeUserSessions(42)
	reqAfterRevoke := httptest.NewRequest(http.MethodGet, "/me", nil)
	reqAfterRevoke.Header.Set("Cookie", cookie)
	w3 := httptest.NewRecorder()
	engine.ServeHTTP(w3, reqAfterRevoke)
	if w3.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 after RevokeUserSessions, got %d %s", w3.Code, w3.Body.String())
	}

	// 3. 重新登录再主动调用 /logout，原 Cookie 也立即失效
	w4 := httptest.NewRecorder()
	engine.ServeHTTP(w4, httptest.NewRequest(http.MethodPost, "/login", nil))
	cookie2 := w4.Header().Get("Set-Cookie")

	reqLogout := httptest.NewRequest(http.MethodPost, "/logout", nil)
	reqLogout.Header.Set("Cookie", cookie2)
	engine.ServeHTTP(httptest.NewRecorder(), reqLogout)

	reqAfterLogout := httptest.NewRequest(http.MethodGet, "/me", nil)
	reqAfterLogout.Header.Set("Cookie", cookie2)
	w5 := httptest.NewRecorder()
	engine.ServeHTTP(w5, reqAfterLogout)
	if w5.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 after logout, got %d", w5.Code)
	}
}
