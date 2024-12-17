// Simple tool to watch directory for new files and upload them to S3

package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/adidenko/s3-file-uploader/internal/cfg"
	"github.com/adidenko/s3-file-uploader/internal/fs"
	"github.com/adidenko/s3-file-uploader/internal/metrics"
	"github.com/adidenko/s3-file-uploader/internal/s3"
	"github.com/adidenko/s3-file-uploader/internal/utils"

	"github.com/google/logger"
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/client_golang/prometheus/push"
)

// Constants and vars
const version = "0.0.1"
const workersCannelSize = 1024
const errorBadHTTPCode = "Bad HTTP status code"

var applog *logger.Logger
var workerStatuses []cfg.WorkerStatus

// Let's use the same buckets for histograms as NGINX Ingress controller
var secondsDurationBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

// Status for future web endpoint
func handleStatus(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)

	myStatus := cfg.AppStatus{
		Workers: workerStatuses,
		Version: version,
	}

	// Set headers
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Make json output
	jsonOut, err := json.Marshal(myStatus)
	applog.Infof("Sending status: %v", myStatus)
	if err != nil {
		applog.Errorf("Failed to json.Marshal() status: %v", err)
		http.Error(w, "Failed to json.Marshal() status", http.StatusInternalServerError)
		return
	}

	fmt.Fprint(w, string(jsonOut))
}

// Health-check handler
func handleHealth(w http.ResponseWriter, r *http.Request) {
	applog.V(8).Info("Got HTTP request for /health")
	healthy := true

	for id, status := range workerStatuses {
		if !status.Running {
			healthy = false
			applog.V(8).Infof("Worker %v is not running", id)
		}
	}

	if healthy {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "All workers are up and running")
		return
	}

	http.Error(w, "Some workers are not running. Check applog for more details", http.StatusInternalServerError)
}

// Prometheus metrics handler
func handleMetrics(config cfg.AppConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		applog.V(8).Info("Got HTTP request for /metrics")

		promhttp.HandlerFor(prometheus.Gatherer(config.Metrics.Registry), promhttp.HandlerOpts{}).ServeHTTP(w, r)
	}
}

// Main web server
func runMainWebServer(config cfg.AppConfig, listen string) {
	// Setup http router
	router := mux.NewRouter().StrictSlash(true)

	// Prometheus metrics
	router.HandleFunc("/metrics", handleMetrics(config)).Methods("GET")

	// Health-check endpoint
	router.HandleFunc("/health", handleHealth).Methods("GET")

	// Status endpoint
	router.HandleFunc("/status", handleStatus).Methods("GET")

	// Log
	applog.Info("Main web server started")

	// Run main http router
	applog.Fatal(http.ListenAndServe(listen, router))
}

// Init client
func initClient(config cfg.AppConfig) (cfg.SenderClient, error) {
	var err error
	client := cfg.SenderClient{}

	tr := &http.Transport{
		DisableKeepAlives: true,
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: true}}
	client.HttpClient = &http.Client{Transport: tr, Timeout: config.SendTimeout}

	return client, err
}

// Close client
func closeClient(config cfg.AppConfig, client cfg.SenderClient) error {
	var err error
	client.HttpClient.CloseIdleConnections()
	return err
}

// Send file to s3 bucket
func sendFile(config cfg.AppConfig, client cfg.SenderClient, file string) error {
	fi, err := os.Stat(file)
	if err != nil {
		return err
	}

	size := utils.HumanizeBytes(fi.Size(), false)
	applog.Infof("Sending %q file (%s) to %s", file, size, config.S3bucket)

	err = fs.GzipFile(config, file)
	if err != nil {
		return err
	}

	err = fs.EncryptFile(config, file)
	if err != nil {
		return err
	}

	// TODO: add metrics and actually send file to s3
	//return s3.UploadFile(config, file+".gz")
	err = s3.FakeUploadFile(config, file)
	if err != nil {
		return err
	}

	return fs.DeleteFile(config, file)
}

