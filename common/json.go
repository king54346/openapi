package common

import (
	"encoding/json"
	"io"

	"github.com/bytedance/sonic"
)

// 全项目统一的 JSON 入口。底层用 sonic（amd64/arm64 上 JIT 加速，其他平台自动回退标准库），
// ConfigStd 与 encoding/json 行为一致：转义 HTML、map 键排序、校验 UTF-8，
// 并支持 json.Marshaler / json.Unmarshaler / json.RawMessage。
var jsonAPI = sonic.ConfigStd

// Unmarshal 解析 JSON。
func Unmarshal(data []byte, v any) error {
	return jsonAPI.Unmarshal(data, v)
}

// UnmarshalJsonStr 从字符串解析 JSON（不额外复制）。
func UnmarshalJsonStr(data string, v any) error {
	return jsonAPI.UnmarshalFromString(data, v)
}

// DecodeJson 从 reader 解析一个 JSON 值。
func DecodeJson(reader io.Reader, v any) error {
	return jsonAPI.NewDecoder(reader).Decode(v)
}

// Marshal 序列化为 JSON。
func Marshal(v any) ([]byte, error) {
	return jsonAPI.Marshal(v)
}

// isJSONSpace JSON 允许的空白字符
func isJSONSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

// firstJSONByte 跳过前导空白后的第一个字节，没有内容时返回 0。
func firstJSONByte(data []byte) byte {
	for _, c := range data {
		if !isJSONSpace(c) {
			return c
		}
	}
	return 0
}

// JSONKind JSON 值的类型。
type JSONKind uint8

const (
	JSONInvalid JSONKind = iota // 空内容或无法识别
	JSONObject
	JSONArray
	JSONString
	JSONBool
	JSONNull
	JSONNumber
)

var jsonKindNames = [...]string{"invalid", "object", "array", "string", "boolean", "null", "number"}

func (k JSONKind) String() string {
	if int(k) < len(jsonKindNames) {
		return jsonKindNames[k]
	}
	return "invalid"
}

// JSONKindOf 按首个非空白字符判断 JSON 值的类型。只看首字符、不校验整体是否合法，
// 用于在解析前决定按哪种结构解析（如字段可能是字符串也可能是对象）。
func JSONKindOf(data []byte) JSONKind {
	switch c := firstJSONByte(data); {
	case c == '{':
		return JSONObject
	case c == '[':
		return JSONArray
	case c == '"':
		return JSONString
	case c == 't' || c == 'f':
		return JSONBool
	case c == 'n':
		return JSONNull
	case c == '-' || (c >= '0' && c <= '9'):
		return JSONNumber
	default:
		return JSONInvalid
	}
}

// DecodeJSONString data 是合法的 JSON 字符串时返回解码后的值和 true，否则返回 false。
func DecodeJSONString(data []byte) (string, bool) {
	if JSONKindOf(data) != JSONString {
		return "", false
	}
	var s string
	if err := Unmarshal(data, &s); err != nil {
		return "", false
	}
	return s, true
}

// trimJSONSpace 去掉首尾 JSON 空白
func trimJSONSpace(data []byte) []byte {
	start, end := 0, len(data)
	for start < end && isJSONSpace(data[start]) {
		start++
	}
	for end > start && isJSONSpace(data[end-1]) {
		end--
	}
	return data[start:end]
}

// JsonRawMessageToString JSON 字符串返回解码后的值，null 或空返回空串，其他值（含非法字符串）原样返回文本。
func JsonRawMessageToString(data json.RawMessage) string {
	switch JSONKindOf(data) {
	case JSONInvalid, JSONNull:
		return ""
	case JSONString:
		if s, ok := DecodeJSONString(data); ok {
			return s
		}
	}
	return string(trimJSONSpace(data))
}
