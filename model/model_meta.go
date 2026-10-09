package model

import (
	"errors"
	"sort"
	"strconv"
	"strings"

	"openapi/common"

	"gorm.io/gorm"
)

// 模型元数据（models 表），参照 new-api model/model_meta.go。
// 计费相关字段只做存取，本项目不据此计费。

// 模型名匹配规则
const (
	NameRuleExact = iota
	NameRulePrefix
	NameRuleContains
	NameRuleSuffix
)

var ErrModelMetaNotFound = errors.New("model not found")

// BoundChannel 提供该模型的渠道
type BoundChannel struct {
	Name string `json:"name"`
	Type int    `json:"type"`
}

// Model 模型元数据
type Model struct {
	Id                 int            `json:"id"`
	ModelName          string         `json:"model_name" gorm:"size:128;not null;uniqueIndex:uk_model_name_delete_at,priority:1"`
	Description        string         `json:"description,omitempty" gorm:"type:text"`
	Icon               string         `json:"icon,omitempty" gorm:"type:varchar(128)"`
	Tags               string         `json:"tags,omitempty" gorm:"type:varchar(255)"`
	VendorID           int            `json:"vendor_id,omitempty" gorm:"index"`
	Endpoints          string         `json:"endpoints,omitempty" gorm:"type:text"`
	Status             int            `json:"status" gorm:"default:1"`
	SyncOfficial       int            `json:"sync_official" gorm:"default:1"`
	BillingType        int            `json:"billing_type" gorm:"default:0"`
	PixelPrice         float64        `json:"pixel_price" gorm:"default:0"`
	VideoSizePrice     float64        `json:"video_size_price" gorm:"default:0"`
	VideoDurationPrice float64        `json:"video_duration_price" gorm:"default:0"`
	CreatedTime        int64          `json:"created_time" gorm:"bigint"`
	UpdatedTime        int64          `json:"updated_time" gorm:"bigint"`
	DeletedAt          gorm.DeletedAt `json:"-" gorm:"index;uniqueIndex:uk_model_name_delete_at,priority:2"`
	NameRule           int            `json:"name_rule" gorm:"default:0"`

	// 以下为查询时填充的附加信息，不入库
	BoundChannels []BoundChannel `json:"bound_channels,omitempty" gorm:"-"`
	EnableGroups  []string       `json:"enable_groups,omitempty" gorm:"-"`
	MatchedModels []string       `json:"matched_models,omitempty" gorm:"-"`
	MatchedCount  int            `json:"matched_count,omitempty" gorm:"-"`
}

// Insert 新建模型元数据
func (mi *Model) Insert() error {
	now := common.GetTimestamp()
	mi.CreatedTime = now
	mi.UpdatedTime = now
	return DB.Create(mi).Error
}

// Update 保存全部可编辑字段（不改 created_time）
func (mi *Model) Update() error {
	mi.UpdatedTime = common.GetTimestamp()
	return DB.Model(&Model{}).Where("id = ?", mi.Id).
		Select("model_name", "description", "icon", "tags", "vendor_id", "endpoints", "status", "sync_official",
			"billing_type", "pixel_price", "video_size_price", "video_duration_price", "name_rule", "updated_time").
		Updates(mi).Error
}

// GetModelMetaById 按 id 取模型元数据
func GetModelMetaById(id int) (*Model, error) {
	var m Model
	if err := DB.First(&m, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrModelMetaNotFound
		}
		return nil, err
	}
	return &m, nil
}

// DeleteModelMetaById 软删除模型元数据
func DeleteModelMetaById(id int) error {
	return DB.Delete(&Model{}, "id = ?", id).Error
}

// UpdateModelMetaStatus 只修改状态
func UpdateModelMetaStatus(id, status int) error {
	return DB.Model(&Model{}).Where("id = ?", id).
		Updates(map[string]any{"status": status, "updated_time": common.GetTimestamp()}).Error
}

// IsModelNameDuplicated 除 id 外是否已有同名模型
func IsModelNameDuplicated(id int, name string) (bool, error) {
	if name == "" {
		return false, nil
	}
	var cnt int64
	err := DB.Model(&Model{}).Where("model_name = ? AND id <> ?", name, id).Count(&cnt).Error
	return cnt > 0, err
}

