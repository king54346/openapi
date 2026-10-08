package middleware

import (
	"compress/gzip"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/andybalholm/brotli"
	gin "github.com/king54346/gin-tiny"
	"github.com/klauspost/compress/zstd"
)

// responseEncoding 响应压缩编码。
type responseEncoding string

const (
	encodingIdentity responseEncoding = "identity"
	encodingGzip     responseEncoding = "gzip"
	encodingBrotli   responseEncoding = "br"
	encodingZstd     responseEncoding = "zstd"
)

// Compress 响应压缩中间件（默认配置）。
//
// 与 DecompressRequestMiddleware（见 decompress.go）配合，构成完整的压缩链路：
// 请求方向由 DecompressRequestMiddleware 解压，响应方向由本中间件按客户端
// Accept-Encoding 协商压缩（优先级 zstd > br > gzip）。
// 项目内统一使用本包的压缩能力，不再直接依赖 gin-tiny 的 gzip 中间件，
// 以便统一豁免规则（SSE 流、WebSocket、已压缩的图片格式不压）。
func Compress() gin.HandlerFunc {
	return CompressWithLevel(gzip.DefaultCompression)
}

// CompressWithLevel 自定义压缩级别的响应压缩中间件，level 取值同 compress/gzip。
// br / zstd 使用各自库的默认级别（响应压缩以吞吐优先，级别差异收益不大）。
func CompressWithLevel(level int, opts ...CompressOption) gin.HandlerFunc {
	cfg := defaultCompressConfig()
	for _, opt := range opts {
		opt(cfg)
	}
	gzipPool := &sync.Pool{
		New: func() any {
			w, err := gzip.NewWriterLevel(io.Discard, level)
			if err != nil {
				panic(err)
			}
			return w
		},
	}
	brotliPool := &sync.Pool{
		New: func() any {
			return brotli.NewWriter(io.Discard)
		},
	}
	zstdPool := &sync.Pool{
		New: func() any {
			w, err := zstd.NewWriter(io.Discard)
			if err != nil {
				panic(err)
			}
			return w
		},
	}
	return func(c gin.Context) {
		enc := negotiateResponseEncoding(c.Request())
		if enc == encodingIdentity {
			c.Next()
			return
		}
		if cfg.isExcludedPath(c.Request().URL.Path) {
			c.Next()
			return
		}
		switch enc {
		case encodingZstd:
			encw, ok := zstdPool.Get().(*zstd.Encoder)
			if !ok {
				c.Next()
				return
			}
			defer zstdPool.Put(encw)
			encw.Reset(io.Discard)
			encw.Reset(c.Response())
			c.SetResponse(&compressWriter{ResponseWriter: c.Response(), writer: encw, encoding: string(enc)})
			defer func() {
				_ = encw.Close()
			}()
			c.Next()
		case encodingBrotli:
			brw, ok := brotliPool.Get().(*brotli.Writer)
			if !ok {
				c.Next()
				return
			}
			defer brotliPool.Put(brw)
			brw.Reset(io.Discard)
			brw.Reset(c.Response())
			c.SetResponse(&compressWriter{ResponseWriter: c.Response(), writer: brw, encoding: string(enc)})
			defer func() {
				_ = brw.Close()
			}()
			c.Next()
		default:
			gz, ok := gzipPool.Get().(*gzip.Writer)
			if !ok {
				c.Next()
				return
			}
			defer gzipPool.Put(gz)
			gz.Reset(io.Discard)
			gz.Reset(c.Response())
			c.SetResponse(&compressWriter{ResponseWriter: c.Response(), writer: gz, encoding: string(encodingGzip)})
			defer func() {
				_ = gz.Close()
			}()
			c.Next()
		}
	}
}

// compressConfig 压缩可调配置。
type compressConfig struct {
	excludedExtensions map[string]bool
	excludedPaths      []string
}

// CompressOption 压缩配置项。
type CompressOption func(*compressConfig)

// WithCompressExcludedPaths 排除指定前缀的路径（如健康检查接口不需要压缩）。
func WithCompressExcludedPaths(paths ...string) CompressOption {
	return func(c *compressConfig) {
		c.excludedPaths = append(c.excludedPaths, paths...)
	}
}

// WithCompressExcludedExtensions 覆盖默认的图片扩展名豁免列表。
func WithCompressExcludedExtensions(exts ...string) CompressOption {
	return func(c *compressConfig) {
		c.excludedExtensions = map[string]bool{}
		for _, e := range exts {
			c.excludedExtensions[strings.ToLower(e)] = true
		}
	}
}

func defaultCompressConfig() *compressConfig {
	return &compressConfig{
		excludedExtensions: map[string]bool{
			".png": true, ".gif": true, ".jpeg": true, ".jpg": true,
			".webp": true, ".avif": true, ".mp4": true, ".webm": true,
		},
	}
}

