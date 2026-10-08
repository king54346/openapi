package ali

import (
	"fmt"
	"strings"

	"openapi/common"
	"openapi/dto"
	relaycommon "openapi/relay/common"

	gin "github.com/king54346/gin-tiny"
	"github.com/samber/lo"
)

func oaiFormEdit2WanxImageEdit(c gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (*AliImageRequest, error) {
	var err error
	var imageRequest AliImageRequest
	imageRequest.Model = request.Model
	imageRequest.ResponseFormat = request.ResponseFormat
	wanInput := WanImageInput{
		Prompt: request.Prompt,
	}

	if err := common.UnmarshalBodyReusable(c, &wanInput); err != nil {
		return nil, err
	}
	if wanInput.Images, err = getImageBase64sFromForm(c, "image"); err != nil {
		return nil, fmt.Errorf("get image base64s from form failed: %w", err)
	}
	//wanParams := WanImageParameters{
	//	N: int(request.N),
	//}
	imageRequest.Input = wanInput
	imageRequest.Parameters = AliImageParameters{
		N: int(lo.FromPtrOr(request.N, 0)),
	}

	return &imageRequest, nil
}

func isOldWanModel(modelName string) bool {
	return strings.Contains(modelName, "wan") && !strings.Contains(modelName, "wan2.6")
}

func isWanModel(modelName string) bool {
	return strings.Contains(modelName, "wan")
}
