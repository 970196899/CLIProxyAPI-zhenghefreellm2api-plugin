package tests

// 本文件是 FreeTier Plus 身份改造的「定点守卫」。
//
// 它把插件身份钉成一个可执行规格：改名为 freetierplus 之后，仓库里不允许
// 再出现任何**身份语境**下的 freetier 字面量。判据不是「全仓无 freetier」——
// go.mod 的模块名与 200+ 处 import 路径必须保留，autoclaw 的上游指纹头、
// opencodezen 的免费层语义也必须保留。这里按「剔除合法项后仍残留」判定。

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"freetier2api-plugin/internal/admin"
	"freetier2api-plugin/internal/app"
	"freetier2api-plugin/internal/core"
)

// TestPluginIdentityIsFreeTierPlus 是目标状态的四条硬断言。
func TestPluginIdentityIsFreeTierPlus(t *testing.T) {
	if app.PluginID != "freetierplus" {
		t.Fatalf("app.PluginID = %q, want freetierplus", app.PluginID)
	}
	if app.ProviderKey != "freetierplus" || core.ProviderKey != "freetierplus" {
		t.Fatalf("provider key = %q / %q, want freetierplus", app.ProviderKey, core.ProviderKey)
	}
	if app.ManagementRoutePrefix != "/plugins/freetierplus" {
		t.Fatalf("management prefix = %q, want /plugins/freetierplus", app.ManagementRoutePrefix)
	}
	if admin.DefaultStateDirName != ".freetierplus-plugin" {
		t.Fatalf("state dir name = %q, want .freetierplus-plugin", admin.DefaultStateDirName)
	}
	if admin.StateDirEnvOverride != "FREETIERPLUS_PLUGIN_HOME" {
		t.Fatalf("state dir env = %q, want FREETIERPLUS_PLUGIN_HOME", admin.StateDirEnvOverride)
	}
}

// residueAllowed 是身份语境下允许保留 freetier 的文件豁免表
// （键为相对仓库根的斜杠路径）。
var residueAllowed = map[string]string{
	"internal/vendors/autoclaw/chat.go": "上游请求指纹头 x_trace_id",
}

// residueLineAllowed 判定某行是否属于「合法残留」。调用方已剔除模块路径与新身份
// （freetier2api-plugin / freetierplus），因此进入本函数的行里出现的 freetier
// 一定是需要裁决的形态。
func residueLineAllowed(rel string, line string) bool {
	if _, ok := residueAllowed[rel]; ok && strings.Contains(line, "x_trace_id") {
		return true
	}
	// opencodezen 的免费层语义（isFreeTierModel / prepareFreeTierBody /
	// FreeTierError / free tier）全部合法。
	if strings.HasPrefix(rel, "internal/vendors/opencodezen/") {
		return true
	}
	// 双开共存回归测试刻意写出原版 ID 来断言「原版凭证仍被正确处置」：
	// 这里的 freetier 是**被测对象**，不是身份残留。只放行含 freetier 字面量的
	// 行，避免整文件豁免后该文件里未来出现的真实身份也被漏掉。
	if rel == "internal/hostbridge/auth_test.go" && strings.Contains(line, "freetier") {
		trackExemption(rel)
		return true
	}
	// 双开隔离的 type 判据用例刻意写出原版 ID 作为**被测输入**：
	// 这些行断言「原版 type 必须被判为别家」，是隔离语义的证据而非残留。
	if rel == "internal/vendors/cline/credential_test.go" && strings.Contains(line, `"freetier"`) {
		trackExemption(rel)
		return true
	}
	if rel == "internal/vendors/workbuddy/credential_test.go" && strings.Contains(line, `"freetier"`) {
		trackExemption(rel)
		return true
	}
	// 写回边界的用例同样以原版 ID 为**被测输入**：断言它必须被排除。
	// 大小写变体（FreeTier）同样是被测对象，按小写化后比较。
	if rel == "internal/auth/pool_owned_test.go" &&
		strings.Contains(strings.ToLower(line), `"freetier"`) {
		trackExemption(rel)
		return true
	}
	// 双开门禁的实现本体必须点名「哪个 type 属于原版」，这是功能而非残留。
	if rel == "internal/core/credential_json.go" {
		trackExemption(rel)
		return true
	}
	// README 的「双开共存须知」整段必须引用旧键名与旧路径，否则用户无从对照升级。
	// 按章节存在性判定：该节内的行一律放行，节外仍受检。
	if rel == "README.md" && strings.Contains(line, "原版") {
		trackExemption(rel)
		return true
	}
	if rel == "README_EN.md" && (strings.Contains(line, "original upstream") || strings.Contains(line, "the original")) {
		trackExemption(rel)
		return true
	}
	return false
}

