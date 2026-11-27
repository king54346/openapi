package middleware

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"openapi/common"

	"github.com/google/uuid"
	"github.com/gorilla/securecookie"
	gorillasessions "github.com/gorilla/sessions"
	ginsessions "github.com/king54346/gin-tiny/middleware/sessions"
	"github.com/king54346/gin-tiny/middleware/sessions/cookie"
)

// NewSessionStore 根据 Redis 启用状态返回服务端可控的 RedisSessionStore，未启用 Redis 时自动回退 CookieStore。
func NewSessionStore(secret []byte, opts ginsessions.Options) ginsessions.Store {
	if common.RedisEnabled && common.RDB != nil {
		store := NewRedisSessionStore(secret)
		store.Options(opts)
		return store
	}
	store := cookie.NewStore(secret)
	store.Options(opts)
	return store
}

// RedisSessionStore 将 session_id 签名写入浏览器 Cookie，真实会话状态存储在 Redis。
type RedisSessionStore struct {
	Codecs  []securecookie.Codec
	options *gorillasessions.Options
}

func NewRedisSessionStore(keyPairs ...[]byte) *RedisSessionStore {
	return &RedisSessionStore{
		Codecs: securecookie.CodecsFromPairs(keyPairs...),
		options: &gorillasessions.Options{
			Path:     "/",
			MaxAge:   2592000,
			HttpOnly: true,
			SameSite: http.SameSiteStrictMode,
		},
	}
}

func (s *RedisSessionStore) Options(opts ginsessions.Options) {
	s.options = opts.ToGorillaOptions()
}

func (s *RedisSessionStore) Get(r *http.Request, name string) (*gorillasessions.Session, error) {
	return gorillasessions.GetRegistry(r).Get(s, name)
}

func (s *RedisSessionStore) New(r *http.Request, name string) (*gorillasessions.Session, error) {
	session := gorillasessions.NewSession(s, name)
	opts := *s.options
	session.Options = &opts
	session.IsNew = true

	c, err := r.Cookie(name)
	if err != nil {
		return session, nil
	}
	var sid string
	if err := securecookie.DecodeMulti(name, c.Value, &sid, s.Codecs...); err != nil || strings.TrimSpace(sid) == "" {
		return session, nil
	}

	values, found, loadErr := common.LoadRedisSession(sid)
	if loadErr != nil {
		common.SysError("redis session load failed: " + loadErr.Error())
		return session, nil
	}
	if !found {
		return session, nil
	}
	session.ID = sid
	session.IsNew = false
	for k, v := range values {
		session.Values[k] = v
	}
	return session, nil
}

func (s *RedisSessionStore) Save(r *http.Request, w http.ResponseWriter, session *gorillasessions.Session) error {
	// MaxAge < 0 或会话内容已清空（如 /api/user/logout 调用 session.Clear()）时，主动删除 Redis 会话与浏览器 Cookie
	if session.Options.MaxAge < 0 || len(session.Values) == 0 {
		if session.ID != "" {
			_ = common.DeleteRedisSession(session.ID, extractUserIDFromValues(session.Values))
		}
		delOpts := *session.Options
		delOpts.MaxAge = -1
		http.SetCookie(w, gorillasessions.NewCookie(session.Name(), "", &delOpts))
		return nil
	}

	if session.ID == "" {
		session.ID = strings.ReplaceAll(uuid.NewString(), "-", "")
	}
	strMap := make(map[string]any, len(session.Values))
	for k, v := range session.Values {
		strMap[fmt.Sprint(k)] = v
	}
	ttl := time.Duration(session.Options.MaxAge) * time.Second
	if ttl <= 0 {
		ttl = 30 * 24 * time.Hour
	}
	if err := common.SaveRedisSession(session.ID, strMap, ttl); err != nil {
		return err
	}

	encoded, err := securecookie.EncodeMulti(session.Name(), session.ID, s.Codecs...)
	if err != nil {
		return err
	}
	http.SetCookie(w, gorillasessions.NewCookie(session.Name(), encoded, session.Options))
	return nil
}

func extractUserIDFromValues(vals map[any]any) int {
	for _, k := range []any{"id", string("id")} {
		if v, ok := vals[k]; ok {
			switch n := v.(type) {
			case int:
				return n
			case int64:
				return int(n)
			case float64:
				return int(n)
			}
		}
	}
	return 0
}
