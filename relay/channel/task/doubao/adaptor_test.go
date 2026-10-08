package doubao

import (
	"encoding/json"
	"testing"

	relaycommon "openapi/relay/common"
)

func TestConvertToRequestPayloadOfficialSeedance25Content(t *testing.T) {
	body := []byte(`{
		"model":"doubao-seedance-2-5-260628",
		"generate_audio":true,
		"ratio":"16:9",
		"duration":15,
		"omni_reference_task_type":"reference",
		"output_format":"mov",
		"content":[
			{"type":"text","text":"参考@图像1和@视频1生成广告片"},
			{"type":"image_url","image_url":{"url":"https://example.com/image.png"},"role":"reference_image"},
			{"type":"video_url","video_url":{"url":"https://example.com/video.mp4"},"role":"reference_video"}
		]
	}`)

	var req relaycommon.TaskSubmitReq
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("unmarshal request: %v", err)
	}
	if req.GetPrompt() == "" {
		t.Fatal("expected prompt from text content")
	}

	payload, err := (&TaskAdaptor{}).convertToRequestPayload(&req)
	if err != nil {
		t.Fatalf("convert request: %v", err)
	}
	if payload.Model != "doubao-seedance-2-5-260628" || payload.Ratio != "16:9" || payload.Duration != 15 {
		t.Fatalf("unexpected basic fields: %+v", payload)
	}
	if payload.GenerateAudio == nil || !bool(*payload.GenerateAudio) {
		t.Fatal("generate_audio was not preserved")
	}
	if payload.OmniReferenceTaskType != "reference" || payload.OutputFormat != "mov" {
		t.Fatalf("2.5 fields were not preserved: %+v", payload)
	}
	if len(payload.Content) != 3 {
		t.Fatalf("expected 3 content items, got %d", len(payload.Content))
	}
	if payload.Content[1].ImageURL == nil || payload.Content[1].Role != "reference_image" {
		t.Fatalf("unexpected image content: %+v", payload.Content[1])
	}
	if payload.Content[2].VideoURL == nil || payload.Content[2].Role != "reference_video" {
		t.Fatalf("unexpected video content: %+v", payload.Content[2])
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	var upstream map[string]any
	if err := json.Unmarshal(encoded, &upstream); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	content := upstream["content"].([]any)
	video := content[2].(map[string]any)
	if _, ok := video["video_url"]; !ok {
		t.Fatalf("upstream payload has no video_url: %s", encoded)
	}
	if _, ok := video["video"]; ok {
		t.Fatalf("official reference video must not use legacy video field: %s", encoded)
	}
}

func TestConvertToRequestPayloadLegacyImagesAddsRoles(t *testing.T) {
	req := relaycommon.TaskSubmitReq{
		Model:  "doubao-seedance-2.0",
		Prompt: "生成视频",
		Images: []string{"https://example.com/1.png", "https://example.com/2.png"},
	}

	payload, err := (&TaskAdaptor{}).convertToRequestPayload(&req)
	if err != nil {
		t.Fatalf("convert request: %v", err)
	}
	if len(payload.Content) != 3 {
		t.Fatalf("expected text and 2 images, got %d items", len(payload.Content))
	}
	for i, item := range payload.Content[1:] {
		if item.Type != "image_url" || item.ImageURL == nil || item.Role != "reference_image" {
			t.Fatalf("image %d is not a reference_image: %+v", i, item)
		}
	}
}

func TestConvertToRequestPayloadAddsMissingOfficialRoles(t *testing.T) {
	req := relaycommon.TaskSubmitReq{
		Content: []relaycommon.TaskContentItem{
			{Type: "text", Text: "生成视频"},
			{Type: "image_url", ImageURL: &relaycommon.TaskContentURL{URL: "https://example.com/image.png"}},
			{Type: "video_url", VideoURL: &relaycommon.TaskContentURL{URL: "https://example.com/video.mp4"}},
		},
	}

	payload, err := (&TaskAdaptor{}).convertToRequestPayload(&req)
	if err != nil {
		t.Fatalf("convert request: %v", err)
	}
	if payload.Content[1].Role != "reference_image" || payload.Content[2].Role != "reference_video" {
		t.Fatalf("missing roles were not defaulted: %+v", payload.Content)
	}
}
