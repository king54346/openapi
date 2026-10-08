package model

import (
	"errors"
	"testing"

	"openapi/common"
)

func withCacheTTL(t *testing.T, seconds int) {
	t.Helper()
	old := common.SyncFrequency
	common.SyncFrequency = seconds
	ClearAuthCache()
	t.Cleanup(func() {
		common.SyncFrequency = old
		ClearAuthCache()
	})
}

func TestTokenCache(t *testing.T) {
	setupTestDB(t)
	withCacheTTL(t, 60)
	tk := &Token{UserId: 1, Name: "t", ExpiredTime: -1, UnlimitedQuota: true}
	if err := tk.Insert(); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateUserToken(tk.Key); err != nil {
		t.Fatal(err)
	}

	// 绕过本服务直接改库：缓存期内仍是旧值
	DB.Model(&Token{}).Where("id = ?", tk.Id).Update("status", common.TokenStatusDisabled)
	if _, err := ValidateUserToken(tk.Key); err != nil {
		t.Fatalf("cached token should still be valid, got %v", err)
	}
	// 失效后读到最新状态
	InvalidateTokenCache(tk.Key)
	if _, err := ValidateUserToken(tk.Key); !errors.Is(err, ErrTokenDisabled) {
		t.Fatalf("want ErrTokenDisabled after invalidation, got %v", err)
	}

	// 通过 model 方法修改会自动失效
	if err := tk.UpdateStatus(common.TokenStatusEnabled); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateUserToken(tk.Key); err != nil {
		t.Fatalf("re-enabled token should be valid, got %v", err)
	}
	if _, err := DeleteUserTokens(1, []int{tk.Id}); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateUserToken(tk.Key); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("deleted token: want ErrTokenInvalid, got %v", err)
	}
}

func TestTokenCacheDoesNotCacheMisses(t *testing.T) {
	setupTestDB(t)
	withCacheTTL(t, 60)
	if _, err := ValidateUserToken("later"); !errors.Is(err, ErrTokenInvalid) {
		t.Fatal("missing token should be invalid")
	}
	mustCreate(t, &Token{UserId: 1, Key: "later", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true})
	if _, err := ValidateUserToken("later"); err != nil {
		t.Fatalf("token created after a miss should be valid immediately, got %v", err)
	}
}

func TestUserCache(t *testing.T) {
	setupTestDB(t)
	withCacheTTL(t, 60)
	u := &User{Username: "u", Role: common.RoleAdminUser}
	if err := u.Insert("password123"); err != nil {
		t.Fatal(err)
	}
	if !IsAdmin(u.Id) {
		t.Fatal("should be admin")
	}
	if err := UpdateUserFields(u.Id, map[string]any{"role": common.RoleCommonUser, "status": common.UserStatusDisabled}, ""); err != nil {
		t.Fatal(err)
	}
	if IsAdmin(u.Id) {
		t.Fatal("role change should invalidate cache")
	}
	if c, _ := GetUserCache(u.Id); c.Status != common.UserStatusDisabled {
		t.Fatal("status change should invalidate cache")
	}
}

func TestAuthCacheDisabledWhenTTLZero(t *testing.T) {
	setupTestDB(t)
	withCacheTTL(t, 0)
	mustCreate(t, &Token{UserId: 1, Key: "k0", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true})
	if _, err := ValidateUserToken("k0"); err != nil {
		t.Fatal(err)
	}
	DB.Model(&Token{}).Where(commonKeyCol+" = ?", "k0").Update("status", common.TokenStatusDisabled)
	if _, err := ValidateUserToken("k0"); !errors.Is(err, ErrTokenDisabled) {
		t.Fatalf("with ttl 0 every lookup hits the db, got %v", err)
	}
}

func TestUserPasswordAndLogin(t *testing.T) {
	setupTestDB(t)
	u := &User{Username: "bob"}
	if err := u.Insert("password123"); err != nil {
		t.Fatal(err)
	}
	if u.Password == "password123" || u.AffCode == "" {
		t.Fatal("password must be hashed and aff_code generated")
	}
	if err := (&User{Username: "bob"}).Insert("password123"); !errors.Is(err, ErrUsernameTaken) {
		t.Fatalf("duplicate username: %v", err)
	}
	if _, err := ValidateUserLogin("bob", "password123"); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateUserLogin("bob", "wrong"); !errors.Is(err, ErrPasswordIncorrect) {
		t.Fatal("wrong password should fail")
	}
	if _, err := ValidateUserLogin("nobody", "password123"); !errors.Is(err, ErrPasswordIncorrect) {
		t.Fatal("unknown user should fail with the same error")
	}
	_ = UpdateUserFields(u.Id, map[string]any{"status": common.UserStatusDisabled}, "")
	if _, err := ValidateUserLogin("bob", "password123"); !errors.Is(err, ErrUserDisabled) {
		t.Fatal("disabled user should not log in")
	}
	// 软删除后用户名仍被占用
	_ = DeleteUserById(u.Id)
	if err := (&User{Username: "bob"}).Insert("password123"); !errors.Is(err, ErrUsernameTaken) {
		t.Fatal("soft-deleted username should stay taken")
	}
}
