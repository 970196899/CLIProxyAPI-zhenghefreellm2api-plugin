package qoder

import "strings"

// 本文件补充「上游 model/list 里查不到、但 key 确实可用」的模型。
//
// 为什么需要这层补充：Qoder 的活动模型（如 HungerG）不进模型目录，只在服务端
// 按 key 放行。判别方式（2026-11-24 实测）：
//   - 不存在的 key（hungerx / 已下线的 lmodel）→ HTTP 200 + code=112（无效模型）
//   - HungerG 的 key hungerg → 进入排队（code=10605, isQueued=true）
// 也就是说上游在排队之前先校验了 key，hungerg 是有效的，只是不列在 model/list 里。
// 只走 model/list 的实现永远看不到它，所以这里按 key 补一条。
//
// 只补 global：HungerG 是国际版 2026-11-23~11-30 的活动模型，国内版目录不含它。

// supplementalModels 返回某区域需要额外补进目录的模型。
func supplementalModels(region Region) []Model {
	if region != RegionGlobal {
		return nil
	}
	free := 0.0 // 活动期限免，倍率显式 0（指针区分「没给」与「免费」）
	return []Model{{
		Key:         "hungerg",
		DisplayName: "HungerG",
		Enable:      true,
		PriceFactor: &free,
	}}
}

// appendSupplemental 把补充模型追加到上游清单末尾；上游若已给出同 key，则以上游为准。
func appendSupplemental(models []Model, region Region) []Model {
	extra := supplementalModels(region)
	if len(extra) == 0 {
		return models
	}
	seen := make(map[string]bool, len(models))
	for _, model := range models {
		seen[strings.TrimSpace(model.Key)] = true
	}
	for _, model := range extra {
		if seen[strings.TrimSpace(model.Key)] {
			continue
		}
		models = append(models, model)
	}
	return models
}
