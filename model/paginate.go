package model

import "gorm.io/gorm"

// paginate 分页查询：先按条件计数，再按 order 取 [offset, offset+limit) 这一页；offset 超出总数时不再查数据。
// tx 需已带上 Model 与筛选条件；items 不会为 nil，序列化为 []。
func paginate[T any](tx *gorm.DB, order string, offset, limit int) ([]T, int64, error) {
	tx = tx.Session(&gorm.Session{}) // 计数与取数据共用条件，互不影响
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	items := make([]T, 0)
	if int64(offset) >= total {
		return items, total, nil
	}
	err := tx.Order(order).Offset(offset).Limit(limit).Find(&items).Error
	return items, total, err
}
