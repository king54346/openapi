package system

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"
)

var (
	ErrAdmissionOverHardLimit = errors.New("server busy: active requests exceeded hard watermark")
	ErrAdmissionQueueFull     = errors.New("server busy: admission wait queue is full")
	ErrAdmissionQueueTimeout  = errors.New("server busy: admission wait queue timed out")
)

// AdmissionConfig 准入控制与平滑背压配置。
type AdmissionConfig struct {
	Enabled           bool   `json:"enabled"`
	SoftLimit         int    `json:"soft_limit"`
	HardLimit         int    `json:"hard_limit"`
	MaxQueueSize      int    `json:"max_queue_size"`
	MaxWaitMs         int    `json:"max_wait_ms"`
	PriorityWhitelist string `json:"priority_whitelist"`
}

// AdmissionStats 准入控制实时运行指标。
type AdmissionStats struct {
	Enabled           bool   `json:"enabled"`
	InFlight          int64  `json:"in_flight"`
	SoftLimit         int    `json:"soft_limit"`
	HardLimit         int    `json:"hard_limit"`
	MaxQueueSize      int    `json:"max_queue_size"`
	MaxWaitMs         int    `json:"max_wait_ms"`
	PriorityWhitelist string `json:"priority_whitelist"`
	Queued            int64  `json:"queued"`
	TotalAdmitted     uint64 `json:"total_admitted"`
	TotalQueued       uint64 `json:"total_queued"`
	TotalRejected     uint64 `json:"total_rejected"`
	TotalTimeout      uint64 `json:"total_timeout"`
}

var admissionState = struct {
	mu            sync.Mutex
	cfg           AdmissionConfig
	notifyCh      chan struct{}
	inFlight      int64
	queued        int64
	totalAdmitted uint64
	totalQueued   uint64
	totalRejected uint64
	totalTimeout  uint64
}{
	cfg: AdmissionConfig{
		Enabled:           false,
		SoftLimit:         64,
		HardLimit:         128,
		MaxQueueSize:      256,
		MaxWaitMs:         30000,
		PriorityWhitelist: "",
	},
	notifyCh: make(chan struct{}),
}

func normalizeAdmissionConfig(cfg AdmissionConfig) AdmissionConfig {
	if cfg.SoftLimit <= 0 {
		cfg.SoftLimit = 64
	}
	if cfg.HardLimit < cfg.SoftLimit {
		cfg.HardLimit = cfg.SoftLimit
	}
	if cfg.MaxQueueSize < 0 {
		cfg.MaxQueueSize = 0
	}
	if cfg.MaxWaitMs <= 0 {
		cfg.MaxWaitMs = 30000
	}
	return cfg
}

func GetAdmissionConfig() AdmissionConfig {
	admissionState.mu.Lock()
	defer admissionState.mu.Unlock()
	return admissionState.cfg
}

func SetAdmissionConfig(cfg AdmissionConfig) {
	cfg = normalizeAdmissionConfig(cfg)
	admissionState.mu.Lock()
	admissionState.cfg = cfg
	close(admissionState.notifyCh)
	admissionState.notifyCh = make(chan struct{})
	admissionState.mu.Unlock()
}

func GetAdmissionStats() AdmissionStats {
	admissionState.mu.Lock()
	defer admissionState.mu.Unlock()
	return AdmissionStats{
		Enabled:           admissionState.cfg.Enabled,
		InFlight:          admissionState.inFlight,
		SoftLimit:         admissionState.cfg.SoftLimit,
		HardLimit:         admissionState.cfg.HardLimit,
		MaxQueueSize:      admissionState.cfg.MaxQueueSize,
		MaxWaitMs:         admissionState.cfg.MaxWaitMs,
		PriorityWhitelist: admissionState.cfg.PriorityWhitelist,
		Queued:            admissionState.queued,
		TotalAdmitted:     admissionState.totalAdmitted,
		TotalQueued:       admissionState.totalQueued,
		TotalRejected:     admissionState.totalRejected,
		TotalTimeout:      admissionState.totalTimeout,
	}
}

