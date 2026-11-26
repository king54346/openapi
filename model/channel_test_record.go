package model

import (
	"errors"
	"openapi/common"

	"gorm.io/gorm"
)

// 渠道测速记录（channel_test_records 表）。无删除标记，删除即物理删除。

// 测速状态：1 成功，0 失败（被跳过的模型同样记 0，错误信息里说明跳过原因）。
const (
	ChannelTestStatusFail    = 0
	ChannelTestStatusSuccess = 1
)

var ErrChannelTestRecordNotFound = errors.New("channel test record not found")

type ChannelTestRecord struct {
	Id           int    `json:"id"`
	CreatedAt    int64  `json:"created_at" gorm:"column:created_at;bigint"` // 秒级时间戳，复测时刷新
	ChannelId    int    `json:"channel_id" gorm:"index"`
	ChannelName  string `json:"channel_name" gorm:"type:varchar(100)"`
	ChannelTag   string `json:"channel_tag" gorm:"type:varchar(200)"`
	ModelName    string `json:"model_name" gorm:"type:varchar(200)"`
	Status       int    `json:"status"`
	ResponseTime int    `json:"response_time"` // 毫秒
	ErrorMessage string `json:"error_message" gorm:"type:text"`
	TestType     string `json:"test_type" gorm:"type:varchar(20)"`
}

// Insert 写入一条测速记录。
func (r *ChannelTestRecord) Insert() error {
	if r.CreatedAt == 0 {
		r.CreatedAt = common.GetTimestamp()
	}
	return DB.Create(r).Error
}

// ChannelTestRecordFilter 测速记录的筛选条件，零值表示不过滤。
type ChannelTestRecordFilter struct {
	ChannelId  int
	ModelName  string
	Status     *int // nil 表示不过滤
	TestType   string
	ChannelTag string
	StartTime  int64 // created_at 下限
	EndTime    int64 // created_at 上限
}

// Empty 是否没有任何筛选条件。
func (f ChannelTestRecordFilter) Empty() bool {
	return f.ChannelId <= 0 && f.ModelName == "" && f.Status == nil && f.TestType == "" &&
		f.ChannelTag == "" && f.StartTime <= 0 && f.EndTime <= 0
}

func applyTestRecordFilter(db *gorm.DB, f ChannelTestRecordFilter) *gorm.DB {
	if f.ChannelId > 0 {
		db = db.Where("channel_id = ?", f.ChannelId)
	}
	if f.ModelName != "" {
		db = db.Where("model_name = ?", f.ModelName)
	}
	if f.Status != nil {
		db = db.Where("status = ?", *f.Status)
	}
	if f.TestType != "" {
		db = db.Where("test_type = ?", f.TestType)
	}
	if f.ChannelTag != "" {
		db = db.Where("channel_tag = ?", f.ChannelTag)
	}
	if f.StartTime > 0 {
		db = db.Where("created_at >= ?", f.StartTime)
	}
	if f.EndTime > 0 {
		db = db.Where("created_at <= ?", f.EndTime)
	}
	return db
}

// ListChannelTestRecords 按条件分页查询测速记录（按 id 倒序）。
func ListChannelTestRecords(f ChannelTestRecordFilter, offset, limit int) ([]*ChannelTestRecord, int64, error) {
	return paginate[*ChannelTestRecord](applyTestRecordFilter(DB.Model(&ChannelTestRecord{}), f), "id desc", offset, limit)
}

// FindChannelTestRecords 按条件取出全部测速记录（不分页）。
func FindChannelTestRecords(f ChannelTestRecordFilter) ([]*ChannelTestRecord, error) {
	records := make([]*ChannelTestRecord, 0)
	err := applyTestRecordFilter(DB.Model(&ChannelTestRecord{}), f).Order("id desc").Find(&records).Error
	return records, err
}

// DeleteChannelTestRecords 按条件删除测速记录，返回删除条数。
func DeleteChannelTestRecords(f ChannelTestRecordFilter) (int64, error) {
	result := applyTestRecordFilter(DB, f).Delete(&ChannelTestRecord{})
	return result.RowsAffected, result.Error
}

// ClearChannelTestRecords 清空全部测速记录，返回删除条数。
func ClearChannelTestRecords() (int64, error) {
	result := DB.Where("1 = 1").Delete(&ChannelTestRecord{})
	return result.RowsAffected, result.Error
}

// GetChannelTestRecordById 按 id 取测速记录。
func GetChannelTestRecordById(id int) (*ChannelTestRecord, error) {
	var r ChannelTestRecord
	if err := DB.First(&r, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrChannelTestRecordNotFound
		}
		return nil, err
	}
	return &r, nil
}

// SaveTestResult 用一次测试结果刷新记录（复测后写回状态、耗时、错误与时间）。
func (r *ChannelTestRecord) SaveTestResult(status, responseTime int, errorMessage string, timestamp int64) error {
	return DB.Model(&ChannelTestRecord{}).Where("id = ?", r.Id).
		Updates(map[string]any{
			"status":        status,
			"response_time": responseTime,
			"error_message": errorMessage,
			"created_at":    timestamp,
		}).Error
}
