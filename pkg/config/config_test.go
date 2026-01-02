package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "noraneko-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Test loading with no existing config file
	cfg, err := Load(tmpDir)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Check defaults
	if cfg.Branch != DefaultBranch {
		t.Errorf("Expected branch %s, got %s", DefaultBranch, cfg.Branch)
	}

	if cfg.UpdateSelf != true {
		t.Error("Expected UpdateSelf to be true by default")
	}

	if cfg.IgnoreCrlErrors != false {
		t.Error("Expected IgnoreCrlErrors to be false by default")
	}

	// Check that config file was created
	if _, err := os.Stat(cfg.ConfigFile); os.IsNotExist(err) {
		t.Error("Config file was not created")
	}
}

func TestLoadExistingConfig(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "noraneko-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a config file with custom settings
	configContent := `[Settings]
Path=C:\Program Files\Noraneko\noraneko.exe
WorkDir=D:\Temp
UpdateSelf=0
IgnoreCrlErrors=1
Branch=beta
`
	configPath := filepath.Join(tmpDir, ConfigFileName)
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Load the config
	cfg, err := Load(tmpDir)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Check values
	if cfg.Path != "C:\\Program Files\\Noraneko\\noraneko.exe" {
		t.Errorf("Expected path 'C:\\Program Files\\Noraneko\\noraneko.exe', got '%s'", cfg.Path)
	}

	if cfg.WorkDir != "D:\\Temp" {
		t.Errorf("Expected workdir 'D:\\Temp', got '%s'", cfg.WorkDir)
	}

	if cfg.UpdateSelf != false {
		t.Error("Expected UpdateSelf to be false")
	}

	if cfg.IgnoreCrlErrors != true {
		t.Error("Expected IgnoreCrlErrors to be true")
	}

	if cfg.Branch != "beta" {
		t.Errorf("Expected branch 'beta', got '%s'", cfg.Branch)
	}
}

func TestSave(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "noraneko-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &Config{
		Path:            "C:\\Test\\noraneko.exe",
		WorkDir:         "D:\\Temp",
		UpdateSelf:      false,
		IgnoreCrlErrors: true,
		Branch:          "stable",
		ExeDir:          tmpDir,
		ConfigFile:      filepath.Join(tmpDir, ConfigFileName),
	}

	if err := cfg.Save(); err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	// Read the file and verify content
	data, err := os.ReadFile(cfg.ConfigFile)
	if err != nil {
		t.Fatalf("Failed to read saved config: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "Path=C:\\Test\\noraneko.exe") {
		t.Error("Saved config missing correct path")
	}

	if !strings.Contains(content, "UpdateSelf=0") {
		t.Error("Saved config missing UpdateSelf=0")
	}

	if !strings.Contains(content, "IgnoreCrlErrors=1") {
		t.Error("Saved config missing IgnoreCrlErrors=1")
	}

	if !strings.Contains(content, "Branch=stable") {
		t.Error("Saved config missing Branch=stable")
	}
}

func TestLogEntry(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "noraneko-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create initial config
	cfg, err := Load(tmpDir)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Add log entries
	if err := cfg.LogEntry("LastRun", "2024-01-01 12:00:00"); err != nil {
		t.Fatalf("Failed to write log entry: %v", err)
	}

	if err := cfg.LogEntry("LastResult", "No new version found"); err != nil {
		t.Fatalf("Failed to write log entry: %v", err)
	}

	// Verify the entries
	data, err := os.ReadFile(cfg.ConfigFile)
	if err != nil {
		t.Fatalf("Failed to read config file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "[Log]") {
		t.Error("Config missing [Log] section")
	}

	if !strings.Contains(content, "LastRun=2024-01-01 12:00:00") {
		t.Error("Config missing LastRun entry")
	}

	if !strings.Contains(content, "LastResult=No new version found") {
		t.Error("Config missing LastResult entry")
	}
}
