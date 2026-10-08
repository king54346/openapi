package ali

var ModelList = []string{
	// 图生视频（i2v）
	"happyhorse-1.1-i2v",    // 快马 1.1 图生视频（720P/1080P，按秒计费）
	"wan2.7-i2v-2026-04-25", // 万相2.7 图生视频（2026版，支持 media 数组 / 视频续写）
	"wan2.7-i2v",            // 万相2.7 图生视频（有声视频）
	"wan2.6-i2v",            // 万相2.6 图生视频（有声视频）
	"wan2.6-i2v-flash",      // 万相2.6极速版 图生视频（有声/无声可选）
	"wan2.5-i2v-preview",    // 万相2.5 preview（有声视频）推荐
	"wan2.2-i2v-flash",      // 万相2.2极速版（无声视频）
	"wan2.2-i2v-plus",       // 万相2.2专业版（无声视频）
	"wanx2.1-i2v-plus",      // 万相2.1专业版（无声视频）
	"wanx2.1-i2v-turbo",     // 万相2.1极速版（无声视频）
	// 文生视频（t2v）
	"wan2.7-t2v-2026-04-25", // 万相2.7 文生视频（2026版，支持 resolution+ratio 参数）
	"wan2.7-t2v",            // 万相2.7 文生视频
	"wan2.6-t2v",            // 万相2.6 文生视频
	"wan2.5-t2v-preview",    // 万相2.5 文生视频 preview
	"wan2.2-t2v-plus",       // 万相2.2文生视频专业版
	"wanx2.1-t2v-turbo",     // 万相2.1文生视频极速版
	"wanx2.1-t2v-plus",      // 万相2.1文生视频专业版
	"wan2.2-t2v",
}

var ChannelName = "ali"
