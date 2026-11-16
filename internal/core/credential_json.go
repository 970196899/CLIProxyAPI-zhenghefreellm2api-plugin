package core

// 本文件提供凭证 JSON 的合并骨架。
//
// 背景：六个供应商各自实现 core.StorageMerger（MergeStorageJSON），其中
// codearts / trae / minimaxcode / zcode 四份的骨架完全一样——
//
//	原文件为空或不是合法对象 → 整体替换为新凭证
//	否则 → 反序列化成 map，就地覆盖若干字段，再缩进序列化
//
// 只有「覆盖哪些字段、空值是否覆盖」各家不同。把骨架收口到这里，各家只保留
// 自己的字段清单；将来新增供应商不必再抄一遍空值/损坏分支。
//
// 不收口的两家（形态确实特殊，各自保留）：
//   - cline：键名要跟随原文件（下划线/驼峰），不能由骨架决定；
//   - workbuddy：深合并（mergeMaps）且补 type 兜底，不是「覆盖字段」。

import (
	"encoding/json"
	"strings"
)

// MergeCredentialJSON 把 updated 的字段合并进 original，返回缩进后的 JSON。
//
// original 为空或无法反序列化成对象时，直接返回 fresh()。否则把 original
// 解成 map，交给 apply 覆盖字段，再序列化。
//
// apply 只负责写字段，不负责判空：「空值是否覆盖」是各家协议的一部分，
// 由供应商自己决定（access_token 通常无条件覆盖，refresh_token 通常仅在非空时覆盖）。
func MergeCredentialJSON(original []byte, fresh func() ([]byte, error), apply func(map[string]any)) ([]byte, error) {
	if len(original) == 0 {
		return fresh()
	}
	var target map[string]any
	if err := json.Unmarshal(original, &target); err != nil || target == nil {
		return fresh()
	}
	apply(target)
	merged, errMarshal := json.MarshalIndent(target, "", "  ")
	if errMarshal != nil {
		return fresh()
	}
	return merged, nil
}

// CredentialOwnedByType 判定凭证是否属于本插件（用于写入门禁）。
//
// 双开场景下，「provider」无法区分两种相反的条目：
//   - 原版插件写入的凭证（type=freetier）；
//   - 本插件自己的凭证，但被前置插件把 provider 兜底改写成了别家值。
//
// 两者 provider 都可能不是本插件 key，但只有前者应当被拒绝。区分它们的是
// 凭证内容里的 type。
//
// 判据是**排除法**：只把「已知别家插件的身份」判为不属于本插件，其余一律放行。
// 这样两端都不会错：
//   - 原版插件写的一定是 freetier，被正确排除；
//   - 本插件承认的历史形态（workbuddy / codebuddy / cline 手工导入）与空 type
//     （cline/qoder/autoclaw/raccoon/minimaxcode 这五家凭证本就不写 type）
//     都按本方处理，不会误伤。
//
// 这是一道**写入门禁**：只用于停用/删除/保活/签到等会改动数据或产生上游
// 副作用的路径。列表展示不能用它，否则被抢走的本方账号会从页面上消失。
func CredentialOwnedByType(raw []byte) bool {
	if len(raw) == 0 {
		return true
	}
	var payload struct {
		Type string `json:"type"`
	}
	if errUnmarshal := json.Unmarshal(raw, &payload); errUnmarshal != nil {
		return true
	}
	return OwnedCredentialType(payload.Type)
}

// foreignCredentialTypes 是已知属于**其他插件**的凭证 type（小写）。
//
// 目前只有原版 freetier。将来若要与更多同源插件共存，在此追加即可——
// 新增在这里的值会被写入门禁排除，不影响其余形态。
var foreignCredentialTypes = map[string]bool{
	"freetier": true,
}

// OwnedCredentialType 报告某个 type 值是否属于本插件。
//
// 排除法：type 不在 foreignCredentialTypes 里就算本方。空值、本插件 key、
// 以及本插件承认的历史形态（workbuddy / codebuddy / cline）都放行。
func OwnedCredentialType(rawType string) bool {
	value := strings.ToLower(strings.TrimSpace(rawType))
	if value == "" {
		return true
	}
	return !foreignCredentialTypes[value]
}
