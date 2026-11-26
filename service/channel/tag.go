package channel

import (
	"encoding/json"
	"fmt"
	"strings"

	"openapi/model"
)

type TagInput struct {
	Tag            string  `json:"tag"`
	NewTag         *string `json:"new_tag"`
	ModelMapping   *string `json:"model_mapping"`
	Models         *string `json:"models"`
	Groups         *string `json:"groups"`
	Priority       *int64  `json:"priority"`
	Weight         *uint   `json:"weight"`
	ParamOverride  *string `json:"param_override"`
	HeaderOverride *string `json:"header_override"`
}

func trimJSONField(name string, v *string) (*string, error) {
	if v == nil {
		return nil, nil
	}
	trimmed := strings.TrimSpace(*v)
	if trimmed != "" && !json.Valid([]byte(trimmed)) {
		return nil, validationErr(fmt.Sprintf("%s must be valid JSON", name))
	}
	return &trimmed, nil
}

func DisableTagChannels(tag string) error {
	if err := model.DisableChannelByTag(tag); err != nil {
		return err
	}
	model.RefreshChannelCache()
	return nil
}

func EnableTagChannels(tag string) error {
	if err := model.EnableChannelByTag(tag); err != nil {
		return err
	}
	model.RefreshChannelCache()
	return nil
}

func EditTagChannels(req TagInput) error {
	var err error
	if req.ParamOverride, err = trimJSONField("param_override", req.ParamOverride); err != nil {
		return err
	}
	if req.HeaderOverride, err = trimJSONField("header_override", req.HeaderOverride); err != nil {
		return err
	}
	if err := model.EditChannelByTag(req.Tag, req.NewTag, req.ModelMapping, req.Models,
		req.Priority, req.Weight, req.ParamOverride, req.HeaderOverride); err != nil {
		return err
	}
	model.RefreshChannelCache()
	return nil
}

func BatchSetChannelTag(ids []int, tag *string) error {
	if err := model.BatchSetChannelTag(ids, tag); err != nil {
		return err
	}
	model.RefreshChannelCache()
	return nil
}

func GetTagModels(tag string) (string, error) {
	if tag == "" {
		return "", validationErr("tag is required")
	}
	channels, err := model.GetChannelsByTag(tag, false, false)
	if err != nil {
		return "", err
	}
	longest, maxCount := "", 0
	for _, ch := range channels {
		if n := len(ch.GetModels()); n > maxCount {
			maxCount, longest = n, ch.Models
		}
	}
	return longest, nil
}
