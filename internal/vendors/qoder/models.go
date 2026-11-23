package qoder

// 本文件实现 Qoder 的模型清单。
//
// 清单只有一条来源：上游 model/list 接口（需要 cosy 会话）。**不维护内置
// 兜底清单**——写死的模型名会随上游变化而过时，反而让客户端看到一份
// 并不存在的目录。拉不到就返回空清单，由控制台提示「刷新模型」。

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"freetier2api-plugin/internal/logger"
)

// modelIndex 是某区域实例的「对外模型 ID / 上游 key → 上游 key」索引。
//
// 为什么是实例状态而不是包级表：Qoder 国内版与国际版是两个 adapter 实例，
// 同一个上游 key 在两个区域可能对应不同模型（如 mmodel 在国内是
// MiniMax-M2.7、国际是 MiniMax-M3）。放进实例里，两边各自维护，
// 与仓库「实例隔离」的约定一致。
//
// 为什么是动态建立而不是写死静态表：slug → key 的对应关系只存在于上游
// 清单的 {key, display_name} 配对里。写死一张表意味着上游每换一个模型就要
// 改代码，而且容易把两个不同模型误绑到同一个名字；从清单推导则天然跟随上游。
type modelIndex struct {
	mu     sync.RWMutex
	byName map[string]string
}

// replace 用一批上游模型条目重建索引，返回「上游 key → 对外 ID」。
//
// 重复 ID（上游出现同名 display_name）时只有第一条保留，其余不出现在返回值里，
// 由调用方跳过——宿主按 ID 匹配模型，注册两个相同 ID 只会让其中一个永远选不到。
func (i *modelIndex) replace(models []Model) map[string]string {
	byName := make(map[string]string, len(models)*2)
	slugByKey := make(map[string]string, len(models))
	for _, model := range models {
		key := strings.TrimSpace(model.Key)
		if key == "" {
			continue
		}
		id := canonicalModelID(model.DisplayName, key)
		if id == "" {
			continue
		}
		if _, exists := byName[id]; exists {
			logger.Info("qoder：模型 ID %q 重复（键 %s），已跳过", id, key)
			continue
		}
		slugByKey[key] = id
		byName[id] = key
		// 老客户端仍可能直接发上游 key（dmodel / gmodel …）：一并接受。
		// 对外 ID 优先——万一某个 slug 恰好等于另一个模型的上游 key，
		// 以清单里声明的 slug 为准。
		lowerKey := strings.ToLower(key)
		if _, exists := byName[lowerKey]; !exists {
			byName[lowerKey] = key
		}
	}
	i.mu.Lock()
	i.byName = byName
	i.mu.Unlock()
	return slugByKey
}

// UpstreamKey 把对外模型名解析成上游 model_config.key。大小写不敏感。
func (i *modelIndex) UpstreamKey(name string) (string, bool) {
	trimmed := strings.ToLower(strings.TrimSpace(name))
	if trimmed == "" {
		return "", false
	}
	i.mu.RLock()
	defer i.mu.RUnlock()
	key, ok := i.byName[trimmed]
	return key, ok
}

// canonicalModelID 把上游 display_name 规范成对外模型 ID：
// 小写，空白折叠成单个 "-"（"DeepSeek-V4-Pro" -> "deepseek-v4-pro"）。
//
// display_name 缺失时退回上游 key 的小写形式，保证总能拿到一个可用 ID。
func canonicalModelID(displayName, key string) string {
	slug := strings.ToLower(strings.TrimSpace(displayName))
	slug = strings.Join(strings.Fields(slug), "-")
	if slug == "" {
		slug = strings.ToLower(strings.TrimSpace(key))
	}
	return slug
}

// ---- 目录外的活动模型（按账号权限自动上下架） ----
//
// 为什么需要这层补充：Qoder 的活动模型（如 HungerG）不进模型目录，只在服务端
// 按 key 放行；放不放行还随账号权益变化（2026-11-24 实测：同一账号 07:41 还能
// 进队，08:54 起直接返回 112 + pricingUrl）。写死「一定下发」会让客户端看到一个
// 必然 403 的模型，所以这里先探一次权限，再决定要不要放进目录。
//
// 判别依据（上游先校验模型、再排队）：
//   - 放行 → 排队 10605 isQueued（或正常返回内容）
//   - 没权限 / key 不存在 → code=112（带 pricingUrl 购买链接）
//
// 只考虑 global：HungerG 是国际版 2026-11-23~11-30 的活动模型，国内版目录不含它。

