package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	Path                  string `json:"path"`
	TargetPattern         string `json:"target_pattern"`
	WriteThresholdSectors int64  `json:"threshold_sectors"`
	PollInterval          int    `json:"poll_interval"`
	CooldownSeconds       int    `json:"cooldown_seconds"`
	RestartDelay          int    `json:"restart_delay"`
	AutoRestart           bool   `json:"auto_restart"`
	RestartBin            string `json:"ide_binary"`
	RestartFlags          string `json:"restart_flags"`
	CleanupDumps          bool   `json:"cleanup_dumps"`
}

func NewDefaultConfig() *Config {
	return &Config{
		TargetPattern:         "antigravity-ide",
		WriteThresholdSectors: 1000000,
		PollInterval:          2,
		CooldownSeconds:       60,
		RestartDelay:          5,
		AutoRestart:           true,
		RestartBin:            "",
		RestartFlags:          "--disable-crash-reporter",
		CleanupDumps:          true,
	}
}

func FindConfigFile(explicitPath string) string {
	if explicitPath != "" {
		if _, err := os.Stat(explicitPath); err == nil {
			return explicitPath
		}
	}

	if envPath := os.Getenv("KILLGRAVITY_CONFIG"); envPath != "" {
		if _, err := os.Stat(envPath); err == nil {
			return envPath
		}
	}

	home, _ := os.UserHomeDir()
	candidates := []string{
		filepath.Join(home, ".config", "killgravity", "killgravity.conf"),
		"/etc/killgravity/killgravity.conf",
	}

	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && !info.IsDir() {
			return c
		}
	}
	return ""
}

func DefaultSavePath() string {
	if os.Geteuid() == 0 {
		return "/etc/killgravity/killgravity.conf"
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "killgravity", "killgravity.conf")
}

func LoadConfig(explicitPath string) *Config {
	cfg := NewDefaultConfig()
	cfgPath := FindConfigFile(explicitPath)
	if cfgPath == "" {
		return cfg
	}

	file, err := os.Open(cfgPath)
	if err != nil {
		return cfg
	}
	defer file.Close()

	cfg.Path = cfgPath
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		k := strings.TrimSpace(parts[0])
		v := strings.Trim(strings.TrimSpace(parts[1]), `"'`)

		switch k {
		case "TARGET_PATTERN":
			cfg.TargetPattern = v
		case "WRITE_THRESHOLD_SECTORS":
			if val, err := strconv.ParseInt(v, 10, 64); err == nil {
				cfg.WriteThresholdSectors = val
			}
		case "POLL_INTERVAL":
			if val, err := strconv.Atoi(v); err == nil {
				cfg.PollInterval = val
			}
		case "COOLDOWN_SECONDS":
			if val, err := strconv.Atoi(v); err == nil {
				cfg.CooldownSeconds = val
			}
		case "RESTART_DELAY":
			if val, err := strconv.Atoi(v); err == nil {
				cfg.RestartDelay = val
			}
		case "AUTO_RESTART":
			cfg.AutoRestart = strings.ToLower(v) == "true" || v == "1" || strings.ToLower(v) == "yes" || strings.ToLower(v) == "on"
		case "RESTART_BIN":
			cfg.RestartBin = v
		case "RESTART_FLAGS":
			cfg.RestartFlags = v
		case "CLEANUP_DUMPS":
			cfg.CleanupDumps = strings.ToLower(v) == "true" || v == "1" || strings.ToLower(v) == "yes" || strings.ToLower(v) == "on"
		}
	}

	return cfg
}

func (c *Config) SetValue(key, value string) error {
	savePath := c.Path
	if savePath == "" {
		savePath = DefaultSavePath()
	}

	if err := os.MkdirAll(filepath.Dir(savePath), 0755); err != nil {
		return err
	}

	var lines []string
	keyFound := false

	if data, err := os.ReadFile(savePath); err == nil {
		scanner := bufio.NewScanner(strings.NewReader(string(data)))
		for scanner.Scan() {
			line := scanner.Text()
			trimmed := strings.TrimSpace(line)
			if !strings.HasPrefix(trimmed, "#") && strings.Contains(line, "=") {
				parts := strings.SplitN(trimmed, "=", 2)
				if strings.TrimSpace(parts[0]) == key {
					lines = append(lines, fmt.Sprintf("%s=%q", key, value))
					keyFound = true
					continue
				}
			}
			lines = append(lines, line)
		}
	}

	if !keyFound {
		lines = append(lines, fmt.Sprintf("%s=%q", key, value))
	}

	outContent := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(savePath, []byte(outContent), 0644); err != nil {
		return err
	}

	c.Path = savePath
	// Update in-memory struct
	switch key {
	case "TARGET_PATTERN":
		c.TargetPattern = value
	case "WRITE_THRESHOLD_SECTORS":
		if val, err := strconv.ParseInt(value, 10, 64); err == nil {
			c.WriteThresholdSectors = val
		}
	case "POLL_INTERVAL":
		if val, err := strconv.Atoi(value); err == nil {
			c.PollInterval = val
		}
	case "COOLDOWN_SECONDS":
		if val, err := strconv.Atoi(value); err == nil {
			c.CooldownSeconds = val
		}
	case "RESTART_DELAY":
		if val, err := strconv.Atoi(value); err == nil {
			c.RestartDelay = val
		}
	case "AUTO_RESTART":
		c.AutoRestart = strings.ToLower(value) == "true" || value == "1" || strings.ToLower(value) == "yes" || strings.ToLower(value) == "on"
	case "RESTART_BIN":
		c.RestartBin = value
	case "RESTART_FLAGS":
		c.RestartFlags = value
	case "CLEANUP_DUMPS":
		c.CleanupDumps = strings.ToLower(value) == "true" || value == "1" || strings.ToLower(value) == "yes" || strings.ToLower(value) == "on"
	}

	return nil
}

