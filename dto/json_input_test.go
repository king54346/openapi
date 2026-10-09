package dto

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestResponsesParseInput(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  []MediaInput
	}{
		{"nil", ``, nil},
		{"plain string", `"hello"`, []MediaInput{{Type: "input_text", Text: "hello"}}},
		{"string with spaces", `  "hi"  `, []MediaInput{{Type: "input_text", Text: "hi"}}},
		{"message with string content", `[{"role":"user","content":"hi"}]`, []MediaInput{{Type: "input_text", Text: "hi"}}},
		{"content parts", `[{"role":"user","content":[
			{"type":"input_text","text":"look"},
			{"type":"input_image","image_url":"https://a/1.png"},
			{"type":"input_image","image_url":{"url":"https://a/2.png"}},
			{"type":"input_file","file_url":"https://a/f.pdf"},
			{"type":"input_file","file_url":{"url":"https://a/g.pdf"}},
			{"type":"unknown","text":"skip"}
		]}]`, []MediaInput{
			{Type: "input_text", Text: "look"},
			{Type: "input_image", ImageUrl: "https://a/1.png"},
			{Type: "input_image", ImageUrl: "https://a/2.png"},
			{Type: "input_file", FileUrl: "https://a/f.pdf"},
			{Type: "input_file", FileUrl: "https://a/g.pdf"},
		}},
		{"multiple messages", `[{"content":"a"},{"content":[{"type":"input_text","text":"b"}]}]`,
			[]MediaInput{{Type: "input_text", Text: "a"}, {Type: "input_text", Text: "b"}}},
		// 单个元素格式不对时跳过该元素，其他元素照常解析
		{"bad element skipped", `[{"content":"a"}, 123, {"content":[{"type":"input_text","text":"b"}, "junk"]}]`,
			[]MediaInput{{Type: "input_text", Text: "a"}, {Type: "input_text", Text: "b"}}},
		{"number input", `42`, nil},
		{"object input", `{"content":"x"}`, nil},
	}
	for _, tc := range cases {
		r := &OpenAIResponsesRequest{}
		if tc.input != "" {
			r.Input = json.RawMessage(tc.input)
		}
		if got := r.ParseInput(); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s:\n got  %+v\n want %+v", tc.name, got, tc.want)
		}
	}
}

func TestGeneralErrorResponseMessage(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
		oai  bool // TryToOpenAIError 是否返回非 nil
	}{
		{"openai object", `{"error":{"message":"bad key","type":"invalid_request_error"}}`, "bad key", true},
		{"error string", `{"error":"quota exceeded"}`, "quota exceeded", false},
		{"error object without message falls back", `{"error":{"code":1},"message":"fallback"}`, "fallback", false},
		{"error null falls back", `{"error":null,"msg":"from msg"}`, "from msg", false},
		{"error number kept as text", `{"error":500}`, "500", false},
		{"error array kept as text", `{"error":["a","b"]}`, `["a","b"]`, false},
		{"empty error string falls back", `{"error":"","detail":"from detail"}`, "from detail", false},
		{"header message", `{"header":{"message":"from header"}}`, "from header", false},
		{"response error", `{"response":{"error":{"message":"nested"}}}`, "nested", false},
		{"nothing", `{}`, "", false},
	}
	for _, tc := range cases {
		var e GeneralErrorResponse
		if err := json.Unmarshal([]byte(tc.body), &e); err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if got := e.ToMessage(); got != tc.want {
			t.Errorf("%s: ToMessage = %q, want %q", tc.name, got, tc.want)
		}
		if got := e.TryToOpenAIError() != nil; got != tc.oai {
			t.Errorf("%s: TryToOpenAIError non-nil = %v, want %v", tc.name, got, tc.oai)
		}
	}
}
