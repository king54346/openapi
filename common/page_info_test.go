package common

import (
	"net/http/httptest"
	"testing"

	gin "github.com/king54346/gin-tiny"
)

func TestGetPageQuery(t *testing.T) {
	cases := []struct {
		query          string
		page, pageSize int
	}{
		{"", 1, ItemsPerPage},
		{"p=3&page_size=20", 3, 20},
		{"page=2&ps=5", 2, 5},                 // 兼容 page、ps
		{"p=4&page=2&size=7", 4, 7},           // p 优先于 page
		{"p=-1&page_size=0", 1, ItemsPerPage}, // 非法值取默认
		{"p=abc&page_size=1000", 1, maxPageSize},
	}
	for _, tc := range cases {
		var got *PageInfo
		engine := gin.New()
		engine.GET("/", func(c gin.Context) { got = GetPageQuery(c) })
		engine.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/?"+tc.query, nil))
		if got == nil || got.Page != tc.page || got.PageSize != tc.pageSize {
			t.Fatalf("%q: got %+v, want page=%d size=%d", tc.query, got, tc.page, tc.pageSize)
		}
		if want := (tc.page - 1) * tc.pageSize; got.GetStartIdx() != want {
			t.Fatalf("%q: start idx %d, want %d", tc.query, got.GetStartIdx(), want)
		}
	}
}
