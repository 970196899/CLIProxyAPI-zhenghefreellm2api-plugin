package tests

// 本文件锁定账号概览「剩余 / 总额」列的状态语义。
//
// 真实事故：某纯 key 供应商上游没有额度接口，它的账号在概览里永久停在
// 「查询中…」。根因不是查询失败，而是页面把「没有结果」一律当成「正在查询」，
// 于是「不支持额度」与「还没查」这两种状态在数据上完全无法区分。
//
// 修复把它拆成两件事：
//  1. 后端给出静态能力 supports_quota（接口断言）；
//  2. 额度缓存**成功与失败都写**，让「不支持」也能作为结果被记住。
//
// 两个断言都必须通过，否则页面会重新退化成「一直查询中」。

import (
	"encoding/json"
	"net/http"
	"testing"
)

// managementCall 与 managementCallWithBody 见 testutil_test.go。

// statusAccounts 拉一次 /status 并取出账号列表。
func statusAccounts(t *testing.T) []struct {
	AuthID        string `json:"auth_id"`
	VendorID      string `json:"vendor_id"`
	SupportsQuota bool   `json:"supports_quota"`
	Quota         *struct {
		OK      bool   `json:"ok"`
		Message string `json:"message"`
	} `json:"quota"`
} {
	t.Helper()
	resp := managementCall(t, "GET", "/status")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/status = %d: %s", resp.StatusCode, resp.Body)
	}
	var payload struct {
		Accounts []struct {
			AuthID        string `json:"auth_id"`
			VendorID      string `json:"vendor_id"`
			SupportsQuota bool   `json:"supports_quota"`
			Quota         *struct {
				OK      bool   `json:"ok"`
				Message string `json:"message"`
			} `json:"quota"`
		} `json:"accounts"`
	}
	if errUnmarshal := json.Unmarshal(resp.Body, &payload); errUnmarshal != nil {
		t.Fatalf("decode status: %v (%s)", errUnmarshal, resp.Body)
	}
	return payload.Accounts
}

// TestClineReportsQuotaSupport 锁定「实现了额度接口就必须报 true」。
//
// Cline 补上 /users/me + /users/{id}/balance 之后必须报 supports_quota=true，
// 否则页面永远不查询、只显示「-- / --」。查询前的 quota 必须为空，
// 表示「还没查过」而不是「查询失败」。
func TestClineReportsQuotaSupport(t *testing.T) {
	host := installFakeHost(t)
	setupTestPlugin(t)
	host.AddAccount("1", "cline-test.json", `{"type":"cline","refreshToken":"workos:rt-test"}`)

	accounts := statusAccounts(t)
	if len(accounts) != 1 {
		t.Fatalf("want 1 account, got %d: %+v", len(accounts), accounts)
	}
	if accounts[0].VendorID != "cline" {
		t.Fatalf("vendor = %q, want cline", accounts[0].VendorID)
	}
	if !accounts[0].SupportsQuota {
		t.Fatal("cline now implements QuotaProvider: supports_quota must be true")
	}
	if accounts[0].Quota != nil {
		t.Fatalf("quota must be absent before any query: %+v", accounts[0].Quota)
	}
}
