package frontend

import (
	"embed"
	"io/fs"
)

// BuildFS 内嵌 Vite 生产构建目录 frontend/dist。
//
//go:embed all:dist
var BuildFS embed.FS

// IndexPage 读取内嵌的 SPA 入口页 dist/index.html。
func IndexPage() []byte {
	data, err := fs.ReadFile(BuildFS, "dist/index.html")
	if err != nil {
		return nil
	}
	return data
}
