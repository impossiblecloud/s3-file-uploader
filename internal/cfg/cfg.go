package cfg

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"time"

	"github.com/adidenko/s3-file-uploader/internal/metrics"
	"github.com/google/logger"
)

// Config is the main app config struct
type AppConfig struct {
	Applog            *logger.Logger
	Workers           int
	WorkersCannelSize int
	Verbose           bool
	SendTimeout       time.Duration
	S3bucket          string
	S3path            string
	PathToWatch       string
	EnvVarGPGPass     string
	GpgPassword       string

	Gzip    bool
	Encrypt bool

	GzipDir    string
	EncryptDir string

	ExitOnFilename string
	CancelFunction context.CancelFunc

	PushGateway  string
	PushInterval time.Duration

	Metrics metrics.AppMetrics
}

// Message that is sent to workers
type Message struct {
	File string
}

// Client stores pointers to configured remote endpoint writes/clients
type SenderClient struct {
	HttpClient *http.Client

	SocketConn   net.Conn
	SocketWriter *bufio.Writer
}

// Workers status
type WorkerStatus struct {
	ID      int  `json:"id"`
	Running bool `json:"running"`
}

// Status defines status
type AppStatus struct {
	Workers []WorkerStatus `json:"workers"`
	Version string         `json:"version"`
}
