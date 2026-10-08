package openai

import (
	"fmt"
	"regexp"
	"strings"

	"openapi/dto"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// ApplyResponseRewriteRules 对 body 依次应用所有规则，返回改写后的 JSON。
// 如果某条规则出错则跳过该规则，不中断整体处理。
func ApplyResponseRewriteRules(body []byte, rules []dto.ResponseRewriteRule) []byte {
	if len(rules) == 0 || len(body) == 0 {
		return body
	}
	result := string(body)
	for _, rule := range rules {
		var err error
		result, err = applyRule(result, rule)
		if err != nil {
			continue
		}
	}
	return []byte(result)
}

func applyRule(jsonStr string, rule dto.ResponseRewriteRule) (string, error) {
	if rule.Path == "" || rule.Action == "" {
		return jsonStr, nil
	}

	if rule.Action == "delete" {
		return deleteField(jsonStr, rule.Path)
	}

	if strings.Contains(rule.Path, "#") {
		return applyRuleBatch(jsonStr, rule)
	}

	current := gjson.Get(jsonStr, rule.Path)
	newVal, err := computeNewValue(current.String(), rule)
	if err != nil {
		return jsonStr, err
	}
	return sjson.Set(jsonStr, rule.Path, newVal)
}

// applyRuleBatch 处理包含 # 的批量路径，如 data.#.url
func applyRuleBatch(jsonStr string, rule dto.ResponseRewriteRule) (string, error) {
	hashIdx := strings.Index(rule.Path, "#")
	arrayPath := strings.TrimRight(rule.Path[:hashIdx], ".")
	fieldSuffix := strings.TrimLeft(rule.Path[hashIdx+1:], ".")

	arr := gjson.Get(jsonStr, arrayPath)
	if !arr.IsArray() {
		return jsonStr, nil
	}

	result := jsonStr
	var applyErr error
	arr.ForEach(func(key, value gjson.Result) bool {
		idx := key.Int()
		var itemPath string
		if fieldSuffix != "" {
			itemPath = fmt.Sprintf("%s.%d.%s", arrayPath, idx, fieldSuffix)
		} else {
			itemPath = fmt.Sprintf("%s.%d", arrayPath, idx)
		}
		current := gjson.Get(result, itemPath)
		newVal, err := computeNewValue(current.String(), rule)
		if err != nil {
			applyErr = err
			return false
		}
		result, applyErr = sjson.Set(result, itemPath, newVal)
		return applyErr == nil
	})
	if applyErr != nil {
		return jsonStr, applyErr
	}
	return result, nil
}

func computeNewValue(original string, rule dto.ResponseRewriteRule) (string, error) {
	switch rule.Action {
	case "set":
		return rule.Value, nil
	case "prefix":
		return rule.Value + original, nil
	case "prefix_if_relative":
		if strings.HasPrefix(original, "/") {
			return strings.TrimRight(rule.Value, "/") + original, nil
		}
		return original, nil
	case "suffix":
		return original + rule.Value, nil
	case "replace":
		re, err := regexp.Compile(rule.Value)
		if err != nil {
			return original, fmt.Errorf("invalid regex %q: %w", rule.Value, err)
		}
		return re.ReplaceAllString(original, rule.With), nil
	default:
		return original, fmt.Errorf("unknown action: %s", rule.Action)
	}
}

func deleteField(jsonStr string, path string) (string, error) {
	if strings.Contains(path, "#") {
		hashIdx := strings.Index(path, "#")
		arrayPath := strings.TrimRight(path[:hashIdx], ".")
		fieldSuffix := strings.TrimLeft(path[hashIdx+1:], ".")

		arr := gjson.Get(jsonStr, arrayPath)
		if !arr.IsArray() {
			return jsonStr, nil
		}
		result := jsonStr
		var lastErr error
		arr.ForEach(func(key, _ gjson.Result) bool {
			idx := key.Int()
			var itemPath string
			if fieldSuffix != "" {
				itemPath = fmt.Sprintf("%s.%d.%s", arrayPath, idx, fieldSuffix)
			} else {
				itemPath = fmt.Sprintf("%s.%d", arrayPath, idx)
			}
			var err error
			result, err = sjson.Delete(result, itemPath)
			if err != nil {
				lastErr = err
			}
			return true
		})
		return result, lastErr
	}
	return sjson.Delete(jsonStr, path)
}
