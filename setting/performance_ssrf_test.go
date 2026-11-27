package setting_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"openapi/common"
	"openapi/common/system"
	"openapi/middleware"
	"openapi/service/files"
	perfsvc "openapi/service/performance"
	"openapi/setting"

	gin "github.com/king54346/gin-tiny"
)

func TestSSRFProtectionAndApplyOptions(t *testing.T) {
	origSSRF := common.GetSSRFConfig()
	t.Cleanup(func() {
		common.SetSSRFConfig(origSSRF)
	})

	errs := setting.ApplyOptions(map[string]string{
		"fetch_setting.enable_ssrf_protection":     "true",
		"fetch_setting.allow_private_ip":           "false",
		"fetch_setting.apply_ip_filter_for_domain": "false",
		"fetch_setting.domain_filter_mode":         "false",
		"fetch_setting.ip_filter_mode":             "false",
		"fetch_setting.domain_list":                `["blocked.example.com","*.evil.org"]`,
		"fetch_setting.ip_list":                    `["8.8.4.0/24"]`,
		"fetch_setting.allowed_ports":              `["80","443","8000-8010"]`,
	})
	if len(errs) > 0 {
		t.Fatalf("unexpected ApplyOptions errors: %v", errs)
	}

	// 1. 私有 IP 与云元数据地址拦截
	for _, target := range []string{
		"http://127.0.0.1/secret",
		"http://192.168.1.10:80/api",
		"http://169.254.169.254/latest/meta-data/",
		"http://[::1]:80/test",
	} {
		if err := common.ValidateOutboundURL(target); err == nil {
			t.Fatalf("expected private IP URL %s to be blocked by SSRF", target)
		}
	}

	// 2. 非 HTTP/HTTPS 协议拦截
	if err := common.ValidateOutboundURL("file:///etc/passwd"); err == nil {
		t.Fatal("expected file:// protocol to be rejected")
	}

	// 3. 端口与端口范围校验
	if err := common.ValidateOutboundURL("https://api.openai.com:8005/v1/models"); err != nil {
		t.Fatalf("expected port 8005 in range 8000-8010 to be allowed, got: %v", err)
	}
	if err := common.ValidateOutboundURL("https://api.openai.com:6379/v1/models"); err == nil {
		t.Fatal("expected port 6379 outside allowed ports to be blocked")
	}

	// 4. 域名通配符黑名单 + CIDR IP 黑名单校验
	if err := common.ValidateOutboundURL("https://sub.evil.org/test"); err == nil {
		t.Fatal("expected wildcard blacklisted domain sub.evil.org to be blocked")
	}
	if err := common.ValidateOutboundURL("http://8.8.4.4/dns"); err == nil {
		t.Fatal("expected CIDR blacklisted IP 8.8.4.4 to be blocked")
	}
	if err := common.ValidateOutboundURL("http://1.1.1.1/dns"); err != nil {
		t.Fatalf("expected public IP 1.1.1.1 on port 80 to be allowed, got: %v", err)
	}

	// 5. 图片下载入口联动 SSRF 拦截
	if _, _, err := files.GetImageFromUrl("http://127.0.0.1:80/a.png"); err == nil || !strings.Contains(err.Error(), "SSRF") {
		t.Fatalf("expected GetImageFromUrl to block loopback URL via SSRF, got: %v", err)
	}

	// 6. 开启域名白名单模式与允许内网 IP
	setting.ApplyOptions(map[string]string{
		"fetch_setting.allow_private_ip":   "true",
		"fetch_setting.domain_filter_mode": "true",
		"fetch_setting.domain_list":        `["*.trusted.internal"]`,
	})
	if err := common.ValidateOutboundURL("http://127.0.0.1:80/local"); err != nil {
		t.Fatalf("expected loopback to be allowed when allow_private_ip=true, got: %v", err)
	}
	if err := common.ValidateOutboundURL("https://api.trusted.internal/v1"); err != nil {
		t.Fatalf("expected whitelisted wildcard domain to pass, got: %v", err)
	}
	if err := common.ValidateOutboundURL("https://untrusted.com/v1"); err == nil {
		t.Fatal("expected non-whitelisted domain to be blocked in whitelist mode")
	}
}

