package user

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"openapi/common"
	"openapi/model"
)

const (
	minPasswordLength = 8
	maxPasswordLength = 64
	maxUsernameLength = 20
)

var (
	ErrValidation   = errors.New("validation error")
	ErrNoPermission = errors.New("no permission to manage this user")
)

func validationErr(msg string) error {
	return fmt.Errorf("%w: %s", ErrValidation, msg)
}

func ValidatePassword(password string) error {
	if n := utf8.RuneCountInString(password); n < minPasswordLength || n > maxPasswordLength {
		return validationErr(fmt.Sprintf("password must be %d-%d characters", minPasswordLength, maxPasswordLength))
	}
	return nil
}

func ValidateUsername(username string) error {
	if username == "" || utf8.RuneCountInString(username) > maxUsernameLength {
		return validationErr(fmt.Sprintf("username must be 1-%d characters", maxUsernameLength))
	}
	if strings.ContainsAny(username, " \t\r\n") {
		return validationErr("username must not contain whitespace")
	}
	return nil
}

func CanManage(myId, myRole int, target *model.User) bool {
	if target.Id == myId {
		return false
	}
	if myRole == common.RoleRootUser {
		return true
	}
	return myRole > target.Role
}

func Login(username, password string) (*model.User, error) {
	return model.ValidateUserLogin(strings.TrimSpace(username), password)
}

func GetSelf(userID int) (*model.User, error) {
	return model.GetUserById(userID)
}

func GenerateAccessToken(userID int) (string, error) {
	return model.GenerateUserAccessToken(userID)
}

func GetAvailableModels() []string {
	models := model.GetEnabledModels()
	out := make([]string, 0, len(models))
	for _, name := range models {
		if name != "" {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}
