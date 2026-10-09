package model

import (
	"strings"

	"openapi/common"

	"gorm.io/gorm"
)

// 渠道列表 / 搜索：筛选、计数、分页都在数据库完成，不把全部渠道读进内存。

// ChannelStatusFilter 渠道状态筛选
type ChannelStatusFilter int

const (
	ChannelStatusAny          ChannelStatusFilter = iota // 不过滤
	ChannelStatusOnlyEnabled                             // 只看启用
	ChannelStatusOnlyDisabled                            // 只看禁用（含手动与自动禁用）
)

// ChannelFilter 渠道筛选条件，零值字段不参与过滤。
type ChannelFilter struct {
	Keyword string              // id 精确、名称与 base_url 模糊、完整 key 精确
	Group   string              // 分组（逗号分隔的 group 列中的一项）
	Model   string              // 模型名模糊匹配
	Status  ChannelStatusFilter // 状态
	Type    int                 // 渠道类型，0（Unknown）表示不过滤
	IdSort  bool                // true 按 id 倒序，否则按优先级倒序
}

// where 把筛选条件加到查询上；withType=false 时忽略类型条件（用于按类型计数）。
func (f ChannelFilter) where(tx *gorm.DB, withType bool) *gorm.DB {
	if kw := strings.TrimSpace(f.Keyword); kw != "" {
		like := "%" + kw + "%"
		tx = tx.Where("(id = ? OR name LIKE ? OR "+commonKeyCol+" = ? OR base_url LIKE ?)",
			common.String2Int(kw), like, kw, like)
	}
	if m := strings.TrimSpace(f.Model); m != "" {
		tx = tx.Where("models LIKE ?", "%"+m+"%")
	}
	if g := strings.TrimSpace(f.Group); g != "" && g != "null" {
		groupExpr := "(',' || " + commonGroupCol + " || ',')"
		if usingMySQL {
			groupExpr = "CONCAT(',', " + commonGroupCol + ", ',')"
		}
		tx = tx.Where(groupExpr+" LIKE ?", "%,"+g+",%")
	}
	switch f.Status {
	case ChannelStatusOnlyEnabled:
		tx = tx.Where("status = ?", common.ChannelStatusEnabled)
	case ChannelStatusOnlyDisabled:
		tx = tx.Where("status <> ?", common.ChannelStatusEnabled)
	}
	if withType && f.Type > 0 {
		tx = tx.Where("type = ?", f.Type)
	}
	return tx
}

func (f ChannelFilter) order() string {
	if f.IdSort {
		return "id desc"
	}
	return "priority desc, id desc"
}

// ListChannels 分页查询满足条件的渠道（不含 key），返回当页数据与总数。
func ListChannels(f ChannelFilter, offset, limit int) ([]*Channel, int64, error) {
	return paginate[*Channel](f.where(DB.Model(&Channel{}).Omit("key"), true), f.order(), offset, limit)
}

// tagged 只看有标签的渠道。
func tagged(tx *gorm.DB) *gorm.DB {
	return tx.Where("tag IS NOT NULL AND tag <> ''")
}

// ListChannelTags 分页查询标签：至少有一个渠道满足条件的非空标签，按标签名排序，返回当页标签与标签总数。
func ListChannelTags(f ChannelFilter, offset, limit int) ([]string, int64, error) {
	var total int64
	if err := tagged(f.where(DB.Model(&Channel{}), true)).Distinct("tag").Count(&total).Error; err != nil {
		return nil, 0, err
	}
	tags := make([]string, 0)
	if int64(offset) >= total {
		return tags, total, nil
	}
	err := tagged(f.where(DB.Model(&Channel{}), true)).Distinct().Order("tag").
		Offset(offset).Limit(limit).Pluck("tag", &tags).Error
	return tags, total, err
}

// ListChannelsByTags 取这些标签下满足条件的渠道（不含 key），按标签名分组，组内按排序规则。
func ListChannelsByTags(f ChannelFilter, tags []string) ([]*Channel, error) {
	channels := make([]*Channel, 0)
	if len(tags) == 0 {
		return channels, nil
	}
	err := f.where(DB.Omit("key"), true).Where("tag IN ?", tags).
		Order("tag").Order(f.order()).Find(&channels).Error
	return channels, err
}

// CountChannelsByType 满足条件（忽略类型条件）的渠道按类型计数，供前端在类型标签上显示数量。
func CountChannelsByType(f ChannelFilter) (map[int64]int64, error) {
	var rows []struct {
		Type  int64
		Count int64
	}
	err := f.where(DB.Model(&Channel{}), false).Select("type, count(*) as count").Group("type").Find(&rows).Error
	if err != nil {
		return nil, err
	}
	counts := make(map[int64]int64, len(rows))
	for _, r := range rows {
		counts[r.Type] = r.Count
	}
	return counts, nil
}
