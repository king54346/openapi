package model

import (
	"errors"
	"openapi/common"

	"gorm.io/gorm"
)

// 供应商元数据（vendors 表）。name+deleted_at 唯一，软删除后允许重名。

var ErrVendorNotFound = errors.New("vendor not found")

type Vendor struct {
	Id          int            `json:"id"`
	Name        string         `json:"name" gorm:"not null;size:128;uniqueIndex:uk_vendor_name_deleted_at,priority:1"`
	Description string         `json:"description,omitempty" gorm:"type:varchar(255)"`
	Icon        string         `json:"icon,omitempty" gorm:"type:varchar(128)"`
	Status      int            `json:"status" gorm:"default:1"`
	CreatedTime int64          `json:"created_time" gorm:"bigint"`
	UpdatedTime int64          `json:"updated_time" gorm:"bigint"`
	DeletedAt   gorm.DeletedAt `json:"-" gorm:"index;uniqueIndex:uk_vendor_name_deleted_at,priority:2"`
}

// Insert 新建供应商，状态默认 1。
func (v *Vendor) Insert() error {
	now := common.GetTimestamp()
	v.CreatedTime = now
	v.UpdatedTime = now
	if v.Status == 0 {
		v.Status = 1
	}
	return DB.Create(v).Error
}

// Update 保存可编辑字段（不含 created_time）。
func (v *Vendor) Update() error {
	v.UpdatedTime = common.GetTimestamp()
	return DB.Model(&Vendor{}).Where("id = ?", v.Id).
		Select("name", "description", "icon", "status", "updated_time").
		Updates(v).Error
}

// Delete 软删除供应商。
func (v *Vendor) Delete() error {
	return DB.Delete(v).Error
}

// GetVendorById 按 id 取供应商。
func GetVendorById(id int) (*Vendor, error) {
	var v Vendor
	if err := DB.First(&v, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrVendorNotFound
		}
		return nil, err
	}
	return &v, nil
}

// IsVendorNameDuplicated 在未删除的供应商中是否存在同名（排除自身）。
func IsVendorNameDuplicated(id int, name string) (bool, error) {
	if name == "" {
		return false, nil
	}
	var cnt int64
	err := DB.Model(&Vendor{}).Where("name = ? AND id <> ?", name, id).Count(&cnt).Error
	return cnt > 0, err
}

// ListVendors 分页列出供应商（按 id 倒序）。
func ListVendors(offset, limit int) ([]*Vendor, int64, error) {
	return paginate[*Vendor](DB.Model(&Vendor{}), "id desc", offset, limit)
}

// SearchVendors 按名称/描述模糊搜索，可按状态过滤。
func SearchVendors(keyword string, status *int, offset, limit int) ([]*Vendor, int64, error) {
	db := DB.Model(&Vendor{})
	if keyword != "" {
		like := "%" + keyword + "%"
		db = db.Where("name LIKE ? OR description LIKE ?", like, like)
	}
	if status != nil {
		db = db.Where("status = ?", *status)
	}
	return paginate[*Vendor](db, "id desc", offset, limit)
}
