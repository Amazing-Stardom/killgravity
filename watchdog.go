package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type LiveState struct {
	Timestamp                float64 `json:"timestamp"`
	WriteSectorsPerSec       int64   `json:"write_sectors_per_sec"`
	WriteMBPerSec            float64 `json:"write_mb_per_sec"`
	TargetWriteSectorsPerSec int64   `json:"target_write_sectors_per_sec"`
	TargetWriteMBPerSec      float64 `json:"target_write_mb_per_sec"`
	TargetCount              int     `json:"target_count"`
	TargetPIDs               []int   `json:"target_pids"`
	ThresholdSectors         int64   `json:"threshold_sectors"`
	ThresholdMB              float64 `json:"threshold_mb"`
	SurgeDetected            bool    `json:"surge_detected"`
	ServiceActive            bool    `json:"service_active"`
	CachedFromDaemon         bool    `json:"cached_from_daemon"`
}

var ShellNames = map[string]bool{
	"bash":   true,
	"zsh":    true,
	"sh":     true,
	"fish":   true,
	"dash":   true,
	"tmux":   true,
	"screen": true,
}

func GetLiveStateFilePath() string {
	return fmt.Sprintf("/tmp/killgravity-live-%d.json", os.Getuid())
}

func GetWriteSectors() int64 {
	file, err := os.Open("/proc/diskstats")
	if err != nil {
		return 0
	}
	defer file.Close()

	var totalSectors int64
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 10 {
			if sectors, err := strconv.ParseInt(fields[9], 10, 64); err == nil {
				totalSectors += sectors
			}
		}
	}
	return totalSectors
}

func FindTargetPIDs(pattern string) []int {
	myPid := os.Getpid()
	ppid := os.Getppid()
	var pids []int

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return pids
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}

		// Security audit check: process filtering excludes self (pid != my_pid) and parent (ppid)
		if pid == myPid || pid == ppid {
			continue
		}

		// Inspect executable path
		exePath, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
		var exeBase string
		if err == nil {
			exeBase = filepath.Base(exePath)
			if ShellNames[exeBase] {
				continue
			}
		}

		// Inspect command line
		cmdlineBytes, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
		if err != nil || len(cmdlineBytes) == 0 {
			continue
		}

		cmdlineArgs := strings.Split(string(cmdlineBytes), "\x00")
		firstArg := filepath.Base(cmdlineArgs[0])
		if ShellNames[firstArg] {
			continue
		}

		fullCmd := strings.Join(cmdlineArgs, " ")

		// Match logic:
		// 1. Direct match on executable base (e.g. antigravity-ide)
		// 2. Direct match on argv[0] (e.g. mock test: exec -a mock-antigravity-ide-test)
		// 3. Full cmdline match, verifying it is not a foreign CLI tool (grep, cat, python running other script, etc.)
		matches := false
		if strings.Contains(exeBase, pattern) || strings.Contains(firstArg, pattern) {
			matches = true
		} else if strings.Contains(fullCmd, pattern) {
			// If cmdline matched because of an argument, check if exe is a generic tool
			if exeBase == "" || strings.Contains(exeBase, "antigravity") || strings.Contains(exeBase, "electron") || strings.Contains(exeBase, "code") {
				matches = true
			}
		}

		if matches {
			pids = append(pids, pid)
		}
	}

	return pids
}

func GetPIDsWriteBytes(pids []int) int64 {
	var total int64
	for _, pid := range pids {
		file, err := os.Open(fmt.Sprintf("/proc/%d/io", pid))
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "write_bytes:") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					if val, err := strconv.ParseInt(fields[1], 10, 64); err == nil {
						total += val
					}
				}
				break
			}
		}
		file.Close()
	}
	return total
}

func HasActiveCrashDumps(withinSeconds float64) bool {
	home, _ := os.UserHomeDir()
	dumpDirs := []string{
		filepath.Join(home, ".config", "Antigravity", "Crashpad", "pending"),
		filepath.Join(home, ".config", "Antigravity IDE", "Crashpad", "pending"),
		"/tmp",
		"/var/lib/systemd/coredump",
	}

	now := time.Now()
	patterns := []string{"*.dmp", "core.*", "*antigravity*core*"}

	for _, d := range dumpDirs {
		if fi, err := os.Stat(d); err != nil || !fi.IsDir() {
			continue
		}
		for _, pat := range patterns {
			matches, err := filepath.Glob(filepath.Join(d, pat))
			if err != nil {
				continue
			}
			for _, m := range matches {
				info, err := os.Stat(m)
				if err == nil {
					if now.Sub(info.ModTime()).Seconds() < withinSeconds {
						return true
					}
				}
			}
		}
	}
	return false
}

