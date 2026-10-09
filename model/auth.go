package model

import (
	"errors"
	"strings"
	"time"
	"unicode"

	"openapi/common"
	"openapi/constant"

	gin "github.com/king54346/gin-tiny"
	"gorm.io/gorm"
)

var (
	ErrTokenEmpty     = errors.New("no token provided")
	ErrTokenInvalid   = errors.New("invalid token")
	ErrTokenDisabled  = errors.New("token is disabled")
	ErrTokenExpired   = errors.New("token has expired")
	ErrTokenExhausted = errors.New("token quota is exhausted")
	ErrUserNotFound   = errors.New("user not found")
	errAuthDB         = errors.New("token validation failed, please try again later")
)

// User 用户。只映射本项目用到的列，users 表其余列（第三方登录、邀请等）保持不动。
type User struct {
	Id           int            `json:"id"`
	Username     string         `json:"username" gorm:"unique;index"`
	Password     string         `json:"-" gorm:"not null"` // bcrypt 哈希，永不输出
	DisplayName  string         `json:"display_name" gorm:"index"`
	Role         int            `json:"role" gorm:"default:1"`
	Status       int            `json:"status" gorm:"default:1"`
	Email        string         `json:"email" gorm:"index"`
	Group        string         `json:"group" gorm:"type:varchar(64);default:'default'"`
	AccessToken  *string        `json:"-" gorm:"type:char(32);column:access_token;uniqueIndex"`
	Quota        int            `json:"quota" gorm:"default:0"`
	UsedQuota    int            `json:"used_quota" gorm:"default:0"`
	RequestCount int            `json:"request_count" gorm:"default:0"`
	AffCode      string         `json:"-" gorm:"type:varchar(32);uniqueIndex"`
	Remark       string         `json:"remark" gorm:"type:varchar(255)"`
	DeletedAt    gorm.DeletedAt `json:"-" gorm:"index"`
}

// Token API 令牌。key 不含 "sk-" 前缀。
type Token struct {
	DeletedAt          gorm.DeletedAt `json:"-" gorm:"index"`
	Key                string         `json:"key" gorm:"type:char(48);uniqueIndex"`
	Name               string         `json:"name" gorm:"index"`
	ModelLimits        string         `json:"model_limits" gorm:"type:varchar(1024);default:''"`
	IpLimits           string         `json:"allow_ips" gorm:"column:allow_ips;default:''"`
	Group              string         `json:"group" gorm:"default:''"`
	Id                 int            `json:"id"`
	UserId             int            `json:"user_id" gorm:"index"`
	Status             int            `json:"status" gorm:"default:1"`
	CreatedTime        int64          `json:"created_time" gorm:"bigint"`
	AccessedTime       int64          `json:"accessed_time" gorm:"bigint"`
	ExpiredTime        int64          `json:"expired_time" gorm:"bigint;default:-1"` // -1 表示永不过期
	UsedQuota          int            `json:"used_quota" gorm:"default:0"`
	RemainQuota        int            `json:"remain_quota" gorm:"default:0"`
	ModelLimitsType    int            `json:"model_limits_type" gorm:"default:0"` // 0 白名单，1 黑名单
	UnlimitedQuota     bool           `json:"unlimited_quota"`
	ModelLimitsEnabled bool           `json:"model_limits_enabled"`
	CrossGroupRetry    bool           `json:"cross_group_retry"`
}

// UserCache 鉴权时写入请求上下文的用户信息。
type UserCache struct {
	Username string
	Role     int
	Status   int
	Group    string
}

// ValidateAccessToken 校验管理端 access token（Authorization 头，可带 Bearer 前缀）并返回对应用户。
// 无效时返回 nil。
func ValidateAccessToken(accessToken string) *User {
	accessToken = strings.TrimSpace(accessToken)
	if len(accessToken) > 7 && strings.EqualFold(accessToken[:7], "Bearer ") {
		accessToken = strings.TrimSpace(accessToken[7:])
	}
	if accessToken == "" {
		return nil
	}
	var user User
	if err := DB.Where("access_token = ?", accessToken).First(&user).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			common.SysError("failed to validate access token: " + err.Error())
		}
		return nil
	}
	return &user
}

