package vendor

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

type VendorWithCount struct {
	*model.Vendor
	ModelCount int64 `json:"model_count"`
}

type UpdateVendorInput struct {
	Id          int     `json:"id"`
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Icon        *string `json:"icon"`
	Status      *int    `json:"status"`
}

func ListVendors(startIdx, pageSize int) ([]*model.Vendor, int64, error) {
	return model.ListVendors(startIdx, pageSize)
}

func SearchVendors(keyword string, status *int, startIdx, pageSize int) ([]*model.Vendor, int64, error) {
	return model.SearchVendors(strings.TrimSpace(keyword), status, startIdx, pageSize)
}

func GetVendor(id int) (*VendorWithCount, error) {
	v, err := model.GetVendorById(id)
	if err != nil {
		return nil, err
	}
	counts, _ := model.GetVendorModelCounts()
	return &VendorWithCount{Vendor: v, ModelCount: counts[int64(id)]}, nil
}

func checkVendorName(id int, name string) error {
	if strings.TrimSpace(name) == "" {
		return validationErr("vendor name is required")
	}
	dup, err := model.IsVendorNameDuplicated(id, name)
	if err != nil {
		return err
	}
	if dup {
		return validationErr("vendor name already exists")
	}
	return nil
}

func CreateVendor(v model.Vendor) (*model.Vendor, error) {
	v.Id = 0
	v.Name = strings.TrimSpace(v.Name)
	if err := checkVendorName(0, v.Name); err != nil {
		return nil, err
	}
	if err := v.Insert(); err != nil {
		return nil, err
	}
	return &v, nil
}

func UpdateVendor(in UpdateVendorInput) (*model.Vendor, error) {
	if in.Id <= 0 {
		return nil, validationErr("id is required")
	}
	v, err := model.GetVendorById(in.Id)
	if err != nil {
		return nil, err
	}
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if err := checkVendorName(in.Id, name); err != nil {
			return nil, err
		}
		v.Name = name
	}
	if in.Description != nil {
		v.Description = *in.Description
	}
	if in.Icon != nil {
		v.Icon = *in.Icon
	}
	if in.Status != nil {
		v.Status = *in.Status
	}
	if err := v.Update(); err != nil {
		return nil, err
	}
	return model.GetVendorById(in.Id)
}

func DeleteVendor(id int) error {
	v, err := model.GetVendorById(id)
	if err != nil {
		return err
	}
	return v.Delete()
}

func UnwrapBusinessErr(err error) (string, bool) {
	if errors.Is(err, ErrValidation) {
		return strings.TrimPrefix(err.Error(), ErrValidation.Error()+": "), true
	}
	if errors.Is(err, model.ErrVendorNotFound) {
		return "vendor not found", true
	}
	return "", false
}
