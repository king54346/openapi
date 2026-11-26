package token

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"openapi/common"
	"openapi/model"
)

const maxTokenNameLength = 50

var ErrValidation = errors.New("validation error")

func validationErr(msg string) error {
	return fmt.Errorf("%w: %s", ErrValidation, msg)
}

type TokenInput struct {
	Name               string `json:"name"`
	ModelLimits        string `json:"model_limits"`
	AllowIps           string `json:"allow_ips"`
	Group              string `json:"group"`
	Id                 int    `json:"id"`
	Status             int    `json:"status"`
	ExpiredTime        int64  `json:"expired_time"`
	RemainQuota        int    `json:"remain_quota"`
	ModelLimitsType    int    `json:"model_limits_type"`
	UnlimitedQuota     bool   `json:"unlimited_quota"`
	ModelLimitsEnabled bool   `json:"model_limits_enabled"`
	CrossGroupRetry    bool   `json:"cross_group_retry"`
}

func (r *TokenInput) validate() error {
	r.Name = strings.TrimSpace(r.Name)
	if utf8.RuneCountInString(r.Name) > maxTokenNameLength {
		return validationErr(fmt.Sprintf("token name must be at most %d characters", maxTokenNameLength))
	}
	if r.ExpiredTime != -1 && r.ExpiredTime != 0 && r.ExpiredTime < time.Now().Unix() {
		return validationErr("expired time must be in the future, or -1 for never")
	}
	if r.ExpiredTime == 0 {
		r.ExpiredTime = -1
	}
	if r.RemainQuota < 0 {
		return validationErr("remain quota must not be negative")
	}
	if r.ModelLimitsType != 0 && r.ModelLimitsType != 1 {
		return validationErr("model_limits_type must be 0 (whitelist) or 1 (blacklist)")
	}
	return nil
}

func (r *TokenInput) applyTo(t *model.Token) {
	t.Name = r.Name
	t.ExpiredTime = r.ExpiredTime
	t.RemainQuota = r.RemainQuota
	t.UnlimitedQuota = r.UnlimitedQuota
	t.ModelLimitsEnabled = r.ModelLimitsEnabled
	t.ModelLimits = r.ModelLimits
	t.ModelLimitsType = r.ModelLimitsType
	t.IpLimits = r.AllowIps
	t.CrossGroupRetry = r.CrossGroupRetry
}

func ListUserTokens(userID, startIdx, pageSize int) ([]*model.Token, int64, error) {
	return model.GetUserTokens(userID, startIdx, pageSize)
}

func SearchUserTokens(userID int, keyword, key string, startIdx, pageSize int) ([]*model.Token, int64, error) {
	return model.SearchUserTokens(userID, keyword, key, startIdx, pageSize)
}

func GetUserToken(id, userID int) (*model.Token, error) {
	return model.GetUserTokenById(id, userID)
}

func CreateToken(userID int, in TokenInput) (*model.Token, error) {
	if err := in.validate(); err != nil {
		return nil, err
	}
	t := &model.Token{UserId: userID}
	in.applyTo(t)
	if err := t.Insert(); err != nil {
		return nil, err
	}
	return t, nil
}

func UpdateToken(userID int, in TokenInput, statusOnly bool) (*model.Token, error) {
	t, err := model.GetUserTokenById(in.Id, userID)
	if err != nil {
		return nil, err
	}
	if statusOnly {
		if in.Status == common.TokenStatusEnabled {
			if t.ExpiredTime != -1 && t.ExpiredTime < time.Now().Unix() {
				return nil, validationErr("token has expired, update its expired time first")
			}
			if !t.UnlimitedQuota && t.RemainQuota <= 0 {
				return nil, validationErr("token quota is exhausted, update its quota first")
			}
		} else if in.Status != common.TokenStatusDisabled {
			return nil, validationErr("status must be 1 (enabled) or 2 (disabled)")
		}
		if err := t.UpdateStatus(in.Status); err != nil {
			return nil, err
		}
		return t, nil
	}

	if err := in.validate(); err != nil {
		return nil, err
	}
	in.applyTo(t)
	if t.Status == common.TokenStatusExpired && (t.ExpiredTime == -1 || t.ExpiredTime > time.Now().Unix()) {
		t.Status = common.TokenStatusEnabled
	}
	if t.Status == common.TokenStatusExhausted && (t.UnlimitedQuota || t.RemainQuota > 0) {
		t.Status = common.TokenStatusEnabled
	}
	if err := t.Update(); err != nil {
		return nil, err
	}
	return t, nil
}

func DeleteToken(userID, id int) error {
	rows, err := model.DeleteUserTokens(userID, []int{id})
	if err != nil {
		return err
	}
	if rows == 0 {
		return model.ErrTokenNotFound
	}
	return nil
}

func DeleteTokenBatch(userID int, ids []int) (int64, error) {
	return model.DeleteUserTokens(userID, ids)
}

func UnwrapBusinessErr(err error) (string, bool) {
	if errors.Is(err, ErrValidation) {
		return strings.TrimPrefix(err.Error(), ErrValidation.Error()+": "), true
	}
	if errors.Is(err, model.ErrTokenNotFound) {
		return "token not found", true
	}
	if errors.Is(err, model.ErrTokenAccessTemplateNotFound) {
		return "template not found", true
	}
	return "", false
}
