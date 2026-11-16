package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"freetier2api-plugin/cpasdk/pluginabi"
	"freetier2api-plugin/cpasdk/pluginapi"
	"freetier2api-plugin/internal/core"
	"freetier2api-plugin/internal/hostbridge"
)

// TestCredentialOwnedByTypeDistinguishesCoexistenceCases 锁定写回门禁的判据维度。
//
// 背景：双开时 ListAuths 按文件名兜底收下的清单里混着两类条目，它们的
// provider 字段可能长得一模一样：
//   - 原版插件写入的凭证（旧 type）——不该被本插件改写；
//   - 本插件自己的凭证，但 provider 被前置插件兜底改写成别家值——必须保留，
//     否则用户自己的账号会静默掉线。
//
// 唯一能区分两者的是凭证内容里的 type。用 provider 做判据会把后者一起误杀，
// 这正是本用例要防的回归。
func TestCredentialOwnedByTypeDistinguishesCoexistenceCases(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want bool
	}{
		{"本插件凭证", `{"type":"freetierplus","auth":{}}`, true},
		{"本插件凭证大小写变体", `{"type":"FreeTierPlus","auth":{}}`, true},
		{"原版插件写入的凭证", `{"type":"freetier","auth":{}}`, false},
		{"原版插件凭证大写", `{"type":"FreeTier","auth":{}}`, false},
		{"历史凭证没有 type 字段", `{"auth":{"accessToken":"at"}}`, true},
		{"type 是空串", `{"type":"","auth":{}}`, true},
		{"type 只有空白", `{"type":"   ","auth":{}}`, true},
		// 本插件自己在归属判定里承认的历史形态，必须放行，
		// 否则这些账号会在保活里静默掉线、删除时被错误拒绝。
		{"手工导入的 cline 凭证", `{"type":"cline","refreshToken":"rt"}`, true},
		{"旧版 workbuddy 形态", `{"type":"workbuddy"}`, true},
		{"旧版 codebuddy 形态", `{"type":"codebuddy"}`, true},
		{"空内容", ``, true},
		{"非法 JSON", `{not json`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := core.CredentialOwnedByType([]byte(tc.raw)); got != tc.want {
				t.Fatalf("CredentialOwnedByType(%s) = %v, want %v", tc.raw, got, tc.want)
			}
		})
	}
}

// TestCredentialOwnedByTypeKeepsHijackedOwnAccount 是上表的重点分支：
// provider 已被改成别人的值，但 type 仍是我方的凭证必须保留。
func TestCredentialOwnedByTypeKeepsHijackedOwnAccount(t *testing.T) {
	hijacked := []byte(`{"type":"freetierplus","auth":{"accessToken":"at"}}`)
	if !core.CredentialOwnedByType(hijacked) {
		t.Fatal("provider 被前置插件抢走、但 type 属于本插件的账号必须保留，否则会静默掉线")
	}
}

// ownedTestVendor 是门禁接线测试用的最小供应商实现。
// 只提供 Load 链路真正会调到的 Region()，其余方法留空壳。
type ownedTestVendor struct {
	core.Meta
}

func (v *ownedTestVendor) Match(string, string, map[string]any) bool { return true }

func (v *ownedTestVendor) Parse(raw []byte, _ string) (*core.Credential, error) {
	return &core.Credential{}, nil
}

func (v *ownedTestVendor) StaticModels(context.Context) ([]pluginapi.ModelInfo, error) {
	return nil, nil
}

func (v *ownedTestVendor) Execute(context.Context, *core.Credential, *core.ExecuteRequest) (*pluginapi.ExecutorResponse, error) {
	return nil, nil
}

func (v *ownedTestVendor) ExecuteStream(context.Context, *core.Credential, *core.ExecuteRequest, core.StreamSink) error {
	return nil
}

// TestLoadSkipsForeignCredentialsWhenOwnedOnly 是门禁的接入断言。
//
// 上一条用例只锁了纯函数；若 pool.go 里那行 `if opts.OwnedOnly && ...`
// 被误删，纯函数测试仍然全绿。本用例驱动完整的 Load 链路（真实临时文件 +
// 可注入的 Resolver），把门禁的接线本身钉死：原版凭证被跳过、本插件凭证保留。
func TestLoadSkipsForeignCredentialsWhenOwnedOnly(t *testing.T) {
	dir := t.TempDir()
	oursPath := filepath.Join(dir, "workbuddy-ours.json")
	origPath := filepath.Join(dir, "workbuddy-original.json")
	if errWrite := os.WriteFile(oursPath, []byte(`{"type":"freetierplus","auth":{}}`), 0o600); errWrite != nil {
		t.Fatalf("write ours: %v", errWrite)
	}
	if errWrite := os.WriteFile(origPath, []byte(`{"type":"freetier","auth":{}}`), 0o600); errWrite != nil {
		t.Fatalf("write original: %v", errWrite)
	}

	hostbridge.Configure(func(_, method string, payload any) (json.RawMessage, error) {
		if method != pluginabi.MethodHostAuthList {
			return nil, fmt.Errorf("unexpected host method %s", method)
		}
		raw, errMarshal := json.Marshal(map[string]any{"auths": []map[string]string{
			{"id": "1", "auth_index": "1", "name": "workbuddy-ours.json", "provider": core.ProviderKey, "path": oursPath},
			{"id": "2", "auth_index": "2", "name": "workbuddy-original.json", "provider": "freetier", "path": origPath},
		}})
		if errMarshal != nil {
			return nil, errMarshal
		}
		return raw, nil
	})
	t.Cleanup(func() { hostbridge.Configure(nil) })

	vendor := &ownedTestVendor{Meta: core.Meta{VendorID: "workbuddycn", VendorRegion: "cn"}}
	resolver := func(_ context.Context, _ string, _ []byte, _ string, _ map[string]string) (*core.Credential, core.Vendor, error) {
		return &core.Credential{}, vendor, nil
	}

	pool := NewPool(core.NewRegistry(), resolver)
	loaded, _ := pool.Load(context.Background(), "", Options{OwnedOnly: true})

	if len(loaded) != 1 {
		t.Fatalf("OwnedOnly 下应只剩本插件凭证，got %d", len(loaded))
	}
	if loaded[0].Entry.Name != "workbuddy-ours.json" {
		t.Fatalf("保留的应是本插件凭证，got %s", loaded[0].Entry.Name)
	}

	// 不开 OwnedOnly 时原版凭证也在：列表展示的容错不能被门禁改变。
	// 用独立的 pool 避免上一轮的凭证缓存影响判定。
	allPool := NewPool(core.NewRegistry(), resolver)
	all, allFailures := allPool.Load(context.Background(), "", Options{IncludeDisabled: true})
	if len(all) != 2 {
		t.Fatalf("默认行为必须保留全部条目，got %d（failures=%+v）", len(all), allFailures)
	}
}
