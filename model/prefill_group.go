package model

import (
	"encoding/json"
	"errors"
	"openapi/common"

	"gorm.io/gorm"
)

// 预填充内容组（prefill_groups 表）。items 存任意 JSON 文本，进行时按原样序列化。

var ErrPrefillGroupNotFound = errors.New("prefill group not found")

type PrefillGroup struct {
	Id          int             `json:"id"`
	Name        string          `json:"name" gorm:"not null;size:128"`
	Type        string          `json:"type" gorm:"not null;size:64"`
	Items       json.RawMessage `json:"items,omitempty" gorm:"type:text"`
	Description string          `json:"description,omitempty" gorm:"type:varchar(255)"`
	CreatedTime int64           `json:"created_time" gorm:"bigint"`
	UpdatedTime int64           `json:"updated_time" gorm:"bigint"`
	DeletedAt   gorm.DeletedAt  `json:"-" gorm:"index"`
}

// Insert 新建预填充组。
func (g *PrefillGroup) Insert() error {
	now := common.GetTimestamp()
	g.CreatedTime = now
	g.UpdatedTime = now
	return DB.Create(g).Error
}

// Update 保存可编辑字段（不含 created_time）。
func (g *PrefillGroup) Update() error {
	g.UpdatedTime = common.GetTimestamp()
	return DB.Model(&PrefillGroup{}).Where("id = ?", g.Id).
		Select("name", "type", "items", "description", "updated_time").
		Updates(g).Error
}

// Delete 软删除预填充组。
func (g *PrefillGroup) Delete() error {
	return DB.Delete(g).Error
}

// GetPrefillGroupById 按 id 取预填充组。
func GetPrefillGroupById(id int) (*PrefillGroup, error) {
	var g PrefillGroup
	if err := DB.First(&g, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPrefillGroupNotFound
		}
		return nil, err
	}
	return &g, nil
}

// IsPrefillGroupNameDuplicated 未删除的预填充组中是否已存在同名（排除自身）。
func IsPrefillGroupNameDuplicated(id int, name string) (bool, error) {
	if name == "" {
		return false, nil
	}
	var cnt int64
	err := DB.Model(&PrefillGroup{}).Where("name = ? AND id <> ?", name, id).Count(&cnt).Error
	return cnt > 0, err
}

// ListPrefillGroups 分页列出预填充组（按 id 倒序），可按类型过滤。
func ListPrefillGroups(typ string, offset, limit int) ([]*PrefillGroup, int64, error) {
	db := DB.Model(&PrefillGroup{})
	if typ != "" {
		db = db.Where("type = ?", typ)
	}
	return paginate[*PrefillGroup](db, "id desc", offset, limit)
}
