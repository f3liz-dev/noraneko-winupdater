// Package updater implements the core update logic for Noraneko Browser
package updater

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/f3liz-dev/noraneko-winupdater/pkg/config"
)

// Options holds command-line options for the updater
type Options struct {
	Scheduled  bool
	Portable   bool
	CheckOnly  bool
	CreateTask bool
	RemoveTask bool
	Version    string
}

// Updater handles browser updates
type Updater struct {
	cfg     *config.Config
	opts    Options
	client  *http.Client
	release *Release
}

// Release represents a GitHub release
type Release struct {
	TagName string  `json:"tag_name"`
	Name    string  `json:"name"`
	Assets  []Asset `json:"assets"`
}

// Asset represents a release asset
type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

// New creates a new Updater instance
func New(cfg *config.Config, opts Options) *Updater {
	return &Updater{
		cfg:  cfg,
		opts: opts,
		client: &http.Client{
			Timeout: 5 * time.Minute,
		},
	}
}

// Run executes the update check and installation
func (u *Updater) Run() error {
	fmt.Printf("Noraneko WinUpdater v%s\n", u.opts.Version)
	fmt.Println("Checking for updates...")

	// Check connection
	if err := u.checkConnection(); err != nil {
		return fmt.Errorf("connection check failed: %w", err)
	}

	// Get current version
	currentVersion, err := u.getCurrentVersion()
	if err != nil {
		// If we can't get the current version, this might be a fresh install
		fmt.Printf("Could not determine current version: %v\n", err)
		currentVersion = "0.0.0"
	}
	fmt.Printf("Current version: %s\n", currentVersion)

	// Get latest release
	release, err := u.getLatestRelease()
	if err != nil {
		return fmt.Errorf("failed to get latest release: %w", err)
	}
	u.release = release

	newVersion := strings.TrimPrefix(release.TagName, "v")
	fmt.Printf("Latest version: %s\n", newVersion)

	// Compare versions
	if !u.isNewerVersion(currentVersion, newVersion) {
		fmt.Println("No new version available.")
		u.logResult("No new version found")
		return nil
	}

	fmt.Printf("New version available: %s -> %s\n", currentVersion, newVersion)

	if u.opts.CheckOnly {
		fmt.Println("Check-only mode, not installing.")
		return nil
	}

	// Download and install
	if err := u.downloadAndInstall(); err != nil {
		return fmt.Errorf("update failed: %w", err)
	}

	fmt.Println("Update completed successfully!")
	u.logResult(fmt.Sprintf("Updated from %s to %s", currentVersion, newVersion))
	return nil
}

// checkConnection verifies we can reach the API
func (u *Updater) checkConnection() error {
	resp, err := u.client.Get(config.ConnectCheckURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("API returned status %d", resp.StatusCode)
	}
	return nil
}

// getCurrentVersion gets the current installed version
func (u *Updater) getCurrentVersion() (string, error) {
	browserPath := u.cfg.GetBrowserPath()
	if browserPath == "" {
		return "", fmt.Errorf("browser not found")
	}

	// For Windows, we would read the file version info
	// For now, we'll try to find an application.ini or version file
	browserDir := filepath.Dir(browserPath)
	
	// Try application.ini
	appIniPath := filepath.Join(browserDir, "application.ini")
	if data, err := os.ReadFile(appIniPath); err == nil {
		re := regexp.MustCompile(`(?m)^Version=(.+)$`)
		matches := re.FindStringSubmatch(string(data))
		if len(matches) > 1 {
			return strings.TrimSpace(matches[1]), nil
		}
	}

	// Try version file
	versionPath := filepath.Join(browserDir, "version")
	if data, err := os.ReadFile(versionPath); err == nil {
		return strings.TrimSpace(string(data)), nil
	}

	return "", fmt.Errorf("could not determine version")
}

// getLatestRelease fetches the latest release from GitHub
func (u *Updater) getLatestRelease() (*Release, error) {
	url := config.ReleaseAPIURL + "/latest"
	
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "Noraneko-WinUpdater/"+u.opts.Version)

	resp, err := u.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("failed to decode release info: %w", err)
	}

	return &release, nil
}

