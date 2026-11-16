package workbuddy

import "testing"

// TestMergeModelCatalogsCorrectsFreeModelCredits 锁定免费模型倍率纠偏：
// v3 目录把 hy4flash-free 标成 x0.03，企业计费端点给的才是真实扣费口径 x0.00。
func TestMergeModelCatalogsCorrectsFreeModelCredits(t *testing.T) {
	primary := []ModelInfo{{ID: "hy4flash-free", Name: "HY4 Flash Free", Credits: "x0.03"}}
	fallback := []ModelInfo{{ID: "hy4flash-free", Name: "企业侧名字", Credits: "x0.00"}}

	merged := mergeModelCatalogs(primary, fallback)
	if len(merged) != 1 {
		t.Fatalf("merged = %d, want 1（不得因覆盖倍率而新增条目）", len(merged))
	}
	if merged[0].Credits != "x0.00" {
		t.Fatalf("credits = %q, want x0.00（免费模型的真实倍率以企业端点为准）", merged[0].Credits)
	}
	if merged[0].Name != "HY4 Flash Free" {
		t.Fatalf("name = %q, want v3 骨架值（只允许改 Credits）", merged[0].Name)
	}
}

// TestMergeModelCatalogsFillsMissingCredits 锁定 v3 缺倍率时用企业端点补齐。
func TestMergeModelCatalogsFillsMissingCredits(t *testing.T) {
	primary := []ModelInfo{{ID: "default", Name: "Default"}}
	fallback := []ModelInfo{{ID: "default", Credits: "x2.20"}}

	merged := mergeModelCatalogs(primary, fallback)
	if len(merged) != 1 || merged[0].Credits != "x2.20" {
		t.Fatalf("merged = %+v, want single entry with x2.20", merged)
	}
}

// TestMergeModelCatalogsKeepsPrimaryWhenFallbackHasNoCredits 锁定「企业端点没给
// 明确倍率时不得清空 v3 值」：空倍率是「未知」，不是「免费」。
func TestMergeModelCatalogsKeepsPrimaryWhenFallbackHasNoCredits(t *testing.T) {
	primary := []ModelInfo{{ID: "glm-5.3", Name: "GLM", Credits: "x0.71"}}
	fallback := []ModelInfo{{ID: "glm-5.3", Name: "另一名字"}}

	merged := mergeModelCatalogs(primary, fallback)
	if merged[0].Credits != "x0.71" {
		t.Fatalf("credits = %q, want x0.71", merged[0].Credits)
	}
	if merged[0].Name != "GLM" {
		t.Fatalf("name = %q, want 骨架值", merged[0].Name)
	}
}

// TestMergeModelCatalogsAppendsFallbackOnlyIDs 锁定企业独有模型仍然补齐。
func TestMergeModelCatalogsAppendsFallbackOnlyIDs(t *testing.T) {
	primary := []ModelInfo{{ID: "glm-5.3"}}
	fallback := []ModelInfo{{ID: "deepseek-v4", Credits: "x0.10"}}

	merged := mergeModelCatalogs(primary, fallback)
	if len(merged) != 2 || merged[1].ID != "deepseek-v4" {
		t.Fatalf("merged = %+v, want appended deepseek-v4", merged)
	}
}

// TestMergeModelCatalogsTreatsEquivalentCreditsAsEqual 锁定归一化比较：
// "x0.05" 与 "x0.05 credits" 是同一倍率，不该触发无意义覆盖。
func TestMergeModelCatalogsTreatsEquivalentCreditsAsEqual(t *testing.T) {
	primary := []ModelInfo{{ID: "kimi-k3.1-pro", Credits: "x0.05"}}
	fallback := []ModelInfo{{ID: "kimi-k3.1-pro", Credits: "x0.05 credits"}}

	merged := mergeModelCatalogs(primary, fallback)
	if merged[0].Credits != "x0.05" {
		t.Fatalf("credits = %q, want 保留骨架原串 x0.05", merged[0].Credits)
	}
}

// TestDisplayCreditsMarksAutoAsFloating 锁定 auto 的空白倍率不再呈现为空白。
func TestDisplayCreditsMarksAutoAsFloating(t *testing.T) {
	if got := displayCredits(ModelInfo{ID: "auto"}); got != "浮动倍率" {
		t.Fatalf("auto credits = %q, want 浮动倍率", got)
	}
	if got := displayCredits(ModelInfo{ID: "auto", Credits: "x1.00"}); got != "1.00X" {
		t.Fatalf("auto with explicit credits = %q, want 1.00X（有值时不覆盖）", got)
	}
	if got := displayCredits(ModelInfo{ID: "glm-5.3", Credits: "x0.05"}); got != "0.05X" {
		t.Fatalf("numeric credits = %q, want 0.05X", got)
	}
	if got := displayCredits(ModelInfo{ID: "glm-5.3"}); got != "" {
		t.Fatalf("unknown credits = %q, want empty", got)
	}
}
