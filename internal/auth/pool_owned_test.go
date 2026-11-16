package auth

import (
	"testing"

	"freetier2api-plugin/internal/core"
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
