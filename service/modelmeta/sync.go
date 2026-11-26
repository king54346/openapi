package modelmeta

import (
	"sort"
	"strings"

	"openapi/common"
	"openapi/model"
)

func missingModels(candidate []string) ([]string, error) {
	if len(candidate) == 0 {
		return []string{}, nil
	}
	var existing []string
	if err := model.DB.Model(&model.Model{}).Where("model_name IN ?", candidate).Pluck("model_name", &existing).Error; err != nil {
		return nil, err
	}
	known := make(map[string]bool, len(existing))
	for _, name := range existing {
		known[name] = true
	}
	missing := make([]string, 0, len(candidate))
	for _, name := range candidate {
		if name != "" && !known[name] {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	return missing, nil
}

func GetMissingModels() ([]string, error) {
	return missingModels(model.GetEnabledModels())
}

func channelsForSync(channelIds []int) ([]*model.Channel, error) {
	if len(channelIds) > 0 {
		return model.GetChannelsByIds(channelIds)
	}
	var channels []*model.Channel
	err := model.DB.Where("status = ?", common.ChannelStatusEnabled).Order("id").Find(&channels).Error
	return channels, err
}

func collectProvidedModels(channels []*model.Channel) []string {
	seen := make(map[string]bool)
	provided := make([]string, 0)
	for _, ch := range channels {
		for _, modelName := range ch.GetModels() {
			if modelName = strings.TrimSpace(modelName); modelName != "" && !seen[modelName] {
				seen[modelName] = true
				provided = append(provided, modelName)
			}
		}
	}
	sort.Strings(provided)
	return provided
}

func SyncUpstreamPreview(channelIds []int) (createList []string, updateList []string, total int, err error) {
	channels, err := channelsForSync(channelIds)
	if err != nil {
		return nil, nil, 0, err
	}
	provided := collectProvidedModels(channels)
	missing, err := missingModels(provided)
	if err != nil {
		return nil, nil, 0, err
	}
	missingSet := make(map[string]bool, len(missing))
	for _, name := range missing {
		missingSet[name] = true
	}
	existing := make([]string, 0, len(provided)-len(missing))
	for _, name := range provided {
		if !missingSet[name] {
			existing = append(existing, name)
		}
	}
	return missing, existing, len(provided), nil
}

func SyncUpstreamModels(channelIds []int, dryRun bool) (map[string]any, error) {
	channels, err := channelsForSync(channelIds)
	if err != nil {
		return nil, err
	}
	provided := collectProvidedModels(channels)
	missing, err := missingModels(provided)
	if err != nil {
		return nil, err
	}
	if dryRun {
		return map[string]any{"created": 0, "skipped": len(missing), "dry_run": true, "models": missing}, nil
	}
	created := 0
	for _, modelName := range missing {
		meta := &model.Model{ModelName: modelName, Status: 1, SyncOfficial: 1}
		if err := meta.Insert(); err != nil {
			common.SysError("failed to create model metadata " + modelName + ": " + err.Error())
			continue
		}
		created++
	}
	return map[string]any{"created": created, "skipped": len(provided) - created}, nil
}
