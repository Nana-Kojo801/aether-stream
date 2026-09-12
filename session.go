package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

func loadSessionStore() {
	sessionStore = &SessionStore{
		Sessions: make(map[string]*Session),
	}
	data, err := os.ReadFile(sessionFile)
	if err != nil {
		return
	}
	if err := json.Unmarshal(data, sessionStore); err != nil {
		fmt.Printf("%s[!] Warning: failed to load sessions: %v%s\n", ColorYellow, err, ColorReset)
	}
}

func saveSessionStore() error {
	data, err := json.MarshalIndent(sessionStore, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(sessionFile, data, 0644)
}

func generateSessionToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func generateSessionID() string {
	return fmt.Sprintf("sess_%d", time.Now().UnixNano())
}

func createSessionWithPersistence(name, ip, port string, persistent bool, description string) error {
	for _, sess := range sessionStore.Sessions {
		if sess.Name == name {
			return fmt.Errorf("session '%s' already exists", name)
		}
	}
	sess := &Session{
		ID:          generateSessionID(),
		Name:        name,
		IP:          ip,
		Port:        port,
		Token:       generateSessionToken(),
		CreatedAt:   time.Now(),
		LastUsed:    time.Now(),
		Description: description,
		Persistent:  persistent,
	}
	sessionStore.Sessions[sess.ID] = sess
	sessionStore.ActiveSessionID = sess.ID
	currentSession = sess
	return saveSessionStore()
}

func getSessionByName(name string) *Session {
	for _, sess := range sessionStore.Sessions {
		if sess.Name == name && !sess.Destroyed {
			return sess
		}
	}
	return nil
}

func findSessionByToken(token string) *Session {
	for _, s := range sessionStore.Sessions {
		if !s.Destroyed && s.Token == token {
			return s
		}
	}
	return nil
}

func deleteSession(name string) error {
	for id, sess := range sessionStore.Sessions {
		if sess.Name == name {
			if currentSession != nil && currentSession.ID == sess.ID && activeConn != nil {
				activeConn.Close()
				activeConn = nil
			}
			delete(sessionStore.Sessions, id)
			if sessionStore.ActiveSessionID == id {
				sessionStore.ActiveSessionID = ""
				currentSession = nil
			}
			return saveSessionStore()
		}
	}
	return fmt.Errorf("session '%s' not found", name)
}

func deleteAllSessions() error {
	if len(sessionStore.Sessions) == 0 {
		return fmt.Errorf("no sessions to delete")
	}

	count := 0
	for _, sess := range sessionStore.Sessions {
		if !sess.Destroyed {
			count++
		}
	}

	fmt.Printf("%s[!] WARNING: This will delete all %d active session(s)!%s\n", ColorYellow, count, ColorReset)
	var confirmation string
	if rl != nil {
		rl.SetPrompt(fmt.Sprintf("%s[?] Type 'yes' to confirm: %s", ColorYellow, ColorReset))
		confirmation, _ = rl.Readline()
		rl.SetPrompt(getPrompt())
	} else {
		fmt.Printf("%s[?] Type 'yes' to confirm: %s", ColorYellow, ColorReset)
		fmt.Scanln(&confirmation)
	}

	if strings.ToLower(strings.TrimSpace(confirmation)) != "yes" {
		return fmt.Errorf("operation cancelled")
	}

	if activeConn != nil {
		activeConn.Close()
		activeConn = nil
	}

	sessionStore.Sessions = make(map[string]*Session)
	sessionStore.ActiveSessionID = ""
	currentSession = nil
	return saveSessionStore()
}

func renameSession(oldName, newName string) error {
	sess := getSessionByName(oldName)
	if sess == nil {
		return fmt.Errorf("session '%s' not found", oldName)
	}
	if getSessionByName(newName) != nil {
		return fmt.Errorf("session '%s' already exists", newName)
	}
	sess.Name = newName
	return saveSessionStore()
}

func getSessionDownloadPath(sess *Session) string {
	if sess.DownloadPath != "" {
		return sess.DownloadPath
	}
	return fmt.Sprintf("downloads/%s", sess.Name)
}

func listSessions() {
	activeCount := 0
	for _, sess := range sessionStore.Sessions {
		if !sess.Destroyed {
			activeCount++
		}
	}

	if activeCount == 0 {
		fmt.Printf("%s[*] No active sessions%s\n", ColorYellow, ColorReset)
		return
	}

	fmt.Printf("\n%s╔══════════════════════════════════════════════════════════════════════════════╗%s\n", ColorCyan, ColorReset)
	fmt.Printf("%s║                              Saved Sessions                                  ║%s\n", ColorCyan, ColorReset)
	fmt.Printf("%s╠══════════════════════════════════════════════════════════════════════════════╣%s\n", ColorCyan, ColorReset)

	for _, sess := range sessionStore.Sessions {
		if sess.Destroyed {
			continue
		}

		activeTag := ""
		if sess.ID == sessionStore.ActiveSessionID {
			activeTag = fmt.Sprintf(" %s[ACTIVE]%s", ColorGreen, ColorReset)
		}

		connTag := ""
		if sess.Persistent {
			if sess.Connected {
				connTag = fmt.Sprintf(" %s[CONNECTED]%s", ColorGreen, ColorReset)
			} else if !sess.LastSeen.IsZero() {
				ago := time.Since(sess.LastSeen)
				connTag = fmt.Sprintf(" %s[LAST SEEN %s AGO]%s", ColorYellow, formatDurationSec(ago.Seconds()), ColorReset)
			} else {
				connTag = fmt.Sprintf(" %s[PERSISTENT]%s", ColorYellow, ColorReset)
			}
		}

		dlPath := "default"
		if sess.DownloadPath != "" {
			dlPath = sess.DownloadPath
		}

		fmt.Printf("\n  %sName:%s      %s%s%s\n", ColorYellow, ColorReset, sess.Name, activeTag, connTag)
		fmt.Printf("  %sEndpoint:%s  %s:%s\n", ColorYellow, ColorReset, sess.IP, sess.Port)
		fmt.Printf("  %sToken:%s     %s...\n", ColorYellow, ColorReset, sess.Token[:8])
		fmt.Printf("  %sDownloads:%s %s\n", ColorYellow, ColorReset, dlPath)
		if !sess.LastSeen.IsZero() {
			fmt.Printf("  %sLast Seen:%s %s\n", ColorYellow, ColorReset, sess.LastSeen.Format("2006-01-02 15:04:05"))
		}
		if sess.Description != "" {
			fmt.Printf("  %sDesc:%s      %s\n", ColorYellow, ColorReset, sess.Description)
		}
		fmt.Println(strings.Repeat("─", 80))
	}

	fmt.Printf("%s╚══════════════════════════════════════════════════════════════════════════════╝%s\n\n", ColorCyan, ColorReset)
}
