package model

import (
	"errors"
	"strings"

	"openapi/common"

	"gorm.io/gorm"
)

var ErrTokenNotFound = errors.New("token not found")

// GetUserTokens 分页列出用户的令牌，按 id 倒序。
func GetUserTokens(userId, startIdx, num int) ([]*Token, int64, error) {
	query := DB.Model(&Token{}).Where("user_id = ?", userId)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var tokens []*Token
	err := query.Order("id desc").Limit(num).Offset(startIdx).Find(&tokens).Error
	return tokens, total, err
}

// SearchUserTokens 按名称模糊匹配或 key 精确匹配（可带 sk- 前缀）搜索用户的令牌。
func SearchUserTokens(userId int, keyword, key string) ([]*Token, error) {
	query := DB.Where("user_id = ?", userId)
	if keyword = strings.TrimSpace(keyword); keyword != "" {
		query = query.Where("name LIKE ?", "%"+keyword+"%")
	}
	if key = strings.TrimPrefix(strings.TrimSpace(key), "sk-"); key != "" {
		query = query.Where(commonKeyCol+" = ?", key)
	}
	var tokens []*Token
	err := query.Order("id desc").Limit(common.MaxRecentItems).Find(&tokens).Error
	return tokens, err
}

// GetUserTokenById 取用户自己的令牌，不属于该用户时返回 ErrTokenNotFound。
func GetUserTokenById(id, userId int) (*Token, error) {
	var token Token
	if err := DB.Where("id = ? AND user_id = ?", id, userId).First(&token).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTokenNotFound
		}
		return nil, err
	}
	return &token, nil
}

// Insert 创建令牌，自动生成 key。
func (t *Token) Insert() error {
	key, err := common.GenerateKey()
	if err != nil {
		return err
	}
	t.Key = key
	t.CreatedTime = common.GetTimestamp()
	t.AccessedTime = t.CreatedTime
	if t.Status == 0 {
		t.Status = common.TokenStatusEnabled
	}
	return DB.Create(t).Error
}

// tokenUpdatableColumns 允许用户修改的令牌字段（key、user_id、额度用量等不可改）
var tokenUpdatableColumns = []string{
	"name", "status", "expired_time", "remain_quota", "unlimited_quota",
	"model_limits_enabled", "model_limits", "model_limits_type", "allow_ips", "group", "cross_group_retry",
}

// Update 保存可修改字段（零值也会写入）。
func (t *Token) Update() error {
	if err := DB.Model(t).Select(tokenUpdatableColumns).Updates(t).Error; err != nil {
		return err
	}
	InvalidateTokenCache(t.Key)
	return nil
}

// UpdateStatus 只修改令牌状态。
func (t *Token) UpdateStatus(status int) error {
	if err := DB.Model(t).Update("status", status).Error; err != nil {
		return err
	}
	t.Status = status
	InvalidateTokenCache(t.Key)
	return nil
}

// DeleteUserTokens 软删除用户自己的令牌，返回删除条数。
func DeleteUserTokens(userId int, ids []int) (int64, error) {
	var tokens []*Token
	if err := DB.Select("id", commonKeyCol).Where("user_id = ? AND id IN ?", userId, ids).Find(&tokens).Error; err != nil {
		return 0, err
	}
	if len(tokens) == 0 {
		return 0, nil
	}
	result := DB.Where("user_id = ? AND id IN ?", userId, ids).Delete(&Token{})
	if result.Error != nil {
		return 0, result.Error
	}
	for _, t := range tokens {
		InvalidateTokenCache(t.Key)
	}
	return result.RowsAffected, nil
}
