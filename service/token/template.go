package token

import (
	"strings"

	"openapi/model"
)

const maxTokenTemplateNameLength = 64

type TemplateInput struct {
	Id              int    `json:"id"`
	Name            string `json:"name"`
	ModelLimitsType int    `json:"model_limits_type"`
	ModelLimits     string `json:"model_limits"`
	AllowIps        string `json:"allow_ips"`
}

func (r *TemplateInput) validate() error {
	r.Name = strings.TrimSpace(r.Name)
	if r.Name == "" {
		return validationErr("name is required")
	}
	if len([]rune(r.Name)) > maxTokenTemplateNameLength {
		return validationErr("name must be at most 64 characters")
	}
	if r.ModelLimitsType != 0 && r.ModelLimitsType != 1 {
		return validationErr("model_limits_type must be 0 (whitelist) or 1 (blacklist)")
	}
	if len(r.ModelLimits) > 2048 {
		return validationErr("model_limits is too long")
	}
	return nil
}

func ListTemplates(userID, startIdx, pageSize int) ([]*model.TokenAccessTemplate, int64, error) {
	return model.ListUserTokenAccessTemplates(userID, startIdx, pageSize)
}

func CreateTemplate(userID int, in TemplateInput) (*model.TokenAccessTemplate, error) {
	if err := in.validate(); err != nil {
		return nil, err
	}
	tpl := &model.TokenAccessTemplate{
		UserId:          userID,
		Name:            in.Name,
		ModelLimitsType: in.ModelLimitsType,
		ModelLimits:     in.ModelLimits,
		AllowIps:        in.AllowIps,
	}
	if err := tpl.Insert(); err != nil {
		return nil, err
	}
	return tpl, nil
}

func UpdateTemplate(userID int, in TemplateInput) (*model.TokenAccessTemplate, error) {
	if in.Id <= 0 {
		return nil, validationErr("invalid request")
	}
	if err := in.validate(); err != nil {
		return nil, err
	}
	tpl, err := model.GetTokenAccessTemplateById(in.Id, userID)
	if err != nil {
		return nil, err
	}
	tpl.Name = in.Name
	tpl.ModelLimitsType = in.ModelLimitsType
	tpl.ModelLimits = in.ModelLimits
	tpl.AllowIps = in.AllowIps
	if err := tpl.Update(); err != nil {
		return nil, err
	}
	return tpl, nil
}

func DeleteTemplate(userID, id int) error {
	tpl, err := model.GetTokenAccessTemplateById(id, userID)
	if err != nil {
		return err
	}
	return tpl.Delete()
}
