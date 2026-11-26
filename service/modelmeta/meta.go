package modelmeta

import (
	"errors"
	"fmt"
	"strings"

	"openapi/model"
)

var ErrValidation = errors.New("validation error")

func validationErr(msg string) error {
	return fmt.Errorf("%w: %s", ErrValidation, msg)
}

func checkModelName(id int, name string) error {
	if strings.TrimSpace(name) == "" {
		return validationErr("model_name is required")
	}
	dup, err := model.IsModelNameDuplicated(id, name)
	if err != nil {
		return err
	}
	if dup {
		return validationErr("model_name already exists")
	}
	return nil
}

func ListModelsMeta(startIdx, pageSize int) ([]*model.Model, int64, map[int64]int64, error) {
	models, total, err := model.GetAllModels(startIdx, pageSize)
	if err != nil {
		return nil, 0, nil, err
	}
	model.EnrichModels(models)
	vendorCounts, _ := model.GetVendorModelCounts()
	return models, total, vendorCounts, nil
}

func GetModelTags() ([]string, error) {
	return model.GetAllModelTags()
}

func SearchModelsMeta(keyword, vendor string, tags []string, startIdx, pageSize int) ([]*model.Model, int64, error) {
	models, total, err := model.SearchModels(keyword, vendor, tags, startIdx, pageSize)
	if err != nil {
		return nil, 0, err
	}
	model.EnrichModels(models)
	return models, total, nil
}

func GetModelMeta(id int) (*model.Model, error) {
	m, err := model.GetModelMetaById(id)
	if err != nil {
		return nil, err
	}
	model.EnrichModels([]*model.Model{m})
	return m, nil
}

func CreateModelMeta(m model.Model) (*model.Model, error) {
	m.Id = 0
	m.ModelName = strings.TrimSpace(m.ModelName)
	if err := checkModelName(0, m.ModelName); err != nil {
		return nil, err
	}
	if err := m.Insert(); err != nil {
		return nil, err
	}
	return &m, nil
}

func UpdateModelMeta(m model.Model, statusOnly bool) (*model.Model, error) {
	if m.Id <= 0 {
		return nil, validationErr("id is required")
	}
	if _, err := model.GetModelMetaById(m.Id); err != nil {
		return nil, err
	}
	if statusOnly {
		if err := model.UpdateModelMetaStatus(m.Id, m.Status); err != nil {
			return nil, err
		}
	} else {
		m.ModelName = strings.TrimSpace(m.ModelName)
		if err := checkModelName(m.Id, m.ModelName); err != nil {
			return nil, err
		}
		if err := m.Update(); err != nil {
			return nil, err
		}
	}
	return model.GetModelMetaById(m.Id)
}

func DeleteModelMeta(id int) error {
	return model.DeleteModelMetaById(id)
}

func UnwrapBusinessErr(err error) (string, bool) {
	if errors.Is(err, ErrValidation) {
		return strings.TrimPrefix(err.Error(), ErrValidation.Error()+": "), true
	}
	if errors.Is(err, model.ErrModelMetaNotFound) {
		return "model not found", true
	}
	return "", false
}
