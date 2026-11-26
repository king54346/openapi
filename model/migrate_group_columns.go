package model

import (
	"fmt"
	"openapi/common"
)

// DropGroupColumns 一次性清理历史「分组」数据：
//   - 删除 users/tokens/channels/tasks/logs 表上的 group 列
//   - 删除 options 表中分组相关配置项
//
// 分组功能已从代码中移除，这些数据不再被读写；逐列判断存在性，失败只记日志不阻塞启动。
func DropGroupColumns() {
	if DB == nil {
		return
	}
	for _, target := range []struct {
		table string
		col   string
	}{
		{"users", "group"},
		{"tokens", "group"},
		{"channels", "group"},
		{"tasks", "group"},
		{"logs", "group"},
	} {
		if !DB.Migrator().HasColumn(target.table, target.col) {
			continue
		}
		if err := DB.Migrator().DropColumn(target.table, target.col); err != nil {
			common.SysError(fmt.Sprintf("failed to drop column %s.%s: %v", target.table, target.col, err))
			continue
		}
		common.SysLog(fmt.Sprintf("dropped column %s.%s", target.table, target.col))
	}

	dropGroupOptions()
	DropLegacyTables()
}

// legacyTables 已废弃的表：abilities（分组-模型-渠道映射，已被渠道缓存取代，且主键含 group）与
// search_channels（旧的渠道搜索快照）。代码不再读写，直接删除。
var legacyTables = []string{"abilities", "search_channels"}

// DropLegacyTables 删除已废弃的遗留表（幂等）。
func DropLegacyTables() {
	if DB == nil {
		return
	}
	for _, table := range legacyTables {
		if !DB.Migrator().HasTable(table) {
			continue
		}
		if err := DB.Migrator().DropTable(table); err != nil {
			common.SysError(fmt.Sprintf("failed to drop legacy table %s: %v", table, err))
			continue
		}
		common.SysLog("dropped legacy table " + table)
	}
}

// groupOptionKeys 随分组功能一起废弃的配置项。
var groupOptionKeys = []string{
	"UserUsableGroups",
	"GroupRatio",
	"AutoGroups",
	"DefaultUseAutoGroup",
	"GroupSpecialUsableGroup",
	"group_ratio_setting.group_special_usable_group",
	"group_ratio_setting.group_extra_visible_tags",
	"group_ratio_setting.tag_access_groups",
}

// dropGroupOptions 删除 options 表中的分组配置项并清理内存缓存。
func dropGroupOptions() {
	if !DB.Migrator().HasTable(&Option{}) {
		return
	}
	for _, key := range groupOptionKeys {
		if err := DeleteOptionValue(key); err != nil {
			common.SysError(fmt.Sprintf("failed to delete option %s: %v", key, err))
		}
	}
}
