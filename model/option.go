package model

import (
	"errors"
	"openapi/common"
	"openapi/setting"
	"strconv"
	"strings"
	"sync"
)

// options 表：系统配置的持久化键值对（key 为配置名，value 为文本或 JSON 文本）。
// 只映射 key/value 两列，表结构与 new-api 兼容。

// ErrOptionKeyRequired 配置名为空
var ErrOptionKeyRequired = errors.New("option key is required")

// Option 一条系统配置
type Option struct {
	Key   string `json:"key" gorm:"primaryKey;column:key"`
	Value string `json:"value" gorm:"column:value"`
}

// TableName 固定表名（避免 gorm 复数化差异）
func (Option) TableName() string { return "options" }

var (
	optionMapMu sync.RWMutex
	optionMap   = map[string]string{}
)

func init() {
	common.RegisterClusterEventHandler(common.EventTopicOptionRefresh, func(string) {
		if DB == nil {
			return
		}
		if err := loadOptionsFromDB(true); err != nil {
			common.SysError("failed to reload options on cluster event: " + err.Error())
		}
	})
}

// LoadOptionsFromDB 读取 options 表到内存，并把已知配置同步到 setting 包。
// 表不存在（旧库）时视为无配置，保持默认值。
func LoadOptionsFromDB() error {
	return loadOptionsFromDB(false)
}

func loadOptionsFromDB(quiet bool) error {
	if DB == nil || !DB.Migrator().HasTable(&Option{}) {
		return nil
	}
	var options []Option
	if err := DB.Find(&options).Error; err != nil {
		return err
	}
	values := make(map[string]string, len(options))
	for _, option := range options {
		values[option.Key] = option.Value
	}

	optionMapMu.Lock()
	optionMap = values
	optionMapMu.Unlock()

	for _, err := range setting.ApplyOptions(values) {
		common.SysError("failed to apply option: " + err.Error())
	}
	if !quiet || common.DebugEnabled {
		common.SysLog("options loaded: " + strconv.Itoa(len(values)))
	}
	return nil
}

// GetAllOptionValues 返回全部配置的内存副本。
func GetAllOptionValues() map[string]string {
	optionMapMu.RLock()
	defer optionMapMu.RUnlock()
	values := make(map[string]string, len(optionMap))
	for key, value := range optionMap {
		values[key] = value
	}
	return values
}

// GetOptionValue 取单个配置，第二个返回值表示是否存在。
func GetOptionValue(key string) (string, bool) {
	optionMapMu.RLock()
	defer optionMapMu.RUnlock()
	value, ok := optionMap[key]
	return value, ok
}

// UpdateOptionValue 写入（或覆盖）一个配置项：落库 + 更新内存 + 同步运行时配置。
func UpdateOptionValue(key, value string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return ErrOptionKeyRequired
	}
	if DB == nil {
		return errors.New("database is not initialized")
	}
	// options 表以 key 为主键，不同数据库对 key 的转义写法不同，这里用手写 SQL 之外的
	// gorm upsert：先尝试更新，未命中再插入，避免依赖各数据库的 ON CONFLICT 语法差异。
	result := DB.Model(&Option{}).Where(commonKeyCol+" = ?", key).Update("value", value)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		if err := DB.Create(&Option{Key: key, Value: value}).Error; err != nil {
			// 并发下可能已被其它请求插入，此时改回更新
			if updateErr := DB.Model(&Option{}).Where(commonKeyCol+" = ?", key).Update("value", value).Error; updateErr != nil {
				return errors.Join(err, updateErr)
			}
		}
	}

	optionMapMu.Lock()
	optionMap[key] = value
	optionMapMu.Unlock()

	for _, err := range setting.ApplyOptions(map[string]string{key: value}) {
		common.SysError("failed to apply option: " + err.Error())
	}
	common.PublishClusterEvent(common.EventTopicOptionRefresh, key)
	return nil
}

// DeleteOptionValue 删除配置项（不存在时也返回成功）。
func DeleteOptionValue(key string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return ErrOptionKeyRequired
	}
	if DB == nil {
		return errors.New("database is not initialized")
	}
	if err := DB.Where(commonKeyCol+" = ?", key).Delete(&Option{}).Error; err != nil {
		return err
	}
	optionMapMu.Lock()
	delete(optionMap, key)
	optionMapMu.Unlock()
	common.PublishClusterEvent(common.EventTopicOptionRefresh, key)
	return nil
}

// ListOptionsFromDB 直接从数据库读取全部配置（管理端展示用，包含未知键）。
func ListOptionsFromDB() ([]*Option, error) {
	if DB == nil || !DB.Migrator().HasTable(&Option{}) {
		return []*Option{}, nil
	}
	var options []*Option
	if err := DB.Order(commonKeyCol).Find(&options).Error; err != nil {
		return nil, err
	}
	return options, nil
}

// IsSensitiveOptionKey 配置名以 Secret / Token / Key 结尾时视为敏感项，对外展示需要脱敏。
func IsSensitiveOptionKey(key string) bool {
	trimmed := strings.TrimSpace(key)
	if trimmed == "" {
		return false
	}
	lower := strings.ToLower(trimmed)
	if idx := strings.LastIndexAny(lower, "._"); idx >= 0 {
		lower = lower[idx+1:]
	}
	switch {
	case strings.HasSuffix(lower, "secret"), strings.HasSuffix(lower, "token"), strings.HasSuffix(lower, "key"):
		return true
	}
	return false
}

// MaskOptionValue 敏感配置对外返回固定占位符，空值保持为空。
func MaskOptionValue(key, value string) string {
	if !IsSensitiveOptionKey(key) || value == "" {
		return value
	}
	return "******"
}
