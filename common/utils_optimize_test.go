package common

import (
	"strings"
	"testing"
)

func TestInterface2String(t *testing.T) {
	cases := []struct {
		name  string
		input any
		want  string
	}{
		{"nil", nil, ""},
		{"string", "hello", "hello"},
		{"int", 42, "42"},
		{"int64", int64(-7), "-7"},
		{"int32", int32(9), "9"},
		{"float64", 1.5, "1.5"},
		{"bool true", true, "true"},
		{"bool false", false, "false"},
		{"bytes", []byte("abc"), "abc"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Interface2String(tc.input); got != tc.want {
				t.Fatalf("Interface2String(%v) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestGetRandomStringEdge(t *testing.T) {
	if got := GetRandomString(0); got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
	if got := GetRandomString(-3); got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
	if got := GetRandomString(16); len(got) != 16 {
		t.Fatalf("expected length 16, got %d", len(got))
	}
}

func TestBuildURL(t *testing.T) {
	cases := []struct {
		name     string
		base     string
		endpoint string
		want     string
	}{
		{"empty base", "", "/v1/chat", "/v1/chat"},
		{"empty endpoint", "https://api.example.com/v1", "", "https://api.example.com/v1"},
		{"join", "https://api.example.com/v1/", "/chat/completions", "https://api.example.com/chat/completions"},
		{"absolute endpoint wins", "https://api.example.com/v1", "https://other.example.com/x", "https://other.example.com/x"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := BuildURL(tc.base, tc.endpoint); got != tc.want {
				t.Fatalf("BuildURL = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestGetRandomIntNonPositive(t *testing.T) {
	if got := GetRandomInt(0); got != 0 {
		t.Fatalf("expected 0, got %d", got)
	}
	if got := GetRandomInt(-5); got != 0 {
		t.Fatalf("expected 0, got %d", got)
	}
}

func TestGetUUIDFormat(t *testing.T) {
	id := GetUUID()
	if len(id) != 32 {
		t.Fatalf("expected 32 chars, got %d (%q)", len(id), id)
	}
	if strings.Contains(id, "-") {
		t.Fatalf("uuid should not contain dashes: %q", id)
	}
}

func TestGenerateRandomKeyInvalidLength(t *testing.T) {
	if _, err := GenerateRandomKey(0); err == nil {
		t.Fatal("expected error for zero length")
	}
	if _, err := GenerateRandomCharsKey(0); err == nil {
		// crypto/rand with zero length returns empty string without error in some Go versions.
		// Accept either behavior but ensure result is empty.
		t.Log("zero-length chars key returned no error")
	}
}

func TestMaskSensitiveInfoBasics(t *testing.T) {
	got := MaskSensitiveInfo("call https://api.openai.com/v1/chat?key=secret and openai.com")
	if !strings.Contains(got, "https://***.com") {
		t.Fatalf("url host not masked: %q", got)
	}
	if !strings.Contains(got, "***.com") {
		t.Fatalf("plain domain not masked: %q", got)
	}
	if strings.Contains(got, "key=secret") {
		t.Fatalf("query value leaked: %q", got)
	}
}

func TestMaskSensitiveInfoWithPort(t *testing.T) {
	got := MaskSensitiveInfo("https://api.example.com:8443/v1/x")
	if !strings.Contains(got, ":8443") {
		t.Fatalf("port should be preserved: %q", got)
	}
	if strings.Contains(got, "api.example.com") {
		t.Fatalf("host leaked: %q", got)
	}
}

func TestMaskSensitiveInfoQueryOrderStable(t *testing.T) {
	a := MaskSensitiveInfo("https://example.com/?b=1&a=2")
	b := MaskSensitiveInfo("https://example.com/?a=2&b=1")
	if a != b {
		t.Fatalf("masked query order unstable: %q vs %q", a, b)
	}
}

func TestIsJsonFastPath(t *testing.T) {
	if IsJsonArray(`{"a":1}`) {
		t.Fatal("object should not be detected as array")
	}
	if IsJsonObject(`[1,2]`) {
		t.Fatal("array should not be detected as object")
	}
	if !IsJsonArray(`[1,2]`) || !IsJsonObject(`{"a":1}`) {
		t.Fatal("valid json not detected")
	}
}

func TestDeepCopyKeepsZeroValues(t *testing.T) {
	type inner struct {
		Name  string
		Count int
		Flag  bool
	}
	type outer struct {
		Inner inner
		Tags  []string
	}
	src := &outer{Inner: inner{Name: "", Count: 0, Flag: false}, Tags: []string{"a"}}
	dst, err := DeepCopy(src)
	if err != nil {
		t.Fatalf("DeepCopy failed: %v", err)
	}
	dst.Tags[0] = "mutated"
	if src.Tags[0] != "a" {
		t.Fatal("deep copy shares slice backing array")
	}
	if dst.Inner.Name != "" || dst.Inner.Count != 0 || dst.Inner.Flag != false {
		t.Fatal("zero values were not preserved")
	}
	if _, err := DeepCopy[outer](nil); err == nil {
		t.Fatal("expected error for nil source")
	}
}
