package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type StatusOutput struct {
	Version string           `json:"version"`
	Service ServiceStatus    `json:"service"`
	Config  ConfigStatus     `json:"config"`
	Target  TargetStatus     `json:"target"`
	IO      IOStatus         `json:"io"`
}

type ServiceStatus struct {
	Status  string `json:"status"`
	Active  bool   `json:"active"`
	Enabled bool   `json:"enabled"`
	Scope   string `json:"scope"`
}

type ConfigStatus struct {
	Path             string  `json:"path"`
	TargetPattern    string  `json:"target_pattern"`
	ThresholdSectors int64   `json:"threshold_sectors"`
	ThresholdMB      float64 `json:"threshold_mb"`
	PollInterval     int     `json:"poll_interval"`
	CooldownSeconds  int     `json:"cooldown_seconds"`
	AutoRestart      bool    `json:"auto_restart"`
	IDEBinary        *string `json:"ide_binary"`
}

type TargetStatus struct {
	Pattern string `json:"pattern"`
	Active  bool   `json:"active"`
	Count   int    `json:"count"`
	PIDs    []int  `json:"pids"`
}

type IOStatus struct {
	WriteSectorsPerSec       int64   `json:"write_sectors_per_sec"`
	WriteMBPerSec            float64 `json:"write_mb_per_sec"`
	SystemWriteSectorsPerSec int64   `json:"system_write_sectors_per_sec"`
	SystemWriteMBPerSec      float64 `json:"system_write_mb_per_sec"`
	TargetWriteSectorsPerSec int64   `json:"target_write_sectors_per_sec"`
	TargetWriteMBPerSec      float64 `json:"target_write_mb_per_sec"`
	ThresholdSectorsPerSec   int64   `json:"threshold_sectors_per_sec"`
	ThresholdMBPerSec        float64 `json:"threshold_mb_per_sec"`
	HeadroomPct              float64 `json:"headroom_pct"`
	SurgeDetected            bool    `json:"surge_detected"`
}