// isNewerVersion compares two version strings
func (u *Updater) isNewerVersion(current, latest string) bool {
	// Simple comparison - could be improved with semantic versioning
	current = strings.TrimPrefix(current, "v")
	latest = strings.TrimPrefix(latest, "v")
	
	if current == "" || current == "0.0.0" {
		return true
	}
	
	return latest != current
}

// downloadAndInstall downloads and installs the update
func (u *Updater) downloadAndInstall() error {
	// Find the appropriate asset
	asset, err := u.findAsset()
	if err != nil {
		return fmt.Errorf("failed to find download: %w", err)
	}

	fmt.Printf("Downloading %s...\n", asset.Name)

	// Download to temp directory
	downloadPath := filepath.Join(u.cfg.WorkDir, asset.Name)
	if err := u.downloadFile(asset.BrowserDownloadURL, downloadPath); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer os.Remove(downloadPath)

	// Verify checksum if available
	if checksumAsset := u.findChecksumAsset(); checksumAsset != nil {
		fmt.Println("Verifying checksum...")
		if err := u.verifyChecksum(downloadPath, checksumAsset, asset.Name); err != nil {
			return fmt.Errorf("checksum verification failed: %w", err)
		}
		fmt.Println("Checksum verified.")
	}

	// Install or extract
	isPortable := u.cfg.IsPortable() || u.opts.Portable
	if isPortable || strings.HasSuffix(asset.Name, ".zip") {
		fmt.Println("Extracting...")
		return u.extractPortable(downloadPath)
	}

	fmt.Println("Installing...")
	return u.runInstaller(downloadPath)
}

// findAsset finds the appropriate download asset for this platform
func (u *Updater) findAsset() (*Asset, error) {
	// Determine what we're looking for
	isPortable := u.cfg.IsPortable() || u.opts.Portable
	arch := "x86_64"
	if runtime.GOARCH == "386" {
		arch = "i686"
	}

	var suffix string
	if isPortable {
		suffix = fmt.Sprintf("windows-%s-portable.zip", arch)
	} else {
		suffix = fmt.Sprintf("windows-%s-setup.exe", arch)
	}

	// Also try alternative naming patterns
	suffixes := []string{
		suffix,
		fmt.Sprintf("win64.zip"),
		fmt.Sprintf("win64-setup.exe"),
		fmt.Sprintf("windows.zip"),
		fmt.Sprintf("windows-setup.exe"),
	}

	for _, asset := range u.release.Assets {
		name := strings.ToLower(asset.Name)
		for _, s := range suffixes {
			if strings.Contains(name, strings.ToLower(s)) || strings.HasSuffix(name, strings.ToLower(s)) {
				return &asset, nil
			}
		}
	}

	// If no specific match, look for any Windows executable or zip
	for _, asset := range u.release.Assets {
		name := strings.ToLower(asset.Name)
		if (strings.Contains(name, "windows") || strings.Contains(name, "win")) &&
			(strings.HasSuffix(name, ".exe") || strings.HasSuffix(name, ".zip")) {
			return &asset, nil
		}
	}

	return nil, fmt.Errorf("no suitable download found for this platform")
}

// findChecksumAsset finds the checksum file asset
func (u *Updater) findChecksumAsset() *Asset {
	for _, asset := range u.release.Assets {
		name := strings.ToLower(asset.Name)
		if strings.Contains(name, "sha256") || strings.HasSuffix(name, ".sha256") {
			return &asset
		}
	}
	return nil
}

// downloadFile downloads a file from URL to local path
func (u *Updater) downloadFile(url, filepath string) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Noraneko-WinUpdater/"+u.opts.Version)

	resp, err := u.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

