// Package config 配置管理（开发文档 8.4、9.3）：
// config/settings.yaml 为主配置，支持环境变量覆盖（前缀 OPENCOURT_）。
// 敏感信息（API Key 等）严禁写入配置文件，必须经 .env 或 Secret Manager 注入。
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Settings 主配置。
type Settings struct {
	Env     string `yaml:"env"` // dev / staging / prod
	Profile struct {
		Active string `yaml:"active"` // court / plaintiff / defendant / party
	} `yaml:"profile"`
	// Model 统一走 OpenAI 兼容接口（/chat/completions、/embeddings），
	// 不假定具体服务商；默认禁用；API Key 只能经环境变量注入。
	Model struct {
		Provider       string `yaml:"provider"` // openai（兼容接口）
		BaseURL        string `yaml:"base_url"`
		APIKeyEnv      string `yaml:"api_key_env"`
		ReasoningModel string `yaml:"reasoning_model"`
		EmbeddingModel string `yaml:"embedding_model"`
		Enabled        bool   `yaml:"enabled"` // 默认 false：默认离线
	} `yaml:"model"`
	Retrieve struct {
		VectorDim int `yaml:"vector_dim"`
		TopK      int `yaml:"top_k"`
	} `yaml:"retrieve"`
	Security struct {
		RedactLevel string `yaml:"redact_level"` // low / medium / high
		Telemetry   bool   `yaml:"telemetry_enabled"`
	} `yaml:"security"`
	// OpenLaw 法条源（M4 对接）：OpenLaw 数据目录（db/ pdf/ md/ provenance/
	// manifest/）。空 = 未配置法条源，应用降级运行（框架不内置法条数据）。
	OpenLaw struct {
		DataDir  string `yaml:"data_dir"`
		IndexURL string `yaml:"index_url"` // 数据包发布索引（应用内勾选下载）
	} `yaml:"openlaw"`
	// BrowserOS 浏览器辅助（外部可选组件，默认禁用；AGPL-3.0，本仓库不分发）：
	// 指向用户自装 BrowserOS neo 的 MCP 端点（本机回环，端口随其构建变化，
	// 从浏览器内 MCP 页复制）。空 = 未配置，联网查阅功能整体不可用（默认离线）。
	BrowserOS struct {
		Endpoint string `yaml:"endpoint"`
		Enabled  bool   `yaml:"enabled"`
	} `yaml:"browseros"`
	// App 程序自身更新（GitHub Releases latest API；空 = 关闭自动检查）。
	App struct {
		UpdateAPI string `yaml:"update_api"`
	} `yaml:"app"`
	DataDir string `yaml:"data_dir"`
}

// Default 返回默认配置：默认离线、默认遥测关闭、脱敏级别 high。
func Default() *Settings {
	s := &Settings{Env: "dev", DataDir: "data"}
	s.Profile.Active = "court"
	s.Model.Provider = "openai"
	s.Model.BaseURL = "https://api.openai.com/v1"
	s.Model.APIKeyEnv = "OPENCOURT_API_KEY"
	s.Model.ReasoningModel = "gpt-4o-mini"
	s.Model.EmbeddingModel = "text-embedding-3-small"
	s.Model.Enabled = false
	s.Retrieve.VectorDim = 768
	s.Retrieve.TopK = 5
	s.Security.RedactLevel = "high"
	s.Security.Telemetry = false
	// 数据包发布索引默认源：OpenLaw 仓库 GitHub Releases 的 index.json
	// （发布流程由 scripts/package-lawdata.sh 生成并上传；镜像用 index_url 覆盖）。
	s.OpenLaw.IndexURL = "https://github.com/futuremeng/OpenLaw/releases/latest/download/index.json"
	s.App.UpdateAPI = "https://api.github.com/repos/futuremeng/OpenCourt/releases/latest"
	return s
}

// Load 读取 YAML 配置并叠加 OPENCOURT_* 环境变量覆盖。
// path 不存在时返回默认配置（仍应用环境变量覆盖）。
func Load(path string) (*Settings, error) {
	s := Default()
	if data, err := os.ReadFile(path); err == nil {
		if err := yaml.Unmarshal(data, s); err != nil {
			return nil, fmt.Errorf("解析配置文件失败: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	s.applyEnv()
	if err := s.validate(); err != nil {
		return nil, err
	}
	return s, nil
}

// LoadDotEnv 读取 .env 文件并注入环境变量（已存在的变量不覆盖）。
// 仅用于本地开发/桌面端注入敏感信息（如 API Key）；文件本身在
// .gitignore 中，严禁提交。格式：KEY=VALUE，# 开头为注释。
func LoadDotEnv(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for i, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			return fmt.Errorf(".env 第 %d 行格式错误（应为 KEY=VALUE）", i+1)
		}
		key = strings.TrimSpace(key)
		val = strings.Trim(strings.TrimSpace(val), `"'`)
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, val)
		}
	}
	return nil
}

// applyEnv 应用 OPENCOURT_ 前缀的环境变量覆盖（开发文档 8.4）。
func (s *Settings) applyEnv() {
	get := func(key string) (string, bool) { return os.LookupEnv("OPENCOURT_" + key) }
	if v, ok := get("ENV"); ok {
		s.Env = v
	}
	if v, ok := get("PROFILE_ACTIVE"); ok {
		s.Profile.Active = v
	}
	if v, ok := get("MODEL_PROVIDER"); ok {
		s.Model.Provider = v
	}
	if v, ok := get("MODEL_BASE_URL"); ok {
		s.Model.BaseURL = v
	}
	if v, ok := get("MODEL_REASONING"); ok {
		s.Model.ReasoningModel = v
	}
	if v, ok := get("MODEL_EMBEDDING"); ok {
		s.Model.EmbeddingModel = v
	}
	if v, ok := get("MODEL_ENABLED"); ok {
		s.Model.Enabled = parseBool(v)
	}
	if v, ok := get("OPENLAW_DATA_DIR"); ok {
		s.OpenLaw.DataDir = v
	}
	if v, ok := get("BROWSEROS_ENDPOINT"); ok {
		s.BrowserOS.Endpoint = v
	}
	if v, ok := get("BROWSEROS_ENABLED"); ok {
		s.BrowserOS.Enabled = parseBool(v)
	}
	if v, ok := get("OPENLAW_INDEX_URL"); ok {
		s.OpenLaw.IndexURL = v
	}
	if v, ok := get("UPDATE_API"); ok {
		s.App.UpdateAPI = v
	}
	if v, ok := get("RETRIEVE_VECTOR_DIM"); ok {
		if n, err := strconv.Atoi(v); err == nil {
			s.Retrieve.VectorDim = n
		}
	}
	if v, ok := get("RETRIEVE_TOP_K"); ok {
		if n, err := strconv.Atoi(v); err == nil {
			s.Retrieve.TopK = n
		}
	}
	if v, ok := get("SECURITY_REDACT_LEVEL"); ok {
		s.Security.RedactLevel = v
	}
	if v, ok := get("TELEMETRY"); ok {
		s.Security.Telemetry = parseBool(v)
	}
	if v, ok := get("DATA_DIR"); ok {
		s.DataDir = v
	}
}

func parseBool(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func (s *Settings) validate() error {
	switch s.Profile.Active {
	case "court", "plaintiff", "defendant", "party":
	default:
		return fmt.Errorf("非法 profile.active: %q", s.Profile.Active)
	}
	switch s.Security.RedactLevel {
	case "low", "medium", "high":
	default:
		return fmt.Errorf("非法 security.redact_level: %q", s.Security.RedactLevel)
	}
	return nil
}
