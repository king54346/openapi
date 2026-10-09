package dto

import (
	"encoding/json"

	"openapi/common"
	"openapi/types"
)

//type OpenAIError struct {
//	Message string `json:"message"`
//	Type    string `json:"type"`
//	Param   string `json:"param"`
//	Code    any    `json:"code"`
//}

type OpenAIErrorWithStatusCode struct {
	Error      types.OpenAIError `json:"error"`
	StatusCode int               `json:"status_code"`
	LocalError bool
}

type GeneralErrorResponse struct {
	Error    json.RawMessage `json:"error"`
	Message  string          `json:"message"`
	Msg      string          `json:"msg"`
	Err      string          `json:"err"`
	ErrorMsg string          `json:"error_msg"`
	Metadata json.RawMessage `json:"metadata,omitempty"`
	Detail   string          `json:"detail,omitempty"`
	Header   struct {
		Message string `json:"message"`
	} `json:"header"`
	Response struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	} `json:"response"`
}

// TryToOpenAIError error 字段是带 message 的 OpenAI 错误对象时返回它，否则返回 nil。
func (e GeneralErrorResponse) TryToOpenAIError() *types.OpenAIError {
	if common.JSONKindOf(e.Error) != common.JSONObject {
		return nil
	}
	var openAIError types.OpenAIError
	if err := common.Unmarshal(e.Error, &openAIError); err != nil || openAIError.Message == "" {
		return nil
	}
	return &openAIError
}

// errorFieldMessage 从 error 字段取错误信息：OpenAI 错误对象取 message，字符串取其值，
// 数字、数组等原样返回文本；null、空值或取不到时返回空串。
func (e GeneralErrorResponse) errorFieldMessage() string {
	switch common.JSONKindOf(e.Error) {
	case common.JSONObject:
		if oaiErr := e.TryToOpenAIError(); oaiErr != nil {
			return oaiErr.Message
		}
		return ""
	case common.JSONString:
		msg, _ := common.DecodeJSONString(e.Error)
		return msg
	case common.JSONInvalid, common.JSONNull:
		return ""
	default:
		return string(e.Error)
	}
}

// ToMessage 按优先级从各家上游的错误格式中取出第一条非空错误信息。
func (e GeneralErrorResponse) ToMessage() string {
	for _, msg := range []string{
		e.errorFieldMessage(),
		e.Message,
		e.Msg,
		e.Err,
		e.ErrorMsg,
		e.Detail,
		e.Header.Message,
		e.Response.Error.Message,
	} {
		if msg != "" {
			return msg
		}
	}
	return ""
}
