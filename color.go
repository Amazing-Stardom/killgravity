package main

import (
	"fmt"
	"os"
	"time"
)

func isTTY() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func colorize(text, code string) string {
	if !isTTY() {
		return text
	}
	return fmt.Sprintf("\033[%sm%s\033[0m", code, text)
}

func Red(s string) string    { return colorize(s, "0;31") }
func Green(s string) string  { return colorize(s, "0;32") }
func Yellow(s string) string { return colorize(s, "0;33") }
func Blue(s string) string   { return colorize(s, "0;34") }
func Cyan(s string) string   { return colorize(s, "0;36") }
func Bold(s string) string   { return colorize(s, "1") }

func fmtPrintln(a ...any) {
	fmt.Println(a...)
}

func logInfo(msg string) {
	ts := time.Now().Format("2006-01-02 15:04:05")
	fmt.Printf("%s %s - %s\n", Blue("[INFO]"), ts, msg)
}

func logWarn(msg string) {
	ts := time.Now().Format("2006-01-02 15:04:05")
	fmt.Fprintf(os.Stderr, "%s %s - %s\n", Yellow("[WARN]"), ts, msg)
}

func logAlert(msg string) {
	ts := time.Now().Format("2006-01-02 15:04:05")
	fmt.Fprintf(os.Stderr, "%s %s - %s\n", Red("[ALERT]"), ts, msg)
}

func logSuccess(msg string) {
	ts := time.Now().Format("2006-01-02 15:04:05")
	fmt.Printf("%s %s - %s\n", Green("[OK]"), ts, msg)
}