func printHelp() {
	fmt.Printf(`usage: killgravity [-h] [-v] [-j] [--config CONFIG_PATH] [--pattern PATTERN]
                   [command] ...

killgravity v%s - Antigravity Dump Guard & SSD TBW Watchdog

positional arguments:
  command               Command to run
  args                  Arguments for command

options:
  -h, --help            show this help message and exit
  -v, --version         show program's version number and exit
  -j, --json            Output machine-readable JSON
  --config CONFIG_PATH  Path to custom config file
  --pattern PATTERN     Override TARGET_PATTERN

Service Management:
  sample                     Fast 250ms I/O sampling (or instant from daemon)
  status                     Display live daemon, service, and NVMe I/O status
  enable                     Enable service to start on boot/login
  disable                    Disable service from auto-starting
  start                      Start the watchdog service
  stop                       Stop the watchdog service
  restart                    Restart the watchdog service
  logs [-f]                  View service journal logs

Configuration:
  threshold [VALUE]          View or set write speed trigger (e.g. 500M, 1G, 1000000)
  time [subcommand] [val]    View or set timings (interval, cooldown, delay)
  target [PATTERN]           View or set target process regex pattern
  auto-restart [on|off]      Enable or disable auto-restarting the IDE
  config                     Display effective configuration

Operations:
  daemon | run               Run watchdog loop in foreground
  kill                       Manually terminate active target processes
  clean                      Purge leftover crashpad (.dmp) and core dumps
  test                       Run diagnostic probe on diskstats and process matching
  mcp                        Run MCP JSON-RPC stdio server for Antigravity
`, Version)
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

func cmdStatus(cfg *Config, asJSON bool) {
	scope := DetectScope()
	isActive := IsServiceActive()
	isEnabled := IsServiceEnabled()
	thresh := cfg.WriteThresholdSectors
	binPath := FindIDEBinary(cfg.RestartBin)
	pids := FindTargetPIDs(cfg.TargetPattern)

	live := GetLiveState(cfg, true)
	rate := live.WriteSectorsPerSec
	rateMB := round2(live.WriteMBPerSec)
	targetRate := live.TargetWriteSectorsPerSec
	targetRateMB := round2(live.TargetWriteMBPerSec)
	threshMB := round2(float64(thresh) / 2048.0)

	var headroom float64
	if thresh > 0 {
		headroom = round2(math.Max(0.0, (1.0-(float64(rate)/float64(thresh)))*100.0))
	}

	var ideBinPtr *string
	if binPath != "" {
		ideBinPtr = &binPath
	}

	if asJSON {
		statusStr := "STOPPED"
		if isActive {
			statusStr = "RUNNING"
		}

		if pids == nil {
			pids = []int{}
		}

		out := StatusOutput{
			Version: Version,
			Service: ServiceStatus{
				Status:  statusStr,
				Active:  isActive,
				Enabled: isEnabled,
				Scope:   scope,
			},
			Config: ConfigStatus{
				Path:             cfg.Path,
				TargetPattern:    cfg.TargetPattern,
				ThresholdSectors: thresh,
				ThresholdMB:      threshMB,
				PollInterval:     cfg.PollInterval,
				CooldownSeconds:  cfg.CooldownSeconds,
				AutoRestart:      cfg.AutoRestart,
				IDEBinary:        ideBinPtr,
			},
			Target: TargetStatus{
				Pattern: cfg.TargetPattern,
				Active:  len(pids) > 0,
				Count:   len(pids),
				PIDs:    pids,
			},
			IO: IOStatus{
				WriteSectorsPerSec:       rate,
				WriteMBPerSec:            rateMB,
				SystemWriteSectorsPerSec: rate,
				SystemWriteMBPerSec:      rateMB,
				TargetWriteSectorsPerSec: targetRate,
				TargetWriteMBPerSec:      targetRateMB,
				ThresholdSectorsPerSec:   thresh,
				ThresholdMBPerSec:        threshMB,
				HeadroomPct:              headroom,
				SurgeDetected:            live.SurgeDetected,
			},
		}

		data, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(data))
		return
	}

	fmt.Println(Bold(fmt.Sprintf("killgravity Status (v%s Go Engine)", Version)))
	fmt.Println(strings.Repeat("=", 50))

	actStr := Yellow("STOPPED")
	if isActive {
		actStr = Green("RUNNING")
	}
	enbStr := Yellow("DISABLED")
	if isEnabled {
		enbStr = Green("ENABLED")
	}
	fmt.Printf("Service Status:    %s (%s service)\n", actStr, scope)
	fmt.Printf("Service Enabled:   %s (starts on boot/login)\n", enbStr)

	cfgStr := Yellow("None (using defaults)")
	if cfg.Path != "" {
		cfgStr = Green(cfg.Path)
	}
	fmt.Printf("Config File:       %s\n", cfgStr)
	fmt.Println(strings.Repeat("-", 50))

	fmt.Printf("Target Pattern:    %s\n", Cyan(cfg.TargetPattern))
	fmt.Printf("Threshold:         %s sectors/s (~%d MB/s)\n", Cyan(strconv.FormatInt(thresh, 10)), thresh/2048)
	fmt.Printf("Poll Interval:     %s\n", Cyan(fmt.Sprintf("%ds", cfg.PollInterval)))
	fmt.Printf("Cooldown:          %s\n", Cyan(fmt.Sprintf("%ds", cfg.CooldownSeconds)))
	fmt.Printf("Auto-Restart:      %s\n", Cyan(fmt.Sprintf("%t", cfg.AutoRestart)))

	binStr := Yellow("Not found (set RESTART_BIN)")
	if binPath != "" {
		binStr = Green(binPath)
	}
	fmt.Printf("IDE Binary:        %s\n", binStr)
	fmt.Println(strings.Repeat("-", 50))

	if len(pids) > 0 {
		fmt.Printf("Target Process:    %s (%d processes detected)\n", Green("ACTIVE"), len(pids))
	} else {
		fmt.Printf("Target Process:    %s (no matching processes)\n", Yellow("INACTIVE"))
	}

	fmt.Printf("System Disk Write: %d sectors/s (~%.2f MB/s)\n", rate, rateMB)
	fmt.Printf("Target IDE Write:   %d sectors/s (~%.2f MB/s)\n", targetRate, targetRateMB)
	fmt.Println(strings.Repeat("=", 50))
}

