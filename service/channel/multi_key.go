package channel

import (
	"fmt"
	"strings"

	"openapi/common"
	"openapi/model"
)

const (
	keyStatusEnabled        = common.ChannelStatusEnabled
	keyStatusManualDisabled = common.ChannelStatusManuallyDisabled
	keyStatusAutoDisabled   = common.ChannelStatusAutoDisabled
)

type MultiKeyManageInput struct {
	ChannelId int    `json:"channel_id"`
	Action    string `json:"action"`
	KeyIndex  *int   `json:"key_index,omitempty"`
	Page      int    `json:"page,omitempty"`
	PageSize  int    `json:"page_size,omitempty"`
	Status    *int   `json:"status,omitempty"`
}

type KeyStatus struct {
	Index        int    `json:"index"`
	Status       int    `json:"status"`
	DisabledTime int64  `json:"disabled_time,omitempty"`
	Reason       string `json:"reason,omitempty"`
	KeyPreview   string `json:"key_preview"`
}

type MultiKeyStatusResponse struct {
	Keys                []KeyStatus `json:"keys"`
	Total               int         `json:"total"`
	Page                int         `json:"page"`
	PageSize            int         `json:"page_size"`
	TotalPages          int         `json:"total_pages"`
	EnabledCount        int         `json:"enabled_count"`
	ManualDisabledCount int         `json:"manual_disabled_count"`
	AutoDisabledCount   int         `json:"auto_disabled_count"`
}

func keyStatusOf(info *model.ChannelInfo, i int) int {
	if s, ok := info.MultiKeyStatusList[i]; ok {
		return s
	}
	return keyStatusEnabled
}

func ensureKeyStatusMaps(info *model.ChannelInfo) {
	if info.MultiKeyStatusList == nil {
		info.MultiKeyStatusList = make(map[int]int)
	}
	if info.MultiKeyDisabledTime == nil {
		info.MultiKeyDisabledTime = make(map[int]int64)
	}
	if info.MultiKeyDisabledReason == nil {
		info.MultiKeyDisabledReason = make(map[int]string)
	}
}

func removeKeys(ch *model.Channel, shouldDelete func(i, status int) bool) int {
	info := &ch.ChannelInfo
	var remaining []string
	statusList := make(map[int]int)
	disabledTime := make(map[int]int64)
	disabledReason := make(map[int]string)
	deleted := 0
	for i, key := range ch.GetKeys() {
		status := keyStatusOf(info, i)
		if shouldDelete(i, status) {
			deleted++
			continue
		}
		idx := len(remaining)
		remaining = append(remaining, key)
		if status != keyStatusEnabled {
			statusList[idx] = status
			if t, ok := info.MultiKeyDisabledTime[i]; ok {
				disabledTime[idx] = t
			}
			if r, ok := info.MultiKeyDisabledReason[i]; ok {
				disabledReason[idx] = r
			}
		}
	}
	if deleted == 0 || len(remaining) == 0 {
		return 0
	}
	ch.Key = strings.Join(remaining, "\n")
	ch.Keys = nil
	info.MultiKeySize = len(remaining)
	info.MultiKeyStatusList = statusList
	info.MultiKeyDisabledTime = disabledTime
	info.MultiKeyDisabledReason = disabledReason
	return deleted
}

