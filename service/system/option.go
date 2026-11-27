package system

import (
	"strings"

	"openapi/model"
	"openapi/setting"
)

var legacyConsoleOptions = map[string]string{
	"console_setting.api_info":       "ApiInfo",
	"console_setting.faq":            "FAQ",
	"console_setting.announcements":  "Announcements",
	"console_setting.allow_redirect": "AllowRedirect",
}

func GetMaskedOptions() (map[string]string, error) {
	options, err := model.ListOptionsFromDB()
	if err != nil {
		return nil, err
	}
	values := make(map[string]string, len(options))
	for _, option := range options {
		values[option.Key] = model.MaskOptionValue(option.Key, option.Value)
	}
	return values, nil
}

func UpdateOption(key, value string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return validationErr("key is required")
	}
	if model.IsSensitiveOptionKey(key) && isMaskedOptionValue(value) {
		return nil
	}
	return model.UpdateOptionValue(key, value)
}

func isMaskedOptionValue(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return false
	}
	for _, r := range trimmed {
		if r != '*' {
			return false
		}
	}
	return true
}

func MigrateConsoleSettings(keys []string) (migrated []string, skipped []string, err error) {
	candidates := make([]string, 0, len(legacyConsoleOptions))
	if len(keys) == 0 {
		for legacy := range legacyConsoleOptions {
			candidates = append(candidates, legacy)
		}
	} else {
		candidates = append(candidates, keys...)
	}

	values := model.GetAllOptionValues()
	migrated = make([]string, 0, len(candidates))
	skipped = make([]string, 0, len(candidates))
	for _, legacy := range candidates {
		legacy = strings.TrimSpace(legacy)
		target, known := legacyConsoleOptions[legacy]
		if !known {
			skipped = append(skipped, legacy)
			continue
		}
		value, exists := values[legacy]
		if !exists {
			skipped = append(skipped, legacy)
			continue
		}
		if _, targetExists := values[target]; targetExists {
			skipped = append(skipped, legacy)
			continue
		}
		if err := model.UpdateOptionValue(target, value); err != nil {
			return nil, nil, err
		}
		migrated = append(migrated, legacy+" -> "+target)
	}
	return migrated, skipped, nil
}

func GetNotice() string          { return setting.GetGlobalSettings().Notice }
func GetAbout() string           { return setting.GetGlobalSettings().About }
func GetHomePageContent() string { return setting.GetGlobalSettings().HomePageContent }
func GetUserAgreement() string   { return setting.GetGlobalSettings().UserAgreement }
func GetPrivacyPolicy() string   { return setting.GetGlobalSettings().PrivacyPolicy }