// Main loop
func upload(ctx context.Context, config cfg.AppConfig, comm *chan cfg.Message) {
	applog.Info("Main upload loop started")

	//tick := time.Tick(time.Duration(1 * time.Second))

	// Keep uploading until we receive exit signal
	for {
		select {
		// Exit signal
		case <-ctx.Done():
			applog.Info("Upload function exiting")
			close(*comm)
			return
		}
	}
}

// Metrics updater
func updateMetrics(config cfg.AppConfig, comm *chan cfg.Message) {
	// Updating every 2 seconds is frequent enough
	tick := time.Tick(2 * time.Second)

	for {
		select {
		// Tick handler
		case <-tick:
			config.Metrics.ChannelLength.WithLabelValues().Set(float64(len(*comm)))
		}
	}
}

// Worker
func worker(wg *sync.WaitGroup, ctx context.Context, id int, config cfg.AppConfig, comm chan cfg.Message, status *cfg.WorkerStatus) {

	applog.Infof("Worker %d started", id)
	defer wg.Done()
	status.ID = id
	status.Running = true

	// Init client per worker to use keep alive where possible
	client, err := initClient(config)
	if err != nil {
		status.Running = false
		applog.Errorf("Worker %v: Failed to initialize sender client: %s", id, err.Error())
		applog.Errorf("Worker %v failed, exiting", id)
		return
	}

	// Main select
	for {
		select {

		case <-ctx.Done():
			status.Running = false

			err = closeClient(config, client)
			if err != nil {
				applog.Errorf("Worker %v: Error closing sender client: %s", id, err.Error())
			}

			applog.Infof("Worker %d exiting", id)
			return

		case msg := <-comm:

			if config.ExitOnFilename != "" && msg.File == config.ExitOnFilename {
				config.Applog.Infof("Worker %d: triggering exit on file: %q", id, msg.File)
				config.CancelFunction()
				return
			}

			applog.Infof("Worker %d: processing file %q", id, msg.File)

			err := sendFile(config, client, msg.File)
			config.Metrics.FileSendCount.WithLabelValues().Inc()

			if err != nil {
				config.Metrics.FileSendErrors.WithLabelValues().Inc()
				// TODO: change to Fatalf
				applog.Errorf("Got error while sending file: %s - upload is inconsistent, exiting", err.Error())
			} else {
				config.Metrics.FileSendSuccess.WithLabelValues().Inc()
			}
		}
	}
}

// Functions for pushing metrics
func prometheusMetricsPusher(config cfg.AppConfig) {
	tick := time.Tick(config.PushInterval)

	pusher := push.New(config.PushGateway, "app").Gatherer(config.Metrics.Registry)

	for {
		select {
		// Tick event
		case <-tick:

			applog.Info("Pushing metrics to Prometheus Pushgateway")

			if err := pusher.Add(); err != nil {
				applog.Errorf("Could not push to Pushgateway: %s", err.Error())
			}
		}
	}
}

// Check URL
func validateUrl(inURL string) error {

	u, err := url.Parse(inURL)

	if err != nil {
		return err
	}

	if u.Scheme == "" {
		return fmt.Errorf("can't find scheme in URL %q", inURL)
	}

	if u.Host == "" {
		return fmt.Errorf("can't find host in URL %q", inURL)
	}

	return nil
}