func cmdSample(cfg *Config, asJSON bool) {
	state := GetLiveState(cfg, true)
	if asJSON {
		if state.TargetPIDs == nil {
			state.TargetPIDs = []int{}
		}
		data, _ := json.MarshalIndent(state, "", "  ")
		fmt.Println(string(data))
	} else {
		statusStr := Yellow("STOPPED")
		if state.ServiceActive {
			statusStr = Green("ACTIVE")
		}
		cachedStr := ""
		if state.CachedFromDaemon {
			cachedStr = " (daemon live feed)"
		}
		fmt.Printf("Service: %s%s | Target PIDs: %d\n", statusStr, cachedStr, state.TargetCount)
		fmt.Printf("System Write: %d sectors/s (~%.2f MB/s)\n", state.WriteSectorsPerSec, state.WriteMBPerSec)
		fmt.Printf("Target IDE Write: %d sectors/s (~%.2f MB/s)\n", state.TargetWriteSectorsPerSec, state.TargetWriteMBPerSec)
	}
}

func cmdThreshold(cfg *Config, value string) {
	if value == "" {
		thresh := cfg.WriteThresholdSectors
		fmt.Printf("Current write threshold: %s sectors/s (~%d MB/s)\n", Cyan(strconv.FormatInt(thresh, 10)), thresh/2048)
		fmt.Println("To change: killgravity threshold <sectors|number[M|MB|G|GB]>")
		return
	}

	re := regexp.MustCompile(`^([0-9]+)\s*([mM][bB]?|[gG][bB]?)$`)
	matches := re.FindStringSubmatch(value)

	var sectors int64
	if len(matches) == 3 {
		num, _ := strconv.ParseInt(matches[1], 10, 64)
		unit := strings.ToUpper(matches[2])
		if strings.Contains(unit, "G") {
			sectors = num * 1024 * 2048
		} else {
			sectors = num * 2048
		}
	} else if num, err := strconv.ParseInt(value, 10, 64); err == nil {
		if num < 10000 {
			fmtPrintln(Blue(fmt.Sprintf("Interpreting %d as %d MB (%d sectors/s)...", num, num, num*2048)))
			sectors = num * 2048
		} else {
			sectors = num
		}
	} else {
		logWarn(fmt.Sprintf("Invalid threshold format: %s. Examples: 500M, 1G, 1000000", value))
		return
	}

	_ = cfg.SetValue("WRITE_THRESHOLD_SECTORS", strconv.FormatInt(sectors, 10))
	fmtPrintln(Green(fmt.Sprintf("✓ Updated WRITE_THRESHOLD_SECTORS=%d in %s", sectors, cfg.Path)))
	RestartServiceIfRunning()
}

