package controller

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"openapi/common"
	"openapi/model"

	gin "github.com/king54346/gin-tiny"
	"github.com/king54346/gin-tiny/middleware/sessions"
)

// 用户接口：登录/登出/个人信息（需登录），以及用户管理（需管理员）。
// 权限规则：管理员只能操作角色低于自己的用户；root 可操作除自己以外的所有用户；
// 只有 root 能把用户提升为管理员。

const (
	minPasswordLength = 8
	maxPasswordLength = 64
	maxUsernameLength = 20
)

func validatePassword(password string) error {
	if n := utf8.RuneCountInString(password); n < minPasswordLength || n > maxPasswordLength {
		return fmt.Errorf("password must be %d-%d characters", minPasswordLength, maxPasswordLength)
	}
	return nil
}

func validateUsername(username string) error {
	if username == "" || utf8.RuneCountInString(username) > maxUsernameLength {
		return fmt.Errorf("username must be 1-%d characters", maxUsernameLength)
	}
	if strings.ContainsAny(username, " \t\r\n") {
		return errors.New("username must not contain whitespace")
	}
	return nil
}

// canManage 当前用户（myId/myRole）能否管理目标用户。
func canManage(myId, myRole int, target *model.User) bool {
	if target.Id == myId {
		return false
	}
	if myRole == common.RoleRootUser {
		return true
	}
	return myRole > target.Role
}

// ---------- 登录与个人信息 ----------

// Login 用户名密码登录，成功后写入 session。
func Login(c gin.Context) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Username == "" || req.Password == "" {
		apiErrorMsg(c, "username and password are required")
		return
	}
	user, err := model.ValidateUserLogin(strings.TrimSpace(req.Username), req.Password)
	if err != nil {
		if errors.Is(err, model.ErrPasswordIncorrect) || errors.Is(err, model.ErrUserDisabled) {
			apiErrorMsg(c, err.Error())
			return
		}
		apiError(c, err)
		return
	}
	session := sessions.Default(c)
	session.Set("id", user.Id)
	session.Set("username", user.Username)
	session.Set("role", user.Role)
	session.Set("status", user.Status)
	session.Set("group", user.Group)
	if err := session.Save(); err != nil {
		apiError(c, err)
		return
	}
	apiSuccess(c, gin.H{
		"id":           user.Id,
		"username":     user.Username,
		"display_name": user.DisplayName,
		"role":         user.Role,
		"status":       user.Status,
		"group":        user.Group,
	})
}

// Logout 清除 session。
func Logout(c gin.Context) {
	session := sessions.Default(c)
	session.Clear()
	if err := session.Save(); err != nil {
		apiError(c, err)
		return
	}
	apiOK(c)
}

// GetSelf 当前登录用户的信息。
func GetSelf(c gin.Context) {
	user, err := model.GetUserById(c.GetInt("id"))
	if err != nil {
		apiErrorMsg(c, "user not found")
		return
	}
	apiSuccess(c, user)
}

// GenerateAccessToken 为当前用户生成新的 access token（旧的立即失效）。
func GenerateAccessToken(c gin.Context) {
	token, err := model.GenerateUserAccessToken(c.GetInt("id"))
	if err != nil {
		apiError(c, err)
		return
	}
	apiSuccess(c, token)
}

// ---------- 用户管理（管理员） ----------

// GetAllUsers 分页列出用户。
func GetAllUsers(c gin.Context) {
	page := getPageQuery(c)
	users, total, err := model.GetAllUsers(page.offset(), page.PageSize)
	if err != nil {
		apiError(c, err)
		return
	}
	apiSuccess(c, page.result(users, total))
}

// SearchUsers 按 keyword（id/用户名/显示名/邮箱）与 group 搜索用户。
func SearchUsers(c gin.Context) {
	page := getPageQuery(c)
	users, total, err := model.SearchUsers(c.Query("keyword"), c.Query("group"), page.offset(), page.PageSize)
	if err != nil {
		apiError(c, err)
		return
	}
	apiSuccess(c, page.result(users, total))
}

// loadManagedUser 取 id 对应用户并校验当前管理员有权操作，失败时已写回错误。
func loadManagedUser(c gin.Context, id int) (*model.User, bool) {
	user, err := model.GetUserById(id)
	if err != nil {
		if errors.Is(err, model.ErrUserNotFound) {
			apiErrorMsg(c, "user not found")
		} else {
			apiError(c, err)
		}
		return nil, false
	}
	if !canManage(c.GetInt("id"), c.GetInt("role"), user) {
		apiErrorMsg(c, "no permission to manage this user")
		return nil, false
	}
	return user, true
}

// GetUser 查看单个用户（只能查看权限低于自己的用户，root 不受限）。
func GetUser(c gin.Context) {
	id, ok := paramID(c)
	if !ok {
		return
	}
	if id == c.GetInt("id") {
		GetSelf(c)
		return
	}
	if user, ok := loadManagedUser(c, id); ok {
		apiSuccess(c, user)
	}
}