func ResetAdmissionStats() {
	admissionState.mu.Lock()
	admissionState.totalAdmitted = 0
	admissionState.totalQueued = 0
	admissionState.totalRejected = 0
	admissionState.totalTimeout = 0
	admissionState.mu.Unlock()
}

// MatchAdmissionPriority 判断给定标识（用户名、令牌名等）是否命中高优先白名单。
func MatchAdmissionPriority(candidates ...string) bool {
	admissionState.mu.Lock()
	raw := admissionState.cfg.PriorityWhitelist
	admissionState.mu.Unlock()

	if strings.TrimSpace(raw) == "" {
		return false
	}
	allow := make(map[string]struct{})
	for _, part := range strings.Split(raw, ",") {
		if k := strings.ToLower(strings.TrimSpace(part)); k != "" {
			allow[k] = struct{}{}
		}
	}
	if len(allow) == 0 {
		return false
	}
	for _, c := range candidates {
		if k := strings.ToLower(strings.TrimSpace(c)); k != "" {
			if _, ok := allow[k]; ok {
				return true
			}
		}
	}
	return false
}

// AcquireAdmission 按软/硬水位与平滑背压规则获取执行槽位：
//   - inFlight < SoftLimit：所有请求直接放行
//   - SoftLimit <= inFlight < HardLimit：高优先直接放行，普通请求进入排队
//   - inFlight >= HardLimit：普通请求立即快速拒绝，高优先请求进入排队等待槽位释放
func AcquireAdmission(ctx context.Context, isPriority bool) (release func(), err error) {
	var once sync.Once
	makeRelease := func() func() {
		return func() {
			once.Do(func() {
				admissionState.mu.Lock()
				if admissionState.inFlight > 0 {
					admissionState.inFlight--
				}
				close(admissionState.notifyCh)
				admissionState.notifyCh = make(chan struct{})
				admissionState.mu.Unlock()
			})
		}
	}

	enteredQueue := false
	var timer *time.Timer
	var timeoutCh <-chan time.Time
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()

	admissionState.mu.Lock()
	for {
		cfg := admissionState.cfg
		if !cfg.Enabled {
			if enteredQueue && admissionState.queued > 0 {
				admissionState.queued--
			}
			admissionState.inFlight++
			admissionState.totalAdmitted++
			admissionState.mu.Unlock()
			return makeRelease(), nil
		}

		limit := cfg.SoftLimit
		if isPriority {
			limit = cfg.HardLimit
		}
		if admissionState.inFlight < int64(limit) {
			if enteredQueue && admissionState.queued > 0 {
				admissionState.queued--
			}
			admissionState.inFlight++
			admissionState.totalAdmitted++
			admissionState.mu.Unlock()
			return makeRelease(), nil
		}

		// 普通请求达到硬水位且尚未入队时，直接快速拒绝
		if !isPriority && !enteredQueue && admissionState.inFlight >= int64(cfg.HardLimit) {
			admissionState.totalRejected++
			admissionState.mu.Unlock()
			return nil, ErrAdmissionOverHardLimit
		}

		if !enteredQueue {
			if cfg.MaxQueueSize > 0 && admissionState.queued >= int64(cfg.MaxQueueSize) {
				admissionState.totalRejected++
				admissionState.mu.Unlock()
				return nil, ErrAdmissionQueueFull
			}
			enteredQueue = true
			admissionState.queued++
			admissionState.totalQueued++
			timer = time.NewTimer(time.Duration(cfg.MaxWaitMs) * time.Millisecond)
			timeoutCh = timer.C
		}

		waitCh := admissionState.notifyCh
		admissionState.mu.Unlock()

		select {
		case <-waitCh:
			admissionState.mu.Lock()
		case <-timeoutCh:
			admissionState.mu.Lock()
			if admissionState.queued > 0 {
				admissionState.queued--
			}
			admissionState.totalTimeout++
			admissionState.totalRejected++
			admissionState.mu.Unlock()
			return nil, ErrAdmissionQueueTimeout
		case <-ctx.Done():
			admissionState.mu.Lock()
			if admissionState.queued > 0 {
				admissionState.queued--
			}
			admissionState.mu.Unlock()
			return nil, ctx.Err()
		}
	}
}