func cmdTime(cfg *Config, setting, seconds string) {
	if setting == "" {
		fmt.Println("Current timing settings:")
		fmt.Printf("  Poll interval:    %s seconds\n", Cyan(strconv.Itoa(cfg.PollInterval)))
		fmt.Printf("  Cooldown time:    %s seconds\n", Cyan(strconv.Itoa(cfg.CooldownSeconds)))
		fmt.Printf("  Restart delay:    %s seconds\n", Cyan(strconv.Itoa(cfg.RestartDelay)))
		fmt.Println("\nTo change:")
		fmt.Println("  killgravity time interval <seconds>")
		fmt.Println("  killgravity time cooldown <seconds>")
		fmt.Println("  killgravity time delay <seconds>")
		return
	}

	keyMap := map[string]string{
		"interval": "POLL_INTERVAL",
		"poll":     "POLL_INTERVAL",
		"cooldown": "COOLDOWN_SECONDS",
		"delay":    "RESTART_DELAY",
	}

	if key, ok := keyMap[setting]; ok {
		if seconds != "" {
			_ = cfg.SetValue(key, seconds)
			fmtPrintln(Green(fmt.Sprintf("✓ Updated %s=%s in %s", key, seconds, cfg.Path)))
			RestartServiceIfRunning()
		} else {
			switch key {
			case "POLL_INTERVAL":
				fmt.Printf("%s: %d seconds\n", setting, cfg.PollInterval)
			case "COOLDOWN_SECONDS":
				fmt.Printf("%s: %d seconds\n", setting, cfg.CooldownSeconds)
			case "RESTART_DELAY":
				fmt.Printf("%s: %d seconds\n", setting, cfg.RestartDelay)
			}
		}
	} else if num, err := strconv.Atoi(setting); err == nil {
		_ = cfg.SetValue("POLL_INTERVAL", strconv.Itoa(num))
		fmtPrintln(Green(fmt.Sprintf("✓ Updated POLL_INTERVAL=%d in %s", num, cfg.Path)))
		RestartServiceIfRunning()
	} else {
		logWarn(fmt.Sprintf("Unknown timing parameter: %s", setting))
	}
}

func cmdTarget(cfg *Config, pattern string) {
	if pattern == "" {
		fmt.Printf("Current target pattern: %s\n", Cyan(cfg.TargetPattern))
		fmt.Println("To change: killgravity target <pattern>")
		return
	}
	_ = cfg.SetValue("TARGET_PATTERN", pattern)
	fmtPrintln(Green(fmt.Sprintf("✓ Updated TARGET_PATTERN=%s in %s", pattern, cfg.Path)))
	RestartServiceIfRunning()
}

func cmdAutoRestart(cfg *Config, state string) {
	if state == "" {
		fmt.Printf("Auto-restart is currently: %s\n", Cyan(fmt.Sprintf("%t", cfg.AutoRestart)))
		fmt.Println("To change: killgravity auto-restart on|off")
		return
	}
	s := strings.ToLower(state)
	enabled := s == "on" || s == "true" || s == "1" || s == "enable" || s == "yes"
	_ = cfg.SetValue("AUTO_RESTART", fmt.Sprintf("%t", enabled))
	fmtPrintln(Green(fmt.Sprintf("✓ Updated AUTO_RESTART=%t in %s", enabled, cfg.Path)))
	RestartServiceIfRunning()
}

