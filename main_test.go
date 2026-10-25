package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSettingsAppDirPriority(t *testing.T) {
	appDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(appDir, "settings.yaml"),
		[]byte("profile:\n  active: plaintiff\nopenlaw:\n  data_dir: /x/data\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	wd := t.TempDir()
	if err := os.MkdirAll(filepath.Join(wd, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wd, "config", "settings.yaml"),
		[]byte("profile:\n  active: court\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(wd)

	s, err := loadSettings(appDir)
	if err != nil {
		t.Fatal(err)
	}
	if s.Profile.Active != "plaintiff" || s.OpenLaw.DataDir != "/x/data" {
		t.Errorf("应用数据目录应优先: %+v", s)
	}
}

func TestLoadSettingsDevFallback(t *testing.T) {
	appDir := t.TempDir() // 空：无正式配置
	wd := t.TempDir()
	if err := os.MkdirAll(filepath.Join(wd, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wd, "config", "settings.yaml"),
		[]byte("profile:\n  active: defendant\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(wd)

	s, err := loadSettings(appDir)
	if err != nil {
		t.Fatal(err)
	}
	if s.Profile.Active != "defendant" {
		t.Errorf("开发模式回退应生效: %+v", s)
	}
}

func TestLoadSettingsBrokenFallsBack(t *testing.T) {
	appDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(appDir, "settings.yaml"),
		[]byte("profile: [broken"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := loadSettings(appDir) // 坏配置 → 默认而非崩溃
	if err != nil {
		t.Fatal(err)
	}
	if s.Profile.Active != "court" {
		t.Errorf("坏配置应回退默认: %+v", s)
	}
}
