package ali

import (
	"openapi/dto"

	"github.com/samber/lo"
)

// https://help.aliyun.com/document_detail/613695.html?spm=a2c4g.2399480.0.0.1adb778fAdzP9w#341800c0f8w0r

const EnableSearchModelSuffix = "-internet"

// requestOpenAI2Ali 通义要求 top_p 在 (0, 1) 开区间内，越界时收敛到边界值
func requestOpenAI2Ali(request dto.GeneralOpenAIRequest) *dto.GeneralOpenAIRequest {
	if request.TopP != nil {
		if *request.TopP >= 1 {
			request.TopP = lo.ToPtr(0.999)
		} else if *request.TopP <= 0 {
			request.TopP = lo.ToPtr(0.001)
		}
	}
	return &request
}
