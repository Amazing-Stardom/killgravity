package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func DetectScope() string {
	if _, err := exec.LookPath("systemctl"); err == nil {
		cmd := exec.Command("systemctl", "--user", "is-active", "killgravity.service")
		if err := cmd.Run(); err == nil {
			return "user"
		}
		home, _ := os.UserHomeDir()
		userUnit := filepath.Join(home, ".config", "systemd", "user", "killgravity.service")
		if _, err := os.Stat(userUnit); err == nil {
			return "user"
		}

		cmd = exec.Command("systemctl", "is-active", "killgravity.service")
		if err := cmd.Run(); err == nil {
			return "system"
		}
		if _, err := os.Stat("/etc/systemd/system/killgravity.service"); err == nil {
			return "system"
		}
	}

	if os.Geteuid() == 0 {
		return "system"
	}
	return "user"
}

func IsServiceActive() bool {
	scope := DetectScope()
	args := []string{}
	if scope == "user" {
		args = append(args, "--user")
	}
	args = append(args, "is-active", "--quiet", "killgravity.service")
	cmd := exec.Command("systemctl", args...)
	return cmd.Run() == nil
}

func IsServiceEnabled() bool {
	scope := DetectScope()
	args := []string{}
	if scope == "user" {
		args = append(args, "--user")
	}
	args = append(args, "is-enabled", "killgravity.service")
	out, err := exec.Command("systemctl", args...).Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "enabled"
}

func ServiceAction(action string) error {
	scope := DetectScope()
	var cmd *exec.Cmd

	args := []string{}
	if scope == "user" {
		args = append(args, "--user")
	}
	args = append(args, action, "killgravity.service")

	if scope == "system" && os.Geteuid() != 0 && action != "status" && action != "is-active" && action != "is-enabled" {
		cmd = exec.Command("sudo", append([]string{"systemctl"}, args...)...)
	} else {
		cmd = exec.Command("systemctl", args...)
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func RestartServiceIfRunning() {
	if IsServiceActive() {
		fmtPrintln(Blue("-> Restarting service to apply changes..."))
		_ = ServiceAction("restart")
	}
}

func ServiceLogs(extraArgs []string) error {
	scope := DetectScope()
	args := []string{}
	if scope == "user" {
		args = append(args, "--user")
	}
	args = append(args, "-u", "killgravity.service")
	args = append(args, extraArgs...)

	cmd := exec.Command("journalctl", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

