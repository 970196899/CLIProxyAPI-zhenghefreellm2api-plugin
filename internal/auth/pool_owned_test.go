package auth

import (
	"testing"

	"freetier2api-plugin/internal/core"
)

// TestOwnedByPluginExcludesForeignProviders 锁定双开共存下的写回边界。
//
// 背景：ListAuths 按文件名兜底收下「provider 被前置插件抢走」的凭证，
// 这是列表展示必需的容错（否则用户账号会从页面上凭空消失）。
// 但保活/自动任务/401 刷新这些**写回**链路若复用同一份清单，双开时会把
// 原版插件的账号也拿来续期改写——两个插件各自轮换同一份 refresh token，
// 可能互相把对方刷失效。因此写回链路用 OwnedOnly 收紧为「只处理自己的」。
func TestOwnedByPluginExcludesForeignProviders(t *testing.T) {
	cases := []struct {
		name     string
		provider string
		want     bool
	}{
		{"本插件自己的凭证", core.ProviderKey, true},
		{"本插件凭证大小写变体", "FreeTierPlus", true},
		{"原版插件写入的凭证", "freetier", false},
		{"别家插件的凭证", "qoder", false},
		{"宿主未填 provider（归因不明，不能排除）", "", true},
		{"只有空白字符的 provider", "   ", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			entry := core.HostAuth{Name: "workbuddy-a.json", Provider: tc.provider}
			if got := ownedByPlugin(entry); got != tc.want {
				t.Fatalf("ownedByPlugin(provider=%q) = %v, want %v", tc.provider, got, tc.want)
			}
		})
	}
}
