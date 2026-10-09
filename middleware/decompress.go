package middleware

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"fmt"
	"io"
	"net/http"
	"strings"

	"openapi/common"

	"github.com/andybalholm/brotli"
	gin "github.com/king54346/gin-tiny"
	"github.com/klauspost/compress/zstd"
)

// DecompressRequestMiddleware 按 Content-Encoding 解压请求体
// （gzip / deflate / br / zstd）。
//
// 与 Compress（见 compress.go）配合构成完整的压缩链路：请求方向解压、
// 响应方向按客户端 Accept-Encoding 协商压缩。解压后会用明文替换
// c.Request().Body 并删除 Content-Encoding 头，后续流程无感知。
// 解压体积受 MaxRequestBodyMB 限制，不支持的编码直接返回 400。
func DecompressRequestMiddleware() gin.HandlerFunc {
	return func(c gin.Context) {
		encoding := strings.TrimSpace(c.Request().Header.Get("Content-Encoding"))
		if encoding == "" || strings.EqualFold(encoding, "identity") {
			c.Next()
			return
		}
		decoded, err := decodeRequestBody(c.Request().Body, encoding, common.MaxRequestBodyBytes()) // 解压后与请求体共用同一上限，防止压缩炸弹
		if err != nil {
			abortWithOpenAiMessage(c, http.StatusBadRequest, fmt.Sprintf("failed to decode %q request body: %v", encoding, err))
			return
		}
		req := c.Request()
		req.Body = io.NopCloser(bytes.NewReader(decoded))
		req.ContentLength = int64(len(decoded))
		req.Header.Del("Content-Encoding")
		req.Header.Del("Content-Length")
		c.Next()
	}
}

// decodeRequestBody 按编码解压，超限时返回错误。
func decodeRequestBody(body io.ReadCloser, encoding string, maxBytes int64) ([]byte, error) {
	if body == nil {
		return nil, nil
	}
	defer body.Close()

	var reader io.Reader = body
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "gzip", "x-gzip":
		gr, err := gzip.NewReader(body)
		if err != nil {
			return nil, err
		}
		defer gr.Close()
		reader = gr
	case "deflate":
		// RFC 7230 的 deflate 指 zlib 包裹格式；部分客户端发裸 deflate，这里都兼容。
		if zr, err := zlib.NewReader(body); err == nil {
			defer zr.Close()
			reader = zr
		} else {
			reader = flate.NewReader(body)
			if closer, ok := reader.(io.Closer); ok {
				defer closer.Close()
			}
		}
	case "br":
		reader = brotli.NewReader(body)
	case "zstd":
		zr, err := zstd.NewReader(body)
		if err != nil {
			return nil, err
		}
		defer zr.Close()
		reader = zr
	default:
		return nil, fmt.Errorf("unsupported content-encoding (only gzip/deflate/br/zstd are supported)")
	}

	decoded, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(decoded)) > maxBytes {
		return nil, fmt.Errorf("decompressed body exceeds %d MB", maxBytes>>20)
	}
	return decoded, nil
}
