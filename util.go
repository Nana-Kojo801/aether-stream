package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func formatDuration(d interface{ Seconds() float64 }) string {
	// Accept time.Duration via interface
	return formatDurationSec(d.Seconds())
}

func formatDurationSec(sec float64) string {
	if sec < 60 {
		return fmt.Sprintf("%ds", int(sec))
	} else if sec < 3600 {
		return fmt.Sprintf("%dm", int(sec/60))
	}
	return fmt.Sprintf("%dh", int(sec/3600))
}

func getLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1"
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	return "127.0.0.1"
}

func clearScreen() {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", "cls")
	} else {
		cmd = exec.Command("clear")
	}
	cmd.Stdout = os.Stdout
	cmd.Run()
}

func getLocalFiles(prefix string) []string {
	var dir, base string
	switch {
	case prefix == "":
		dir, base = ".", ""
	case strings.HasSuffix(prefix, "/") || strings.HasSuffix(prefix, string(filepath.Separator)):
		dir, base = prefix, ""
	default:
		dir = filepath.Dir(prefix)
		base = filepath.Base(prefix)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var results []string
	for _, e := range entries {
		name := e.Name()
		if base != "" && !strings.HasPrefix(name, base) {
			continue
		}
		full := name
		if dir != "." {
			full = filepath.ToSlash(filepath.Join(dir, name))
		}
		if e.IsDir() {
			full += "/"
		}
		results = append(results, full)
	}
	return results
}

func getPayloadFiles() []string {
	entries, err := os.ReadDir(payloadsDir)
	if err != nil {
		return nil
	}
	var results []string
	for _, e := range entries {
		if !e.IsDir() {
			results = append(results, e.Name())
		}
	}
	return results
}

func nextFreePort(start int) (string, error) {
	used := make(map[string]bool)
	for _, s := range sessionStore.Sessions {
		if !s.Destroyed {
			used[s.Port] = true
		}
	}
	for p := start; p <= 65535; p++ {
		portStr := fmt.Sprintf("%d", p)
		if used[portStr] {
			continue
		}
		ln, err := net.Listen("tcp", ":"+portStr)
		if err == nil {
			ln.Close()
			return portStr, nil
		}
	}
	return "", fmt.Errorf("no free port found from %d", start)
}

func isPortArg(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