// exemptionHits 记录本次扫描中各文件的豁免命中数，由 TestNoResidualFreeTierIdentity
// 每次运行时重置。与 exemptionBudget 双向校验：未登记预算的文件直接失败。
var exemptionHits = map[string]int{}

func trackExemption(rel string) {
	exemptionHits[rel]++
}

// exemptionBudget 给每个被豁免的文件设定上限：豁免数超出预算即失败。
//
// 豁免本身也要受约束——否则「放行一行」会悄悄退化成「放行整个文件」，
// 守卫给出的零残留信号就失真了。
var exemptionBudget = map[string]int{
	"internal/hostbridge/auth_test.go":              4,
	"internal/vendors/cline/credential_test.go":     1,
	"internal/vendors/workbuddy/credential_test.go": 1,
	"internal/auth/pool_owned_test.go":              4,
	"internal/core/credential_json.go":              4,
	"README.md":                                     6,
	"README_EN.md":                                  6,
}

// TestNoResidualFreeTierIdentity 扫描受管文件集，断言身份语境下零残留。
func TestNoResidualFreeTierIdentity(t *testing.T) {
	root := filepath.Join("..", "..")
	skipDirs := map[string]bool{".git": true, "docs": true, "cpasdk": true, "dist": true}
	// 本文件是扫描器本体：它必然含有 freetier 字面量与失败文案，跳过自身。
	self := "tests/integration/identity_residue_test.go"
	managedExt := map[string]bool{".go": true, ".html": true, ".json": true, ".yml": true, ".md": true, ".sh": true}
	// 大小写不敏感：FreeTier 也是身份残留（显示名/注释）。
	residue := regexp.MustCompile(`(?i)freetier`)

	// 每次运行重置豁免计数（同包内多次调用时互不污染）。
	exemptionHits = map[string]int{}

	var offenders []string
	errWalk := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if skipDirs[entry.Name()] {
				return fs.SkipDir
			}
			return nil
		}
		if !managedExt[strings.ToLower(filepath.Ext(path))] {
			return nil
		}
		raw, errRead := os.ReadFile(path)
		if errRead != nil {
			return errRead
		}
		rel, errRel := filepath.Rel(root, path)
		if errRel != nil {
			return errRel
		}
		rel = filepath.ToSlash(rel)
		if rel == self {
			return nil
		}
		for index, line := range strings.Split(string(raw), "\n") {
			// 先剔除 Go 模块路径与新身份：freetierplus 本身含 freetier 前缀，
			// 不剔掉它会把所有正确写法判成残留。
			// 大小写三种形态都要剔（freetierplus / FreeTierPlus / FreeTier Plus），
			// 否则显示名「FreeTier Plus」会被误判。
			stripped := strings.ReplaceAll(line, "freetier2api-plugin", "")
			for _, identity := range []string{"freetierplus", "FreeTierPlus", "FreeTier Plus", "FREETIERPLUS"} {
				stripped = strings.ReplaceAll(stripped, identity, "")
			}
			if !residue.MatchString(stripped) {
				continue
			}
			if residueLineAllowed(rel, stripped) {
				continue
			}
			offenders = append(offenders, rel+":"+strconv.Itoa(index+1)+": "+strings.TrimSpace(line))
		}
		return nil
	})
	if errWalk != nil {
		t.Fatalf("walk repo: %v", errWalk)
	}
	// 豁免本身也要受约束：未登记预算的文件一律失败，已登记的不得超预算。
	// 双向校验是刻意的——只查「超预算」会留下一个结构性缺口：
	// 新增豁免文件只要忘了登记预算，就完全不受检。
	for rel, used := range exemptionHits {
		budget, ok := exemptionBudget[rel]
		if !ok {
			t.Fatalf("%s 有 %d 处豁免但未登记预算——新增豁免必须同时更新 exemptionBudget", rel, used)
		}
		if used > budget {
			t.Fatalf("%s 的身份豁免用了 %d 处，超出预算 %d——豁免不得成为身份残留的通道",
				rel, used, budget)
		}
	}
	if len(offenders) > 0 {
		t.Fatalf("仍有 %d 处身份残留 freetier（应为 freetierplus）：\n%s",
			len(offenders), strings.Join(offenders, "\n"))
	}
}
