package channel

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"openapi/common"
	"openapi/constant"
	"openapi/model"
	httpclient "openapi/service/http"
)

var (
	ErrValidation      = errors.New("validation error")
	ErrChannelNotFound = errors.New("channel not found")
)

func validationErr(msg string) error {
	return fmt.Errorf("%w: %s", ErrValidation, msg)
}

func ParseStatusFilter(statusParam string) model.ChannelStatusFilter {
	switch strings.ToLower(statusParam) {
	case "enabled", "1":
		return model.ChannelStatusOnlyEnabled
	case "disabled", "0":
		return model.ChannelStatusOnlyDisabled
	default:
		return model.ChannelStatusAny
	}
}

func ClearChannelInfo(ch *model.Channel) {
	if ch == nil {
		return
	}
	ch.Key = ""
	if ch.ChannelInfo.IsMultiKey {
		ch.ChannelInfo.MultiKeyDisabledReason = nil
		ch.ChannelInfo.MultiKeyDisabledTime = nil
	}
}

func ListChannels(filter model.ChannelFilter, tagMode bool, startIdx, pageSize int) ([]*model.Channel, int64, map[int64]int64, error) {
	var channels []*model.Channel
	var total int64
	var err error
	if tagMode {
		var tags []string
		if tags, total, err = model.ListChannelTags(filter, startIdx, pageSize); err == nil {
			channels, err = model.ListChannelsByTags(filter, tags)
		}
	} else {
		channels, total, err = model.ListChannels(filter, startIdx, pageSize)
	}
	if err != nil {
		return nil, 0, nil, err
	}
	typeCounts, err := model.CountChannelsByType(filter)
	if err != nil {
		return nil, 0, nil, err
	}
	for _, ch := range channels {
		ClearChannelInfo(ch)
	}
	return channels, total, typeCounts, nil
}

func GetChannel(id int) (*model.Channel, error) {
	ch, err := model.GetChannelById(id, false)
	if err != nil {
		return nil, ErrChannelNotFound
	}
	ClearChannelInfo(ch)
	return ch, nil
}

func GetChannelKey(actorID, id int) (string, error) {
	ch, err := model.GetChannelById(id, true)
	if err != nil {
		return "", ErrChannelNotFound
	}
	model.RecordLog(actorID, model.LogTypeSystem, fmt.Sprintf("viewed key of channel #%d", id))
	return ch.Key, nil
}

func validateChannel(ch *model.Channel, isAdd bool) error {
	if err := ch.ValidateSettings(); err != nil {
		return validationErr(fmt.Sprintf("invalid channel setting: %v", err))
	}
	if isAdd {
		if strings.TrimSpace(ch.Name) == "" {
			return validationErr("channel name is required")
		}
		if strings.TrimSpace(ch.Key) == "" {
			return validationErr("channel key is required")
		}
		if !constant.IsRelaySupportedChannelType(ch.Type) {
			return validationErr(fmt.Sprintf("channel type %d is not supported", ch.Type))
		}
		if len(ch.GetModels()) == 0 {
			return validationErr("channel models are required")
		}
	}
	for _, m := range ch.GetModels() {
		if len(m) > 255 {
			return validationErr("model name too long: " + m)
		}
	}
	for name, v := range map[string]*string{"param_override": ch.ParamOverride, "header_override": ch.HeaderOverride} {
		if v != nil && strings.TrimSpace(*v) != "" && !json.Valid([]byte(*v)) {
			return validationErr(name + " must be valid JSON")
		}
	}
	return nil
}

func splitKeys(raw string) []string {
	var keys []string
	for _, key := range strings.Split(raw, "\n") {
		if key = strings.TrimSpace(key); key != "" {
			keys = append(keys, key)
		}
	}
	return keys
}

type AddChannelInput struct {
	Mode                      string                `json:"mode"`
	MultiKeyMode              constant.MultiKeyMode `json:"multi_key_mode"`
	BatchAddSetKeyPrefix2Name bool                  `json:"batch_add_set_key_prefix_2_name"`
	Channel                   *model.Channel        `json:"channel"`
}

