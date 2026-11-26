package system

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"openapi/common"
	httpclient "openapi/service/http"
	"openapi/setting"
)

const (
	uptimeKumaTimeout          = 10 * time.Second
	uptimeKumaCacheTTL         = 30 * time.Second
	maxUptimeKumaResponseBytes = 2 << 20
)

var uptimeKumaCache struct {
	mu        sync.Mutex
	body      []byte
	fetchedAt time.Time
}

// GetUptimeKumaStatus 返回 (enabled, rawJSON, err)。未开启时 enabled=false。
func GetUptimeKumaStatus() (bool, []byte, error) {
	settings := setting.GetGlobalSettings()
	if !settings.UptimeKumaStatusEnabled || strings.TrimSpace(settings.UptimeKumaUrl) == "" {
		return false, nil, nil
	}
	body, err := loadUptimeKumaStatus(settings)
	if err != nil {
		return true, nil, err
	}
	return true, body, nil
}

func loadUptimeKumaStatus(settings *setting.GlobalSettings) ([]byte, error) {
	uptimeKumaCache.mu.Lock()
	defer uptimeKumaCache.mu.Unlock()
	if uptimeKumaCache.body != nil && time.Since(uptimeKumaCache.fetchedAt) < uptimeKumaCacheTTL {
		return uptimeKumaCache.body, nil
	}

	baseURL := strings.TrimSuffix(strings.TrimSpace(settings.UptimeKumaUrl), "/")
	slug := strings.TrimSpace(settings.UptimeKumaSlug)
	endpoint := baseURL + "/api/status-page"
	if slug != "" {
		endpoint += "/" + slug
	}

	ctx, cancel := context.WithTimeout(context.Background(), uptimeKumaTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := httpclient.GetHttpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("request uptime kuma: %s", common.MaskSensitiveInfo(err.Error()))
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxUptimeKumaResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("read uptime kuma response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("uptime kuma returned status %d", resp.StatusCode)
	}

	uptimeKumaCache.body = body
	uptimeKumaCache.fetchedAt = time.Now()
	return body, nil
}
