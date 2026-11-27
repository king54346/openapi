package channel

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"openapi/common"
	"openapi/constant"
	"openapi/model"
	httpclient "openapi/service/http"
)

const (
	balanceQueryTimeout     = 30 * time.Second
	maxBalanceResponseBytes = 1 << 20
)

type BalanceQueryUnsupported struct{ ChannelType int }

func (e BalanceQueryUnsupported) Error() string {
	return fmt.Sprintf("balance query is not supported for channel type %s",
		constant.GetChannelTypeName(e.ChannelType))
}

func queryChannelBalance(ch *model.Channel) (float64, error) {
	baseURL := strings.TrimSuffix(ch.GetBaseURL(), "/")
	key, _, apiErr := ch.GetNextEnabledKey()
	if apiErr != nil {
		return 0, fmt.Errorf("no available key: %s", apiErr.Error())
	}
	client, err := httpclient.GetHttpClientWithProxy(ch.GetSetting().Proxy)
	if err != nil {
		return 0, err
	}

	var endpoint string
	switch ch.Type {
	case constant.ChannelTypeOpenRouter:
		if strings.HasSuffix(baseURL, "/v1") {
			endpoint = baseURL + "/credits"
		} else {
			endpoint = baseURL + "/v1/credits"
		}
	case constant.ChannelTypeDeepSeek:
		endpoint = strings.TrimSuffix(baseURL, "/v1") + "/user/balance"
	case constant.ChannelTypeMoonshot:
		if strings.HasSuffix(baseURL, "/v1") {
			endpoint = baseURL + "/users/me/balance"
		} else {
			endpoint = baseURL + "/v1/users/me/balance"
		}
	case constant.ChannelTypeSiliconFlow:
		if strings.HasSuffix(baseURL, "/v1") {
			endpoint = baseURL + "/user/info"
		} else {
			endpoint = baseURL + "/v1/user/info"
		}
	default:
		return 0, BalanceQueryUnsupported{ChannelType: ch.Type}
	}

	body, err := fetchJSON(client, endpoint, strings.TrimSpace(key))
	if err != nil {
		return 0, err
	}

	switch ch.Type {
	case constant.ChannelTypeOpenRouter:
		var resp struct {
			Data struct {
				TotalCredits float64 `json:"total_credits"`
				TotalUsage   float64 `json:"total_usage"`
			} `json:"data"`
		}
		if err := common.Unmarshal(body, &resp); err != nil {
			return 0, err
		}
		return resp.Data.TotalCredits - resp.Data.TotalUsage, nil
	case constant.ChannelTypeDeepSeek:
		var resp struct {
			BalanceInfos []struct {
				Currency     string `json:"currency"`
				TotalBalance string `json:"total_balance"`
			} `json:"balance_infos"`
		}
		if err := common.Unmarshal(body, &resp); err != nil {
			return 0, err
		}
		for _, preferred := range []string{"USD", "CNY", ""} {
			for _, info := range resp.BalanceInfos {
				if preferred == "" || strings.EqualFold(info.Currency, preferred) {
					return parseFloatString(info.TotalBalance)
				}
			}
		}
		return 0, fmt.Errorf("no balance info in upstream response")
	case constant.ChannelTypeSiliconFlow:
		var resp struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
			Data    struct {
				TotalBalance string `json:"totalBalance"`
			} `json:"data"`
		}
		if err := common.Unmarshal(body, &resp); err != nil {
			return 0, err
		}
		if resp.Code != 20000 {
			return 0, fmt.Errorf("code: %d, message: %s", resp.Code, resp.Message)
		}
		return parseFloatString(resp.Data.TotalBalance)
	default:
		var resp struct {
			Data struct {
				AvailableBalance float64 `json:"available_balance"`
			} `json:"data"`
		}
		if err := common.Unmarshal(body, &resp); err != nil {
			return 0, err
		}
		return resp.Data.AvailableBalance, nil
	}
}

func fetchJSON(client *http.Client, endpoint, key string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), balanceQueryTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request upstream: %s", common.MaskSensitiveInfo(err.Error()))
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBalanceResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("read upstream response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream returned status %d: %s", resp.StatusCode, truncateStr(string(body), 200))
	}
	return body, nil
}

func parseFloatString(value string) (float64, error) {
	var result float64
	if _, err := fmt.Sscanf(strings.TrimSpace(value), "%f", &result); err != nil {
		return 0, fmt.Errorf("invalid balance value %q", value)
	}
	return result, nil
}

func UpdateChannelBalance(id int) (float64, error) {
	ch, err := model.GetChannelById(id, true)
	if err != nil {
		return 0, ErrChannelNotFound
	}
	balance, err := queryChannelBalance(ch)
	if err != nil {
		var unsupported BalanceQueryUnsupported
		if errors.As(err, &unsupported) {
			return 0, validationErr(unsupported.Error())
		}
		return 0, validationErr("failed to query balance: " + err.Error())
	}
	ch.UpdateBalance(balance)
	return balance, nil
}

func UpdateAllChannelsBalance() (map[string]any, error) {
	channels, err := enabledChannels()
	if err != nil {
		return nil, err
	}

	var (
		mu                       sync.Mutex
		updated, skipped, failed int
		details                  []map[string]any
	)
	var wg sync.WaitGroup
	sem := make(chan struct{}, channelTestConcurrency)
	for _, ch := range channels {
		wg.Add(1)
		sem <- struct{}{}
		go func(ch *model.Channel) {
			defer func() {
				<-sem
				wg.Done()
			}()
			balance, queryErr := queryChannelBalance(ch)
			mu.Lock()
			defer mu.Unlock()
			detail := map[string]any{"channel_id": ch.Id, "name": ch.Name}
			if queryErr == nil {
				ch.UpdateBalance(balance)
				updated++
				detail["balance"] = balance
			} else {
				var unsupported BalanceQueryUnsupported
				if errors.As(queryErr, &unsupported) {
					skipped++
					detail["skipped"] = unsupported.Error()
				} else {
					failed++
					detail["error"] = queryErr.Error()
				}
			}
			details = append(details, detail)
		}(ch)
	}
	wg.Wait()

	return map[string]any{"updated": updated, "skipped": skipped, "failed": failed, "details": details}, nil
}
