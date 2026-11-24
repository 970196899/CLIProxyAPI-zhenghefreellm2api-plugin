package admin

// 本文件是凭证文件的唯一入口：
//   - saveAuthFile：落盘（添加账号、粘贴回调登录、启用/停用）；
//   - safeAuthFilePath：改写或删除前的全部校验。
//
// 「添加 API Key 账号」与「粘贴 localhost 回调完成登录」两条路径都要把一份凭证
// 交给宿主，只有走这里才不会各自长出一套「宿主写 + 兜底写盘」的副本。启用/停用
// 与删除同理：路径归一化在 hostbridge 边界完成，这里只负责「这个路径能不能动」。

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"freetier2api-plugin/internal/core"
	"freetier2api-plugin/internal/hostbridge"
	"freetier2api-plugin/internal/logger"
)

// authFileDirs 是宿主不可用时的兜底落盘目录：容器里是 /root/.cli-proxy-api，
// 本地直跑时是进程工作目录。
//
// 工作目录只作为最后一道兜底：宿主进程的 CWD 往往就是安装目录，插件不该往那里
// 掉凭证文件。Service.saveAuthFile 会先把 <state_dir>/auths 插到最前面。
var authFileDirs = []string{"/root/.cli-proxy-api", "."}

// authFallbackDirs 返回本实例的兜底目录：状态目录下的 auths 优先，
// 其次是宿主默认凭证目录，最后才是工作目录。
func (s *Service) authFallbackDirs() []string {
	dirs := make([]string, 0, len(authFileDirs)+1)
	if s != nil && s.deps != nil {
		if stateDir := strings.TrimSpace(s.deps.Config().StateDir); stateDir != "" {
			dirs = append(dirs, filepath.Join(stateDir, "auths"))
		}
	}
	return append(dirs, authFileDirs...)
}

// saveAuthFile 落盘一份凭证。
//
// 首选 host.auth.save：它写 auths 目录并热更新宿主内存里的注册表。只有这条
// 回调不可用时才直接写盘（与凭证续期写盘兜底同一个理由）。两条都失败才算失败——
// 调用方必须能把「没保存成功」如实告诉用户，不能出现页面显示成功而磁盘上什么都没有。
func saveAuthFile(callbackID, fileName string, raw []byte) error {
	return saveAuthFileInto(authFileDirs, false, callbackID, fileName, raw)
}

// saveAuthFile 是 saveAuthFileInto 的实例版：兜底目录优先落在状态目录。
func (s *Service) saveAuthFile(callbackID, fileName string, raw []byte) error {
	return saveAuthFileInto(s.authFallbackDirs(), true, callbackID, fileName, raw)
}

// createFirst 为 true 时允许创建第一个目录（状态目录下的 auths 未必已存在）。
func saveAuthFileInto(dirs []string, createFirst bool, callbackID, fileName string, raw []byte) error {
	name := sanitizeAuthFileName(fileName)
	if name == "" || len(raw) == 0 {
		return fmt.Errorf("凭证缺少文件名或内容")
	}

	// hostbridge.SaveAuth 同时负责 uid 大小写归一：落盘兜底必须写同一份内容，
	// 否则磁盘上的 uid 与宿主登记键不一致，Windows 上会多出一条重复账号。
	aligned, errSave := hostbridge.SaveAuth(callbackID, name, raw)
	if errSave == nil {
		logger.Info("授权文件 %s 已通过宿主接口保存", name)
		return nil
	}
	logger.Debug("宿主授权保存失败（%s）：%v", name, errSave)

	for i, dir := range dirs {
		fi, errStat := os.Stat(dir)
		if errStat != nil || !fi.IsDir() {
			if i != 0 || !createFirst {
				continue
			}
			if errMkdir := os.MkdirAll(dir, 0o700); errMkdir != nil {
				continue
			}
		} else if !fi.IsDir() {
			continue
		}
		if errWrite := os.WriteFile(filepath.Join(dir, name), aligned, 0o600); errWrite == nil {
			logger.Info("授权文件 %s 已保存到磁盘", name)
			return nil
		}
	}
	return fmt.Errorf("host.auth.save 不可用且落盘失败（%s）", name)
}

// sanitizeAuthFileName 把供应商给的凭证文件名收敛成一个安全的基名。
//
// 文件名一路从上游响应（登录流程）/账号昵称传到这里，早先是直接
// filepath.Join(dir, name) 落盘：带 `/` 或 `..` 的取值会把凭证写到目录外，
// 绝对路径更会直接忽略 dir。这里只保留基名、去掉控制字符与路径分隔符，
// 扩展名统一 .json；收敛后为空则拒绝写入。
func sanitizeAuthFileName(fileName string) string {
	name := strings.TrimSpace(fileName)
	if name == "" {
		return ""
	}
	// filepath.Base 在 Linux 上不认反斜杠，所以下面还要再替换一次。
	name = filepath.Base(name)
	name = strings.Map(func(r rune) rune {
		switch {
		case r < 0x20, r == 0x7f:
			return -1
		case r == '/', r == '\\', r == ':', r == '*', r == '?', r == '"', r == '<', r == '>', r == '|':
			return '_'
		}
		return r
	}, name)
	name = strings.Trim(name, ". ")
	if name == "" || name == "." || name == ".." {
		return ""
	}
	if !strings.EqualFold(filepath.Ext(name), ".json") {
		name += ".json"
	}
	return name
}

