package system

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"openapi/common"
	"openapi/model"
)

var ErrValidation = errors.New("validation error")

func validationErr(msg string) error {
	return fmt.Errorf("%w: %s", ErrValidation, msg)
}

func ValidateUsername(username string) error {
	if username == "" || utf8.RuneCountInString(username) > 20 {
		return validationErr("username must be 1-20 characters")
	}
	if strings.ContainsAny(username, " \t\r\n") {
		return validationErr("username must not contain whitespace")
	}
	return nil
}

func ValidatePassword(password string) error {
	if n := utf8.RuneCountInString(password); n < 8 || n > 64 {
		return validationErr("password must be 8-64 characters")
	}
	return nil
}

type SetupStatus struct {
	Initialized bool   `json:"initialized"`
	RootUser    bool   `json:"root_user"`
	Version     string `json:"version"`
}

func GetSetupStatus() (*SetupStatus, error) {
	initialized, err := model.IsSetupInitialized()
	if err != nil {
		return nil, err
	}
	rootExists, err := model.HasRootUser()
	if err != nil {
		return nil, err
	}
	return &SetupStatus{
		Initialized: initialized,
		RootUser:    rootExists,
		Version:     common.Version,
	}, nil
}

type SetupInput struct {
	Username        string `json:"username"`
	Password        string `json:"password"`
	ConfirmPassword string `json:"confirm_password"`
	DisplayName     string `json:"display_name"`
}

func InitializeSystem(in SetupInput) (*model.User, error) {
	username := strings.TrimSpace(in.Username)
	if username == "" {
		return nil, validationErr("username is required")
	}
	if err := ValidateUsername(username); err != nil {
		return nil, err
	}
	if err := ValidatePassword(in.Password); err != nil {
		return nil, err
	}
	if in.ConfirmPassword != "" && in.ConfirmPassword != in.Password {
		return nil, validationErr("passwords do not match")
	}
	return model.CreateRootUserAndMarkSetup(username, in.Password, strings.TrimSpace(in.DisplayName))
}
