package common

import (
	"encoding/json"
	"testing"
)

func TestTaskSubmitReqGetDurationSeconds(t *testing.T) {
	cases := []struct {
		name string
		body string
		want int
	}{
		{"duration number", `{"prompt":"p","duration":10}`, 10},
		{"duration string", `{"prompt":"p","duration":"10"}`, 10},
		{"seconds string", `{"prompt":"p","seconds":"8"}`, 8},
		{"seconds number", `{"prompt":"p","seconds":12}`, 12},
		{"metadata duration string", `{"prompt":"p","metadata":{"duration":"10"}}`, 10},
		{"metadata duration number", `{"prompt":"p","metadata":{"duration":10}}`, 10},
		{"metadata durationSeconds", `{"prompt":"p","metadata":{"durationSeconds":6}}`, 6},
		{"metadata parameters duration", `{"prompt":"p","metadata":{"parameters":{"duration":10,"resolution":"720P"}}}`, 10},
		{"duration wins over metadata", `{"prompt":"p","duration":10,"metadata":{"duration":5}}`, 10},
		{"unspecified", `{"prompt":"p"}`, 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var req TaskSubmitReq
			if err := json.Unmarshal([]byte(tc.body), &req); err != nil {
				t.Fatalf("unmarshal failed: %v", err)
			}
			if got := req.GetDurationSeconds(); got != tc.want {
				t.Fatalf("GetDurationSeconds() = %d, want %d", got, tc.want)
			}
		})
	}
}
