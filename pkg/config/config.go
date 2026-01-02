// Package config handles configuration for Noraneko WinUpdater
package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	BrowserName     = "Noraneko"
	BrowserExe      = "noraneko.exe"
	DefaultBranch   = "nightly"
	ConfigFileName  = "Noraneko-WinUpdater.ini"
	ReleaseAPIURL   = "https://api.github.com/repos/f3liz-dev/noraneko-runtime/releases"
	ConnectCheckURL = "https://api.github.com"
)

// Config holds the updater configuration
type Config struct {
	// Path to the browser executable
	Path string

	// Working directory for downloads/extraction
	WorkDir string

	// Whether to update the updater itself
	UpdateSelf bool

	// Whether to ignore certificate revocation errors
	IgnoreCrlErrors bool

	// Release branch to track (nightly, beta, stable)
	Branch string

	// Executable directory
	ExeDir string

	// Config file path
	ConfigFile string
}

// Load reads the configuration from the INI file or creates defaults
func Load(exeDir string) (*Config, error) {
	cfg := &Config{
		Path:            "",
		WorkDir:         os.TempDir(),
		UpdateSelf:      true,
		IgnoreCrlErrors: false,
		Branch:          DefaultBranch,
		ExeDir:          exeDir,
		ConfigFile:      filepath.Join(exeDir, ConfigFileName),
	}

	// Check if config file exists
	if _, err := os.Stat(cfg.ConfigFile); os.IsNotExist(err) {
		// Create default config file
		if err := cfg.Save(); err != nil {
			return nil, fmt.Errorf("failed to create config file: %w", err)
		}
		return cfg, nil
	}

	// Read existing config
	file, err := os.Open(cfg.ConfigFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file: %w", err)
	}
	defer file.Close()

	section := ""
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}

		// Check for section header
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToLower(strings.TrimSuffix(strings.TrimPrefix(line, "["), "]"))
			continue
		}

		// Parse key=value pairs
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(strings.ToLower(parts[0]))
		value := strings.TrimSpace(parts[1])

		if section == "settings" {
			switch key {
			case "path":
				if value != "0" && value != "" {
					cfg.Path = value
				}
			case "workdir":
				if value != "" {
					if value == "." {
						cfg.WorkDir = exeDir
					} else {
						cfg.WorkDir = value
					}
				}
			case "updateself":
				cfg.UpdateSelf = value == "1" || strings.ToLower(value) == "true"
			case "ignorecrlerrors":
				cfg.IgnoreCrlErrors = value == "1" || strings.ToLower(value) == "true"
			case "branch":
				if value != "" {
					cfg.Branch = value
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	return cfg, nil
}

// Save writes the configuration to the INI file
func (c *Config) Save() error {
	var content strings.Builder

	content.WriteString("[Settings]\n")
	if c.Path != "" {
		content.WriteString(fmt.Sprintf("Path=%s\n", c.Path))
	} else {
		content.WriteString("Path=0\n")
	}

	workDir := c.WorkDir
	if workDir == c.ExeDir {
		workDir = "."
	} else if workDir == os.TempDir() {
		workDir = ""
	}
	content.WriteString(fmt.Sprintf("WorkDir=%s\n", workDir))

	if c.UpdateSelf {
		content.WriteString("UpdateSelf=1\n")
	} else {
		content.WriteString("UpdateSelf=0\n")
	}

	if c.IgnoreCrlErrors {
		content.WriteString("IgnoreCrlErrors=1\n")
	} else {
		content.WriteString("IgnoreCrlErrors=0\n")
	}

	content.WriteString(fmt.Sprintf("Branch=%s\n", c.Branch))

	return os.WriteFile(c.ConfigFile, []byte(content.String()), 0644)
}

// LogEntry writes a log entry to the INI file
func (c *Config) LogEntry(key, value string) error {
	// Read existing content
	existingContent := ""
	if data, err := os.ReadFile(c.ConfigFile); err == nil {
		existingContent = string(data)
	}

	// Check if [Log] section exists
	if !strings.Contains(existingContent, "[Log]") {
		existingContent += "\n[Log]\n"
	}

	// Find and update or append the key
	lines := strings.Split(existingContent, "\n")
	found := false
	inLogSection := false
	for i, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if strings.HasPrefix(trimmedLine, "[Log]") {
			inLogSection = true
			continue
		}
		if strings.HasPrefix(trimmedLine, "[") && strings.HasSuffix(trimmedLine, "]") {
			inLogSection = false
			continue
		}
		if inLogSection && strings.HasPrefix(strings.ToLower(trimmedLine), strings.ToLower(key)+"=") {
			lines[i] = fmt.Sprintf("%s=%s", key, value)
			found = true
			break
		}
	}

	if !found {
		// Append to Log section
		newLines := []string{}
		addedToLog := false
		inLogSection = false
		for _, line := range lines {
			trimmedLine := strings.TrimSpace(line)
			if strings.HasPrefix(trimmedLine, "[Log]") {
				inLogSection = true
				newLines = append(newLines, line)
				continue
			}
			if inLogSection && !addedToLog && (trimmedLine == "" || (strings.HasPrefix(trimmedLine, "[") && strings.HasSuffix(trimmedLine, "]"))) {
				newLines = append(newLines, fmt.Sprintf("%s=%s", key, value))
				addedToLog = true
			}
			if strings.HasPrefix(trimmedLine, "[") && strings.HasSuffix(trimmedLine, "]") && trimmedLine != "[Log]" {
				inLogSection = false
			}
			newLines = append(newLines, line)
		}
		if !addedToLog {
			newLines = append(newLines, fmt.Sprintf("%s=%s", key, value))
		}
		lines = newLines
	}

	return os.WriteFile(c.ConfigFile, []byte(strings.Join(lines, "\n")), 0644)
}

// GetBrowserPath returns the path to the browser executable
// It will try to auto-detect if not configured
func (c *Config) GetBrowserPath() string {
	if c.Path != "" {
		return c.Path
	}

	// Try to find in common locations
	programFiles := os.Getenv("ProgramFiles")
	if programFiles == "" {
		programFiles = "C:\\Program Files"
	}

	possiblePaths := []string{
		filepath.Join(c.ExeDir, BrowserName, BrowserExe),
		filepath.Join(programFiles, BrowserName, BrowserExe),
	}

	// Check for portable version in exe directory
	portablePath := filepath.Join(c.ExeDir, BrowserName+"-Portable.exe")
	if _, err := os.Stat(portablePath); err == nil {
		possiblePaths = append([]string{filepath.Join(c.ExeDir, BrowserName, BrowserExe)}, possiblePaths...)
	}

	for _, p := range possiblePaths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	return ""
}

// IsPortable returns true if running in portable mode
func (c *Config) IsPortable() bool {
	portablePath := filepath.Join(c.ExeDir, BrowserName+"-Portable.exe")
	_, err := os.Stat(portablePath)
	return err == nil
}
