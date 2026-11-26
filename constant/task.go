package constant

import "strings"

type TaskPlatform string

const (
	// TaskPlatformSuno Suno 音乐任务平台（distributor 引用，待对接）。
	TaskPlatformSuno TaskPlatform = "suno"

	TaskActionGenerate          = "generate"
	TaskActionTextGenerate      = "textGenerate"
	TaskActionImageGenerate     = "imageGenerate"
	TaskActionFirstTailGenerate = "firstTailGenerate"
	TaskActionReferenceGenerate = "referenceGenerate"
	TaskActionRemix             = "remixGenerate"
)

func CoverTaskActionToModelName(platform TaskPlatform, action string) string {
	return strings.ToLower(string(platform)) + "_" + strings.ToLower(action)
}