func (c *compressConfig) isExcludedPath(path string) bool {
	for _, prefix := range c.excludedPaths {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

// negotiateResponseEncoding 按 Accept-Encoding 协商响应编码。
// 豁免 SSE/WS/图片等不值得压缩的场景时返回 identity（不压缩）。
// 多编码都接受时优先级：zstd > br > gzip（同 q 值下按压缩率排序）。
func negotiateResponseEncoding(req *http.Request) responseEncoding {
	if req == nil {
		return encodingIdentity
	}
	// WebSocket 升级与 SSE 流不压缩。
	if headerHasToken(req.Header.Values("Connection"), "upgrade") {
		return encodingIdentity
	}
	if strings.Contains(req.Header.Get("Accept"), "text/event-stream") {
		return encodingIdentity
	}
	// 已压缩的图片/视频扩展名不压。
	if ext := strings.ToLower(filepath.Ext(req.URL.Path)); ext != "" {
		if defaultCompressConfig().excludedExtensions[ext] {
			return encodingIdentity
		}
	}
	q := parseAcceptEncoding(req.Header.Values("Accept-Encoding"))
	best := encodingIdentity
	bestQ := 0.0
	// 优先级顺序：zstd > br > gzip，同 q 值时排前面的胜出。
	for _, enc := range []responseEncoding{encodingZstd, encodingBrotli, encodingGzip} {
		name := string(enc)
		v, ok := q[name]
		if !ok && enc == encodingGzip {
			// x-gzip 是 gzip 别名。
			v, ok = q["x-gzip"]
		}
		if !ok {
			// 通配符 * 兜底（gzip 一直可用，br/zstd 也接受通配）。
			v, ok = q["*"]
		}
		if !ok || v <= 0 {
			continue
		}
		if best == encodingIdentity || v > bestQ {
			best, bestQ = enc, v
		}
	}
	return best
}

// parseAcceptEncoding 按 RFC 9110 解析 Accept-Encoding，返回各编码的 q 值。
// 非法 q 值视为 0（拒绝），未出现的编码不在 map 中。
func parseAcceptEncoding(values []string) map[string]float64 {
	q := map[string]float64{}
	for _, v := range values {
		for _, part := range strings.Split(v, ",") {
			coding, params, _ := strings.Cut(part, ";")
			coding = strings.TrimSpace(strings.ToLower(coding))
			if coding == "" {
				continue
			}
			qv := 1.0
			for _, p := range strings.Split(params, ";") {
				if name, val, ok := strings.Cut(strings.TrimSpace(p), "="); ok && strings.EqualFold(strings.TrimSpace(name), "q") {
					parsed, err := strconv.ParseFloat(strings.TrimSpace(val), 64)
					if err != nil || parsed < 0 || parsed > 1 {
						parsed = 0
					}
					qv = parsed
				}
			}
			if prev, ok := q[coding]; !ok || qv > prev {
				q[coding] = qv
			}
		}
	}
	return q
}

// acceptsGzipEncoding 是否接受 gzip（保留给外部调用与单测，内部协商走 negotiateResponseEncoding）。
func acceptsGzipEncoding(values []string) bool {
	q := parseAcceptEncoding(values)
	if v, ok := q["gzip"]; ok {
		return v > 0
	}
	if v, ok := q["x-gzip"]; ok {
		return v > 0
	}
	if v, ok := q["*"]; ok {
		return v > 0
	}
	return false
}

// headerHasToken 判断逗号分隔的头部是否包含某个 token（忽略大小写）。
func headerHasToken(values []string, token string) bool {
	for _, v := range values {
		for _, part := range strings.Split(v, ",") {
			if strings.EqualFold(strings.TrimSpace(part), token) {
				return true
			}
		}
	}
	return false
}

// compressWriter 压缩响应写入器，内嵌 gin.ResponseWriter 以透传其余方法。
// writer 为 gzip / br / zstd 三者之一（均满足 io.Writer + Flush + Close）。
type compressWriter struct {
	gin.ResponseWriter
	writer interface {
		io.Writer
		Flush() error
		Close() error
	}
	encoding    string
	headerSent  bool
	passthrough bool
}

// ensureHeader 在第一次写入前设置压缩头；204/304 等无 body 状态码直接透传。
func (w *compressWriter) ensureHeader() {
	if w.headerSent {
		return
	}
	w.headerSent = true
	status := w.ResponseWriter.Status()
	if status == http.StatusNoContent || status == http.StatusNotModified {
		w.passthrough = true
		return
	}
	w.Header().Set("Content-Encoding", w.encoding)
	w.Header().Set("Vary", "Accept-Encoding")
	w.Header().Del("Content-Length")
}

func (w *compressWriter) WriteHeader(code int) {
	if code == http.StatusNoContent || code == http.StatusNotModified {
		w.passthrough = true
		w.headerSent = true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *compressWriter) Write(data []byte) (int, error) {
	w.ensureHeader()
	if w.passthrough {
		return w.ResponseWriter.Write(data)
	}
	w.Header().Del("Content-Length")
	return w.writer.Write(data)
}

// WriteString 必须覆盖，否则会绕过压缩直接写底层。
func (w *compressWriter) WriteString(s string) (int, error) {
	return w.Write([]byte(s))
}

// Flush 把 gzip 缓冲刷出，SSE/流式场景及时送达。
func (w *compressWriter) Flush() {
	w.ensureHeader()
	if !w.passthrough {
		_ = w.writer.Flush()
	}
	w.ResponseWriter.Flush()
}