func CleanupDumps() int {
	home, _ := os.UserHomeDir()
	dumpDirs := []string{
		filepath.Join(home, ".config", "Antigravity", "Crashpad", "pending"),
		filepath.Join(home, ".config", "Antigravity IDE", "Crashpad", "pending"),
		"/tmp",
		"/var/lib/systemd/coredump",
	}

	patterns := []string{"*.dmp", "core.*", "*antigravity*core*"}
	cleaned := 0

	for _, d := range dumpDirs {
		if fi, err := os.Stat(d); err != nil || !fi.IsDir() {
			continue
		}
		for _, pat := range patterns {
			matches, err := filepath.Glob(filepath.Join(d, pat))
			if err != nil {
				continue
			}
			for _, m := range matches {
				if err := os.Remove(m); err == nil {
					cleaned++
				}
			}
		}
	}
	return cleaned
}

func FindIDEBinary(explicitBin string) string {
	if explicitBin != "" {
		if _, err := os.Stat(explicitBin); err == nil {
			return explicitBin
		}
	}

	candidates := []string{
		"antigravity-ide",
		"antigravity",
	}

	for _, c := range candidates {
		if p, err := exec.LookPath(c); err == nil {
			return p
		}
	}

	optMatches, _ := filepath.Glob("/opt/antigravity-ide-*/antigravity-ide")
	if len(optMatches) > 0 {
		return optMatches[len(optMatches)-1]
	}

	optCandidates := []string{
		"/opt/Antigravity/antigravity",
		"/opt/antigravity/antigravity",
		"/opt/antigravity-ide/antigravity-ide",
	}
	for _, c := range optCandidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}

	return ""
}

func WriteLiveState(sysSpeed, targetSpeed int64, pids []int, thresh int64, isSurge bool) {
	state := LiveState{
		Timestamp:                float64(time.Now().UnixNano()) / 1e9,
		WriteSectorsPerSec:       sysSpeed,
		WriteMBPerSec:            float64(sysSpeed) / 2048.0,
		TargetWriteSectorsPerSec: targetSpeed,
		TargetWriteMBPerSec:      float64(targetSpeed) / 2048.0,
		TargetCount:              len(pids),
		TargetPIDs:               pids,
		ThresholdSectors:         thresh,
		ThresholdMB:              float64(thresh) / 2048.0,
		SurgeDetected:            isSurge,
		ServiceActive:            true,
		CachedFromDaemon:         true,
	}

	data, err := json.Marshal(state)
	if err != nil {
		return
	}

	liveFile := GetLiveStateFilePath()
	tmpFile := liveFile + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err == nil {
		_ = os.Rename(tmpFile, liveFile)
	}
}

func GetLiveState(cfg *Config, fast bool) *LiveState {
	liveFile := GetLiveStateFilePath()
	if info, err := os.Stat(liveFile); err == nil {
		if time.Since(info.ModTime()) < 4*time.Second {
			if data, err := os.ReadFile(liveFile); err == nil {
				var state LiveState
				if err := json.Unmarshal(data, &state); err == nil {
					state.CachedFromDaemon = true
					state.ServiceActive = IsServiceActive()
					return &state
				}
			}
		}
	}

	sampleDuration := 250 * time.Millisecond
	if !fast {
		sampleDuration = 1000 * time.Millisecond
	}

	pids := FindTargetPIDs(cfg.TargetPattern)
	s1 := GetWriteSectors()
	t1 := GetPIDsWriteBytes(pids)

	time.Sleep(sampleDuration)

	s2 := GetWriteSectors()
	t2 := GetPIDsWriteBytes(pids)

	seconds := sampleDuration.Seconds()
	sysDelta := s2 - s1
	if sysDelta < 0 {
		sysDelta = 0
	}
	sysSpeed := int64(float64(sysDelta) / seconds)

	targetDelta := t2 - t1
	if targetDelta < 0 {
		targetDelta = 0
	}
	targetSpeed := int64((float64(targetDelta) / 512.0) / seconds)

	thresh := cfg.WriteThresholdSectors
	targetMB := float64(targetSpeed) / 2048.0
	isTargetSurge := targetSpeed > (thresh / 4)
	isSystemSurge := sysSpeed > thresh
	hasDumps := HasActiveCrashDumps(60)
	isSurge := isTargetSurge || (isSystemSurge && (targetMB >= 20.0 || hasDumps))

	return &LiveState{
		Timestamp:                float64(time.Now().UnixNano()) / 1e9,
		WriteSectorsPerSec:       sysSpeed,
		WriteMBPerSec:            float64(sysSpeed) / 2048.0,
		TargetWriteSectorsPerSec: targetSpeed,
		TargetWriteMBPerSec:      targetMB,
		TargetCount:              len(pids),
		TargetPIDs:               pids,
		ThresholdSectors:         thresh,
		ThresholdMB:              float64(thresh) / 2048.0,
		SurgeDetected:            isSurge,
		ServiceActive:            IsServiceActive(),
		CachedFromDaemon:         false,
	}
}