func cmdKill(cfg *Config) {
	target := cfg.TargetPattern
	logInfo(fmt.Sprintf("Searching for active '%s' processes...", target))

	pids := FindTargetPIDs(target)
	if len(pids) == 0 {
		logInfo(fmt.Sprintf("No matching processes found for pattern '%s'.", target))
		return
	}

	logAlert(fmt.Sprintf("Terminating target PIDs: %v...", pids))
	myPid := os.Getpid()
	ppid := os.Getppid()

	for _, pid := range pids {
		// process filtering excludes self (pid != my_pid) and parent (ppid)
		if pid != myPid && pid != ppid {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	}

	logSuccess("Processes terminated.")

	if cfg.CleanupDumps {
		CleanupDumps()
	}
}

func cmdClean() {
	logInfo("Purging stale crashpad and core dump files...")
	count := CleanupDumps()
	logSuccess(fmt.Sprintf("Cleaned up %d dump files.", count))
}

func cmdTest(cfg *Config) {
	fmtPrintln(Bold("Running diagnostic probe..."))
	s1 := GetWriteSectors()
	fmt.Printf("1. /proc/diskstats read check: initial count = %d\n", s1)
	fmt.Println("2. Measuring write speed over 2 seconds...")
	time.Sleep(2 * time.Second)
	s2 := GetWriteSectors()
	delta := s2 - s1
	if delta < 0 {
		delta = 0
	}
	rate := delta / 2
	fmt.Printf("   Rate: %d sectors/s (~%d MB/s)\n", rate, rate/2048)

	pids := FindTargetPIDs(cfg.TargetPattern)
	if len(pids) > 0 {
		fmt.Printf("3. Target detection ('%s'): %s\n", cfg.TargetPattern, Green(fmt.Sprintf("FOUND PIDs %v", pids)))
	} else {
		fmt.Printf("3. Target detection ('%s'): %s\n", cfg.TargetPattern, Yellow("No matching targets running"))
	}

	binPath := FindIDEBinary(cfg.RestartBin)
	if binPath != "" {
		fmt.Printf("4. IDE Binary discovery: %s\n", Green(binPath))
	} else {
		fmt.Printf("4. IDE Binary discovery: %s\n", Yellow("Not found automatically"))
	}

	fmtPrintln(Green("✓ Diagnostic complete."))
}

func main() {
	var explicitConfig string
	var patternOverride string
	var asJSON bool

	args := os.Args[1:]
	var filteredArgs []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "-v" || arg == "--version" {
			fmt.Printf("killgravity version %s\n", Version)
			return
		}
		if arg == "-h" || arg == "--help" {
			printHelp()
			return
		}
		if arg == "-j" || arg == "--json" {
			asJSON = true
			continue
		}
		if arg == "--config" && i+1 < len(args) {
			explicitConfig = args[i+1]
			i++
			continue
		}
		if strings.HasPrefix(arg, "--config=") {
			explicitConfig = strings.TrimPrefix(arg, "--config=")
			continue
		}
		if arg == "--pattern" && i+1 < len(args) {
			patternOverride = args[i+1]
			i++
			continue
		}
		if strings.HasPrefix(arg, "--pattern=") {
			patternOverride = strings.TrimPrefix(arg, "--pattern=")
			continue
		}
		filteredArgs = append(filteredArgs, arg)
	}

	cfg := LoadConfig(explicitConfig)
	if patternOverride != "" {
		cfg.TargetPattern = patternOverride
	}

	cmd := "status"
	var cmdArgs []string
	if len(filteredArgs) > 0 {
		cmd = strings.ToLower(filteredArgs[0])
		cmdArgs = filteredArgs[1:]
	}

	for _, a := range cmdArgs {
		if a == "--json" || a == "-j" {
			asJSON = true
		}
	}

	switch cmd {
	case "version":
		fmt.Printf("killgravity version %s\n", Version)
	case "help":
		printHelp()
	case "status":
		cmdStatus(cfg, asJSON)
	case "sample":
		cmdSample(cfg, asJSON)
	case "daemon", "run":
		RunDaemon(cfg)
	case "start":
		fmtPrintln(Blue("-> Starting killgravity service..."))
		_ = ServiceAction("start")
		logSuccess("killgravity service started.")
	case "stop":
		fmtPrintln(Blue("-> Stopping killgravity service..."))
		_ = ServiceAction("stop")
		logSuccess("killgravity service stopped.")
	case "restart":
		fmtPrintln(Blue("-> Restarting killgravity service..."))
		_ = ServiceAction("restart")
		logSuccess("killgravity service restarted.")
	case "enable":
		fmtPrintln(Blue("-> Enabling killgravity service..."))
		_ = ServiceAction("enable")
		logSuccess("killgravity service enabled.")
	case "disable":
		fmtPrintln(Blue("-> Disabling killgravity service..."))
		_ = ServiceAction("disable")
		logSuccess("killgravity service disabled.")
	case "logs":
		_ = ServiceLogs(cmdArgs)
	case "threshold":
		val := ""
		if len(cmdArgs) > 0 && cmdArgs[0] != "--json" && cmdArgs[0] != "-j" {
			val = cmdArgs[0]
		}
		cmdThreshold(cfg, val)
	case "time":
		sub := ""
		sec := ""
		if len(cmdArgs) > 0 {
			sub = cmdArgs[0]
		}
		if len(cmdArgs) > 1 {
			sec = cmdArgs[1]
		}
		cmdTime(cfg, sub, sec)
	case "interval", "poll":
		val := ""
		if len(cmdArgs) > 0 {
			val = cmdArgs[0]
		}
		cmdTime(cfg, "interval", val)
	case "cooldown":
		val := ""
		if len(cmdArgs) > 0 {
			val = cmdArgs[0]
		}
		cmdTime(cfg, "cooldown", val)
	case "target":
		pat := ""
		if len(cmdArgs) > 0 {
			pat = cmdArgs[0]
		}
		cmdTarget(cfg, pat)
	case "auto-restart", "autorestart":
		st := ""
		if len(cmdArgs) > 0 {
			st = cmdArgs[0]
		}
		cmdAutoRestart(cfg, st)
	case "kill":
		cmdKill(cfg)
	case "clean":
		cmdClean()
	case "test":
		cmdTest(cfg)
	case "config":
		if len(cmdArgs) == 0 {
			cfgPath := cfg.Path
			if cfgPath == "" {
				cfgPath = "None (defaults)"
			}
			fmtPrintln(Bold(fmt.Sprintf("Configuration file: %s", cfgPath)))
			fmt.Printf("TARGET_PATTERN=%q\n", cfg.TargetPattern)
			fmt.Printf("WRITE_THRESHOLD_SECTORS=%d\n", cfg.WriteThresholdSectors)
			fmt.Printf("POLL_INTERVAL=%d\n", cfg.PollInterval)
			fmt.Printf("COOLDOWN_SECONDS=%d\n", cfg.CooldownSeconds)
			fmt.Printf("RESTART_DELAY=%d\n", cfg.RestartDelay)
			fmt.Printf("AUTO_RESTART=%t\n", cfg.AutoRestart)
			fmt.Printf("RESTART_BIN=%q\n", cfg.RestartBin)
			fmt.Printf("RESTART_FLAGS=%q\n", cfg.RestartFlags)
			fmt.Printf("CLEANUP_DUMPS=%t\n", cfg.CleanupDumps)
		} else if cmdArgs[0] == "path" {
			p := cfg.Path
			if p == "" {
				p = DefaultSavePath()
			}
			fmt.Println(p)
		} else if cmdArgs[0] == "set" && len(cmdArgs) >= 3 {
			_ = cfg.SetValue(cmdArgs[1], cmdArgs[2])
		}
	case "mcp":
		mcpScript := filepath.Join(filepath.Dir(os.Args[0]), "..", "mcp", "server.py")
		if _, err := os.Stat(mcpScript); err != nil {
			home, _ := os.UserHomeDir()
			mcpScript = filepath.Join(home, ".local", "bin", "killgravity-mcp")
		}
		pyBin, err := exec.LookPath("python3")
		if err != nil {
			logWarn("python3 not found to execute MCP server")
			os.Exit(1)
		}
		mcpArgs := append([]string{pyBin, mcpScript}, cmdArgs...)
		cmdExec := exec.Command(mcpArgs[0], mcpArgs[1:]...)
		cmdExec.Stdin = os.Stdin
		cmdExec.Stdout = os.Stdout
		cmdExec.Stderr = os.Stderr
		if err := cmdExec.Run(); err != nil {
			os.Exit(1)
		}
	default:
		logWarn(fmt.Sprintf("Unknown command: %s", cmd))
		printHelp()
		os.Exit(1)
	}
}

