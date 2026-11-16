package hostbridge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"freetier2api-plugin/internal/core"
)

// TestListHostAuthsClaimsHijackedCredentials 验证被前置插件抢走的凭证仍被列出。
//
// 宿主遍历插件解析凭证时，一旦某个插件报错或认领就中止循环，且会把
// 「当前询问的插件的 identifier」兜底填进 provider。归属判定过宽的前置插件
// 会把本插件的文件抢走（provider 变成对方），只按 provider 过滤会让这些
// 账号从页面凭空消失。文件名仍是本插件约定，因此按文件名兜底认领。
func TestListHostAuthsClaimsHijackedCredentials(t *testing.T) {
	registry := core.NewRegistry()
	if !fileNameBelongsToPlugin("workbuddy-79fdc1fc-de43-40da-b6f0-fe05d9e4367b.json", registry) {
		t.Fatal("workbuddy-<uuid>.json must be recognized by file name")
	}
	if !fileNameBelongsToPlugin("my-workbuddy.json", registry) {
		t.Fatal("file name containing the workbuddy segment must be recognized")
	}
	// 别家的文件名不能被认领，否则会把别人的账号当成自己的。
	for _, foreign := range []string{"qoder-login-x.json", "claude-1.json", "codex-a@b.com.json", "auth.json"} {
		if fileNameBelongsToPlugin(foreign, registry) {
			t.Fatalf("foreign file %q must not be claimed", foreign)
		}
	}

	// provider 被抢走（qoder）但文件名是本插件的：仍要列出来。
	entries := filterPluginAuths([]core.HostAuth{
		{Name: "workbuddy-79fdc1fc-de43-40da-b6f0-fe05d9e4367b.json", Provider: "qoder", AuthIndex: "1"},
		{Name: "workbuddy-110693a0-d32d-4d81-8a0a-7a3445069f96.json", Provider: "workbuddy", AuthIndex: "2"},
		{Name: "qoder-login-f34c07f7.json", Provider: "qoder", AuthIndex: "3"},
	}, registry)
	if len(entries) != 2 {
		t.Fatalf("want 2 workbuddy credentials (one hijacked), got %d: %+v", len(entries), entries)
	}
	for _, entry := range entries {
		if entry.Provider == "qoder" && strings.Contains(entry.Name, "qoder-login") {
			t.Fatalf("foreign credential leaked in: %+v", entry)
		}
	}
}

// TestResolveAuthPathNormalizesHostPath 锁定宿主路径只在边界换算一次。
//
// 宿主不一定给绝对路径：EasyCLIProxyAPI 把 oauth.auth-dir 配成 ../oauth，
// 扫盘得到的相对路径原样经 host.auth.list / host.auth.get 返回。所有路径消费方
// （删除、stat、读文件兜底、续期写回）都拿这里的结果，不能再各自换算。
func TestResolveAuthPathNormalizesHostPath(t *testing.T) {
	cwd, errGetwd := os.Getwd()
	if errGetwd != nil {
		t.Fatalf("getwd: %v", errGetwd)
	}
	absolute := filepath.Join(cwd, "workbuddy-1.json")
	relative := filepath.Join("..", "oauth", "traecn-1.json")
	resolved := filepath.Clean(filepath.Join(cwd, "..", "oauth", "traecn-1.json"))

	cases := []struct {
		name string
		in   string
		want string
	}{
		{"绝对路径原样保留", absolute, filepath.Clean(absolute)},
		{"相对路径按工作目录解析", relative, resolved},
		{"空路径仍为空", "  ", ""},
		{"首尾空白被去掉", " " + absolute + "\n", filepath.Clean(absolute)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveAuthPath(tc.in)
			if got != tc.want {
				t.Fatalf("ResolveAuthPath(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestCoexistenceAuthListKeepsCurrentClaimBehavior 锁定双开共存时的凭证列出行为。
//
// 背景：本插件改名为 freetierplus 后，与原版 freetier 同时加载时，
// 同一份 workbuddy*-*.json 对两个插件都满足「文件名属于本插件」的第二层判据。
// 这是**既有的容错设计**（见 filterPluginAuths 的说明：兜住被前置插件抢走、
// provider 被兜底改写的凭证），不是本次改造引入的行为。本用例钉死：
//   - provider=freetierplus 的条目被收下（第一层判据）；
//   - provider=freetier（原版插件写的）但文件名是 workbuddy* 的条目**仍被收下**
//     （第二层判据，与改造前一致——不得因 ProviderKey 改名而改变）；
//   - 别家的文件名（qoder-login-*）在任何情况下都不收。
func TestCoexistenceAuthListKeepsCurrentClaimBehavior(t *testing.T) {
	registry := core.NewRegistry()
	entries := filterPluginAuths([]core.HostAuth{
		{Name: "workbuddy-u1.json", Provider: core.ProviderKey, AuthIndex: "1"},
		{Name: "workbuddy-u2.json", Provider: "freetier", AuthIndex: "2"},
		{Name: "qoder-login-f34c07f7.json", Provider: "freetier", AuthIndex: "3"},
		{Name: "claude-1.json", Provider: "anthropic", AuthIndex: "4"},
	}, registry)

	got := make(map[string]bool, len(entries))
	for _, entry := range entries {
		got[entry.Name] = true
	}
	if !got["workbuddy-u1.json"] {
		t.Fatal("本插件自己的凭证必须被列出")
	}
	if !got["workbuddy-u2.json"] {
		t.Fatal("原版插件写入、但文件名属于本插件约定的凭证必须仍被列出（第二层判据不得因改名失效）")
	}
	if got["qoder-login-f34c07f7.json"] || got["claude-1.json"] {
		t.Fatalf("别家凭证泄漏：%+v", entries)
	}
}