// ValidateUserToken 校验 API 令牌：存在、启用、未过期、额度未耗尽。
// 只读校验，不回写令牌状态；额度扣减由计费钩子负责。
func ValidateUserToken(key string) (*Token, error) {
	if key == "" {
		return nil, ErrTokenEmpty
	}
	token, ok := tokenCache.get(key)
	if !ok {
		// 只缓存查到的令牌，不缓存"不存在"，避免新建令牌后要等缓存过期
		if err := DB.Where(commonKeyCol+" = ?", key).First(&token).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, ErrTokenInvalid
			}
			common.SysError("failed to query token: " + err.Error())
			return nil, errAuthDB
		}
		tokenCache.set(key, token)
	}
	// 状态与过期判断每次都基于当前时间重新计算，不依赖缓存时的结论
	switch token.Status {
	case common.TokenStatusEnabled:
	case common.TokenStatusExpired:
		return nil, ErrTokenExpired
	case common.TokenStatusExhausted:
		return nil, ErrTokenExhausted
	default:
		return nil, ErrTokenDisabled
	}
	if token.ExpiredTime != -1 && token.ExpiredTime < time.Now().Unix() {
		return nil, ErrTokenExpired
	}
	if !token.UnlimitedQuota && token.RemainQuota <= 0 {
		return nil, ErrTokenExhausted
	}
	return &token, nil
}

// getCachedUser 按 id 取用户（只含鉴权需要的列），带缓存。
func getCachedUser(userId int) (User, error) {
	if user, ok := userCache.get(userId); ok {
		return user, nil
	}
	var user User
	err := DB.Select("id", "username", "role", "status", commonGroupCol).Where("id = ?", userId).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return User{}, ErrUserNotFound
		}
		common.SysError("failed to query user: " + err.Error())
		return User{}, errAuthDB
	}
	userCache.set(userId, user)
	return user, nil
}

// GetUserCache 获取用户名、状态与分组。
func GetUserCache(userId int) (*UserCache, error) {
	user, err := getCachedUser(userId)
	if err != nil {
		return nil, err
	}
	return &UserCache{Username: user.Username, Role: user.Role, Status: user.Status, Group: user.Group}, nil
}

// IsAdmin 判断用户是否为管理员（含 root）。查询失败时按非管理员处理（fail-closed）。
func IsAdmin(userId int) bool {
	user, err := getCachedUser(userId)
	if err != nil {
		return false
	}
	return user.Role >= common.RoleAdminUser
}

// splitList 按逗号、换行或空白切分列表并去掉空项。
func splitList(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || unicode.IsSpace(r)
	})
}

// GetIpLimits 解析令牌 IP 白名单（逗号或换行分隔，支持纯 IP 与 CIDR）。
func (t *Token) GetIpLimits() []string {
	if t == nil {
		return nil
	}
	return splitList(t.IpLimits)
}

// GetModelLimitsMap 解析模型限制列表（逗号分隔）。
func (t *Token) GetModelLimitsMap() map[string]bool {
	if t == nil {
		return nil
	}
	limits := make(map[string]bool)
	for _, m := range strings.Split(t.ModelLimits, ",") {
		if m = strings.TrimSpace(m); m != "" {
			limits[m] = true
		}
	}
	return limits
}

// WriteContext 将用户缓存写入请求上下文。
func (u *UserCache) WriteContext(c gin.Context) {
	if u == nil {
		return
	}
	common.SetContextKey(c, constant.ContextKeyUserStatus, u.Status)
	common.SetContextKey(c, constant.ContextKeyUserGroup, u.Group)
	// 日志表的 username 列从这里取
	c.Set("username", u.Username)
}
