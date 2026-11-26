package prefill

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"openapi/model"
)

const (
	maxPrefillGroupNameLength = 128
	maxPrefillGroupDescLength = 255
)

var ErrValidation = errors.New("validation error")

func validationErr(msg string) error {
	return fmt.Errorf("%w: %s", ErrValidation, msg)
}

type PrefillGroupInput struct {
	Id          int             `json:"id"`
	Name        string          `json:"name"`
	Type        string          `json:"type"`
	Items       json.RawMessage `json:"items"`
	Description string          `json:"description"`
}

func (r *PrefillGroupInput) validate() error {
	r.Name = strings.TrimSpace(r.Name)
	r.Type = strings.TrimSpace(r.Type)
	if r.Name == "" {
		return validationErr("name is required")
	}
	if len(r.Name) > maxPrefillGroupNameLength {
		return validationErr(fmt.Sprintf("name must be at most %d characters", maxPrefillGroupNameLength))
	}
	if r.Type == "" {
		return validationErr("type is required")
	}
	if len([]rune(r.Description)) > maxPrefillGroupDescLength {
		return validationErr(fmt.Sprintf("description must be at most %d characters", maxPrefillGroupDescLength))
	}
	if len(r.Items) > 0 && !json.Valid(r.Items) {
		return validationErr("items must be valid json")
	}
	return nil
}

func ListPrefillGroups(typ string, startIdx, pageSize int) ([]*model.PrefillGroup, int64, error) {
	return model.ListPrefillGroups(strings.TrimSpace(typ), startIdx, pageSize)
}

func CreatePrefillGroup(in PrefillGroupInput) (*model.PrefillGroup, error) {
	if err := in.validate(); err != nil {
		return nil, err
	}
	duplicated, err := model.IsPrefillGroupNameDuplicated(0, in.Name)
	if err != nil {
		return nil, err
	}
	if duplicated {
		return nil, validationErr("prefill group name already exists")
	}
	group := &model.PrefillGroup{
		Name:        in.Name,
		Type:        in.Type,
		Items:       in.Items,
		Description: in.Description,
	}
	if err := group.Insert(); err != nil {
		return nil, err
	}
	return group, nil
}

func UpdatePrefillGroup(in PrefillGroupInput) (*model.PrefillGroup, error) {
	if in.Id <= 0 {
		return nil, validationErr("invalid request")
	}
	if err := in.validate(); err != nil {
		return nil, err
	}
	group, err := model.GetPrefillGroupById(in.Id)
	if err != nil {
		return nil, err
	}
	duplicated, err := model.IsPrefillGroupNameDuplicated(in.Id, in.Name)
	if err != nil {
		return nil, err
	}
	if duplicated {
		return nil, validationErr("prefill group name already exists")
	}
	group.Name = in.Name
	group.Type = in.Type
	group.Items = in.Items
	group.Description = in.Description
	if err := group.Update(); err != nil {
		return nil, err
	}
	return group, nil
}

func DeletePrefillGroup(id int) error {
	group, err := model.GetPrefillGroupById(id)
	if err != nil {
		return err
	}
	return group.Delete()
}

func UnwrapValidationErr(err error) (string, bool) {
	if errors.Is(err, ErrValidation) {
		return strings.TrimPrefix(err.Error(), ErrValidation.Error()+": "), true
	}
	if errors.Is(err, model.ErrPrefillGroupNotFound) {
		return model.ErrPrefillGroupNotFound.Error(), true
	}
	return "", false
}
