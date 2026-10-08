package dto

// MidjourneyRequest Midjourney 请求（桩实现：待补全字段）。
type MidjourneyRequest struct {
	Prompt string `json:"prompt,omitempty"`
}

// PlayGroundRequest 体验中心请求（桩实现：仅保留鉴权需要的分组字段）。
type PlayGroundRequest struct {
	Model string `json:"model,omitempty"`
	Group string `json:"group,omitempty"`
}