func KillAndRecover(cfg *Config, pids []int, speed int64) {
	logAlert(fmt.Sprintf("Disk write surge detected (%d sectors/s, threshold: %d).", speed, cfg.WriteThresholdSectors))
	logAlert(fmt.Sprintf("Terminating runaway '%s' processes (PIDs: %v)...", cfg.TargetPattern, pids))

	myPid := os.Getpid()
	ppid := os.Getppid()

	for _, pid := range pids {
		if pid != myPid && pid != ppid {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	}

	logSuccess("Processes terminated.")

	if cfg.CleanupDumps {
		cnt := CleanupDumps()
		if cnt > 0 {
			logInfo(fmt.Sprintf("Purged %d leftover crashpad dump artifacts.", cnt))
		}
	}

	if cfg.AutoRestart {
		delay := cfg.RestartDelay
		logInfo(fmt.Sprintf("Waiting %ds before restarting IDE...", delay))
		time.Sleep(time.Duration(delay) * time.Second)

		binPath := FindIDEBinary(cfg.RestartBin)
		if binPath != "" {
			flags := strings.Fields(cfg.RestartFlags)
			logInfo(fmt.Sprintf("Relaunching IDE: %s %s", binPath, strings.Join(flags, " ")))
			cmd := exec.Command(binPath, flags...)
			cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
			if err := cmd.Start(); err == nil {
				logSuccess("Relaunched IDE successfully.")
			} else {
				logWarn(fmt.Sprintf("Failed to relaunch IDE: %v", err))
			}
		} else {
			logWarn("Could not auto-detect IDE binary to restart. Set RESTART_BIN in config.")
		}

		cooldown := cfg.CooldownSeconds
		logInfo(fmt.Sprintf("Entering cooldown period of %ds...", cooldown))
		time.Sleep(time.Duration(cooldown) * time.Second)
	}
}

func RunDaemon(cfg *Config) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	liveFile := GetLiveStateFilePath()
	defer func() {
		_ = os.Remove(liveFile)
	}()

	interval := cfg.PollInterval
	if interval < 1 {
		interval = 2
	}
	thresh := cfg.WriteThresholdSectors
	target := cfg.TargetPattern

	logInfo(fmt.Sprintf("Starting killgravity daemon v%s (Go engine)...", Version))
	logInfo(fmt.Sprintf("Target pattern: '%s' | Threshold: %d sectors/s (~%d MB/s)", target, thresh, thresh/2048))
	logInfo(fmt.Sprintf("Poll interval: %ds | Auto-restart: %v", interval, cfg.AutoRestart))
	if cfg.Path != "" {
		logInfo(fmt.Sprintf("Active configuration: %s", cfg.Path))
	}

	pollTicker := time.NewTicker(time.Duration(interval) * time.Second)
	defer pollTicker.Stop()

	for {
		select {
		case <-sigChan:
			logInfo("Received termination signal. Exiting daemon...")
			return
		case <-pollTicker.C:
			pids := FindTargetPIDs(target)
			if len(pids) == 0 {
				WriteLiveState(0, 0, nil, thresh, false)
				continue
			}

			w1Sys := GetWriteSectors()
			w1Target := GetPIDsWriteBytes(pids)

			time.Sleep(1 * time.Second)

			w2Sys := GetWriteSectors()
			w2Target := GetPIDsWriteBytes(pids)

			sysDelta := w2Sys - w1Sys
			if sysDelta < 0 {
				sysDelta = 0
			}
			sysSpeed := sysDelta

			targetDelta := w2Target - w1Target
			if targetDelta < 0 {
				targetDelta = 0
			}
			targetSpeed := targetDelta / 512

			targetMB := (targetSpeed * 512) / (1024 * 1024)
			sysMB := (sysSpeed * 512) / (1024 * 1024)

			isTargetSurge := targetSpeed > (thresh / 4)
			isSystemSurge := sysSpeed > thresh
			hasDumps := HasActiveCrashDumps(60)

			isSurge := isTargetSurge || (isSystemSurge && (targetMB >= 20 || hasDumps))

			WriteLiveState(sysSpeed, targetSpeed, pids, thresh, isSurge)

			if isSurge {
				freshPIDs := FindTargetPIDs(target)
				if len(freshPIDs) > 0 {
					logAlert(fmt.Sprintf("Runaway IDE write surge confirmed: target IDE I/O = %d MB/s, system I/O = %d MB/s (active dumps: %v).", targetMB, sysMB, hasDumps))
					KillAndRecover(cfg, freshPIDs, sysSpeed)
				}
			} else if isSystemSurge {
				// Terminal build activity (cargo build, npm run build, docker, gcc, etc.)
				logInfo(fmt.Sprintf("System write spike detected (%d MB/s), but target IDE write is low (%d MB/s). Ignoring terminal build activity.", sysMB, targetMB))
			}
		}
	}
}

