package capabilityrouting

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

const (
	Version     = "3.0.1"
	startPrefix = "<!-- contentcloud:routing:start "
	endMarker   = "<!-- contentcloud:routing:end -->"
)

var rules = []string{
	"将本地 Workspace 文件视为跨对话事实源，不从聊天历史推断项目状态。",
	"每个新对话先调用 workspace_context；初始探针只读、离线，不 claim、不写入、不访问云端。",
	"Workspace 只从显式 directory 或 MCP 进程 cwd 唯一解析；无法识别时停止，不扫描子目录猜测项目。",
	"业务状态只写 .contentcloud 和 V3 数字业务目录；普通 Run 禁止修改 .codex、.agents 或 Plugin 安装目录。",
	"30-knowledge/pages/**/*.md 是知识对象唯一可编辑事实源；imports、index 和服务端页面都是可重建投影。",
	"处理客户事实时只使用已接受证据，不用模型常识补全产品事实、权利或合规结论。",
	"开始创作 Run 前调用 environment_execution_plan，只有签名 Manifest、Registry、Lock 和所需 Pack 均 ready 才继续。",
	"缺少任务 Pack 时先调用 environment_prepare_plan 展示精确权限、数据流、费用和新会话影响；仅在用户确认同一 epp_ 计划后调用 environment_prepare_apply。",
	"修改受管业务状态前必须取得所选 Run 的写 claim，并携带最新 context revision；存在多个活动 Run 时必须显式选择，冲突时停止。",
	"所有 Content Work OS 服务端通信都通过 contentcloud CLI 或 contentcloud-local MCP；pull、publish 和服务商副作用必须由用户明确发起。",
	"批准输入先通过 approved_snapshot_inbox/show 从已验证本地缓存读取；只有用户明确要求刷新时才调用 approved_snapshot_pull。",
	"Plugin、Skill、MCP 或路由发生变化后，在验证过的 Workspace Root 中开启新 Codex 对话恢复，不假设当前会话热加载。",
}

type Inspection struct {
	Status  string `json:"status"`
	Version string `json:"version,omitempty"`
	SHA256  string `json:"sha256,omitempty"`
}

func CanonicalText() string {
	return strings.Join(rules, "\n")
}

func SHA256() string {
	sum := sha256.Sum256([]byte(CanonicalText()))
	return hex.EncodeToString(sum[:])
}

func MCPInstructions() string {
	return "Content Work OS 创作路由（版本 " + Version + "，sha256 " + SHA256() + "）：\n- " + strings.Join(rules, "\n- ") + "\n- 首次进入且 onboarding.state 为 needs_project_brief 时，只收集并确认项目简报；确认后重新读取 workspace_context，只展示 onboarding.next_step。"
}

func ManagedBlock() string {
	return fmt.Sprintf(
		"%sversion=%s sha256=%s -->\n## Content Work OS 创作路由\n\n- %s\n%s\n",
		startPrefix,
		Version,
		SHA256(),
		strings.Join(rules, "\n- "),
		endMarker,
	)
}

func Inspect(document string) Inspection {
	start := strings.Index(document, startPrefix)
	if start < 0 {
		return Inspection{Status: "missing"}
	}
	headerEndRelative := strings.Index(document[start:], " -->")
	endRelative := strings.Index(document[start:], endMarker)
	if headerEndRelative < 0 || endRelative < 0 || endRelative < headerEndRelative {
		return Inspection{Status: "outdated"}
	}
	header := document[start+len(startPrefix) : start+headerEndRelative]
	metadata := map[string]string{}
	for _, field := range strings.Fields(header) {
		key, value, ok := strings.Cut(field, "=")
		if ok {
			metadata[key] = value
		}
	}
	inspection := Inspection{Status: "outdated", Version: metadata["version"], SHA256: metadata["sha256"]}
	if inspection.Version == Version && inspection.SHA256 == SHA256() {
		blockEnd := start + endRelative + len(endMarker)
		actual := strings.TrimSpace(document[start:blockEnd]) + "\n"
		if actual == ManagedBlock() {
			inspection.Status = "current"
		}
	}
	return inspection
}

func UpdateManagedBlock(document string) (string, error) {
	start := strings.Index(document, startPrefix)
	if start < 0 {
		trimmed := strings.TrimRight(document, "\n")
		if trimmed == "" {
			return ManagedBlock(), nil
		}
		return trimmed + "\n\n" + ManagedBlock(), nil
	}
	endRelative := strings.Index(document[start:], endMarker)
	if endRelative < 0 {
		return "", errors.New("Content Work OS 创作路由受管块缺少结束标记")
	}
	end := start + endRelative + len(endMarker)
	updated := document[:start] + strings.TrimRight(ManagedBlock(), "\n") + document[end:]
	return strings.TrimRight(updated, "\n") + "\n", nil
}
