package qoder

import "testing"

// TestAppendSupplemental 锁住补充模型的两个约束：只补 global，且不覆盖上游已有 key。
func TestAppendSupplemental(t *testing.T) {
	upstream := []Model{{Key: "dmodel", DisplayName: "DeepSeek-V4-Pro"}}

	got := appendSupplemental(append([]Model{}, upstream...), RegionGlobal)
	if len(got) != 2 {
		t.Fatalf("global 应补出 2 条，得到 %d", len(got))
	}
	if got[1].Key != "hungerg" || got[1].DisplayName != "HungerG" {
		t.Fatalf("补充条目不对：%+v", got[1])
	}
	if got[1].PriceFactor == nil || *got[1].PriceFactor != 0 {
		t.Fatalf("HungerG 应显式倍率 0，得到 %v", got[1].PriceFactor)
	}

	// 上游已经给了同 key 时以上游为准，不重复追加。
	withUpstream := []Model{{Key: "hungerg", DisplayName: "HungerG", PriceFactor: floatPtr(0.5)}}
	if out := appendSupplemental(append([]Model{}, withUpstream...), RegionGlobal); len(out) != 1 {
		t.Fatalf("上游已有时不应重复追加，得到 %d 条", len(out))
	}

	// 国内版不补。
	if out := appendSupplemental(append([]Model{}, upstream...), RegionCN); len(out) != 1 {
		t.Fatalf("cn 不应补充，得到 %d 条", len(out))
	}
}

func floatPtr(v float64) *float64 { return &v }
