// Simple tool to watch directory for new files and upload them to S3

package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/adidenko/s3-file-uploader/internal/metrics"
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
var workerStatuses []workerStatus

// Let's use the same buckets for histograms as NGINX Ingress controller
var secondsDurationBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

// Config is the main app config struct
type appConfig struct {
	workers     int
	verbose     bool
	sendTimeout time.Duration
	s3bucket    string

	pushGateway  string
	pushInterval time.Duration

	metrics metrics.AppMetrics
}

// Client stores pointers to configured remote endpoint writes/clients
type senderClient struct {
	httpClient *http.Client

	socketConn   net.Conn
	socketWriter *bufio.Writer
}

// Message that is sent to workers
type message struct {
	file string
}

// Workers status
type workerStatus struct {
	ID      int  `json:"id"`
	Running bool `json:"running"`
}

// Status defines status
type appStatus struct {
	Workers []workerStatus `json:"workers"`
	Version string         `json:"version"`
}

// Status for future web endpoint
func handleStatus(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)

	myStatus := appStatus{
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
func handleMetrics(config appConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		applog.V(8).Info("Got HTTP request for /metrics")

		promhttp.HandlerFor(prometheus.Gatherer(config.metrics.Registry), promhttp.HandlerOpts{}).ServeHTTP(w, r)
	}
}

// Main web server
func runMainWebServer(config appConfig, listen string) {
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
func initClient(config appConfig) (senderClient, error) {
	var err error
	client := senderClient{}

	tr := &http.Transport{
		DisableKeepAlives: true,
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: true}}
	client.httpClient = &http.Client{Transport: tr, Timeout: config.sendTimeout}

	return client, err
}

// Close client
func closeClient(config appConfig, client senderClient) error {
	var err error
	client.httpClient.CloseIdleConnections()
	return err
}

// Send file to s3 bucket
func sendFile(config appConfig, client senderClient, file string) error {
	applog.Infof("Sending %q file to %s", file, config.s3bucket)
	return nil
}

// Main loop
func upload(ctx context.Context, config appConfig, comm *chan message) {
	applog.Info("Main upload loop started")

	tick := time.Tick(time.Duration(1 * time.Second))

	// Keep uploading until we receive exit signal
	for {
		select {
		// Exit signal
		case <-ctx.Done():
			applog.Info("Upload function exiting")
			close(*comm)
			return
		// Fsnotify event will go here
		case <-tick:
			if len(*comm) < workersCannelSize {
				*comm <- message{file: fmt.Sprintf("file-%v", rand.Int())}
			} else {
				config.metrics.ChannelFullEvents.WithLabelValues().Inc()
			}
		}
	}
}

// Metrics updater
func updateMetrics(config appConfig, comm *chan message) {
	// Updating every 2 seconds is frequent enough
	tick := time.Tick(2 * time.Second)

	for {
		select {
		// Tick handler
		case <-tick:
			config.metrics.ChannelLength.WithLabelValues().Set(float64(len(*comm)))
		}
	}
}

// Worker
func worker(ctx context.Context, id int, config appConfig, comm chan message, status *workerStatus, wg *sync.WaitGroup) {

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

			applog.Infof("Worker %d: processing file %q", id, msg.file)

			err := sendFile(config, client, "file")
			config.metrics.FileSendCount.WithLabelValues().Inc()

			if err != nil {
				config.metrics.FileSendErrors.WithLabelValues().Inc()
			} else {
				config.metrics.FileSendSuccess.WithLabelValues().Inc()
			}
		}
	}
}

// Functions for pushing metrics
func prometheusMetricsPusher(config appConfig) {
	tick := time.Tick(config.pushInterval)

	pusher := push.New(config.pushGateway, "app").Gatherer(config.metrics.Registry)

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

	// Init config
	config := appConfig{}

	//Make a background context.
	ctx := context.Background()
	// Make a new context with cancel, we'll use it to make sure all routines can exit properly.
	ctxWithCancel, cancelFunction := context.WithCancel(ctx)

	// Arguments
	flag.BoolVar(&showVersion, "version", false, "Show version and exit")
	flag.IntVar(&config.workers, "workers", 1, "The number of worker threads")
	flag.StringVar(&listen, "listen", ":8765", "Address:port to listen on for exposing metrics")
	flag.BoolVar(&config.verbose, "verbose", false, "Print INFO level applog to stdout")

	flag.StringVar(&config.s3bucket, "s3-bucket", "", "S3 bucket to upload to")
	flag.DurationVar(&config.sendTimeout, "send-timeout", time.Second*10, "Send request timeout")

	flag.StringVar(&config.pushGateway, "push-gateway", "", "Prometheus Pushgateway URL")
	flag.DurationVar(&config.pushInterval, "push-interval", time.Second*15, "Metrics push interval")

	flag.Parse()

	// Show and exit functions
	if showVersion {
		fmt.Printf("Version: %s\n", version)
		os.Exit(0)
	}

	// Initialize the global status var
	workerStatuses = make([]workerStatus, config.workers)

	// Logger
	applog = logger.Init("s3-file-uploader", config.verbose, false, io.Discard)

	// Some checks
	if config.s3bucket == "" {
		applog.Fatal("-s3-bucket is not specified")
	} else if err := validateUrl(config.s3bucket); err != nil {
		applog.Fatal(err.Error())
	}

	// Push interval sanity check
	if config.pushInterval < 10*time.Second {
		applog.Fatal("-push-interval must be >= 10 seconds")
	}

	applog.Info("Starting program")

	// Init metric
	config.metrics = metrics.InitMetrics(version, workersCannelSize, secondsDurationBuckets)

	// Run a separate routine with http server
	go runMainWebServer(config, listen)

	// Make a channel and start workers
	comm := make(chan message, workersCannelSize)
	for i := 0; i < config.workers; i++ {
		wg.Add(1)
		go worker(ctxWithCancel, i, config, comm, &workerStatuses[i], &wg)
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

	// Start metrics pusher if enabled
	if config.pushGateway != "" {
		go prometheusMetricsPusher(config)
	}

	// Wait for signals to exit and send signal to "exit" channel
	go func() {
		sig := <-sigs
		fmt.Printf("\nReceived signal: %v\n", sig)
		cancelFunction()
		exit <- true
	}()

	applog.Info("Upload is running.")
	<-exit
	duration := time.Since(started).Seconds()

	// Wait for workers to exit
	wg.Wait()
	applog.Infof("Benchmark is complete. Duration %s", utils.HumanizeDurationSeconds(duration))
}
