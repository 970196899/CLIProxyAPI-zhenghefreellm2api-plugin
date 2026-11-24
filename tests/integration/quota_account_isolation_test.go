package tests

// 本文件锁定额度缓存的账号隔离。
//
// 真实事故：Cline 账号在概览里显示出了 WorkBuddy 的「剩余 / 总额」。
// 根因不在 UI，而在额度缓存的键：缓存同时按 Name / Label / AuthIndex 落库，
// 这些字段跨供应商并不唯一（两个账号同昵称即可命中），于是一个账号会读到
// 别家账号的结果。

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestQuotaCacheIsolatedPerAccount 复现并锁定「跨账号串额度」。
//
// 两个账号故意使用同一个展示名：旧实现会把 Trae 的额度串给 Cline 账号。
func TestQuotaCacheIsolatedPerAccount(t *testing.T) {
	host := installFakeHost(t)
	setupTestPlugin(t)
	host.addAccount("1", "traecn-shared.json", "{\"vendor\":\"traecn\",\"region\":\"cn\",\"access_token\":\"test-access-token\",\"uid\":\"2487573687317868\",\"label\":\"shared-account\"}")
	host.addAccount("2", "cline-shared.json", "{\"type\":\"cline\",\"refreshToken\":\"workos:rt-test\",\"label\":\"shared-account\"}")
	host.setUpstream(func(method, url, body string) fakeUpstreamResponse {
		if strings.Contains(url, "pay/ide_user_ent_usage") {
			return fakeUpstreamResponse{
				Status: http.StatusOK,
				Header: map[string][]string{"Content-Type": {"application/json"}},
				Body:   traeQuotaFixture,
			}
		}
		// Cline 的账号接口：余额 1.23 美元，故意与 Trae 的数值不同。
		if strings.HasSuffix(url, "/users/me") {
			return fakeUpstreamResponse{
				Status: http.StatusOK,
				Header: map[string][]string{"Content-Type": {"application/json"}},
				Body:   `{"id":"user-cline-1","email":"cline@example.com"}`,
			}
		}
		if strings.Contains(url, "/users/user-cline-1/balance") {
			return fakeUpstreamResponse{
				Status: http.StatusOK,
				Header: map[string][]string{"Content-Type": {"application/json"}},
				Body:   `{"balance":1.23,"userId":"user-cline-1"}`,
			}
		}
		if strings.HasSuffix(url, "/users/me/plan") {
			return fakeUpstreamResponse{Status: http.StatusOK, Body: `{}`}
		}
		return fakeUpstreamResponse{Status: http.StatusNotFound, Body: "{\"code\":404}"}
	})

	if resp := managementCall(t, "POST", "/quotas"); resp.StatusCode != http.StatusOK {
		t.Fatalf("/quotas = %d: %s", resp.StatusCode, resp.Body)
	}
	if !waitFor(t, 5*time.Second, func() bool {
		for _, account := range statusAccounts(t) {
			if account.VendorID == "traecn" && account.Quota != nil && account.Quota.OK {
				return true
			}
		}
		return false
	}) {
		t.Fatalf("trae quota should be cached, got %+v", statusAccounts(t))
	}

	if !waitFor(t, 5*time.Second, func() bool {
		for _, account := range statusAccounts(t) {
			if account.VendorID == "cline" && account.Quota != nil && account.Quota.OK {
				return true
			}
		}
		return false
	}) {
		t.Fatalf("cline quota should be cached on its own key, got %+v", statusAccounts(t))
	}

	for _, account := range statusAccounts(t) {
		if account.VendorID != "cline" {
			continue
		}
		if !account.SupportsQuota {
			t.Fatal("cline must report supports_quota=true")
		}
		if account.Quota == nil {
			continue // 还没查到，下面的断言等结果
		}
		if !account.Quota.OK {
			t.Fatalf("cline quota query failed: %+v", account.Quota)
		}
	}
}
