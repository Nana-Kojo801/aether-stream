package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	printBanner()
	loadSessionStore()
	setupSignalHandler()

	if len(sessionStore.Sessions) > 0 {
		activeCount := 0
		for _, sess := range sessionStore.Sessions {
			if !sess.Destroyed {
				activeCount++
			}
		}
		fmt.Printf("%s[*] Found %d saved session(s) (%d active)%s\n", ColorBlue, len(sessionStore.Sessions), activeCount, ColorReset)
		if sessionStore.ActiveSessionID != "" {
			if sess, ok := sessionStore.Sessions[sessionStore.ActiveSessionID]; ok && !sess.Destroyed {
				fmt.Printf("%s[*] Active session: %s (%s:%s)%s\n", ColorGreen, sess.Name, sess.IP, sess.Port, ColorReset)
				currentSession = sess
			}
		}
	}

	printHelp()
	operatorShell()
}

func setupSignalHandler() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		fmt.Printf("\n%s[*] Interrupted - saving state...%s\n", ColorYellow, ColorReset)
		if activeConn != nil {
			activeConn.Close()
		}
		stopTemplateServer()
		stopFileServer()
		close(stopDownloaders)
		saveSessionStore()
		os.Exit(0)
	}()
}

func printBanner() {
	aether := `
  █████╗ ███████╗████████╗██╗  ██╗███████╗██████╗
 ██╔══██╗██╔════╝╚══██╔══╝██║  ██║██╔════╝██╔══██╗
 ███████║█████╗     ██║   ███████║█████╗  ██████╔╝
 ██╔══██║██╔══╝     ██║   ██╔══██║██╔══╝  ██╔══██╗
 ██║  ██║███████╗   ██║   ██║  ██║███████╗██║  ██║
 ╚═╝  ╚═╝╚══════╝   ╚═╝   ╚═╝  ╚═╝╚══════╝╚═╝  ╚═╝`

	stream := `
 ███████╗████████╗██████╗ ███████╗ █████╗ ███╗   ███╗
 ██╔════╝╚══██╔══╝██╔══██╗██╔════╝██╔══██╗████╗ ████║
 ███████╗   ██║   ██████╔╝█████╗  ███████║██╔████╔██║
 ╚════██║   ██║   ██╔══██╗██╔══╝  ██╔══██║██║╚██╔╝██║
 ███████║   ██║   ██║  ██║███████╗██║  ██║██║ ╚═╝ ██║
 ╚══════╝   ╚═╝   ╚═╝  ╚═╝╚══════╝╚═╝  ╚═╝╚═╝     ╚═╝
`

	fmt.Printf("%s%s%s", ColorCyan, aether, ColorReset)
	fmt.Printf("%s%s%s\n", ColorPurple, stream, ColorReset)
	fmt.Printf("%s  ══════════════════════════════════════════════════════════%s\n", ColorBlue, ColorReset)
	fmt.Printf("  %s◆%s  Platform Framework: AetherStream %sv3.0.0%s  %s◆%s  C2 Infrastructure Layer\n",
		ColorCyan, ColorReset, ColorGreen, ColorReset, ColorCyan, ColorReset)
	fmt.Printf("%s  ══════════════════════════════════════════════════════════%s\n\n", ColorBlue, ColorReset)
}
