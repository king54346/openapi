package system_test

import (
	"net/http"
	"testing"

	"openapi/common"
	"openapi/controller/common/testhelper"
	"openapi/model"
)

func TestOptionAPI(t *testing.T) {
	s := testhelper.NewTestServer(t)
	testhelper.ResetGlobalSettings(t)
	root := s.CreateUser("root", common.RoleRootUser)
	admin := s.CreateUser("admin", common.RoleAdminUser)

	s.Fail(admin, http.MethodGet, "/api/option/", nil)
	s.Fail(admin, http.MethodPut, "/api/option/", map[string]any{"key": "Notice", "value": "x"})
	if options := testhelper.Decode[map[string]string](t, s.OK(root, http.MethodGet, "/api/option/", nil)); len(options) != 0 {
		t.Fatalf("new database should have no options: %v", options)
	}
	s.Fail(root, http.MethodPut, "/api/option/", map[string]any{"key": "  ", "value": "x"})

	s.OK(root, http.MethodPut, "/api/option/", map[string]any{"key": "Notice", "value": "维护公告"})
	if got := testhelper.Decode[string](t, s.OK(testhelper.Actor{}, http.MethodGet, "/api/notice", nil)); got != "维护公告" {
		t.Fatalf("notice = %q", got)
	}
	for path, want := range map[string]string{
		"/api/about":             "关于内容",
		"/api/home_page_content": "首页内容",
		"/api/user-agreement":    "协议内容",
		"/api/privacy-policy":    "隐私内容",
	} {
		key := map[string]string{
			"/api/about":             "About",
			"/api/home_page_content": "HomePageContent",
			"/api/user-agreement":    "UserAgreement",
			"/api/privacy-policy":    "PrivacyPolicy",
		}[path]
		s.OK(root, http.MethodPut, "/api/option/", map[string]any{"key": key, "value": want})
		if got := testhelper.Decode[string](t, s.OK(testhelper.Actor{}, http.MethodGet, path, nil)); got != want {
			t.Fatalf("%s = %q, want %q", path, got, want)
		}
	}

	s.OK(root, http.MethodPut, "/api/option/", map[string]any{"key": "TurnstileSecretKey", "value": "real-secret"})
	options := testhelper.Decode[map[string]string](t, s.OK(root, http.MethodGet, "/api/option/", nil))
	if options["TurnstileSecretKey"] != "******" {
		t.Fatalf("secret should be masked: %v", options["TurnstileSecretKey"])
	}
	s.OK(root, http.MethodPut, "/api/option/", map[string]any{"key": "TurnstileSecretKey", "value": "******"})
	if value, _ := model.GetOptionValue("TurnstileSecretKey"); value != "real-secret" {
		t.Fatalf("masked value must not overwrite the stored secret: %q", value)
	}

	s.OK(root, http.MethodPut, "/api/option/", map[string]any{"key": "console_setting.api_info", "value": `[{"id":1}]`})
	migrated := testhelper.Decode[map[string][]string](t, s.OK(root, http.MethodPost, "/api/option/migrate_console_setting", nil))
	if len(migrated["migrated"]) != 1 {
		t.Fatalf("first migration: %v", migrated)
	}
	migrated = testhelper.Decode[map[string][]string](t, s.OK(root, http.MethodPost, "/api/option/migrate_console_setting", nil))
	if len(migrated["migrated"]) != 0 || len(migrated["skipped"]) == 0 {
		t.Fatalf("second migration should be a no-op: %v", migrated)
	}
	if value, _ := model.GetOptionValue("ApiInfo"); value != `[{"id":1}]` {
		t.Fatalf("migrated value: %q", value)
	}
}

func TestSetupAndStatusAPI(t *testing.T) {
	s := testhelper.NewTestServer(t)
	testhelper.ResetGlobalSettings(t)

	setup := testhelper.Decode[map[string]any](t, s.OK(testhelper.Actor{}, http.MethodGet, "/api/setup", nil))
	if setup["initialized"] != false || setup["root_user"] != false {
		t.Fatalf("fresh database should be uninitialized: %v", setup)
	}

	s.Fail(testhelper.Actor{}, http.MethodPost, "/api/setup", map[string]any{"username": "root", "password": "short"})
	s.Fail(testhelper.Actor{}, http.MethodPost, "/api/setup", map[string]any{"username": "root", "password": "password123", "confirm_password": "password124"})
	s.OK(testhelper.Actor{}, http.MethodPost, "/api/setup", map[string]any{"username": "root", "password": "password123"})
	setup = testhelper.Decode[map[string]any](t, s.OK(testhelper.Actor{}, http.MethodGet, "/api/setup", nil))
	if setup["initialized"] != true {
		t.Fatalf("should be initialized: %v", setup)
	}
	s.Fail(testhelper.Actor{}, http.MethodPost, "/api/setup", map[string]any{"username": "root2", "password": "password123"})
	if _, err := model.ValidateUserLogin("root", "password123"); err != nil {
		t.Fatalf("root login should work: %v", err)
	}

	status := testhelper.Decode[map[string]any](t, s.OK(testhelper.Actor{}, http.MethodGet, "/api/status", nil))
	if status["version"] != common.Version || status["setup"] != true {
		t.Fatalf("status: %v", status)
	}
	if _, ok := status["turnstile_site_key"]; !ok {
		t.Fatalf("status should expose turnstile_site_key: %v", status)
	}
}

func TestStatusSelfCheck(t *testing.T) {
	s := testhelper.NewTestServer(t)
	user := s.CreateUser("alice", common.RoleCommonUser)
	admin := s.CreateUser("admin", common.RoleAdminUser)

	s.Fail(user, http.MethodGet, "/api/status/test", nil)
	result := testhelper.Decode[map[string]any](t, s.OK(admin, http.MethodGet, "/api/status/test", nil))
	if result["success"] != true {
		t.Fatalf("self check should pass: %v", result)
	}
}
