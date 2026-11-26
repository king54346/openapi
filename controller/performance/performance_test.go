package performance_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"openapi/common"
	"openapi/controller/common/testhelper"
)

func TestPerformanceAPI(t *testing.T) {
	s := testhelper.NewTestServer(t)
	root := s.CreateUser("root", common.RoleRootUser)
	admin := s.CreateUser("admin", common.RoleAdminUser)

	s.Fail(admin, http.MethodGet, "/api/performance/stats", nil)
	stats := testhelper.Decode[map[string]json.RawMessage](t, s.OK(root, http.MethodGet, "/api/performance/stats", nil))
	for _, key := range []string{"cache_stats", "memory_stats", "disk_cache_info", "disk_space_info", "config"} {
		if _, ok := stats[key]; !ok {
			t.Errorf("stats missing %s", key)
		}
	}
	for _, path := range []string{"/api/performance/reset_stats", "/api/performance/gc"} {
		s.OK(root, http.MethodPost, path, nil)
	}
	s.OK(root, http.MethodDelete, "/api/performance/disk_cache", nil)
}
