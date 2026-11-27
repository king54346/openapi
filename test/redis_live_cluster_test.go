package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"openapi/common"

	"github.com/redis/go-redis/v9"
)

func freeTCPPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("allocate free port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port
}

func newNodeClient(t *testing.T, base string) *LiveClient {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar: %v", err)
	}
	return &LiveClient{
		t:    t,
		base: strings.TrimRight(base, "/"),
		http: &http.Client{
			Jar:     jar,
			Timeout: 15 * time.Second,
		},
	}
}

func waitNodeReady(t *testing.T, name, base string, logBuf *bytes.Buffer) {
	t.Helper()
	client := &http.Client{Timeout: 1 * time.Second}
	deadline := time.Now().Add(25 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := client.Get(base + "/api/status")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(150 * time.Millisecond)
	}
	t.Fatalf("%s (%s) did not become ready in time.\nLogs:\n%s", name, base, logBuf.String())
}

func cleanupPrefixedKeys(ctx context.Context, rdb *redis.Client, prefix string) int {
	var deleted int
	var cursor uint64
	for {
		keys, next, err := rdb.Scan(ctx, cursor, prefix+"*", 200).Result()
		if err != nil {
			break
		}
		if len(keys) > 0 {
			n, _ := rdb.Del(ctx, keys...).Result()
			deleted += int(n)
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
	return deleted
}

// TestLiveRedisTwoNodeClusterE2E 连接真实 Redis 服务器并拉起两个独立的网关进程（Node A / Node B），
// 端到端实测分布式锁、Session 共享/注销/强踢、L1+Pub/Sub 毫秒级缓存同步、跨节点亲和性、多 Key 零写库顺序轮询与集群连续失败熔断。
func TestLiveRedisTwoNodeClusterE2E(t *testing.T) {
	redisURL := strings.TrimSpace(os.Getenv("LIVE_REDIS_CONN_STRING"))
	if redisURL == "" {
		t.Skip("skipping live Redis cluster E2E test: set LIVE_REDIS_CONN_STRING to run")
	}

	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		t.Fatalf("parse LIVE_REDIS_CONN_STRING: %v", err)
	}
	rdb := redis.NewClient(opt)
	// 注意：t.Cleanup 按后进先出（LIFO）执行，先注册 rdb.Close()，后注册清理逻辑，确保清理完成后再关闭连接
	t.Cleanup(func() { _ = rdb.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pingStart := time.Now()
	pong, err := rdb.Ping(ctx).Result()
	pingRTT := time.Since(pingStart)
	if err != nil {
		t.Fatalf("PING real Redis failed: %v", err)
	}
	infoServer, _ := rdb.Info(ctx, "server").Result()
	redisVer := "unknown"
	for _, line := range strings.Split(infoServer, "\n") {
		if strings.HasPrefix(line, "redis_version:") {
			redisVer = strings.TrimSpace(strings.TrimPrefix(line, "redis_version:"))
			break
		}
	}
	t.Logf("[Real Redis Connected] addr=%s db=%d pong=%s rtt=%s version=%s", opt.Addr, opt.DB, pong, pingRTT.Round(100*time.Microsecond), redisVer)

	// 顺带清理任何历史中断残留的 openapi_livetest_* 测试键（不影响正式业务键 openapi:*）
	if stale := cleanupPrefixedKeys(ctx, rdb, "openapi_livetest_"); stale > 0 {
		t.Logf("[Pre-Clean] removed %d stale openapi_livetest_* keys from previous run", stale)
	}

	keyPrefix := fmt.Sprintf("openapi_livetest_%d:", time.Now().UnixNano())
	t.Setenv("REDIS_KEY_PREFIX", keyPrefix)
	t.Cleanup(func() {
		cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanCancel()
		n := cleanupPrefixedKeys(cleanCtx, rdb, keyPrefix)
		t.Logf("[Cleanup] removed %d temporary test keys under prefix %q", n, keyPrefix)
	})

	// Step 0: 直接在真实 Redis 上验证 Lua 分布式锁与自动续租
	lockRel, acquired := common.AcquireLockWithRenewOnClient(context.Background(), rdb, true, "verify:renew_lock", 1500*time.Millisecond)
	if !acquired {
		t.Fatal("expected AcquireLockWithRenewOnClient to succeed on real Redis")
	}
	if _, okSecond := common.AcquireLockOnClient(context.Background(), rdb, true, "verify:renew_lock", time.Second); okSecond {
		t.Fatal("second caller must not acquire held Redis lock")
	}
	lockRel()
	if relAfter, okAfter := common.AcquireLockOnClient(context.Background(), rdb, true, "verify:renew_lock", time.Second); !okAfter {
		t.Fatal("expected lock to be re-acquirable immediately after Lua release")
	} else {
		relAfter()
	}
	t.Logf("[PASS] Step 0: Lua distributed lock acquire / renew / release verified on real Redis")

	// 准备模拟上游 OpenAI 服务：记录每次转发收到的 Bearer Key，并支持对特定 Key 返回 503 模拟上游故障
	var upstreamMu sync.Mutex
	var receivedAuthKeys []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		upstreamMu.Lock()
		receivedAuthKeys = append(receivedAuthKeys, auth)
		upstreamMu.Unlock()

		if strings.HasPrefix(auth, "fail503-") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":{"message":"simulated upstream 503","type":"server_error"}}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl-live",
			"object":"chat.completion",
			"created":1700000000,
			"model":"gpt-4o-mini",
			"choices":[{"index":0,"message":{"role":"assistant","content":"pong"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":5,"completion_tokens":2,"total_tokens":7}
		}`))
	}))
	defer upstream.Close()

	// Step 1: 编译网关二进制并同时启动两个独立进程 Node A 与 Node B
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "openapi_cluster_node.exe")
	repoRoot, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	buildCmd := exec.Command("go", "build", "-o", binPath, "./cmd")
	buildCmd.Dir = repoRoot
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("build ./cmd failed: %v\n%s", err, string(out))
	}

	sqliteFile := filepath.ToSlash(filepath.Join(tmpDir, "cluster.db")) + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(10000)"
	portA := freeTCPPort(t)
	portB := freeTCPPort(t)
	baseA := fmt.Sprintf("http://127.0.0.1:%d", portA)
	baseB := fmt.Sprintf("http://127.0.0.1:%d", portB)
	sharedSecret := "live_redis_cluster_secret_987654321"

	startNode := func(name string, port int) (*exec.Cmd, *bytes.Buffer) {
		nodeDir := filepath.Join(tmpDir, name)
		_ = os.MkdirAll(nodeDir, 0o755)
		var buf bytes.Buffer
		cmd := exec.Command(binPath)
		cmd.Dir = nodeDir
		cmd.Stdout = &buf
		cmd.Stderr = &buf
		cmd.Env = append(os.Environ(),
			"PORT="+strconv.Itoa(port),
			"SQL_DSN=",
			"SQLITE_PATH="+sqliteFile,
			"REDIS_CONN_STRING="+redisURL,
			"REDIS_KEY_PREFIX="+keyPrefix,
			"SESSION_SECRET="+sharedSecret,
			"SYNC_FREQUENCY=300", // 故意设为 300s，验证跨节点同步靠的是 Redis Pub/Sub 毫秒级广播而非轮询
			"RETRY_TIMES=0",
			"CHANNEL_AUTO_DISABLE_ENABLED=true",
			"CHANNEL_AUTO_DISABLE_THRESHOLD=3",
			"CRON_ENABLED=true",
			"UPDATE_TASK=true",
		)
		if err := cmd.Start(); err != nil {
			t.Fatalf("start %s: %v", name, err)
		}
		t.Cleanup(func() {
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
				_, _ = cmd.Process.Wait()
			}
		})
		return cmd, &buf
	}

	// 双节点并发冷启动（同时验证 db:migrate 分布式锁在真实 Redis 下零冲突）
	_, logA := startNode("nodeA", portA)
	_, logB := startNode("nodeB", portB)
	waitNodeReady(t, "nodeA", baseA, logA)
	waitNodeReady(t, "nodeB", baseB, logB)
	t.Logf("[PASS] Step 1: Two peer gateway processes started concurrently (%s, %s) with db:migrate lock", baseA, baseB)

	// Step 2: 初始化管理员并在双节点验证 /api/status/test 内置 Redis 健康检查
	nodeAAdmin := newNodeClient(t, baseA)
	nodeAAdmin.MustOK(http.MethodPost, "/api/setup", map[string]string{
		"username":         "admin",
		"password":         "AdminPass123!",
		"confirm_password": "AdminPass123!",
		"display_name":     "Cluster Root",
	})
	nodeAAdmin.Login("admin", "AdminPass123!")

	statusCheckA := mustDecode[struct {
		Success bool `json:"success"`
		Items   []struct {
			Name    string `json:"name"`
			Success bool   `json:"success"`
			Message string `json:"message"`
		} `json:"items"`
	}](t, nodeAAdmin.MustOK(http.MethodGet, "/api/status/test", nil))
	hasRedisItem := false
	for _, it := range statusCheckA.Items {
		if it.Name == "redis" && it.Success {
			hasRedisItem = true
		}
	}
	if !statusCheckA.Success || !hasRedisItem {
		t.Fatalf("expected /api/status/test to report healthy redis item, got %+v", statusCheckA)
	}
	t.Logf("[PASS] Step 2: /api/status/test confirmed Redis health check = true")

	// Step 3: 跨节点共享 Session、异地注销与管理员强制踢下线
	sessKeys, _, err := rdb.Scan(context.Background(), 0, keyPrefix+"session:*", 50).Result()
	if err != nil || len(sessKeys) == 0 {
		t.Fatalf("expected session key stored in real Redis under %ssession:*, got %v (err=%v)", keyPrefix, sessKeys, err)
	}

	// 将 Node A 登录后拿到的 Cookie 直接带去访问 Node B（Node B 从未登录过）
	uURL, _ := http.NewRequest(http.MethodGet, baseA+"/api/user/self", nil)
	cookiesA := nodeAAdmin.http.Jar.Cookies(uURL.URL)
	if len(cookiesA) == 0 {
		t.Fatal("expected session cookie on Node A client")
	}
	rawCookieHeader := cookiesA[0].Name + "=" + cookiesA[0].Value

	nodeBWithACookie := newNodeClient(t, baseB)
	nodeBWithACookie.userID = nodeAAdmin.userID
	codeSelfB, respSelfB, rawSelfB := nodeBWithACookie.DoAPI(http.MethodGet, "/api/user/self", nil, map[string]string{
		"Cookie": rawCookieHeader,
	})
	if codeSelfB != http.StatusOK || !respSelfB.Success {
		t.Fatalf("Node A session cookie failed on Node B: HTTP %d %s", codeSelfB, string(rawSelfB))
	}

	// 创建测试用户 bob -> 在 Node A 登录 -> 验证 Node B 可用 -> 在 Node A 禁用 bob -> 验证 Node B 立即被踢下线
	bob := mustDecode[struct {
		ID int `json:"id"`
	}](t, nodeAAdmin.MustOK(http.MethodPost, "/api/user/", map[string]any{
		"username":     "bob_cluster",
		"password":     "BobPass123!",
		"display_name": "Bob",
	}))
	bobOnA := newNodeClient(t, baseA)
	bobOnA.Login("bob_cluster", "BobPass123!")
	bobCookies := bobOnA.http.Jar.Cookies(uURL.URL)
	bobCookieHdr := bobCookies[0].Name + "=" + bobCookies[0].Value

	bobOnB := newNodeClient(t, baseB)
	bobOnB.userID = bob.ID
	codeBob1, respBob1, _ := bobOnB.DoAPI(http.MethodGet, "/api/user/self", nil, map[string]string{"Cookie": bobCookieHdr})
	if codeBob1 != http.StatusOK || !respBob1.Success {
		t.Fatalf("bob logged in on Node A should succeed on Node B before disable")
	}

	// 管理员在 Node A 禁用 bob（触发 RevokeUserSessions + user:invalidate Pub/Sub）
	nodeAAdmin.MustOK(http.MethodPost, "/api/user/manage", map[string]any{
		"id":     bob.ID,
		"action": "disable",
	})
	codeBobAfter, respBobAfter, _ := bobOnB.DoAPI(http.MethodGet, "/api/user/self", nil, map[string]string{"Cookie": bobCookieHdr})
	if codeBobAfter == http.StatusOK && respBobAfter.Success {
		t.Fatal("expected bob's session on Node B to be revoked immediately after disable on Node A")
	}

	// 测试跨节点主动注销：在 Node A 新登一个会话，去 Node B 调 /api/user/logout，回 Node A 立刻 401
	tempSessionA := newNodeClient(t, baseA)
	tempSessionA.Login("admin", "AdminPass123!")
	tempCookie := tempSessionA.http.Jar.Cookies(uURL.URL)[0]
	tempCookieHdr := tempCookie.Name + "=" + tempCookie.Value

	logoutOnB := newNodeClient(t, baseB)
	logoutOnB.userID = tempSessionA.userID
	codeLogout, respLogout, rawLogout := logoutOnB.DoAPI(http.MethodGet, "/api/user/logout", nil, map[string]string{"Cookie": tempCookieHdr})
	if codeLogout != http.StatusOK || !respLogout.Success {
		t.Fatalf("logout on Node B failed: HTTP %d %s", codeLogout, string(rawLogout))
	}
	checkBackOnA := newNodeClient(t, baseA)
	checkBackOnA.userID = tempSessionA.userID
	codeAfterLogout, respAfterLogout, _ := checkBackOnA.DoAPI(http.MethodGet, "/api/user/self", nil, map[string]string{"Cookie": tempCookieHdr})
	if codeAfterLogout != http.StatusUnauthorized || respAfterLogout.Success {
		t.Fatalf("expected 401 on Node A right after logout on Node B, got HTTP %d", codeAfterLogout)
	}
	t.Logf("[PASS] Step 3: Cross-node Redis session sharing, remote logout, and forced RevokeUserSessions verified")

	// Step 4: L1 本地缓存 + Redis Pub/Sub 跨进程毫秒级失效（Channel & Token）
	nodeBAdmin := newNodeClient(t, baseB)
	nodeBAdmin.Login("admin", "AdminPass123!")

	tk := mustDecode[struct {
		ID  int    `json:"id"`
		Key string `json:"key"`
	}](t, nodeAAdmin.MustOK(http.MethodPost, "/api/token/", map[string]any{
		"name":            "cluster_token",
		"unlimited_quota": true,
		"expired_time":    -1,
	}))

	// 在 Node A 新增渠道，触发 channel:refresh 广播 -> Node B 无需等待 300s 即可转发该模型
	nodeAAdmin.MustOK(http.MethodPost, "/api/channel/", map[string]any{
		"mode": "single",
		"channel": map[string]any{
			"name":     "pubsub-ch",
			"type":     1,
			"base_url": upstream.URL,
			"key":      "pubsub-key-1",
			"models":   "live-pubsub-model",
			"auto_ban": 0,
		},
	})

	callChat := func(base, apiKey, modelName string, extraHeaders map[string]string) (int, []byte) {
		bodyBytes, _ := json.Marshal(map[string]any{
			"model":    modelName,
			"messages": []map[string]string{{"role": "user", "content": "hi"}},
		})
		req, _ := http.NewRequest(http.MethodPost, base+"/v1/chat/completions", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer sk-"+apiKey)
		for k, v := range extraHeaders {
			req.Header.Set(k, v)
		}
		resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
		if err != nil {
			t.Fatalf("POST %s/v1/chat/completions failed: %v", base, err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, raw
	}

	// 等待几十毫秒让 Pub/Sub 推送抵达 Node B，然后分别打热 Node A 与 Node B 的本地 tokenCache
	time.Sleep(120 * time.Millisecond)
	if codeA, rawA := callChat(baseA, tk.Key, "live-pubsub-model", nil); codeA != http.StatusOK {
		t.Fatalf("Node A chat failed: %d %s", codeA, string(rawA))
	}
	if codeB, rawB := callChat(baseB, tk.Key, "live-pubsub-model", nil); codeB != http.StatusOK {
		t.Fatalf("Node B chat failed (channel:refresh Pub/Sub did not sync): %d %s", codeB, string(rawB))
	}

	// 在 Node A 禁用该 Token（发布 token:invalidate）-> 验证 Node B 在 500ms 内立即拒绝（不等 300s TTL）
	nodeAAdmin.MustOK(http.MethodPut, "/api/token/?status_only=true", map[string]any{
		"id":     tk.ID,
		"status": 2,
	})
	invalidatedOnB := false
	invStart := time.Now()
	for time.Since(invStart) < 2*time.Second {
		codeB, _ := callChat(baseB, tk.Key, "live-pubsub-model", nil)
		if codeB == http.StatusUnauthorized {
			invalidatedOnB = true
			break
		}
		time.Sleep(30 * time.Millisecond)
	}
	if !invalidatedOnB {
		t.Fatal("Node B L1 tokenCache was NOT invalidated via Redis Pub/Sub after token disable on Node A")
	}
	t.Logf("[PASS] Step 4: L1 tokenCache + channelCache invalidated across OS processes via Redis Pub/Sub in %s", time.Since(invStart).Round(time.Millisecond))

	// 重新启用 Token 供后续步骤复用
	nodeAAdmin.MustOK(http.MethodPut, "/api/token/?status_only=true", map[string]any{
		"id":     tk.ID,
		"status": 1,
	})
	time.Sleep(100 * time.Millisecond)

	// Step 5: 跨节点会话亲和性（Option Pub/Sub 同步 + Redis 共享会话落点）
	nodeAAdmin.MustOK(http.MethodPut, "/api/option/", map[string]string{
		"key":   "ChannelAffinityEnabled",
		"value": "true",
	})
	// 验证 Node B 通过 option:refresh 实时感知到亲和性开启
	affEnabledOnB := false
	for range 20 {
		statsB := mustDecode[map[string]any](t, nodeBAdmin.MustOK(http.MethodGet, "/api/option/channel_affinity_cache", nil))
		if statsB["enabled"] == true {
			affEnabledOnB = true
			break
		}
		time.Sleep(40 * time.Millisecond)
	}
	if !affEnabledOnB {
		t.Fatal("Node B did not receive option:refresh Pub/Sub for ChannelAffinityEnabled=true")
	}

	for _, keyName := range []string{"aff-key-1", "aff-key-2"} {
		nodeAAdmin.MustOK(http.MethodPost, "/api/channel/", map[string]any{
			"mode": "single",
			"channel": map[string]any{
				"name":     "ch-" + keyName,
				"type":     1,
				"base_url": upstream.URL,
				"key":      keyName,
				"models":   "live-affinity-model",
				"auto_ban": 0,
			},
		})
	}
	time.Sleep(120 * time.Millisecond)

	upstreamMu.Lock()
	receivedAuthKeys = nil
	upstreamMu.Unlock()

	sessHeaders := map[string]string{"X-Session-Id": "conv-real-redis-888"}
	// 第 1 轮打向 Node A（distributor 在响应写出后向远端 Redis 落盘亲和记录，留出 1 个 RTT 确保落盘完成）
	if code, raw := callChat(baseA, tk.Key, "live-affinity-model", sessHeaders); code != http.StatusOK {
		t.Fatalf("affinity round 1 on Node A failed: %d %s", code, string(raw))
	}
	time.Sleep(80 * time.Millisecond)
	// 第 2~7 轮全部打向 Node B，必须 100% 从远端 Redis 命中同一个上游渠道 Key
	for i := range 6 {
		if code, raw := callChat(baseB, tk.Key, "live-affinity-model", sessHeaders); code != http.StatusOK {
			t.Fatalf("affinity round %d on Node B failed: %d %s", i+2, code, string(raw))
		}
	}
	upstreamMu.Lock()
	affPicked := append([]string(nil), receivedAuthKeys...)
	upstreamMu.Unlock()
	if len(affPicked) != 7 {
		t.Fatalf("expected 7 affinity upstream calls, got %v", affPicked)
	}
	for i := 1; i < len(affPicked); i++ {
		if affPicked[i] != affPicked[0] {
			t.Fatalf("cross-node session affinity broken! round 1 on Node A chose %q, round %d on Node B chose %q (all=%v)", affPicked[0], i+1, affPicked[i], affPicked)
		}
	}
	affStats := mustDecode[map[string]any](t, nodeBAdmin.MustOK(http.MethodGet, "/api/option/channel_affinity_cache", nil))
	if hits, _ := affStats["hits"].(float64); hits < 6 {
		t.Fatalf("expected Redis cluster affinity hits >= 6, got %v", affStats)
	}
	t.Logf("[PASS] Step 5: Cross-node Session Affinity locked all 7 turns across Node A -> Node B onto %q (stats=%v)", affPicked[0], affStats)

	// 关闭亲和性以免干扰下一步轮询测试
	nodeAAdmin.MustOK(http.MethodPut, "/api/option/", map[string]string{
		"key":   "ChannelAffinityEnabled",
		"value": "false",
	})
	time.Sleep(120 * time.Millisecond)

	// Step 6: 多 Key 渠道跨节点严格顺序轮询 + 零写库验证
	pollAdd := mustDecode[struct {
		IDs []int `json:"ids"`
	}](t, nodeAAdmin.MustOK(http.MethodPost, "/api/channel/", map[string]any{
		"mode":           "multi_to_single",
		"multi_key_mode": "polling",
		"channel": map[string]any{
			"name":     "live-multi-poll",
			"type":     1,
			"base_url": upstream.URL,
			"key":      "poll-k1\npoll-k2\npoll-k3",
			"models":   "live-poll-model",
			"auto_ban": 0,
		},
	}))
	pollChID := pollAdd.IDs[0]
	time.Sleep(120 * time.Millisecond)

	upstreamMu.Lock()
	receivedAuthKeys = nil
	upstreamMu.Unlock()

	// Node A 与 Node B 交替发起 6 次转发请求
	nodes := []string{baseA, baseB, baseA, baseB, baseA, baseB}
	for i, targetBase := range nodes {
		if code, raw := callChat(targetBase, tk.Key, "live-poll-model", nil); code != http.StatusOK {
			t.Fatalf("multi-key poll call #%d on %s failed: %d %s", i+1, targetBase, code, string(raw))
		}
	}
	upstreamMu.Lock()
	pollPicked := append([]string(nil), receivedAuthKeys...)
	upstreamMu.Unlock()

	wantPoll := []string{"poll-k1", "poll-k2", "poll-k3", "poll-k1", "poll-k2", "poll-k3"}
	for i := range wantPoll {
		if i >= len(pollPicked) || pollPicked[i] != wantPoll[i] {
			t.Fatalf("expected global sequential polling %v across Node A/B, got %v", wantPoll, pollPicked)
		}
	}
	redisCursorVal, err := rdb.Get(context.Background(), fmt.Sprintf("%schannel:polling:%d", keyPrefix, pollChID)).Result()
	if err != nil || redisCursorVal != "6" {
		t.Fatalf("expected Redis polling cursor = 6, got %q (err=%v)", redisCursorVal, err)
	}
	chDetail := mustDecode[struct {
		ChannelInfo struct {
			MultiKeyPollingIndex int `json:"multi_key_polling_index"`
		} `json:"channel_info"`
	}](t, nodeAAdmin.MustOK(http.MethodGet, fmt.Sprintf("/api/channel/%d", pollChID), nil))
	if chDetail.ChannelInfo.MultiKeyPollingIndex != 0 {
		t.Fatalf("expected zero DB writes to multi_key_polling_index (0), got %d", chDetail.ChannelInfo.MultiKeyPollingIndex)
	}
	t.Logf("[PASS] Step 6: Cross-node sequential polling sequence=%v, Redis cursor=%s, DB polling_index=0 (zero DB writes)", pollPicked, redisCursorVal)

	// Step 7: 集群级连续失败自动封禁（Node A 失败 2 次 + Node B 失败第 3 次 -> 触发自动禁用并广播下架）
	banAdd := mustDecode[struct {
		IDs []int `json:"ids"`
	}](t, nodeAAdmin.MustOK(http.MethodPost, "/api/channel/", map[string]any{
		"mode": "single",
		"channel": map[string]any{
			"name":     "live-autoban-ch",
			"type":     1,
			"base_url": upstream.URL,
			"key":      "fail503-key",
			"models":   "live-autoban-model",
			"auto_ban": 1,
		},
	}))
	banChID := banAdd.IDs[0]
	time.Sleep(120 * time.Millisecond)

	// 前 2 次失败发给 Node A（阈值=3，此时不应封禁）
	_, _ = callChat(baseA, tk.Key, "live-autoban-model", nil)
	_, _ = callChat(baseA, tk.Key, "live-autoban-model", nil)

	failCountAfter2, _ := rdb.Get(context.Background(), fmt.Sprintf("%shealth:fail:%d", keyPrefix, banChID)).Result()
	if failCountAfter2 != "2" {
		t.Fatalf("expected Redis failure counter = 2 after 2 errors on Node A, got %q", failCountAfter2)
	}
	chBefore3rd := mustDecode[struct {
		Status int `json:"status"`
	}](t, nodeAAdmin.MustOK(http.MethodGet, fmt.Sprintf("/api/channel/%d", banChID), nil))
	if chBefore3rd.Status != 1 {
		t.Fatalf("channel should still be enabled (status=1) at 2/3 failures, got %d", chBefore3rd.Status)
	}

	// 第 3 次失败发给 Node B -> 跨节点累加达到阈值 3，立即自动禁用渠道（status=3）
	_, _ = callChat(baseB, tk.Key, "live-autoban-model", nil)

	chAfter3rd := mustDecode[struct {
		Status int `json:"status"`
	}](t, nodeAAdmin.MustOK(http.MethodGet, fmt.Sprintf("/api/channel/%d", banChID), nil))
	if chAfter3rd.Status != 3 {
		t.Fatalf("expected channel auto-disabled (status=3) after 3rd failure on Node B, got %d", chAfter3rd.Status)
	}
	t.Logf("[PASS] Step 7: Cluster-wide failure counter reached 3 across Node A(2) + Node B(1) -> channel #%d auto-disabled (status=%d)", banChID, chAfter3rd.Status)
}