func ManageMultiKeys(req MultiKeyManageInput) (message string, data any, err error) {
	ch, err := model.GetChannelById(req.ChannelId, true)
	if err != nil {
		return "", nil, ErrChannelNotFound
	}
	if !ch.ChannelInfo.IsMultiKey {
		return "", nil, validationErr("channel is not in multi-key mode")
	}

	lock := model.GetChannelPollingLock(ch.Id)
	lock.Lock()
	defer lock.Unlock()

	info := &ch.ChannelInfo
	checkIndex := func() (int, error) {
		if req.KeyIndex == nil {
			return 0, validationErr("key_index is required")
		}
		if *req.KeyIndex < 0 || *req.KeyIndex >= info.MultiKeySize {
			return 0, validationErr("key_index out of range")
		}
		return *req.KeyIndex, nil
	}
	save := func(msg string, out any) (string, any, error) {
		if err := ch.Update(); err != nil {
			return "", nil, err
		}
		model.RefreshChannelCache()
		return msg, out, nil
	}

	switch req.Action {
	case "get_key_status":
		return "", buildKeyStatus(ch, req), nil
	case "disable_key":
		idx, err := checkIndex()
		if err != nil {
			return "", nil, err
		}
		ensureKeyStatusMaps(info)
		info.MultiKeyStatusList[idx] = keyStatusManualDisabled
		info.MultiKeyDisabledTime[idx] = common.GetTimestamp()
		return save("key disabled", nil)
	case "enable_key":
		idx, err := checkIndex()
		if err != nil {
			return "", nil, err
		}
		delete(info.MultiKeyStatusList, idx)
		delete(info.MultiKeyDisabledTime, idx)
		delete(info.MultiKeyDisabledReason, idx)
		return save("key enabled", nil)
	case "enable_all_keys":
		count := len(info.MultiKeyStatusList)
		info.MultiKeyStatusList = make(map[int]int)
		info.MultiKeyDisabledTime = make(map[int]int64)
		info.MultiKeyDisabledReason = make(map[int]string)
		return save(fmt.Sprintf("enabled %d keys", count), count)
	case "disable_all_keys":
		ensureKeyStatusMaps(info)
		count := 0
		for i := 0; i < info.MultiKeySize; i++ {
			if keyStatusOf(info, i) == keyStatusEnabled {
				info.MultiKeyStatusList[i] = keyStatusManualDisabled
				info.MultiKeyDisabledTime[i] = common.GetTimestamp()
				count++
			}
		}
		if count == 0 {
			return "", nil, validationErr("no enabled keys to disable")
		}
		return save(fmt.Sprintf("disabled %d keys", count), count)
	case "delete_key":
		idx, err := checkIndex()
		if err != nil {
			return "", nil, err
		}
		if removeKeys(ch, func(i, _ int) bool { return i == idx }) == 0 {
			return "", nil, validationErr("cannot delete the last key")
		}
		return save("key deleted", nil)
	case "delete_disabled_keys":
		count := removeKeys(ch, func(_, status int) bool { return status == keyStatusAutoDisabled })
		if count == 0 {
			return "", nil, validationErr("no auto-disabled keys to delete (or all keys are auto-disabled)")
		}
		return save(fmt.Sprintf("deleted %d auto-disabled keys", count), count)
	default:
		return "", nil, validationErr("unsupported action: " + req.Action)
	}
}

func buildKeyStatus(ch *model.Channel, req MultiKeyManageInput) MultiKeyStatusResponse {
	info := &ch.ChannelInfo
	resp := MultiKeyStatusResponse{Page: max(req.Page, 1), PageSize: req.PageSize}
	if resp.PageSize <= 0 {
		resp.PageSize = 50
	}
	var filtered []KeyStatus
	for i, key := range ch.GetKeys() {
		status := keyStatusOf(info, i)
		switch status {
		case keyStatusEnabled:
			resp.EnabledCount++
		case keyStatusManualDisabled:
			resp.ManualDisabledCount++
		case keyStatusAutoDisabled:
			resp.AutoDisabledCount++
		}
		if req.Status != nil && *req.Status != status {
			continue
		}
		ks := KeyStatus{Index: i, Status: status, KeyPreview: key}
		if len(key) > 10 {
			ks.KeyPreview = key[:10] + "..."
		}
		if status != keyStatusEnabled {
			ks.DisabledTime = info.MultiKeyDisabledTime[i]
			ks.Reason = info.MultiKeyDisabledReason[i]
		}
		filtered = append(filtered, ks)
	}
	resp.Total = len(filtered)
	resp.TotalPages = max((resp.Total+resp.PageSize-1)/resp.PageSize, 1)
	resp.Page = min(resp.Page, resp.TotalPages)
	start := min((resp.Page-1)*resp.PageSize, resp.Total)
	end := min(start+resp.PageSize, resp.Total)
	resp.Keys = filtered[start:end]
	return resp
}