// CreateUser 创建普通用户（角色不能高于等于创建者）。
func CreateUser(c gin.Context) {
	var req struct {
		Username    string `json:"username"`
		Password    string `json:"password"`
		DisplayName string `json:"display_name"`
		Group       string `json:"group"`
		Role        int    `json:"role"`
		Remark      string `json:"remark"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		apiErrorMsg(c, "invalid request: "+err.Error())
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if err := validateUsername(req.Username); err != nil {
		apiErrorMsg(c, err.Error())
		return
	}
	if err := validatePassword(req.Password); err != nil {
		apiErrorMsg(c, err.Error())
		return
	}
	if req.Role == 0 {
		req.Role = common.RoleCommonUser
	}
	if !common.IsValidateRole(req.Role) || (req.Role >= c.GetInt("role") && c.GetInt("role") != common.RoleRootUser) {
		apiErrorMsg(c, "cannot create a user with role equal to or higher than yours")
		return
	}
	if req.Role == common.RoleRootUser {
		apiErrorMsg(c, "cannot create another root user")
		return
	}
	user := &model.User{
		Username:    req.Username,
		DisplayName: req.DisplayName,
		Group:       req.Group,
		Role:        req.Role,
		Remark:      req.Remark,
	}
	if err := user.Insert(req.Password); err != nil {
		if errors.Is(err, model.ErrUsernameTaken) {
			apiErrorMsg(c, err.Error())
			return
		}
		apiError(c, err)
		return
	}
	apiSuccess(c, user)
}

// UpdateUser 修改用户资料；password 非空时重置密码。角色变更请用 ManageUser。
func UpdateUser(c gin.Context) {
	var req struct {
		Id          int     `json:"id"`
		Username    *string `json:"username"`
		Password    string  `json:"password"`
		DisplayName *string `json:"display_name"`
		Email       *string `json:"email"`
		Group       *string `json:"group"`
		Quota       *int    `json:"quota"`
		Remark      *string `json:"remark"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Id <= 0 {
		apiErrorMsg(c, "invalid request")
		return
	}
	user, ok := loadManagedUser(c, req.Id)
	if !ok {
		return
	}
	fields := map[string]any{}
	if req.Username != nil && strings.TrimSpace(*req.Username) != user.Username {
		name := strings.TrimSpace(*req.Username)
		if err := validateUsername(name); err != nil {
			apiErrorMsg(c, err.Error())
			return
		}
		fields["username"] = name
	}
	if req.Password != "" {
		if err := validatePassword(req.Password); err != nil {
			apiErrorMsg(c, err.Error())
			return
		}
	}
	if req.DisplayName != nil {
		fields["display_name"] = *req.DisplayName
	}
	if req.Email != nil {
		fields["email"] = *req.Email
	}
	if req.Group != nil && *req.Group != "" {
		fields["group"] = *req.Group
	}
	if req.Quota != nil {
		fields["quota"] = *req.Quota
	}
	if req.Remark != nil {
		fields["remark"] = *req.Remark
	}
	if err := model.UpdateUserFields(user.Id, fields, req.Password); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			apiErrorMsg(c, model.ErrUsernameTaken.Error())
			return
		}
		apiError(c, err)
		return
	}
	updated, err := model.GetUserById(user.Id)
	if err != nil {
		apiError(c, err)
		return
	}
	apiSuccess(c, updated)
}

// DeleteUser 软删除用户。
func DeleteUser(c gin.Context) {
	id, ok := paramID(c)
	if !ok {
		return
	}
	user, ok := loadManagedUser(c, id)
	if !ok {
		return
	}
	if err := model.DeleteUserById(user.Id); err != nil {
		apiError(c, err)
		return
	}
	apiOK(c)
}

// ManageUser 用户状态与角色操作，请求体 {"id": 1, "action": "disable|enable|delete|promote|demote"}。
func ManageUser(c gin.Context) {
	var req struct {
		Id     int    `json:"id"`
		Action string `json:"action"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Id <= 0 {
		apiErrorMsg(c, "invalid request")
		return
	}
	user, ok := loadManagedUser(c, req.Id)
	if !ok {
		return
	}
	myRole := c.GetInt("role")
	fields := map[string]any{}
	switch req.Action {
	case "disable":
		fields["status"] = common.UserStatusDisabled
	case "enable":
		fields["status"] = common.UserStatusEnabled
	case "delete":
		if err := model.DeleteUserById(user.Id); err != nil {
			apiError(c, err)
			return
		}
		apiOK(c)
		return
	case "promote":
		if myRole != common.RoleRootUser {
			apiErrorMsg(c, "only root can promote users")
			return
		}
		if user.Role >= common.RoleAdminUser {
			apiErrorMsg(c, "user is already an admin")
			return
		}
		fields["role"] = common.RoleAdminUser
	case "demote":
		if user.Role <= common.RoleCommonUser {
			apiErrorMsg(c, "user is already a common user")
			return
		}
		fields["role"] = common.RoleCommonUser
	default:
		apiErrorMsg(c, "unknown action: "+req.Action)
		return
	}
	if err := model.UpdateUserFields(user.Id, fields, ""); err != nil {
		apiError(c, err)
		return
	}
	updated, err := model.GetUserById(user.Id)
	if err != nil {
		apiError(c, err)
		return
	}
	apiSuccess(c, gin.H{"role": updated.Role, "status": updated.Status})
}

// BatchUpdateUserGroup 批量修改用户分组，请求体 {"ids": [...], "group": "vip"}。
// 只会修改当前管理员有权管理的用户。
func BatchUpdateUserGroup(c gin.Context) {
	var req struct {
		Ids   []int  `json:"ids"`
		Group string `json:"group"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Ids) == 0 || strings.TrimSpace(req.Group) == "" {
		apiErrorMsg(c, "ids and group are required")
		return
	}
	myId, myRole := c.GetInt("id"), c.GetInt("role")
	var allowed []int
	for _, id := range req.Ids {
		if user, err := model.GetUserById(id); err == nil && canManage(myId, myRole, user) {
			allowed = append(allowed, id)
		}
	}
	if len(allowed) == 0 {
		apiErrorMsg(c, "no permission to manage these users")
		return
	}
	rows, err := model.BatchUpdateUserGroup(allowed, strings.TrimSpace(req.Group))
	if err != nil {
		apiError(c, err)
		return
	}
	apiSuccess(c, rows)
}
