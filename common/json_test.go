package common

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

type customMarshaler struct{ v string }

func (m customMarshaler) MarshalJSON() ([]byte, error) {
	return []byte(`{"custom":"` + m.v + `"}`), nil
}

type jsonSample struct {
	Name     string            `json:"name"`
	HTML     string            `json:"html"`
	Omit     string            `json:"omit,omitempty"`
	Map      map[string]int    `json:"map"`
	Raw      json.RawMessage   `json:"raw"`
	Ptr      *int              `json:"ptr"`
	Custom   customMarshaler   `json:"custom"`
	Nested   []map[string]any  `json:"nested"`
	Unicode  string            `json:"unicode"`
	Floats   []float64         `json:"floats"`
	Headers  map[string]string `json:"headers,omitempty"`
	internal string
}

// TestJSONMatchesStdlib sonic.ConfigStd 的输出与 encoding/json 一致（HTML 转义、map 键排序等）。
func TestJSONMatchesStdlib(t *testing.T) {
	sample := jsonSample{
		Name:    "a\"b\\c\n",
		HTML:    "<script>&</script>",
		Map:     map[string]int{"z": 1, "a": 2, "m": 3},
		Raw:     json.RawMessage(`{"k":[1,2]}`),
		Custom:  customMarshaler{v: "x"},
		Nested:  []map[string]any{{"b": true, "a": nil}},
		Unicode: "中文   emoji 😀",
		Floats:  []float64{0.1, 1e21, -3},
	}
	got, err := Marshal(sample)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := json.Marshal(sample)
	if !bytes.Equal(got, want) {
		t.Fatalf("marshal mismatch:\n got  %s\n want %s", got, want)
	}

	var back jsonSample
	if err := Unmarshal(got, &back); err != nil {
		t.Fatal(err)
	}
	if back.Name != sample.Name || back.HTML != sample.HTML || back.Map["a"] != 2 || string(back.Raw) != `{"k":[1,2]}` || back.Unicode != sample.Unicode {
		t.Fatalf("round trip: %+v", back)
	}
}

func TestJSONDecodeVariants(t *testing.T) {
	var m map[string]any
	if err := UnmarshalJsonStr(`{"a":1,"b":[true,null]}`, &m); err != nil || m["a"].(float64) != 1 {
		t.Fatalf("unmarshal string: %v %v", m, err)
	}
	var v struct{ A int }
	if err := DecodeJson(strings.NewReader(`{"A":7} trailing`), &v); err != nil || v.A != 7 {
		t.Fatalf("decode first value from stream: %v %v", v, err)
	}
	for _, bad := range []string{`{"a":`, `{"a":1,}`, `nope`} {
		if err := Unmarshal([]byte(bad), &m); err == nil {
			t.Errorf("invalid json %q should fail", bad)
		}
	}
	// 未知字段忽略，大小写不敏感匹配，与标准库一致
	var w struct{ Model string }
	if err := Unmarshal([]byte(`{"MODEL":"x","extra":1}`), &w); err != nil || w.Model != "x" {
		t.Fatalf("case insensitive field: %+v %v", w, err)
	}
}

func TestJSONKindOf(t *testing.T) {
	cases := map[string]JSONKind{
		`{"a":1}`: JSONObject, ` [1]`: JSONArray, "\n\t\"s\"": JSONString, `true`: JSONBool, `false`: JSONBool,
		`null`: JSONNull, `-1.5`: JSONNumber, `0`: JSONNumber, ``: JSONInvalid, `   `: JSONInvalid, `xyz`: JSONInvalid,
	}
	for in, want := range cases {
		if got := JSONKindOf([]byte(in)); got != want {
			t.Errorf("JSONKindOf(%q) = %s, want %s", in, got, want)
		}
	}
	if JSONObject.String() != "object" || JSONKind(99).String() != "invalid" {
		t.Fatal("JSONKind.String")
	}
}

func TestDecodeJSONString(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{` "a\nb" `, "a\nb", true}, {`""`, "", true}, {`"中"`, "中", true},
		{`"bad`, "", false}, {`123`, "", false}, {`null`, "", false}, {``, "", false},
	}
	for _, tc := range cases {
		if got, ok := DecodeJSONString([]byte(tc.in)); got != tc.want || ok != tc.ok {
			t.Errorf("DecodeJSONString(%q) = %q, %v; want %q, %v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestJsonRawMessageToString(t *testing.T) {
	cases := map[string]string{
		` "hello\nworld" `: "hello\nworld", `null`: "", ``: "", `  `: "", `123`: "123", ` {"a":1} `: `{"a":1}`, `"bad`: `"bad`,
	}
	for in, want := range cases {
		if got := JsonRawMessageToString(json.RawMessage(in)); got != want {
			t.Errorf("JsonRawMessageToString(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStringToByteSlice(t *testing.T) {
	if b := StringToByteSlice("abc"); string(b) != "abc" || len(b) != 3 || cap(b) != 3 {
		t.Fatalf("got %q len=%d cap=%d", b, len(b), cap(b))
	}
	if b := StringToByteSlice(""); len(b) != 0 {
		t.Fatalf("empty: %q", b)
	}
}

func BenchmarkUnmarshalChatRequest(b *testing.B) {
	payload := []byte(`{"model":"gpt-4o","stream":true,"max_tokens":512,"temperature":0.7,"messages":[` +
		strings.Repeat(`{"role":"user","content":"hello world, this is a test message with some length"},`, 20) +
		`{"role":"user","content":"end"}]}`)
	var v map[string]any
	b.Run("sonic", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = Unmarshal(payload, &v)
		}
	})
	b.Run("stdlib", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = json.Unmarshal(payload, &v)
		}
	})
}
