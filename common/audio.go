package common

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
)

var ErrAudioFormatNotSupported = errors.New("audio format not supported for duration detection")

// GetAudioDuration 返回音频时长（秒）。目前只解析 WAV 文件头，
// 其他格式返回 ErrAudioFormatNotSupported，调用方应回退到按大小估算。
func GetAudioDuration(_ context.Context, f io.ReadSeeker, ext string) (float64, error) {
	switch strings.ToLower(ext) {
	case ".wav", ".wave":
		return wavDuration(f)
	}
	return 0, fmt.Errorf("%w: %s", ErrAudioFormatNotSupported, ext)
}

// wavDuration 遍历 RIFF chunk，用 fmt 中的字节率和 data 的长度计算时长
func wavDuration(f io.ReadSeeker) (float64, error) {
	var riff [12]byte
	if _, err := io.ReadFull(f, riff[:]); err != nil {
		return 0, fmt.Errorf("read wav header: %w", err)
	}
	if string(riff[0:4]) != "RIFF" || string(riff[8:12]) != "WAVE" {
		return 0, errors.New("invalid wav header")
	}

	var byteRate uint32
	for {
		var chunk [8]byte
		if _, err := io.ReadFull(f, chunk[:]); err != nil {
			return 0, fmt.Errorf("read wav chunk: %w", err)
		}
		id := string(chunk[0:4])
		size := binary.LittleEndian.Uint32(chunk[4:8])
		switch id {
		case "fmt ":
			buf := make([]byte, size)
			if _, err := io.ReadFull(f, buf); err != nil {
				return 0, fmt.Errorf("read wav fmt chunk: %w", err)
			}
			if len(buf) < 12 {
				return 0, errors.New("wav fmt chunk too short")
			}
			byteRate = binary.LittleEndian.Uint32(buf[8:12])
		case "data":
			if byteRate == 0 {
				return 0, errors.New("wav data chunk before fmt chunk")
			}
			return float64(size) / float64(byteRate), nil
		default:
			if _, err := f.Seek(int64(size), io.SeekCurrent); err != nil {
				return 0, fmt.Errorf("skip wav chunk %q: %w", id, err)
			}
		}
		// chunk 按偶数字节对齐
		if size%2 == 1 {
			if _, err := f.Seek(1, io.SeekCurrent); err != nil {
				return 0, err
			}
		}
	}
}