// Main!
func main() {
	var listen string
	var wg sync.WaitGroup
	var showVersion bool
	var ctxWithCancel context.Context

	// Init config
	config := cfg.AppConfig{}
	config.WorkersCannelSize = workersCannelSize

	//Make a background context.
	ctx := context.Background()
	// Make a new context with cancel, we'll use it to make sure all routines can exit properly.
	ctxWithCancel, config.CancelFunction = context.WithCancel(ctx)

	// Arguments
	flag.BoolVar(&showVersion, "version", false, "Show version and exit")
	flag.IntVar(&config.Workers, "workers", 1, "The number of worker threads")
	flag.StringVar(&listen, "listen", ":8765", "Address:port to listen on for exposing metrics")
	flag.BoolVar(&config.Verbose, "verbose", false, "Print INFO level applog to stdout")
	flag.StringVar(&config.PathToWatch, "path-to-watch", "/app/tmp", "FS path to watch for events")
	flag.StringVar(&config.ExitOnFilename, "exit-on-filename", "", "If this filename is detected by fsWatch, the program exits")

	flag.StringVar(&config.S3bucket, "s3-bucket", "", "S3 bucket to upload to")
	flag.StringVar(&config.S3path, "s3-path", "", "S3 path inside a bucket to upload to")
	flag.DurationVar(&config.SendTimeout, "send-timeout", time.Second*10, "Send request timeout")

	flag.BoolVar(&config.Encrypt, "gzip", true, "Wether to gzip a file before uploading")
	flag.BoolVar(&config.Gzip, "encrypt", true, "Wether to encrypt a file before uploading")
	flag.StringVar(&config.GzipDir, "gzip-dir", "/app/gzip", "Directory to store temporary gzipped files in")
	flag.StringVar(&config.EncryptDir, "encrypt-dir", "/app/enc", "Directory to store temporary encrypted files in")
	flag.StringVar(&config.EnvVarGPGPass, "env-var-name-gpg-password", "GPG_PASSWORD", "Env var name with GPG password")

	flag.StringVar(&config.PushGateway, "push-gateway", "", "Prometheus Pushgateway URL")
	flag.DurationVar(&config.PushInterval, "push-interval", time.Second*15, "Metrics push interval")

	flag.Parse()

	// Show and exit functions
	if showVersion {
		fmt.Printf("Version: %s\n", version)
		os.Exit(0)
	}

	// Initialize the global status var
	workerStatuses = make([]cfg.WorkerStatus, config.Workers)

	// Logger
	applog = logger.Init("s3-file-uploader", config.Verbose, false, io.Discard)
	config.Applog = applog

	// Some checks
	if config.S3bucket == "" {
		applog.Fatal("-s3-bucket is not specified")
	} else if config.S3path == "" {
		applog.Fatal("-s3-path is not specified")
	} else {
		config.S3path = strings.TrimSuffix(config.S3path, "/")
	}

	if config.PathToWatch == "" {
		applog.Fatal("-path-to-watch is not specified")
	}

	if config.Encrypt {
		config.GpgPassword = os.Getenv(config.EnvVarGPGPass)
		if config.GpgPassword == "" {
			applog.Fatal("Empty or non existent GGP password env variable")
		}
	}

	if config.PushInterval < 10*time.Second {
		applog.Fatal("-push-interval must be >= 10 seconds")
	}

	// Checks complete, safe to start
	applog.Info("Starting program")

	// Init metric
	config.Metrics = metrics.InitMetrics(version, workersCannelSize, secondsDurationBuckets)

	// Run a separate routine with http server
	go runMainWebServer(config, listen)

	// Make a channel and start workers
	comm := make(chan cfg.Message, workersCannelSize)
	for i := 0; i < config.Workers; i++ {
		wg.Add(1)
		go worker(&wg, ctxWithCancel, i, config, comm, &workerStatuses[i])
	}

	// Channels for signal processing and locking main()
	sigs := make(chan os.Signal, 1)
	exit := make(chan bool, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	// Run metrics updater routine
	go updateMetrics(config, &comm)

	// Upload stuff to the cloud!
	started := time.Now()
	go upload(ctxWithCancel, config, &comm)
	go fs.WatchDirectory(ctxWithCancel, &comm, config)

	// Start metrics pusher if enabled
	if config.PushGateway != "" {
		go prometheusMetricsPusher(config)
	}

	// Wait for signals to exit or for context to be cancaelled to and send signal to "exit" channel
	go func() {
		select {
		case sig := <-sigs:
			fmt.Printf("\nReceived signal: %v\n", sig)
			config.CancelFunction()
			exit <- true
		case <-ctxWithCancel.Done():
			applog.Info("Main function exiting")
			exit <- true
		}
	}()

	applog.Info("Upload started.")
	<-exit
	duration := time.Since(started).Seconds()

	// Wait for workers to exit
	wg.Wait()
	applog.Infof("Complete. Duration %s", utils.HumanizeDurationSeconds(duration))
}