// verifyChecksum verifies the file checksum
func (u *Updater) verifyChecksum(filePath string, checksumAsset *Asset, fileName string) error {
	// Download checksum file
	checksumPath := filepath.Join(u.cfg.WorkDir, checksumAsset.Name)
	if err := u.downloadFile(checksumAsset.BrowserDownloadURL, checksumPath); err != nil {
		return fmt.Errorf("failed to download checksum file: %w", err)
	}
	defer os.Remove(checksumPath)

	// Read checksum file
	data, err := os.ReadFile(checksumPath)
	if err != nil {
		return fmt.Errorf("failed to read checksum file: %w", err)
	}

	// Find the checksum for our file
	expectedHash := ""
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			hash := parts[0]
			name := strings.TrimPrefix(parts[1], "*")
			if strings.EqualFold(name, fileName) || strings.HasSuffix(name, fileName) {
				expectedHash = strings.ToLower(hash)
				break
			}
		}
	}

	if expectedHash == "" {
		return fmt.Errorf("checksum for %s not found in checksum file", fileName)
	}

	// Calculate actual hash
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return err
	}

	actualHash := hex.EncodeToString(hasher.Sum(nil))
	if actualHash != expectedHash {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expectedHash, actualHash)
	}

	return nil
}

// extractPortable extracts a portable zip archive
func (u *Updater) extractPortable(zipPath string) error {
	browserDir := filepath.Dir(u.cfg.GetBrowserPath())
	if browserDir == "" {
		browserDir = filepath.Join(u.cfg.ExeDir, config.BrowserName)
	}

	// Create extract directory
	extractDir := filepath.Join(u.cfg.WorkDir, config.BrowserName+"-Extracted")
	if err := os.RemoveAll(extractDir); err != nil {
		return fmt.Errorf("failed to clean extract directory: %w", err)
	}
	if err := os.MkdirAll(extractDir, 0755); err != nil {
		return fmt.Errorf("failed to create extract directory: %w", err)
	}
	defer os.RemoveAll(extractDir)

	// Extract zip
	if err := u.unzip(zipPath, extractDir); err != nil {
		return fmt.Errorf("extraction failed: %w", err)
	}

	// Find the browser folder in the extracted content
	entries, err := os.ReadDir(extractDir)
	if err != nil {
		return err
	}

	sourceDir := extractDir
	for _, entry := range entries {
		if entry.IsDir() {
			// Use the first directory as the source
			sourceDir = filepath.Join(extractDir, entry.Name())
			break
		}
	}

	// Copy files to browser directory
	if err := u.copyDir(sourceDir, browserDir); err != nil {
		return fmt.Errorf("failed to copy files: %w", err)
	}

	return nil
}

// unzip extracts a zip archive
func (u *Updater) unzip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		// Prevent ZipSlip vulnerability
		fpath := filepath.Join(dest, f.Name)
		if !strings.HasPrefix(fpath, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("illegal file path: %s", fpath)
		}

		if f.FileInfo().IsDir() {
			os.MkdirAll(fpath, os.ModePerm)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
			return err
		}

		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			return err
		}

		_, err = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()

		if err != nil {
			return err
		}
	}

	return nil
}

// copyDir recursively copies a directory
func (u *Updater) copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		return u.copyFile(path, dstPath)
	})
}

// copyFile copies a single file
func (u *Updater) copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}

// runInstaller runs the setup executable
func (u *Updater) runInstaller(setupPath string) error {
	browserDir := filepath.Dir(u.cfg.GetBrowserPath())
	if browserDir == "" {
		browserDir = filepath.Join(os.Getenv("ProgramFiles"), config.BrowserName)
	}

	// Run silent installation
	cmd := exec.Command(setupPath, "/S", "/D="+browserDir)
	if err := cmd.Run(); err != nil {
		// Try interactive installation
		fmt.Println("Silent installation failed, running interactive installer...")
		cmd = exec.Command(setupPath, "/D="+browserDir)
		return cmd.Run()
	}

	return nil
}

// HandleScheduledTask creates or removes a scheduled task
func (u *Updater) HandleScheduledTask() error {
	var scriptName string
	if u.opts.CreateTask {
		scriptName = "ScheduledTask-Create.ps1"
	} else if u.opts.RemoveTask {
		scriptName = "ScheduledTask-Remove.ps1"
	} else {
		return nil
	}

	scriptPath := filepath.Join(u.cfg.ExeDir, scriptName)
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		return fmt.Errorf("scheduled task script not found: %s", scriptPath)
	}

	cmd := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "RemoteSigned", "-File", scriptPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// logResult logs the update result to the config file
func (u *Updater) logResult(result string) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	u.cfg.LogEntry("LastRun", timestamp)
	u.cfg.LogEntry("LastResult", result)
}
