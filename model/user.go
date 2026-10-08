package model

import (
	"errors"
	"strings"
	"sync"

	"openapi/common"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var (
	ErrUsernameTaken     = errors.New("username already exists")
	ErrPasswordIncorrect = errors.New("username or password is incorrect")
	ErrUserDisabled      = errors.New("user is disabled")
)

var getDummyHash = sync.OnceValue(func() []byte {
	hash, _ := bcrypt.GenerateFromPassword([]byte("dummy-password"), bcrypt.DefaultCost)
	return hash
})

// HashPassword 生成 bcrypt 哈希（与 users 表已有数据兼容）。
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hash), err
}

// GetUserById 按 id 取用户（不含已删除）。
func GetUserById(id int) (*User, error) {
	var user User
	if err := DB.First(&user, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return &user, nil
}

// GetAllUsers 分页列出用户，按 id 倒序。
func GetAllUsers(startIdx, num int) ([]*User, int64, error) {
	var users []*User
	var total int64
	if err := DB.Model(&User{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := DB.Order("id desc").Limit(num).Offset(startIdx).Find(&users).Error
	return users, total, err
}

// SearchUsers 按 id、用户名、显示名、邮箱搜索，可按分组过滤。
func SearchUsers(keyword, group string, startIdx, num int) ([]*User, int64, error) {
	query := DB.Model(&User{})
	if keyword = strings.TrimSpace(keyword); keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("id = ? OR username LIKE ? OR display_name LIKE ? OR email LIKE ?",
			common.String2Int(keyword), like, like, like)
	}
	if group != "" {
		query = query.Where(commonGroupCol+" = ?", group)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var users []*User
	err := query.Order("id desc").Limit(num).Offset(startIdx).Find(&users).Error
	return users, total, err
}

// Insert 创建用户，password 为明文，入库前做 bcrypt。
func (user *User) Insert(password string) error {
	var count int64
	// 已软删除的用户名仍占用唯一索引，这里一并检查
	if err := DB.Unscoped().Model(&User{}).Where("username = ?", user.Username).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return ErrUsernameTaken
	}
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	user.Password = hash
	if user.Group == "" {
		user.Group = "default"
	}
	if user.Status == 0 {
		user.Status = common.UserStatusEnabled
	}
	if user.Role == 0 {
		user.Role = common.RoleCommonUser
	}
	if user.DisplayName == "" {
		user.DisplayName = user.Username
	}
	return DB.Create(user).Error
}

// BeforeCreate aff_code 有唯一索引，空串也会冲突，任何方式建用户都必须生成。
func (user *User) BeforeCreate(*gorm.DB) error {
	if user.AffCode != "" {
		return nil
	}
	code, err := common.GenerateRandomCharsKey(8)
	if err != nil {
		return err
	}
	user.AffCode = code
	return nil
}

// UpdateUserFields 按列更新用户；password 非空时同时重置密码。
func UpdateUserFields(id int, fields map[string]any, password string) error {
	if password != "" {
		hash, err := HashPassword(password)
		if err != nil {
			return err
		}
		fields["password"] = hash
	}
	if len(fields) == 0 {
		return nil
	}
	if err := DB.Model(&User{}).Where("id = ?", id).Updates(fields).Error; err != nil {
		return err
	}
	InvalidateUserCache(id)
	return nil
}

// DeleteUserById 软删除用户。
func DeleteUserById(id int) error {
	if err := DB.Delete(&User{}, "id = ?", id).Error; err != nil {
		return err
	}
	InvalidateUserCache(id)
	return nil
}

// BatchUpdateUserGroup 批量修改用户分组。
func BatchUpdateUserGroup(ids []int, group string) (int64, error) {
	result := DB.Model(&User{}).Where("id IN ?", ids).Update(commonGroupCol, group)
	if result.Error != nil {
		return 0, result.Error
	}
	for _, id := range ids {
		InvalidateUserCache(id)
	}
	return result.RowsAffected, nil
}

// ValidateUserLogin 校验用户名和密码，返回用户。
func ValidateUserLogin(username, password string) (*User, error) {
	var user User
	if err := DB.Where("username = ?", username).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// 用户不存在时同样做一次哈希比较，避免通过响应时间探测用户名
			_ = bcrypt.CompareHashAndPassword(getDummyHash(), []byte(password))
			return nil, ErrPasswordIncorrect
		}
		return nil, err
	}
	if bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)) != nil {
		return nil, ErrPasswordIncorrect
	}
	if user.Status != common.UserStatusEnabled {
		return nil, ErrUserDisabled
	}
	return &user, nil
}

// GenerateUserAccessToken 为用户生成新的管理端 access token（32 位）。
func GenerateUserAccessToken(id int) (string, error) {
	token, err := common.GenerateRandomCharsKey(32)
	if err != nil {
		return "", err
	}
	if err := DB.Model(&User{}).Where("id = ?", id).Update("access_token", token).Error; err != nil {
		return "", err
	}
	return token, nil
}