func AddChannels(in AddChannelInput) ([]int, error) {
	if in.Channel == nil {
		return nil, validationErr("invalid request")
	}
	base := *in.Channel
	base.Id = 0
	base.CreatedTime = common.GetTimestamp()
	if base.Status == 0 {
		base.Status = common.ChannelStatusEnabled
	}
	if err := validateChannel(&base, true); err != nil {
		return nil, err
	}

	var keys []string
	switch in.Mode {
	case "", "single":
		keys = []string{base.Key}
	case "batch":
		keys = splitKeys(base.Key)
	case "multi_to_single":
		cleanKeys := splitKeys(base.Key)
		if len(cleanKeys) == 0 {
			return nil, validationErr("channel key is required")
		}
		mode := in.MultiKeyMode
		if mode == "" {
			mode = constant.MultiKeyModeRandom
		}
		if mode != constant.MultiKeyModeRandom && mode != constant.MultiKeyModePolling {
			return nil, validationErr("multi_key_mode must be random or polling")
		}
		base.ChannelInfo.IsMultiKey = true
		base.ChannelInfo.MultiKeyMode = mode
		base.ChannelInfo.MultiKeySize = len(cleanKeys)
		base.Key = strings.Join(cleanKeys, "\n")
		keys = []string{base.Key}
	default:
		return nil, validationErr("unsupported mode: " + in.Mode)
	}
	if len(keys) == 0 {
		return nil, validationErr("channel key is required")
	}

	channels := make([]model.Channel, 0, len(keys))
	for _, key := range keys {
		ch := base
		ch.Key = key
		if in.BatchAddSetKeyPrefix2Name && len(keys) > 1 {
			ch.Name = fmt.Sprintf("%s %s", base.Name, key[:min(8, len(key))])
		}
		channels = append(channels, ch)
	}
	if err := model.BatchInsertChannels(channels); err != nil {
		return nil, err
	}
	httpclient.ResetProxyClientCache()
	model.RefreshChannelCache()
	ids := make([]int, 0, len(channels))
	for _, ch := range channels {
		ids = append(ids, ch.Id)
	}
	return ids, nil
}

type PatchChannelInput struct {
	model.Channel
	MultiKeyMode *string `json:"multi_key_mode"`
	KeyMode      *string `json:"key_mode"`
}

func UpdateChannel(req PatchChannelInput) (*model.Channel, error) {
	if req.Id <= 0 {
		return nil, validationErr("invalid id")
	}
	origin, err := model.GetChannelById(req.Id, true)
	if err != nil {
		return nil, ErrChannelNotFound
	}
	if req.Type != 0 && req.Type != origin.Type && !constant.IsRelaySupportedChannelType(req.Type) {
		return nil, validationErr(fmt.Sprintf("channel type %d is not supported", req.Type))
	}
	if err := validateChannel(&req.Channel, false); err != nil {
		return nil, err
	}

	req.ChannelInfo = origin.ChannelInfo
	if req.MultiKeyMode != nil && *req.MultiKeyMode != "" {
		mode := constant.MultiKeyMode(*req.MultiKeyMode)
		if mode != constant.MultiKeyModeRandom && mode != constant.MultiKeyModePolling {
			return nil, validationErr("multi_key_mode must be random or polling")
		}
		req.ChannelInfo.MultiKeyMode = mode
	}
	if req.KeyMode != nil && *req.KeyMode == "append" && req.ChannelInfo.IsMultiKey && req.Key != "" {
		existing := origin.GetKeys()
		seen := make(map[string]struct{}, len(existing))
		for _, k := range existing {
			seen[strings.TrimSpace(k)] = struct{}{}
		}
		for _, k := range splitKeys(req.Key) {
			if _, dup := seen[k]; !dup {
				seen[k] = struct{}{}
				existing = append(existing, k)
			}
		}
		req.Key = strings.Join(existing, "\n")
	}

	ch := req.Channel
	if err := ch.Update(); err != nil {
		return nil, err
	}
	httpclient.ResetProxyClientCache()
	model.RefreshChannelCache()
	ClearChannelInfo(&ch)
	return &ch, nil
}

func DeleteChannel(id int) error {
	if err := (&model.Channel{Id: id}).Delete(); err != nil {
		return err
	}
	model.RefreshChannelCache()
	return nil
}

func DeleteChannelBatch(ids []int) error {
	if err := model.BatchDeleteChannels(ids); err != nil {
		return err
	}
	model.RefreshChannelCache()
	return nil
}

func CopyChannel(id int, suffix string, resetBalance bool) (int, error) {
	origin, err := model.GetChannelById(id, true)
	if err != nil {
		return 0, ErrChannelNotFound
	}
	clone := *origin
	clone.Id = 0
	clone.Name += suffix
	clone.CreatedTime = common.GetTimestamp()
	clone.TestTime = 0
	clone.ResponseTime = 0
	if resetBalance {
		clone.Balance = 0
		clone.UsedQuota = 0
	}
	if err := clone.Insert(); err != nil {
		return 0, err
	}
	model.RefreshChannelCache()
	return clone.Id, nil
}

func DeleteDisabledChannels() (int64, error) {
	rows, err := model.DeleteDisabledChannel()
	if err != nil {
		return 0, err
	}
	model.RefreshChannelCache()
	return rows, nil
}

func UnwrapBusinessErr(err error) (string, bool) {
	if errors.Is(err, ErrValidation) {
		return strings.TrimPrefix(err.Error(), ErrValidation.Error()+": "), true
	}
	if errors.Is(err, ErrChannelNotFound) {
		return ErrChannelNotFound.Error(), true
	}
	return "", false
}
