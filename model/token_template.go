package model

import (
	"errors"
	"openapi/common"

	"gorm.io/gorm"
)

// 令牌「访问限制」模板（token_access_templates 表）。按 user_id 严格隔离，硬删除。
// 创建/编辑令牌时套用：model_limits_type（0 白名单 / 1 黑名单）、model_limits、allow_ips。

var ErrTokenAccessTemplateNotFound = errors.New("token access template not found")

type TokenAccessTemplate struct {
	Id              int    `json:"id"`
	UserId          int    `json:"user_id" gorm:"index"`
	Name            string `json:"name" gorm:"type:varchar(64)"`
	ModelLimitsType int    `json:"model_limits_type" gorm:"default:0"` // 0 白名单，1 黑名单
	ModelLimits     string `json:"model_limits" gorm:"type:varchar(2048);default:''"`
	AllowIps        string `json:"allow_ips" gorm:"column:allow_ips;type:text"`
	CreatedTime     int64  `json:"created_time" gorm:"bigint"`
	UpdatedTime     int64  `json:"updated_time" gorm:"bigint"`
}

// Insert 新建模板。
func (t *TokenAccessTemplate) Insert() error {
	now := common.GetTimestamp()
	t.CreatedTime = now
	t.UpdatedTime = now
	return DB.Create(t).Error
}

// Update 保存可编辑字段（不含 user_id、created_time）。
func (t *TokenAccessTemplate) Update() error {
	t.UpdatedTime = common.GetTimestamp()
	return DB.Model(&TokenAccessTemplate{}).Where("id = ?", t.Id).
		Select("name", "model_limits_type", "model_limits", "allow_ips", "updated_time").
		Updates(t).Error
}

// Delete 硬删除模板（只能删自己的）。
func (t *TokenAccessTemplate) Delete() error {
	return DB.Where("id = ? AND user_id = ?", t.Id, t.UserId).Delete(&TokenAccessTemplate{}).Error
}

// ListUserTokenAccessTemplates 分页列出用户的模板（按 id 倒序）。
func ListUserTokenAccessTemplates(userId, offset, limit int) ([]*TokenAccessTemplate, int64, error) {
	return paginate[*TokenAccessTemplate](DB.Model(&TokenAccessTemplate{}).Where("user_id = ?", userId),
		"id desc", offset, limit)
}

// GetTokenAccessTemplateById 取用户自己的模板，不属于该用户时返回 ErrTokenAccessTemplateNotFound。
func GetTokenAccessTemplateById(id, userId int) (*TokenAccessTemplate, error) {
	var t TokenAccessTemplate
	if err := DB.Where("id = ? AND user_id = ?", id, userId).First(&t).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTokenAccessTemplateNotFound
		}
		return nil, err
	}
	return &t, nil
}
