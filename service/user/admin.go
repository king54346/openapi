package user

import (
	"errors"
	"strings"

	"openapi/common"
	"openapi/model"
)

func ListUsers(startIdx, pageSize int) ([]*model.User, int64, error) {
	return model.GetAllUsers(startIdx, pageSize)
}

func SearchUsers(keyword string, startIdx, pageSize int) ([]*model.User, int64, error) {
	return model.SearchUsers(keyword, startIdx, pageSize)
}

func LoadManagedUser(actorID, actorRole, targetID int) (*model.User, error) {
	u, err := model.GetUserById(targetID)
	if err != nil {
		return nil, err
	}
	if !CanManage(actorID, actorRole, u) {
		return nil, ErrNoPermission
	}
	return u, nil
}

type CreateUserInput struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
	Role        int    `json:"role"`
	Remark      string `json:"remark"`
}

func CreateUser(actorRole int, in CreateUserInput) (*model.User, error) {
	username := strings.TrimSpace(in.Username)
	if err := ValidateUsername(username); err != nil {
		return nil, err
	}
	if err := ValidatePassword(in.Password); err != nil {
		return nil, err
	}
	role := in.Role
	if role == 0 {
		role = common.RoleCommonUser
	}
	if !common.IsValidateRole(role) || (role >= actorRole && actorRole != common.RoleRootUser) {
		return nil, validationErr("cannot create a user with role equal to or higher than yours")
	}
	if role == common.RoleRootUser {
		return nil, validationErr("cannot create another root user")
	}
	u := &model.User{
		Username:    username,
		DisplayName: in.DisplayName,
		Role:        role,
		Remark:      in.Remark,
	}
	if err := u.Insert(in.Password); err != nil {
		return nil, err
	}
	return u, nil
}

type UpdateUserInput struct {
	Id          int     `json:"id"`
	Username    *string `json:"username"`
	Password    string  `json:"password"`
	DisplayName *string `json:"display_name"`
	Email       *string `json:"email"`
	Quota       *int    `json:"quota"`
	Remark      *string `json:"remark"`
}

func UpdateUser(actorID, actorRole int, in UpdateUserInput) (*model.User, error) {
	target, err := LoadManagedUser(actorID, actorRole, in.Id)
	if err != nil {
		return nil, err
	}
	fields := map[string]any{}
	if in.Username != nil && strings.TrimSpace(*in.Username) != target.Username {
		name := strings.TrimSpace(*in.Username)
		if err := ValidateUsername(name); err != nil {
			return nil, err
		}
		fields["username"] = name
	}
	if in.Password != "" {
		if err := ValidatePassword(in.Password); err != nil {
			return nil, err
		}
	}
	if in.DisplayName != nil {
		fields["display_name"] = *in.DisplayName
	}
	if in.Email != nil {
		fields["email"] = *in.Email
	}
	if in.Quota != nil {
		fields["quota"] = *in.Quota
	}
	if in.Remark != nil {
		fields["remark"] = *in.Remark
	}
	if err := model.UpdateUserFields(target.Id, fields, in.Password); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return nil, model.ErrUsernameTaken
		}
		return nil, err
	}
	return model.GetUserById(target.Id)
}

func DeleteUser(actorID, actorRole, targetID int) error {
	target, err := LoadManagedUser(actorID, actorRole, targetID)
	if err != nil {
		return err
	}
	return model.DeleteUserById(target.Id)
}

func ManageUser(actorID, actorRole, targetID int, action string) (deleted bool, updated *model.User, err error) {
	target, err := LoadManagedUser(actorID, actorRole, targetID)
	if err != nil {
		return false, nil, err
	}
	fields := map[string]any{}
	switch action {
	case "disable":
		fields["status"] = common.UserStatusDisabled
	case "enable":
		fields["status"] = common.UserStatusEnabled
	case "delete":
		if err := model.DeleteUserById(target.Id); err != nil {
			return false, nil, err
		}
		return true, nil, nil
	case "promote":
		if actorRole != common.RoleRootUser {
			return false, nil, validationErr("only root can promote users")
		}
		if target.Role >= common.RoleAdminUser {
			return false, nil, validationErr("user is already an admin")
		}
		fields["role"] = common.RoleAdminUser
	case "demote":
		if target.Role <= common.RoleCommonUser {
			return false, nil, validationErr("user is already a common user")
		}
		fields["role"] = common.RoleCommonUser
	default:
		return false, nil, validationErr("unknown action: " + action)
	}
	if err := model.UpdateUserFields(target.Id, fields, ""); err != nil {
		return false, nil, err
	}
	u, err := model.GetUserById(target.Id)
	return false, u, err
}

func IsBusinessErr(err error) (string, bool) {
	if errors.Is(err, ErrValidation) {
		return strings.TrimPrefix(err.Error(), ErrValidation.Error()+": "), true
	}
	if errors.Is(err, ErrNoPermission) {
		return ErrNoPermission.Error(), true
	}
	if errors.Is(err, model.ErrUserNotFound) {
		return "user not found", true
	}
	if errors.Is(err, model.ErrUsernameTaken) {
		return model.ErrUsernameTaken.Error(), true
	}
	return "", false
}
