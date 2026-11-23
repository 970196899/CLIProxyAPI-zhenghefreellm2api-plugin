package qoder

import (
	"testing"
	"time"
)

// TestAppendSupplemental 锁住目录外模型的三个约束：有权限才补、不覆盖上游已有 key、只补 global。
func TestAppendSupplemental(t *testing.T) {
	upstream := []Model{{Key: "dmodel", DisplayName: "DeepSeek-V4-Pro"}}
	allow := func(Model) bool { return true }
	deny := func(Model) bool { return false }

	got := appendSupplemental(append([]Model{}, upstream...), RegionGlobal, allow)
	if len(got) != 2 {
		t.Fatalf("有权限时应补出 2 条，得到 %d", len(got))
	}
	if got[1].Key != "hungerg" || got[1].DisplayName != "HungerG" {
		t.Fatalf("补充条目不对：%+v", got[1])
	}
	if got[1].PriceFactor == nil || *got[1].PriceFactor != 0 {
		t.Fatalf("HungerG 应显式倍率 0，得到 %v", got[1].PriceFactor)
	}

	if out := appendSupplemental(append([]Model{}, upstream...), RegionGlobal, deny); len(out) != 1 {
		t.Fatalf("无权限时不应补充，得到 %d 条", len(out))
	}

	// 上游已经给了同 key：以清单为准，不再探测。
	probed := false
	withUpstream := []Model{{Key: "hungerg", DisplayName: "HungerG", PriceFactor: floatPtr(0.5)}}
	out := appendSupplemental(append([]Model{}, withUpstream...), RegionGlobal, func(Model) bool {
		probed = true
		return true
	})
	if len(out) != 1 || probed {
		t.Fatalf("上游已有时不应探测/重复追加：len=%d probed=%t", len(out), probed)
	}

	// 国内版不补。
	if out := appendSupplemental(append([]Model{}, upstream...), RegionCN, allow); len(out) != 1 {
		t.Fatalf("cn 不应补充，得到 %d 条", len(out))
	}
}

// TestEntitlementCache 锁住缓存语义：命中不重复探测，过期后重新探测。
func TestEntitlementCache(t *testing.T) {
	var cache entitlementCache
	calls := 0

	if !cache.allowed("hungerg", func() bool { calls++; return true }) {
		t.Fatal("首次探测应放行")
	}
	if !cache.allowed("hungerg", func() bool { calls++; return false }) {
		t.Fatal("TTL 内应复用放行结论")
	}
	if calls != 1 {
		t.Fatalf("TTL 内不应重复探测，实际探测 %d 次", calls)
	}

	// 把结论改成「已过期」，验证会重新探测并采纳新结论。
	cache.mu.Lock()
	cache.byKey["hungerg"] = accessVerdict{allowed: true, at: time.Now().Add(-accessAllowedTTL - time.Minute)}
	cache.mu.Unlock()
	if cache.allowed("hungerg", func() bool { calls++; return false }) {
		t.Fatal("过期后应采纳新的拒绝结论")
	}
	if calls != 2 {
		t.Fatalf("过期后应重新探测，实际探测 %d 次", calls)
	}
}

func floatPtr(v float64) *float64 { return &v }
