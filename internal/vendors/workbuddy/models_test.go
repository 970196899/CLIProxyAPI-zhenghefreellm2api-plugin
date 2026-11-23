package workbuddy

import (
	"testing"
	"time"

	"freetier2api-plugin/internal/core"
)

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

// TestModelCreditsExcludesDisplaySentinel 锁定接线：
// ModelCredits 的返回值会被 admin 落盘进 state.json、重启后回灌成
// ModelInfo.Credits，因此绝不能带上 displayCredits 的「浮动倍率」哨兵；
// 只有 ConsoleModels 这一条展示路径才允许出现它。
func TestModelCreditsExcludesDisplaySentinel(t *testing.T) {
	rt := newRuntime(core.Env{})
	rt.loadCachedModels([]ModelInfo{
		{ID: "auto"},
		{ID: "glm-5.3", Credits: "x0.05"},
	}, RegionCN)
	inst := &adapter{rt: rt, region: RegionCN}

	credits := inst.ModelCredits()
	if _, present := credits["auto"]; present {
		t.Fatalf("ModelCredits 不得含 auto 键（哨兵会污染持久化），got %q", credits["auto"])
	}
	if credits["glm-5.3"] != "0.05X" {
		t.Fatalf("glm-5.3 credits = %q, want 0.05X", credits["glm-5.3"])
	}

	// 展示路径必须仍然给 auto 标注浮动倍率。
	var autoCredits string
	for _, model := range inst.ConsoleModels() {
		if model.ID == "auto" {
			autoCredits = model.Credits
		}
	}
	if autoCredits != "浮动倍率" {
		t.Fatalf("ConsoleModels 的 auto credits = %q, want 浮动倍率", autoCredits)
	}
}

// TestNonChatModelFiltersVideoFamilies 锁住视频/音频模型过滤。
//
// 回归点：上游目录把 kling-video-v3、seedance-2.5 当普通模型下发，但调用
// 必报 400 "Backend [maas] is not supported"；列表里出现它们会让客户端选到
// 必失败的模型。
func TestNonChatModelFiltersVideoFamilies(t *testing.T) {
	cases := []struct {
		name  string
		entry dynModelEntry
		want  bool
	}{
		{"kling 视频", dynModelEntry{ID: "kling-video-v3"}, true},
		{"seedance 视频", dynModelEntry{ID: "seedance-2.5"}, true},
		{"sora 视频", dynModelEntry{ID: "sora-2"}, true},
		{"veo 视频", dynModelEntry{ID: "veo-3.1"}, true},
		{"视频标签", dynModelEntry{ID: "mystery-model", Tags: []string{"text-to-video"}}, true},
		{"音频标签", dynModelEntry{ID: "mystery-voice", Tags: []string{"Text-To-Speech"}}, true},
		{"文生图标签", dynModelEntry{ID: "mystery-image", Tags: []string{"text-to-image"}}, true},
		{"普通对话模型", dynModelEntry{ID: "deepseek-v4-flash", MaxOutputTokens: 50000}, false},
		{"带视频理解能力的对话模型", dynModelEntry{ID: "gemini-3.5-pro", Tags: []string{"vision"}, MaxOutputTokens: 64000}, false},
	}
	for _, tc := range cases {
		if got := nonChatModel(tc.entry); got != tc.want {
			t.Fatalf("%s: nonChatModel = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestFilterBlockedModels 锁住 11102 隔离：观察到的坏模型从清单里摘掉，
// 另一个域不受影响，过期后自动回归，全被隔离时不清空清单。
func TestFilterBlockedModels(t *testing.T) {
	rt := newRuntime(core.Env{})
	models := []ModelInfo{{ID: "o4-mini"}, {ID: "deepseek-v4-flash"}}

	rt.markModelBlocked(RegionGlobal, "o4-mini")

	global := rt.filterBlockedModels(RegionGlobal, models)
	if len(global) != 1 || global[0].ID != "deepseek-v4-flash" {
		t.Fatalf("global 过滤后 = %#v", global)
	}
	if cn := rt.filterBlockedModels(RegionCN, models); len(cn) != 2 {
		t.Fatalf("cn 不应被 global 的隔离影响，得到 %#v", cn)
	}

	// 过期后自动放行。
	rt.modelMu.Lock()
	rt.blockedModels[blockedKey(RegionGlobal, "o4-mini")] = time.Now().Add(-blockedModelTTL - time.Minute)
	rt.modelMu.Unlock()
	if got := rt.filterBlockedModels(RegionGlobal, models); len(got) != 2 {
		t.Fatalf("过期后应全部回归，得到 %#v", got)
	}

	// 全部被隔离时不清空清单。
	rt.markModelBlocked(RegionGlobal, "o4-mini")
	rt.markModelBlocked(RegionGlobal, "deepseek-v4-flash")
	if got := rt.filterBlockedModels(RegionGlobal, models); len(got) != 2 {
		t.Fatalf("全隔离时应原样返回，得到 %#v", got)
	}
}
