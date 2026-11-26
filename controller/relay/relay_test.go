package relay_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"openapi/common"
	"openapi/constant"
	relayctrl "openapi/controller/relay"
	"openapi/dto"
	"openapi/model"
	relaysvc "openapi/service/relay"
	"openapi/types"

	"github.com/glebarez/sqlite"
	gin "github.com/king54346/gin-tiny"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupModelCache(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := model.InitDB(db, nil); err != nil {
		t.Fatal(err)
	}
	for _, ch := range []*model.Channel{
		{Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Key: "k", Name: "c1", Models: "a,b"},
		{Type: constant.ChannelTypeDeepSeek, Status: common.ChannelStatusEnabled, Key: "k", Name: "c2", Models: "b,c"},
		{Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Key: "k", Name: "c3", Models: "d"},
		{Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusManuallyDisabled, Key: "k", Name: "c4", Models: "disabled-model"},
	} {
		if err := ch.Insert(); err != nil {
			t.Fatal(err)
		}
	}
	if err := model.InitChannelCache(); err != nil {
		t.Fatal(err)
	}
}

func modelContext(path string, limits map[string]bool, limitType int) (gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.SetRequest(httptest.NewRequest(http.MethodGet, path, nil))
	if limits != nil {
		c.Set("token_model_limit_enabled", true)
		c.Set("token_model_limit", limits)
		c.Set("token_model_limit_type", limitType)
	}
	return c, w
}

func TestRelayRejectsUnknownFormat(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.SetRequest(httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader("{}")))
	relayctrl.Relay(c, types.RelayFormat("bogus"))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	var body struct {
		Error types.OpenAIError `json:"error"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil || body.Error.Message == "" {
		t.Fatalf("expected OpenAI error body, err=%v body=%+v", err, body)
	}
}

func TestUsableModels(t *testing.T) {
	setupModelCache(t)
	cases := []struct {
		name      string
		limits    map[string]bool
		limitType int
		want      []string
	}{
		{"no limit", nil, 0, []string{"a", "b", "c", "d"}},
		{"whitelist", map[string]bool{"a": true, "c": true, "x": true}, 0, []string{"a", "c"}},
		{"empty whitelist allows nothing", map[string]bool{}, 0, []string{}},
		{"blacklist", map[string]bool{"b": true}, 1, []string{"a", "c", "d"}},
		{"empty blacklist allows all", map[string]bool{}, 1, []string{"a", "b", "c", "d"}},
	}
	for _, tc := range cases {
		c, _ := modelContext("/v1/models", tc.limits, tc.limitType)
		got := relaysvc.UsableModels(c)
		if len(got) == 0 && len(tc.want) == 0 {
			continue
		}
		if !slices.Equal(got, tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestListAndRetrieveModels(t *testing.T) {
	setupModelCache(t)

	c, w := modelContext("/v1/models", map[string]bool{"b": true}, 1)
	relayctrl.ListModels(c, constant.ChannelTypeOpenAI)
	var list struct {
		Object string                 `json:"object"`
		Data   []relaysvc.OpenAIModel `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, len(list.Data))
	for _, m := range list.Data {
		ids = append(ids, m.Id)
	}
	if list.Object != "list" || !slices.Equal(ids, []string{"a", "c", "d"}) {
		t.Fatalf("list = %s", w.Body.String())
	}

	// Gemini /v1beta/models format
	gc, gw := modelContext("/v1beta/models", nil, 0)
	relayctrl.ListModels(gc, constant.ChannelTypeGemini)
	var gemList dto.GeminiModelListResponse
	if err := json.Unmarshal(gw.Body.Bytes(), &gemList); err != nil || len(gemList.Models) != 4 || gemList.Models[0].Name != "models/a" {
		t.Fatalf("gemini list = %s (%v)", gw.Body.String(), err)
	}

	for _, tc := range []struct {
		model string
		code  int
	}{{"a", http.StatusOK}, {"b", http.StatusNotFound}, {"d", http.StatusOK}} {
		c, w := modelContext("/v1/models/"+tc.model, map[string]bool{"b": true}, 1)
		c.AddParam("model", tc.model)
		relayctrl.RetrieveModel(c, constant.ChannelTypeOpenAI)
		if w.Code != tc.code {
			t.Errorf("retrieve %s: got %d, want %d (%s)", tc.model, w.Code, tc.code, w.Body.String())
		}
	}
}