// GetAllModels 分页列出模型元数据（按 id 倒序）
func GetAllModels(offset, limit int) ([]*Model, int64, error) {
	var total int64
	if err := DB.Model(&Model{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var models []*Model
	err := DB.Order("id DESC").Offset(offset).Limit(limit).Find(&models).Error
	return models, total, err
}

// SearchModels 按关键字（名称 / 描述 / 标签）、供应商（id 或名称）、标签（需全部命中）搜索
func SearchModels(keyword, vendor string, tags []string, offset, limit int) ([]*Model, int64, error) {
	db := DB.Model(&Model{})
	if keyword != "" {
		like := "%" + keyword + "%"
		db = db.Where("models.model_name LIKE ? OR models.description LIKE ? OR models.tags LIKE ?", like, like, like)
	}
	if vendor != "" {
		if vid, err := strconv.Atoi(vendor); err == nil {
			db = db.Where("models.vendor_id = ?", vid)
		} else {
			db = db.Joins("JOIN vendors ON vendors.id = models.vendor_id").Where("vendors.name LIKE ?", "%"+vendor+"%")
		}
	}
	// 标签以逗号分隔存储，两侧补逗号后精确匹配单个标签
	wrapped := "(',' || models.tags || ',')"
	if usingMySQL {
		wrapped = "CONCAT(',', models.tags, ',')"
	}
	for _, tag := range tags {
		if tag = strings.TrimSpace(tag); tag != "" {
			db = db.Where(wrapped+" LIKE ?", "%,"+tag+",%")
		}
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var models []*Model
	err := db.Order("models.id DESC").Offset(offset).Limit(limit).Find(&models).Error
	return models, total, err
}

// GetAllModelTags 所有模型使用过的标签（去重、已排序）
func GetAllModelTags() ([]string, error) {
	var tagRows []string
	if err := DB.Model(&Model{}).Where("tags <> '' AND tags IS NOT NULL").Pluck("tags", &tagRows).Error; err != nil {
		return nil, err
	}
	set := make(map[string]bool)
	for _, row := range tagRows {
		for _, t := range strings.Split(row, ",") {
			if t = strings.TrimSpace(t); t != "" {
				set[t] = true
			}
		}
	}
	tags := make([]string, 0, len(set))
	for t := range set {
		tags = append(tags, t)
	}
	sort.Strings(tags)
	return tags, nil
}

// GetVendorModelCounts 供应商 id -> 模型数量
func GetVendorModelCounts() (map[int64]int64, error) {
	var stats []struct {
		VendorID int64
		Count    int64
	}
	if err := DB.Model(&Model{}).Select("vendor_id, count(*) as count").Group("vendor_id").Scan(&stats).Error; err != nil {
		return nil, err
	}
	counts := make(map[int64]int64, len(stats))
	for _, s := range stats {
		counts[s.VendorID] = s.Count
	}
	return counts, nil
}

// matchModelName 模型名是否满足元数据的匹配规则
func matchModelName(rule int, pattern, name string) bool {
	switch rule {
	case NameRulePrefix:
		return strings.HasPrefix(name, pattern)
	case NameRuleSuffix:
		return strings.HasSuffix(name, pattern)
	case NameRuleContains:
		return strings.Contains(name, pattern)
	}
	return name == pattern
}

// EnrichModels 从渠道缓存填充附加信息：提供该模型的渠道、可用分组；
// 前缀 / 后缀 / 包含规则的元数据汇总所有命中的已启用模型。
func EnrichModels(models []*Model) {
	cache := getChannelCache()
	if cache == nil || len(models) == 0 {
		return
	}
	// 模型名 -> 渠道 / 分组
	channelsByModel := make(map[string]map[int]*Channel)
	groupsByModel := make(map[string]map[string]bool)
	for group, byModel := range cache.byGroupModel {
		for name, channels := range byModel {
			if groupsByModel[name] == nil {
				groupsByModel[name] = make(map[string]bool)
				channelsByModel[name] = make(map[int]*Channel)
			}
			groupsByModel[name][group] = true
			for _, ch := range channels {
				channelsByModel[name][ch.Id] = ch
			}
		}
	}

	for _, m := range models {
		if m == nil {
			continue
		}
		var matched []string
		for name := range channelsByModel {
			if matchModelName(m.NameRule, m.ModelName, name) {
				matched = append(matched, name)
			}
		}
		sort.Strings(matched)

		channelSet := make(map[int]*Channel)
		groupSet := make(map[string]bool)
		for _, name := range matched {
			for id, ch := range channelsByModel[name] {
				channelSet[id] = ch
			}
			for g := range groupsByModel[name] {
				groupSet[g] = true
			}
		}
		ids := make([]int, 0, len(channelSet))
		for id := range channelSet {
			ids = append(ids, id)
		}
		sort.Ints(ids)
		m.BoundChannels = make([]BoundChannel, 0, len(ids))
		for _, id := range ids {
			m.BoundChannels = append(m.BoundChannels, BoundChannel{Name: channelSet[id].Name, Type: channelSet[id].Type})
		}
		m.EnableGroups = make([]string, 0, len(groupSet))
		for g := range groupSet {
			m.EnableGroups = append(m.EnableGroups, g)
		}
		sort.Strings(m.EnableGroups)
		if m.NameRule != NameRuleExact {
			m.MatchedModels = matched
			m.MatchedCount = len(matched)
		}
	}
}