// safeAuthFilePath 校验一个凭证文件是否归本插件所有且可安全改写/删除，
// 返回绝对路径。
//
// 入参 path 必须已经是 hostbridge 归一化过的路径（ListAuths / AuthJSONAndPath
// 的出口都会做这件事）；这里不再换算，只校验——同一件事在两个层各做一次，
// 迟早会出现「有的调用点忘记归一化」的分歧。
//
// 四道检查的顺序是从「不值得尝试」到「必须实际落盘核对」：
//  1. 宿主给的凭证必须属于本插件；运行时凭证没有文件（RuntimeOnly / 路径为空）
//     → 直接拒绝；
//  2. 路径必须是绝对的；文件必须存在——**不存在不能当成功**：静默返回 nil 会让
//     页面显示「已删除」而账号其实还在；
//  3. 只认普通 `.json` 文件：符号链接、目录、别的扩展名一律拒绝；
//  4. 目标文件里的凭证必须与宿主给的凭证是同一个账号（vendor + UID）——路径
//     归一化用的是进程工作目录，基准不一致时同名文件可能属于另一个账号。
//     只比对身份字段，不读取或记录 token。
func safeAuthFilePath(registry *core.Registry, entry core.HostAuth, raw []byte, path string) (string, error) {
	hostCred, okHost := registry.ParseCredential(raw, hostbridge.AccountIdentity(entry), nil)
	if !okHost {
		return "", fmt.Errorf("该凭证不属于本插件，拒绝删除")
	}
	target := strings.TrimSpace(path)
	if entry.RuntimeOnly || target == "" {
		return "", fmt.Errorf("该账号没有对应的凭证文件（运行时凭证），无法删除")
	}
	if !filepath.IsAbs(target) {
		return "", fmt.Errorf("凭证路径未经宿主边界归一化：%s", target)
	}
	if !strings.HasSuffix(strings.ToLower(target), ".json") {
		return "", fmt.Errorf("拒绝删除非凭证文件：%s", filepath.Base(target))
	}
	info, errStat := os.Lstat(target)
	if errStat != nil {
		if os.IsNotExist(errStat) {
			return "", fmt.Errorf("凭证文件不存在，可能已被宿主移除：%s", filepath.Base(target))
		}
		return "", fmt.Errorf("检查文件失败：%w", errStat)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("拒绝删除符号链接：%s", filepath.Base(target))
	}
	if info.IsDir() {
		return "", fmt.Errorf("拒绝删除目录：%s", filepath.Base(target))
	}

	onDisk, errRead := os.ReadFile(target)
	if errRead != nil {
		return "", fmt.Errorf("读取凭证文件失败：%w", errRead)
	}
	fileCred, okFile := registry.ParseCredential(onDisk, strings.TrimSuffix(filepath.Base(target), ".json"), nil)
	if !okFile || !sameCredentialIdentity(hostCred, fileCred) {
		return "", fmt.Errorf("目标文件属于另一个账号，拒绝删除：%s", filepath.Base(target))
	}
	return target, nil
}

// sameCredentialIdentity 判断两份凭证是否指向同一个账号。
//
// vendor 或 UID 取不到时跳过该维度（部分供应商凭证里没有 UID）：宁可放行，
// 也不要卡住正常删除。只比对身份字段，不涉及 token。
func sameCredentialIdentity(hostCred, fileCred *core.Credential) bool {
	if hostVendor, fileVendor := hostCred.VendorIDValue(), fileCred.VendorIDValue(); hostVendor != "" && fileVendor != "" && hostVendor != fileVendor {
		return false
	}
	hostUID, fileUID := hostCred.UIDValue(), fileCred.UIDValue()
	return hostUID == "" || fileUID == "" || hostUID == fileUID
}

// setCredentialDisabled 在凭证 JSON 上设置/清除宿主的 disabled 标记。
//
// 宿主扫盘时只看凭证文件里的 "disabled" 字段（见 file synthesizer）：置 true
// 的凭证不再参与路由，删掉该字段即恢复。插件的启用/停用因此只能落在这个字段上，
// 没有第二条通道。
//
// 用 map[string]json.RawMessage 而不是 map[string]any：只增删一个键，其余字段
// 原样保留字节，float64 往返不会让超过 2^53 的整数（上游 uid）丢精度。
func setCredentialDisabled(raw []byte, disabled bool) ([]byte, error) {
	var payload map[string]json.RawMessage
	if errUnmarshal := json.Unmarshal(raw, &payload); errUnmarshal != nil || payload == nil {
		return nil, fmt.Errorf("凭证不是合法的 JSON 对象")
	}
	if disabled {
		payload["disabled"] = json.RawMessage("true")
	} else {
		delete(payload, "disabled")
	}
	updated, errMarshal := json.MarshalIndent(payload, "", "  ")
	if errMarshal != nil {
		return nil, errMarshal
	}
	return updated, nil
}