func TestAdmissionSmoothBackpressureAndMiddleware(t *testing.T) {
	origAdm := system.GetAdmissionConfig()
	origDisk := system.GetDiskCacheConfig()
	origMon := common.GetPerformanceMonitorConfig()
	t.Cleanup(func() {
		system.SetAdmissionConfig(origAdm)
		system.SetDiskCacheConfig(origDisk)
		common.SetPerformanceMonitorConfig(origMon)
		system.ResetAdmissionStats()
	})

	errs := setting.ApplyOptions(map[string]string{
		"performance_setting.disk_cache_enabled":           "true",
		"performance_setting.disk_cache_threshold_mb":      "16",
		"performance_setting.disk_cache_max_size_mb":       "2048",
		"performance_setting.monitor_enabled":              "false",
		"performance_setting.monitor_cpu_threshold":        "85",
		"performance_setting.admission_enabled":            "true",
		"performance_setting.admission_soft_limit":         "1",
		"performance_setting.admission_hard_limit":         "2",
		"performance_setting.admission_max_queue_size":     "1",
		"performance_setting.admission_max_wait_ms":        "120",
		"performance_setting.admission_priority_whitelist": "svip,vip_token",
	})
	if len(errs) > 0 {
		t.Fatalf("unexpected ApplyOptions errors: %v", errs)
	}
	system.ResetAdmissionStats()

	statsSnap := perfsvc.GetStats()
	if !statsSnap.Config.DiskCacheEnabled || statsSnap.Config.DiskCacheThresholdMB != 16 {
		t.Fatalf("expected disk cache config hot-updated to 16MB, got %+v", statsSnap.Config)
	}

	// 槽位 1：inFlight=0 < SoftLimit(1)，普通请求直接放行
	rel1, err := system.AcquireAdmission(context.Background(), false)
	if err != nil {
		t.Fatalf("slot 1 expected immediate pass, got: %v", err)
	}
	defer rel1()

	// 槽位 2：inFlight=1 == SoftLimit(1) < HardLimit(2)，高优先请求直接放行！
	if !system.MatchAdmissionPriority("normal_user", "vip_token") {
		t.Fatal("expected vip_token to match priority whitelist")
	}
	rel2, err := system.AcquireAdmission(context.Background(), true)
	if err != nil {
		t.Fatalf("slot 2 priority request expected immediate pass below hard limit, got: %v", err)
	}

	// 此时 inFlight=2 == HardLimit(2)：普通请求立即快速拒绝（不排队）
	if _, err := system.AcquireAdmission(context.Background(), false); !errors.Is(err, system.ErrAdmissionOverHardLimit) {
		t.Fatalf("expected normal request at hard limit to fast-reject with ErrAdmissionOverHardLimit, got: %v", err)
	}

	// 此时 inFlight=2 == HardLimit(2)：高优先请求进入等待队列（MaxQueueSize=1）
	queuedDone := make(chan error, 1)
	var relQueued func()
	go func() {
		var qErr error
		relQueued, qErr = system.AcquireAdmission(context.Background(), true)
		queuedDone <- qErr
	}()

	// 等待高优先请求进入队列
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if system.GetAdmissionStats().Queued == 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if system.GetAdmissionStats().Queued != 1 {
		t.Fatalf("expected 1 queued priority request, got %+v", system.GetAdmissionStats())
	}

	// 队列已满（MaxQueueSize=1）：第二个高优先请求立即返回队列已满
	if _, err := system.AcquireAdmission(context.Background(), true); !errors.Is(err, system.ErrAdmissionQueueFull) {
		t.Fatalf("expected ErrAdmissionQueueFull when queue is full, got: %v", err)
	}

	// 释放槽位 2 -> 队列中的高优先请求立即被唤醒放行
	rel2()
	if waitErr := <-queuedDone; waitErr != nil {
		t.Fatalf("expected queued priority request to wake up and succeed, got: %v", waitErr)
	}
	defer relQueued()

	// 测试排队超时（当前 inFlight=2，释放 rel1 使 inFlight=1 == SoftLimit，普通请求入队并在 120ms 后超时）
	rel1()
	_, err = system.AcquireAdmission(context.Background(), false)
	if !errors.Is(err, system.ErrAdmissionQueueTimeout) {
		t.Fatalf("expected normal request to time out in queue, got: %v", err)
	}

	admStats := system.GetAdmissionStats()
	if admStats.TotalAdmitted != 3 || admStats.TotalQueued != 2 || admStats.TotalTimeout != 1 || admStats.TotalRejected < 3 {
		t.Fatalf("unexpected admission stats counters: %+v", admStats)
	}

	// 验证 Gin 中间件对 Claude /v1/messages 超载 503 响应格式（先占满硬水位 2）
	relExtra, err := system.AcquireAdmission(context.Background(), true)
	if err != nil {
		t.Fatalf("acquire second slot for middleware test: %v", err)
	}
	defer relExtra()

	engine := gin.New()
	engine.POST("/v1/messages", func(c gin.Context) {
		c.Set("username", "regular_bob")
		c.Next()
	}, middleware.AdmissionControl(), func(c gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "overloaded_error") {
		t.Fatalf("expected 503 overloaded_error on /v1/messages, got code=%d body=%s", rec.Code, rec.Body.String())
	}
}
