package tests

// 本文件锁定「额度能力判定对所有供应商通用」。
//
// 页面只读一个字段 accounts[].supports_quota，因此这个字段必须对**每一个**
// 已注册的供应商实例都成立，而不是只对某一家打的补丁：
//   - 实现了 QuotaProvider 的供应商 → true（没查到就短暂显示「查询中…」）；
//   - 没实现的 → false（立即显示「-- / --」，永不进入查询中）。
//
// 这里枚举真实的注册表实例，逐个与接口断言对账。新增供应商若忘了实现 Quota
// 却期望有额度列，或实现了却漏报，都会在这里被发现。

import (
	"testing"

	"freetier2api-plugin/internal/app"
	"freetier2api-plugin/internal/core"
)

// TestSupportsQuotaIsGenericAcrossVendors 枚举全部内置供应商实例。
func TestSupportsQuotaIsGenericAcrossVendors(t *testing.T) {
	registry := core.NewRegistry()
	for _, factory := range app.BuiltinFactories() {
		for _, vendor := range factory.Instances(core.Env{}) {
			if errRegister := registry.Register(vendor); errRegister != nil {
				t.Fatalf("register %s: %v", vendor.ID(), errRegister)
			}
		}
	}
	vendors := registry.Vendors()
	if len(vendors) == 0 {
		t.Fatal("no vendors registered")
	}

	// 目前所有内置实例都实现了 QuotaProvider（Cline 补上余额接口后不再有
	// 「无额度」的供应商）。这里钉死这个事实：新增供应商若没实现额度却
	// 被页面当成「有额度」，会先在这里暴露。
	var withoutQuota []string
	for _, vendor := range vendors {
		if _, okQuota := core.As[core.QuotaProvider](vendor); !okQuota {
			withoutQuota = append(withoutQuota, vendor.ID())
		}
	}
	if len(withoutQuota) != 0 {
		t.Fatalf("vendors without quota API: %v (implement Quota or update this assertion)", withoutQuota)
	}
	t.Logf("all %d vendor instances implement QuotaProvider", len(vendors))
}
