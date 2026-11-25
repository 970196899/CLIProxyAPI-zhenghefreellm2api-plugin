# Zhenghe FreeLLM 2API Plugin

[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![CLIProxyAPI](https://img.shields.io/badge/CLIProxyAPI-Plugin-2496ED?style=flat)](https://github.com/router-for-me/CLIProxyAPI)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

将多个免费额度 AI 供应商接入 [CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI) 的原生动态库插件，直接复用宿主的 OpenAI / Anthropic / Responses 兼容路由、鉴权、代理与负载均衡。

中文 | [English](README_EN.md)

如果这个项目帮到了你，欢迎点个 Star 支持一下。

> **本仓库说明**：上游 `jiandanc/CLIProxyAPI-freetier2api-plugin` 已随作者账号一并从 GitHub 消失（账号与仓库均为 404）。此后由 `cxy2730` 接手维护至 `v0.1.62`：该阶段插件 ID 为 `freetier`。**自 `v0.1.63` 起本仓库由 `970196899` 独立维护，插件 ID 改为 `zhenghefreellm`，显示名为 `Zhenghe FreeLLM 2API`，构建产物与商店归档同步改名。** 插件 ID 变化意味着这是一次**替换安装**：旧的 `freetier` 动态库需换成新的 `zhenghefreellm` 动态库，`config.yaml` 里的配置段也要从 `plugins.configs.freetier` 改为 `plugins.configs.zhenghefreellm`；**凭证与状态无需迁移**——内部 `ProviderKey` 仍是 `freetier`，状态目录仍是 `~/.freetier-plugin`，原有账号可直接沿用。完整提交历史保留，`v0.1.44` 及更早的 tag 只存在于归档仓库 `cxy2730/CLIProxyAPI-freetier2api-plugin-legacy`。

## 兼容性说明（x86-64）

实测定位（详见 PR #2）：**v0.1.60–v0.1.65** 的 linux/amd64 构建在无 AVX 的旧 x86-64 CPU（如 Intel J4125，Goldmont Plus）上 dlopen 即 `SIGILL`——gdb 断点定位在 runtime/cgo 的 `x_cgo_sigaction`，gcc 把 152 字节 struct sigaction 清零向量化成了 `vmovdqu %ymm0`（AVX 指令）。根因：`ubuntu-latest` 于 2026-10-19 起迁移到 Ubuntu 26.04（v0.1.59 构建于 11-12 仍正常，v0.1.60 构建于 11-16 开始 SIGILL），新 runner 的 gcc 给 runtime/cgo 的 C 代码默认开了 AVX；而 `CC`/`CGO_CFLAGS` 均无法覆盖 runtime/cgo 的编译参数（Go 固定行为），只能钉 runner（本 PR：linux 钉 `ubuntu-24.04`）或容器化构建。宿主 CPU 无 AVX 时暂请停留在 v0.1.59 或使用自建构建（同源码 + golang:bookworm 容器构建已验证可正常加载 v0.1.65）。

## 功能特性

- 复用宿主 Anthropic `/v1/messages`、OpenAI `/v1/chat/completions` 与 OpenAI `/v1/responses` 接口，无需额外部署独立网关
- 支持 8 大体系共 14 个国内外免费额度供应商实例（AutoClaw 国内/国际与小浣熊已接入），凭证严格按供应商隔离，账号数量不设上限
- 模型清单动态探测、自定义模型别名与跨供应商同名模型自动负载均衡/故障转移
- 内嵌单页 Web 控制台：OAuth 设备授权 / 本地回调粘贴 / API Key 添加账号、账号别名、批量启停删除、额度可视化、签到、模型启停与设置（任务中心与运行日志页已移除，日志统一走宿主官方页）
- 控制台账号视图按渠道分页（全部 / 各供应商，带账号数），行高压到约 34px，11 个账号一屏看完不用滚动；按钮按语义分组，窄屏排成两列网格，不会被压到比文字还窄
- 请求体原样透传：不注入、不替换、不追加任何提示词，也不改写 `system` / `tools` / 采样参数
- 支持每日自动签到、每日自动任务与 Token 自动续期保活（签到记录只保留当天，跨天自动清空）
- 一键任务遇上游 429 立即收尾：剩余任务记成「已跳过」而不是逐条失败，并给该账号记 10 分钟冷却，冷却期内重跑直接跳过
- 不在宿主面板注册 OAuth 登录项：插件不出现在 CLIProxyAPI「OAuth 登录」列表里，加账号与登录一律走插件控制台
- 纯 Go 标准库实现，零第三方运行时依赖

## 支持的供应商

| 供应商 ID | 展示名称 | 区域 | 认证方式 | 凭证文件名前缀 |
| --- | --- | --- | --- | --- |
| `workbuddycn` | WorkBuddy 国内版 | `cn` | OAuth 设备授权 | `workbuddycn-*.json` |
| `workbuddyglobal` | WorkBuddy 国际版 | `global` | OAuth 设备授权 | `workbuddyglobal-*.json` |
| `autoclawcn` | AutoClaw 国内版 | `cn` | API Key | `autoclawcn-*.json` |
| `autoclawglobal` | AutoClaw 国际版 | `global` | API Key | `autoclawglobal-*.json` |
| `qodercn` | Qoder 国内版 | `cn` | OAuth 设备授权 | `qodercn-*.json` |
| `qoderglobal` | Qoder 国际版 | `global` | OAuth 设备授权 | `qoderglobal-*.json` |
| `raccoon` | 小浣熊 | — | API Key | `raccoon-*.json` |
| `cline` | Cline | `global` | WorkOS 设备授权 | `cline-*.json` |
| `zcodecn` | ZCode 国内版 | `cn` | OAuth CLI 授权（智谱开放平台 bigmodel） | `zcodecn-*.json` |
| `zcodeglobal` | ZCode 国际版 | `global` | OAuth CLI 授权（z.ai） | `zcodeglobal-*.json` |
| `traecn` | Trae 国内版 | `cn` | OAuth 授权 | `traecn-*.json` |
| `traeglobal` | Trae 国际版 | `global` | OAuth 授权 | `traeglobal-*.json` |
| `codearts` | CodeArts | `cn` | OAuth 授权 | `codearts-*.json` |
| `minimaxcodecn` | MiniMax Code 国内版 | `cn` | OAuth 设备授权 | `minimaxcodecn-*.json` |
| `minimaxcodeglobal` | MiniMax Code 国际版 | `global` | OAuth 设备授权 | `minimaxcodeglobal-*.json` |

## 快速开始

> **从 `freetier` 版本升级**：**先删掉 `plugins/` 下的旧 `freetier.so` / `freetier.dll`，再装新版，然后把 `config.yaml` 的配置段改名为 `plugins.configs.zhenghefreellm`。**
>
> 原因：新旧两个插件**可以同时运行在同一进程内**（实测日志可见两者并行加载），而 `plugins.configs.<id>.enabled` 控制的是加载与卸载——只要两个配置段都启用，两个实例就都活着。两库又共用同一凭证标识与状态目录（`state_dir` 默认固定为 `~/.cli-proxy-api/freetier`，不随插件 ID 变化，这正是账号无需迁移的原因），同时启用会写同一份 `state.json`、重复执行签到任务。

### 方式一：通过 CPA 插件商店安装

本仓库自带 `registry.json`，可作为三方插件源使用。在 CPA 的插件源配置里加入：

```text
https://raw.githubusercontent.com/970196899/CLIProxyAPI-zhenghefreellm2api-plugin/main/registry.json
```

1. 打开 CLIProxyAPI 管理面板 → **插件商店**。
2. 搜索 **Zhenghe FreeLLM 2API** 并点击一键安装。

安装后把 `config.yaml` 里的配置段从 `plugins.configs.freetier` 改名为 `plugins.configs.zhenghefreellm`（见下方升级说明）。

### 方式二：手动安装 Release 动态库

从 [Releases](https://github.com/970196899/CLIProxyAPI-zhenghefreellm2api-plugin/releases) 下载对应平台的归档包，解压后将动态库（`zhenghefreellm.so` / `zhenghefreellm.dll`）放入 CPA 插件目录，并在 `config.yaml` 中启用：

```yaml
plugins:
  enabled: true
  dir: "/path/to/cpa/plugins"
  configs:
    zhenghefreellm:
      enabled: true
      enabled_realms: "cn,global"
```

> 升级自 `freetier` 版本时：把 `plugins.configs.freetier` 整段改名为 `plugins.configs.zhenghefreellm`，并移除旧的 `freetier` 动态库。账号与状态目录不需要迁移。
>
> **若你曾自定义过该段里的 `state_dir`**，务必把这一项一并保留。漏掉它插件会回落到默认状态目录，页面显示零账号——账号文件其实还在原处没丢，但看起来像升级把号弄没了。

### 方式三：源码编译

需 Go 1.22+ 与 C 编译器（启用 `CGO_ENABLED=1`），或直接使用仓库内置的 Docker 构建脚本（基于 `golang:1.24` Debian glibc 镜像）：

```bash
git clone https://github.com/970196899/CLIProxyAPI-zhenghefreellm2api-plugin.git
cd CLIProxyAPI-zhenghefreellm2api-plugin
./scripts/build.sh
```

编译产物输出至 `dist/zhenghefreellm.<so|dylib|dll>`。

## 使用方法

### 1. 登录控制台并添加账号

启动 CLIProxyAPI 后，访问内嵌控制台页面：

```text
http://<CPA地址>:8317/v0/resource/plugins/zhenghefreellm/console
```

- 点击 **「添加账号 ▾」** 选择对应供应商，按提示在浏览器完成 OAuth 授权或填入 API Key（账号数量不设上限）。
- 若在远程服务器 / Docker / WSL 部署，使用 Trae 或 CodeArts 登录时浏览器停留在 `127.0.0.1` 回调页，展开登录框下方的 **「回调没到插件？粘贴回调链接」**，将完整回调 URL 粘贴提交即可。

### 2. 调用 API

账号添加后即可通过 CLIProxyAPI 标准接口调用。模型对外 ID 固定为 `<供应商前缀>-<上游模型名>`，前缀写死不可配置，例如 `qodercn-GLM-5.3`、`workbuddyglobal-deepseek-v4.1-flash`，完整清单用 `GET /v1/models` 取：

```bash
# OpenAI Chat Completions
curl http://localhost:8317/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <CPA_API_KEY>" \
  -d '{"model":"qodercn-GLM-5.3","messages":[{"role":"user","content":"你好！"}]}'

# Anthropic Messages
curl http://localhost:8317/v1/messages \
  -H "Content-Type: application/json" \
  -H "anthropic-version: 2023-06-01" \
  -H "x-api-key: <CPA_API_KEY>" \
  -d '{"model":"qodercn-GLM-5.3-Flash","max_tokens":1024,"messages":[{"role":"user","content":"你好！"}]}'
```

### 3. 模型别名与跨供应商路由

在控制台「模型」表格中点击 **「模型 ID」** 列的名称或 `✎` 图标即可设置对外模型别名。将不同供应商的模型设为同一别名（例如把多家供应商的旗舰模型统一设为 `coding-max`），宿主会自动合并并在多个供应商间负载均衡与故障转移。

### 4. 账号别名

账号概览的「账号」列默认显示上游回传的昵称/邮箱，部分平台只给不可修改的默认昵称，多账号时很难分辨。点击该列的名称或 `✎` 图标即可设置自定义别名，编辑框会预填当前名称；展示顺序变为 **别名 → 上游昵称/邮箱 → 文件名 → 账号 ID**。留空保存、或填回原名称，都视为不设别名（直接清除记录、沿用上游名称），未设置别名时行为不变。

别名按**账号身份**（凭据文件名，即 `state.json` 里的 `account_aliases` 键）记录，与上游昵称、token 无关，因此 token 刷新、重登写回同一凭证文件后别名都不会丢。

### 5. 一键任务与限流

控制台「一键任务」会对勾选的账号依次执行签到、成长任务、活动福利等。几个行为约定：

- **已领过的不再重复报错**：供应商回「已领取 / 已签到 / 无此任务」时按正常状态跳过，不计入失败，也不会出现在结果摘要的详情里。
- **命中 429 立即收尾**：同一账号的第一个 429 之后不再继续打剩余任务（上游按账号判风控，重试间隔再长也没用），剩余任务记成「上游限流（429），已跳过剩余任务」，摘要里只保留一条真实错误。
- **账号级冷却**：命中限流后该账号进入 10 分钟冷却（记录在 `state.json` 的 `rate_limited_until`），冷却期内再点一键任务会直接跳过并说明「至 HH:MM」，到期自动恢复，无需手动干预。
- 结果摘要只列真正需要处理的问题；`已领过 / 已签到 / 无此任务 / 跳过剩余任务` 只计入「未执行」的数量，不占详情行。

## 配置说明

在宿主 `config.yaml` 的 `plugins.configs.zhenghefreellm` 下可配置以下常用项（大多数运行期设置可直接在控制台页面调整并即时生效）：

| 配置键 | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `enabled_realms` | string | `cn,global` | 启用的区域（`cn` / `global`） |
| `auto_checkin` | bool | `true` | 每日自动签到开关 |
| `auto_checkin_at` | string | `10:00` | 每日自动签到时间（`HH:MM`，本地时区）。排程粒度为整点，分钟不为 0 时顺延到下一整点，保证不会早于设定时刻执行 |
| `auto_tasks` | bool | `true` | 每日自动任务开关 |
| `auto_raccoon_daily_grant` | bool | `true` | 小浣熊「每日积分」任务开关：每日调用网页端 `setting_info` 触发服务器发放当日每日积分（幂等，已发不重复；默认 08:00 执行） |
| `codearts_credit_enabled` | bool | `false` | 明确开启 CodeArts 积分推理（消耗账号积分）；默认福利通道，失败不自动切换。修改后刷新模型目录 |
| `disabled_models` | string / list | 空 | 禁用模型列表（支持 `vendor:model` 或裸名，与控制台勾选合并） |
| `extra_models` | string | 空 | 额外补充注册的模型名（逗号分隔） |
| `state_dir` | string | `~/.freetier-plugin` | 状态与缓存目录（若存在 `~/.cli-proxy-api` 则默认使用其下 `freetier` 子目录） |
| `log_level` | string | `info` | 日志级别：`debug` / `info` / `error` |
| `log_to_file` | bool | `false` | 额外把日志写入 `<state_dir>/logs/`，按天分文件 |
| `log_to_host` | bool | `true` | 把插件日志镜像进宿主 `main.log`，CLIProxyAPI 面板的日志页可直接查看 |
| `host_log_file` | string | 自动探测 | 宿主日志文件路径。留空时按 `state_dir` 反推 `<base>/auths/logs/main.log` 或 `<base>/logs/main.log`，都不存在则不启用 |
| `host_log_level` | string | `info` | 镜像到宿主日志的最低级别：`debug` / `info` / `error` |

## 项目架构

```text
package main / cgo        C ABI 入口（main.go / cgo.go）
  └─ internal/app         组合根与 ABI 适配（生命周期、RPC 分发、五类能力）
       ├─ internal/admin    管理接口：HTTP 路由/响应 + 全局配置 + 模型目录 + 控制台页
       ├─ internal/tasks    供应商无关的任务目录、排程、队列与执行引擎
       ├─ internal/auth     账号枚举/去重/批量执行/额度格式化 + 登录会话登记
       ├─ internal/vendors/<name>   供应商协议与适配层（单包，文件名封闭白名单）
       │    ├─ adapter.go           [必备] core.Vendor 实现 + Instances/Configure/Match + ConfigFields
       │    ├─ chat.go              [必备] 协议端点/签名/请求头/载荷加工/传输/Chat/ChatStream/SSE
       │    ├─ credential.go        [必备] 凭证解析/归属判定/存储写回/设备身份/凭证辅助
       │    ├─ models.go            [必备] 模型目录/探测 + bundledModels() 兜底
       │    ├─ errors.go            [必备] 上游错误分类与可重试判定
       │    ├─ login.go             [必备] AuthMode + 登录流程 + 续期（只能有一个）
       │    ├─ tasks.go             [可选] 额度/签到/套餐领取/任务/每日奖励/账号资料
       │    ├─ *_test.go            [按需] 与源码同目录；实况测试用 *_live_test.go
       │    └─ assets/              [按需] go:embed 的内嵌资源
       └─ internal/core            供应商无关契约（Vendor、能力接口、注册表、凭证、错误分类）
```

- 供应商包只允许依赖 `core`（契约）+ `hostbridge`（宿主通信）+ `logger`（日志），多一个都不行。
- `internal/core` 只依赖标准库；`internal/app` 是最上层，且只有 `internal/app/vendors.go` 可以 import 具体供应商。
- 新增内部包必须显式声明分层边界（`layerRules` / `internalPackages`）。
- 根包（C ABI）的单测与源码同目录（`cgo_test.go`；宿主内存 / 只读页用例在 POSIX 上的 `cgo_hostbuffer_test.go`），不放进 `tests/`。

## 致谢与第三方说明

本项目的宿主契约、部分供应商协议研究与模型元数据参考或衍生自以下开源项目，特此致谢：

- **[router-for-me/CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI)**（MIT）：宿主插件 ABI/API 契约（`cpasdk/`）与 HTTP/SSE 桥接设计。
- **[Sliverkiss/workbuddy2api](https://github.com/Sliverkiss/workbuddy2api)** 与 **[linguo2625469/workbuddy2api-panel](https://github.com/linguo2625469/workbuddy2api-panel)**（MIT）：WorkBuddy 协议适配、提示词过滤与日常任务体系参考。
- **[aimod-cc/Agent2API](https://github.com/aimod-cc/Agent2API)**：AutoClaw 与小浣熊协议适配（头集合/签名、proxy 通道、授权登录、模型目录、积分与签到口径）参考。

## 参与贡献

欢迎友好交流。遇到问题时，建议先让 Claude Code、Codex 等工具帮忙排查一下，大部分问题都能自己解决。如果能直接提个 PR 就更好了。

## 许可证

[MIT](LICENSE)
