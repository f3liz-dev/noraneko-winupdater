package updater

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/f3liz-dev/noraneko-winupdater/pkg/config"
)

func TestNew(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "noraneko-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		ExeDir:   tmpDir,
		WorkDir:  tmpDir,
		Branch:   "nightly",
	}

	opts := Options{
		Scheduled: false,
		Portable:  false,
		CheckOnly: true,
		Version:   "1.0.0",
	}

	u := New(cfg, opts)
	if u == nil {
		t.Fatal("New returned nil")
	}

	if u.cfg != cfg {
		t.Error("Config not set correctly")
	}

	if u.opts.Version != "1.0.0" {
		t.Errorf("Expected version 1.0.0, got %s", u.opts.Version)
	}
}

func TestIsNewerVersion(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "noraneko-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		ExeDir:  tmpDir,
		WorkDir: tmpDir,
	}

	u := New(cfg, Options{})

	tests := []struct {
		current  string
		latest   string
		expected bool
	}{
		{"1.0.0", "1.0.1", true},
		{"1.0.0", "1.0.0", false},
		{"", "1.0.0", true},
		{"0.0.0", "1.0.0", true},
		{"v1.0.0", "v1.0.1", true},
		{"v1.0.0", "1.0.1", true},
		{"1.0.0", "1.1.0", true},     // Minor version bump
		{"1.1.0", "1.0.1", false},    // Current is newer
		{"1.0.0", "2.0.0", true},     // Major version bump
		{"2.0.0", "1.9.9", false},    // Current major is higher
		{"1.0.0-beta", "1.0.0", false}, // Prerelease vs release (stripped, so equal)
		{"1.10.0", "1.9.0", false},   // Double digit version
		{"1.2.3", "1.2.4", true},     // Patch version
		{"1.2.4", "1.2.3", false},    // Current patch is higher
	}

	for _, tt := range tests {
		result := u.isNewerVersion(tt.current, tt.latest)
		if result != tt.expected {
			t.Errorf("isNewerVersion(%s, %s) = %v, expected %v",
				tt.current, tt.latest, result, tt.expected)
		}
	}
}

func TestUnzip(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "noraneko-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		ExeDir:  tmpDir,
		WorkDir: tmpDir,
	}

	u := New(cfg, Options{})

	// Create a test zip file manually is complex, so we'll just test that the function
	// handles invalid files gracefully
	invalidZip := filepath.Join(tmpDir, "invalid.zip")
	if err := os.WriteFile(invalidZip, []byte("not a zip file"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	destDir := filepath.Join(tmpDir, "extract")
	if err := os.MkdirAll(destDir, 0755); err != nil {
		t.Fatalf("Failed to create dest dir: %v", err)
	}

	err = u.unzip(invalidZip, destDir)
	if err == nil {
		t.Error("Expected error for invalid zip file, got nil")
	}
}

func TestCopyFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "noraneko-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		ExeDir:  tmpDir,
		WorkDir: tmpDir,
	}

	u := New(cfg, Options{})

	// Create source file
	srcFile := filepath.Join(tmpDir, "source.txt")
	content := "Hello, World!"
	if err := os.WriteFile(srcFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create source file: %v", err)
	}

	// Copy file
	dstFile := filepath.Join(tmpDir, "dest.txt")
	if err := u.copyFile(srcFile, dstFile); err != nil {
		t.Fatalf("Failed to copy file: %v", err)
	}

	// Verify content
	data, err := os.ReadFile(dstFile)
	if err != nil {
		t.Fatalf("Failed to read dest file: %v", err)
	}

	if string(data) != content {
		t.Errorf("Expected content '%s', got '%s'", content, string(data))
	}
}

func TestFindAsset(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "noraneko-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		ExeDir:  tmpDir,
		WorkDir: tmpDir,
	}

	u := New(cfg, Options{Portable: true})
	u.release = &Release{
		TagName: "v1.0.0",
		Assets: []Asset{
			{Name: "noraneko-1.0.0-linux-x86_64.tar.gz", BrowserDownloadURL: "https://example.com/linux.tar.gz"},
			{Name: "noraneko-1.0.0-windows-x86_64-portable.zip", BrowserDownloadURL: "https://example.com/win.zip"},
			{Name: "noraneko-1.0.0-windows-x86_64-setup.exe", BrowserDownloadURL: "https://example.com/setup.exe"},
		},
	}

	asset, err := u.findAsset()
	if err != nil {
		t.Fatalf("Failed to find asset: %v", err)
	}

	if asset.Name != "noraneko-1.0.0-windows-x86_64-portable.zip" {
		t.Errorf("Expected portable zip, got %s", asset.Name)
	}

	// Test for installed version
	u2 := New(cfg, Options{Portable: false})
	u2.release = u.release

	// Will find setup.exe or fall back to zip
	asset2, err := u2.findAsset()
	if err != nil {
		t.Fatalf("Failed to find asset for installed: %v", err)
	}

	// Should find either setup.exe or portable.zip depending on naming
	if asset2 == nil {
		t.Error("No asset found for installed version")
	}
}

func TestFindChecksumAsset(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "noraneko-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		ExeDir:  tmpDir,
		WorkDir: tmpDir,
	}

	u := New(cfg, Options{})
	u.release = &Release{
		TagName: "v1.0.0",
		Assets: []Asset{
			{Name: "noraneko-1.0.0-windows.zip", BrowserDownloadURL: "https://example.com/win.zip"},
			{Name: "sha256sums.txt", BrowserDownloadURL: "https://example.com/sha256sums.txt"},
		},
	}

	checksumAsset := u.findChecksumAsset()
	if checksumAsset == nil {
		t.Fatal("Checksum asset not found")
	}

	if checksumAsset.Name != "sha256sums.txt" {
		t.Errorf("Expected sha256sums.txt, got %s", checksumAsset.Name)
	}

	// Test with .sha256 extension
	u.release = &Release{
		TagName: "v1.0.0",
		Assets: []Asset{
			{Name: "noraneko-1.0.0-windows.zip", BrowserDownloadURL: "https://example.com/win.zip"},
			{Name: "noraneko-1.0.0-windows.zip.sha256", BrowserDownloadURL: "https://example.com/checksum.sha256"},
		},
	}

	checksumAsset = u.findChecksumAsset()
	if checksumAsset == nil {
		t.Fatal("Checksum asset with .sha256 extension not found")
	}
}