// supplementalModels 返回某区域需要额外考虑放进目录的模型。
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

// appendSupplemental 用 probe 的结论决定目录外模型要不要下发。
//
// 上游清单里已经出现同 key 时以清单为准（不再探测）；probe 为 nil 表示
// 调用方明确要求无条件下发。
func appendSupplemental(models []Model, region Region, probe func(Model) bool) []Model {
	extra := supplementalModels(region)
	if len(extra) == 0 {
		return models
	}
	seen := make(map[string]bool, len(models))
	for _, model := range models {
		seen[strings.TrimSpace(model.Key)] = true
	}
	for _, model := range extra {
		key := strings.TrimSpace(model.Key)
		if seen[key] {
			continue
		}
		if probe != nil && !probe(model) {
			continue
		}
		models = append(models, model)
	}
	return models
}

// 探测结论的缓存时长：放行缓存久一点（探测要真打一次上游），
// 拒绝缓存短一点，权限恢复后能较快回到目录里。
const (
	accessAllowedTTL = 2 * time.Hour
	accessDeniedTTL  = 30 * time.Minute
)

type accessVerdict struct {
	allowed bool
	at      time.Time
}

// entitlementCache 缓存「目录外模型对当前区域是否可用」的探测结论。
//
// 必须挂在 adapter 实例上：国内版与国际版是两套账号，结论不能互相复用。
type entitlementCache struct {
	mu    sync.Mutex
	byKey map[string]accessVerdict
}

// allowed 返回 key 是否可用；结论过期或不存在时调用 probe 重新探测。
func (c *entitlementCache) allowed(key string, probe func() bool) bool {
	now := time.Now()
	c.mu.Lock()
	if verdict, ok := c.byKey[key]; ok {
		ttl := accessDeniedTTL
		if verdict.allowed {
			ttl = accessAllowedTTL
		}
		if now.Sub(verdict.at) < ttl {
			c.mu.Unlock()
			return verdict.allowed
		}
	}
	c.mu.Unlock()

	allowed := probe()

	c.mu.Lock()
	if c.byKey == nil {
		c.byKey = make(map[string]accessVerdict)
	}
	c.byKey[key] = accessVerdict{allowed: allowed, at: now}
	c.mu.Unlock()
	return allowed
}

// withSupplemental 按「当前账号是否真有权限」把目录外模型补进清单。
func (v *adapter) withSupplemental(ctx context.Context, b *Bridge, models []Model) []Model {
	return appendSupplemental(models, v.region, func(model Model) bool {
		return v.access.allowed(model.Key, func() bool {
			return v.probeModelAccess(ctx, b, model.Key)
		})
	})
}

// probeModelAccess 用一次最小请求判断上游是否对该模型放行。
//
// 代价：一个 max_tokens=1 的请求（排队时不会真正生成内容），且结论按 TTL 缓存，
// 不会每次刷新清单都打上游。
func (v *adapter) probeModelAccess(ctx context.Context, b *Bridge, key string) bool {
	if b == nil {
		return false
	}
	probeCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	messages := []interface{}{map[string]interface{}{"role": "user", "content": "hi"}}
	errProbe := b.CallQoderWithOpts(probeCtx, "", messages, key, nil, CallOpts{MaxTokens: 1}, func(Delta) {})
	if errProbe == nil {
		return true
	}
	var upstream *UpstreamError
	if errors.As(errProbe, &upstream) && upstream.ErrType == ErrTypeModelBusy {
		// 排队说明模型对该账号开放，只是当前要等。
		return true
	}
	detail := errProbe.Error()
	if strings.Contains(detail, `"code":"112"`) || strings.Contains(detail, "pricingUrl") {
		logger.Info("qoder：目录外模型 %s 当前账号无权限，不下发", key)
		return false
	}
	logger.Info("qoder：目录外模型 %s 权限探测失败（%v），不下发", key, errProbe)
	return false
}
