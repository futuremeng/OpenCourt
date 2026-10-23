package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDotEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	content := "# 注释\nFOO_TEST_KEY=bar-value\nQUOTED_TEST_KEY=\"quoted\"\nBAD LINE\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := LoadDotEnv(path); err == nil {
		t.Fatal("格式错误行应报错")
	}
	_ = os.WriteFile(path, []byte("# 注释\nFOO_TEST_KEY=bar-value\nQUOTED_TEST_KEY=\"quoted\"\n"), 0o600)
	if err := LoadDotEnv(path); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("FOO_TEST_KEY") != "bar-value" {
		t.Errorf(".env 注入失败: %q", os.Getenv("FOO_TEST_KEY"))
	}
	if os.Getenv("QUOTED_TEST_KEY") != "quoted" {
		t.Errorf("引号未剥离: %q", os.Getenv("QUOTED_TEST_KEY"))
	}
	// 已存在的环境变量优先，不被覆盖。
	_ = os.Setenv("FOO_TEST_KEY", "existing")
	if err := LoadDotEnv(path); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("FOO_TEST_KEY") != "existing" {
		t.Error("已存在的环境变量不应被 .env 覆盖")
	}
	_ = os.Unsetenv("FOO_TEST_KEY")
	_ = os.Unsetenv("QUOTED_TEST_KEY")
	if err := LoadDotEnv(filepath.Join(dir, "missing.env")); err != nil {
		t.Error("文件不存在应静默返回")
	}
}
