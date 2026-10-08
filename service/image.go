package service

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"

	"openapi/constant"
)

// GetImageFromUrl 下载图片，返回 MIME 类型和 base64 编码的数据
func GetImageFromUrl(url string) (mimeType string, data string, err error) {
	resp, err := GetHttpClient().Get(url)
	if err != nil {
		return "", "", fmt.Errorf("failed to download image: %w", err)
	}
	defer CloseResponseBodyGracefully(resp)

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("failed to download image: HTTP %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType != "application/octet-stream" && !strings.HasPrefix(contentType, "image/") {
		return "", "", fmt.Errorf("invalid content type: %s, required image/*", contentType)
	}

	maxImageSize := int64(constant.MaxFileDownloadMB) << 20
	if resp.ContentLength > maxImageSize {
		return "", "", fmt.Errorf("image size %d exceeds maximum allowed size of %d bytes", resp.ContentLength, maxImageSize)
	}

	buffer := &bytes.Buffer{}
	written, err := io.Copy(buffer, io.LimitReader(resp.Body, maxImageSize+1))
	if err != nil {
		return "", "", fmt.Errorf("failed to read image data: %w", err)
	}
	if written > maxImageSize {
		return "", "", fmt.Errorf("image size exceeds maximum allowed size of %d bytes", maxImageSize)
	}

	mimeType = contentType
	if mimeType == "application/octet-stream" {
		mimeType = http.DetectContentType(buffer.Bytes())
		if !strings.HasPrefix(mimeType, "image/") {
			return "", "", fmt.Errorf("downloaded data is not an image: %s", mimeType)
		}
	}
	return mimeType, base64.StdEncoding.EncodeToString(buffer.Bytes()), nil
}
