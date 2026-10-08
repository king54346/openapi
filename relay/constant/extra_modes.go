package constant

// distributor 引用的扩展 RelayMode（桩实现：数值仅进程内使用）。
const (
	RelayModeGemini int = 100

	RelayModeMidjourneyTaskFetch            int = 110
	RelayModeMidjourneyTaskFetchByCondition int = 111
	RelayModeMidjourneyNotify               int = 112
	RelayModeMidjourneyTaskImageSeed        int = 113

	RelayModeSunoFetch     int = 120
	RelayModeSunoFetchByID int = 121
)

// Path2RelayModeMidjourney 按路径解析 Midjourney relay mode（桩实现：未知）。
func Path2RelayModeMidjourney(path string) int {
	return RelayModeUnknown
}

// Path2RelaySuno 按方法与路径解析 Suno relay mode（桩实现：未知）。
func Path2RelaySuno(method, path string) int {
	return RelayModeUnknown
}
