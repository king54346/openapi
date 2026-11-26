package model

import (
	"errors"
	"openapi/common"

	"gorm.io/gorm"
)

// setups 表：记录系统是否已完成首次初始化（建 root 账号）。
// 只有一行（id=1），不存在即表示尚未初始化。

// Setup 初始化状态
type Setup struct {
	Id            int    `json:"id" gorm:"primaryKey"`
	Version       string `json:"version" gorm:"type:varchar(50);not null"`
	InitializedAt int64  `json:"initialized_at" gorm:"bigint;not null"`
}

// TableName 固定表名
func (Setup) TableName() string { return "setups" }

// ErrAlreadyInitialized 系统已经初始化过，不能重复初始化
var ErrAlreadyInitialized = errors.New("system is already initialized")

// IsSetupInitialized 系统是否已完成初始化：存在 setups 记录且存在 root 账号。
func IsSetupInitialized() (bool, error) {
	if DB == nil {
		return false, errors.New("database is not initialized")
	}
	var count int64
	if err := DB.Model(&Setup{}).Count(&count).Error; err != nil {
		return false, err
	}
	if count == 0 {
		return false, nil
	}
	rootExists, err := HasRootUser()
	if err != nil {
		return false, err
	}
	return rootExists, nil
}

// HasRootUser 是否已存在 root 账号。
func HasRootUser() (bool, error) {
	if DB == nil {
		return false, errors.New("database is not initialized")
	}
	var count int64
	if err := DB.Model(&User{}).Where("role = ?", common.RoleRootUser).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// CreateRootUserAndMarkSetup 首次初始化：创建 root 账号并写入 setups 记录。
// 已初始化返回 ErrAlreadyInitialized，避免并发下建出多个 root。
func CreateRootUserAndMarkSetup(username, password, displayName string) (*User, error) {
	initialized, err := IsSetupInitialized()
	if err != nil {
		return nil, err
	}
	if initialized {
		return nil, ErrAlreadyInitialized
	}

	root := &User{
		Username:    username,
		DisplayName: displayName,
		Role:        common.RoleRootUser,
		Status:      common.UserStatusEnabled,
	}
	if err := root.Insert(password); err != nil {
		return nil, err
	}

	setup := &Setup{Id: 1, Version: Version, InitializedAt: common.GetTimestamp()}
	if err := DB.Create(setup).Error; err != nil {
		// setups 写入失败时回滚 root 账号，保持"未初始化"状态可重试
		if deleteErr := DeleteUserById(root.Id); deleteErr != nil {
			return nil, errors.Join(err, deleteErr)
		}
		return nil, err
	}
	return root, nil
}

// GetSetupRecord 读取初始化记录，不存在时返回 nil。
func GetSetupRecord() (*Setup, error) {
	if DB == nil {
		return nil, errors.New("database is not initialized")
	}
	var setup Setup
	if err := DB.Order("id").First(&setup).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &setup, nil
}
