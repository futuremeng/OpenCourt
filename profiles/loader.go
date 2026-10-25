// Package profiles 立场层（开发文档 第五章）：
// “内核中立，立场外置”。所有诉讼策略、抗辩思路均通过外部 Profile
// 配置文件按需加载，启动时根据用户角色自动加载对应 Profile。
package profiles

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// embedded 内置四立场（随二进制分发）：双击 .app 启动（工作目录非仓库）
// 时兜底；磁盘 profiles/ 目录优先（允许用户自定义立场覆盖内置）。
//
//go:embed court/profile.yaml plaintiff/profile.yaml defendant/profile.yaml party/profile.yaml
var embedded embed.FS

// Profile 一份立场配置（开发文档 5.6 配置表）。
type Profile struct {
	Name          string   `yaml:"name"`
	Role          string   `yaml:"role"` // court / plaintiff / defendant / party
	EnabledAgents []string `yaml:"enabled_agents"`
	DisabledTools []string `yaml:"disabled_tools"`
	DataDir       string   `yaml:"data_dir"`
	OutputRules   []string `yaml:"output_rules"`
}

// ToolEnabled 报告某工具在该 Profile 下是否可用（默认拒绝：出现在
// disabled_tools 中即禁用）。
func (p Profile) ToolEnabled(tool string) bool {
	for _, d := range p.DisabledTools {
		if d == tool {
			return false
		}
	}
	return true
}

// AgentEnabled 报告某 Agent 是否在该 Profile 的启用集合中。
func (p Profile) AgentEnabled(agent string) bool {
	for _, a := range p.EnabledAgents {
		if a == agent {
			return true
		}
	}
	return false
}

// validRoles 四类用户角色（开发文档 1.3）。
var validRoles = map[string]bool{
	"court": true, "plaintiff": true, "defendant": true, "party": true,
}

// Load 从 dir/<role>/profile.yaml 加载 Profile。
func Load(dir, role string) (Profile, error) {
	if !validRoles[role] {
		return Profile{}, fmt.Errorf("非法角色: %q（允许: court/plaintiff/defendant/party）", role)
	}
	path := filepath.Join(dir, role, "profile.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		// 磁盘缺 → 内置兜底（桌面分发双击可用），仍缺才报错
		if data, err = embedded.ReadFile(role + "/profile.yaml"); err != nil {
			return Profile{}, fmt.Errorf("加载 Profile 失败: %w", err)
		}
	}
	var p Profile
	if err := yaml.Unmarshal(data, &p); err != nil {
		return Profile{}, fmt.Errorf("解析 Profile 失败: %w", err)
	}
	if p.Role != role {
		return Profile{}, fmt.Errorf("Profile 角色不一致: 文件名 %s, 内容 %s", role, p.Role)
	}
	return p, nil
}
