package common

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
)

// 请求体存储：同一请求的原始 Body 只读取一次，存进 BodyStorage 后由各中间件、业务函数重复读取。
// 小请求体放内存；开启磁盘缓存且超过阈值时落盘，请求结束由 CleanupBodyStorage 统一释放。
// 每次读取都通过 NewReader 拿到独立游标的 reader，互不影响；磁盘存储用 ReadAt 读取，可并发使用。

// ErrStorageClosed 存储已关闭（请求结束后不应再读取）
var ErrStorageClosed = errors.New("body storage is closed")

// ErrRequestBodyTooLarge 请求体超过 MAX_REQUEST_BODY_MB
var ErrRequestBodyTooLarge = errors.New("request body too large")

// BodyStorage 一份请求体数据
type BodyStorage interface {
	// Bytes 完整内容。内存存储返回内部切片（只读，不要修改）；磁盘存储每次从文件读取一份新的
	Bytes() ([]byte, error)
	// NewReader 从头开始读取的独立 reader
	NewReader() (io.Reader, error)
	// Size 数据大小（字节）
	Size() int64
	// IsDisk 是否落盘
	IsDisk() bool
	// Close 释放资源（磁盘存储会删除临时文件），可重复调用
	Close() error
}

// BodyDiskCache 请求体落盘能力，由 common/system 注册（磁盘缓存的配置与统计都在那里）。
// 未注册时请求体只存内存。
type BodyDiskCache interface {
	// ShouldUseDisk 该大小的请求体是否落盘（已包含开关、阈值与剩余容量判断）
	ShouldUseDisk(size int64) bool
	// CreateFile 创建一个临时缓存文件
	CreateFile() (path string, file *os.File, err error)
	// OnStore / OnRelease 存储创建与释放时回调，用于统计占用
	OnStore(size int64, disk bool)
	OnRelease(size int64, disk bool)
}

type bodyDiskCacheHolder struct{ cache BodyDiskCache }

var registeredBodyDiskCache atomic.Pointer[bodyDiskCacheHolder]

// RegisterBodyDiskCache 注册请求体落盘实现，传 nil 取消注册。
func RegisterBodyDiskCache(cache BodyDiskCache) {
	if cache == nil {
		registeredBodyDiskCache.Store(nil)
		return
	}
	registeredBodyDiskCache.Store(&bodyDiskCacheHolder{cache: cache})
}

func bodyDiskCache() BodyDiskCache {
	if h := registeredBodyDiskCache.Load(); h != nil {
		return h.cache
	}
	return nil
}

// ---------- 内存存储 ----------

type memoryStorage struct {
	data   []byte
	closed atomic.Bool
	cache  BodyDiskCache // 用于释放时回调统计，可为 nil
}

func newMemoryStorage(data []byte, cache BodyDiskCache) *memoryStorage {
	if cache != nil {
		cache.OnStore(int64(len(data)), false)
	}
	return &memoryStorage{data: data, cache: cache}
}

func (m *memoryStorage) Bytes() ([]byte, error) {
	if m.closed.Load() {
		return nil, ErrStorageClosed
	}
	return m.data, nil
}

func (m *memoryStorage) NewReader() (io.Reader, error) {
	if m.closed.Load() {
		return nil, ErrStorageClosed
	}
	return bytes.NewReader(m.data), nil
}

func (m *memoryStorage) Size() int64  { return int64(len(m.data)) }
func (m *memoryStorage) IsDisk() bool { return false }

func (m *memoryStorage) Close() error {
	if m.closed.CompareAndSwap(false, true) && m.cache != nil {
		m.cache.OnRelease(int64(len(m.data)), false)
	}
	return nil
}

// ---------- 磁盘存储 ----------

type diskStorage struct {
	file   *os.File
	path   string
	size   int64
	cache  BodyDiskCache
	mu     sync.Mutex // 保护关闭过程
	closed atomic.Bool
}

// writeDiskStorage 把 reader 的内容（最多 maxBytes）写入已创建的临时文件；超限时删除文件并返回 ErrRequestBodyTooLarge。
func writeDiskStorage(path string, file *os.File, reader io.Reader, maxBytes int64, cache BodyDiskCache) (*diskStorage, error) {
	discard := func(cause error) (*diskStorage, error) {
		_ = file.Close()
		_ = os.Remove(path)
		return nil, cause
	}
	written, err := io.Copy(file, io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return discard(fmt.Errorf("write body cache file: %w", err))
	}
	if written > maxBytes {
		return discard(ErrRequestBodyTooLarge)
	}
	cache.OnStore(written, true)
	return &diskStorage{file: file, path: path, size: written, cache: cache}, nil
}

func (d *diskStorage) Bytes() ([]byte, error) {
	if d.closed.Load() {
		return nil, ErrStorageClosed
	}
	data := make([]byte, d.size)
	if _, err := d.file.ReadAt(data, 0); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("read body cache file: %w", err)
	}
	return data, nil
}

func (d *diskStorage) NewReader() (io.Reader, error) {
	if d.closed.Load() {
		return nil, ErrStorageClosed
	}
	return io.NewSectionReader(d.file, 0, d.size), nil
}

func (d *diskStorage) Size() int64  { return d.size }
func (d *diskStorage) IsDisk() bool { return true }

func (d *diskStorage) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.closed.CompareAndSwap(false, true) {
		return nil
	}
	closeErr := d.file.Close()
	removeErr := os.Remove(d.path)
	d.cache.OnRelease(d.size, true)
	return errors.Join(closeErr, removeErr)
}

// ---------- 创建 ----------

// NewBodyStorage 从 reader 读取请求体（最多 maxBytes 字节）并选择存储方式：
//   - contentLength 已超过上限：不读取，直接返回 ErrRequestBodyTooLarge
//   - 已知长度且需要落盘：边读边写入临时文件，不在内存中保留整份数据
//   - 其余情况读入内存；读完后才知道大小（如分块传输）且需要落盘时，再转存到磁盘
func NewBodyStorage(reader io.Reader, contentLength, maxBytes int64) (BodyStorage, error) {
	if contentLength > maxBytes {
		return nil, ErrRequestBodyTooLarge
	}
	cache := bodyDiskCache()
	if cache != nil && contentLength > 0 && cache.ShouldUseDisk(contentLength) {
		path, file, err := cache.CreateFile()
		if err == nil {
			// 写入过程中出错时 reader 已被部分读取，无法回退到内存，只能报错
			storage, err := writeDiskStorage(path, file, reader, maxBytes, cache)
			if err != nil {
				return nil, err
			}
			return storage, nil
		}
		// 还没读取 reader，可以安全回退到内存
		SysError("failed to create request body cache file, keeping it in memory: " + err.Error())
	}

	data, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return nil, ErrRequestBodyTooLarge
		}
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, ErrRequestBodyTooLarge
	}
	if cache != nil && len(data) > 0 && cache.ShouldUseDisk(int64(len(data))) {
		if storage, err := spillToDisk(data, maxBytes, cache); err == nil {
			return storage, nil
		} else {
			SysError("failed to spill request body to disk, keeping it in memory: " + err.Error())
		}
	}
	return newMemoryStorage(data, cache), nil
}

// spillToDisk 把已读入内存的请求体转存到临时文件。
func spillToDisk(data []byte, maxBytes int64, cache BodyDiskCache) (*diskStorage, error) {
	path, file, err := cache.CreateFile()
	if err != nil {
		return nil, err
	}
	return writeDiskStorage(path, file, bytes.NewReader(data), maxBytes, cache)
}
