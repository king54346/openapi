package common

import (
	"strconv"

	gin "github.com/king54346/gin-tiny"
)

// 分页参数，参照 new-api common/page_info.go。所有列表 / 搜索接口统一用 GetPageQuery 解析。

// ItemsPerPage 未指定 page_size 时的默认每页条数
const ItemsPerPage = 10

// maxPageSize 每页条数上限
const maxPageSize = 100

type PageInfo struct {
	Page     int `json:"page"`      // 页码，从 1 开始
	PageSize int `json:"page_size"` // 每页条数
}

// GetStartIdx 当前页第一条的偏移量（SQL OFFSET）。
func (p *PageInfo) GetStartIdx() int {
	return (p.Page - 1) * p.PageSize
}

func (p *PageInfo) GetPageSize() int {
	return p.PageSize
}

func (p *PageInfo) GetPage() int {
	return p.Page
}

// firstPositiveQuery 依次读取参数，返回第一个正整数，都没有时返回 0。
func firstPositiveQuery(c gin.Context, keys ...string) int {
	for _, key := range keys {
		if v, err := strconv.Atoi(c.Query(key)); err == nil && v > 0 {
			return v
		}
	}
	return 0
}

// GetPageQuery 解析分页参数：页码 p（兼容 page，默认 1），每页条数 page_size（兼容 ps、size，默认 ItemsPerPage，最大 100）。
func GetPageQuery(c gin.Context) *PageInfo {
	pageInfo := &PageInfo{
		Page:     firstPositiveQuery(c, "p", "page"),
		PageSize: firstPositiveQuery(c, "page_size", "ps", "size"),
	}
	if pageInfo.Page < 1 {
		pageInfo.Page = 1
	}
	if pageInfo.PageSize == 0 {
		pageInfo.PageSize = ItemsPerPage
	}
	if pageInfo.PageSize > maxPageSize {
		pageInfo.PageSize = maxPageSize
	}
	return pageInfo
}
