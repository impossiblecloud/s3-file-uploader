package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// AppMetrics contains pointers to prometheus metrics objects
type AppMetrics struct {
	Listen   string
	Registry *prometheus.Registry

	// Counters
	ChannelFullEvents *prometheus.CounterVec
	FileSendCount     *prometheus.CounterVec
	FileOrigBytesSum  *prometheus.CounterVec
	FileSendBytesSum  *prometheus.CounterVec
	FileSendErrors    *prometheus.CounterVec
	FileSendSuccess   *prometheus.CounterVec

	// Gauges
	ConfigWorkers       *prometheus.GaugeVec
	ChannelLength       *prometheus.GaugeVec
	ChannelConfigLength *prometheus.GaugeVec
	Config              *prometheus.GaugeVec

	// Historgams
	HistFileSendDuration *prometheus.HistogramVec
}

func InitMetrics(version string, workersCannelSize int, secondsDurationBuckets []float64) AppMetrics {

	am := AppMetrics{}
	am.Registry = prometheus.NewRegistry()

	// Config info
	am.Config = promauto.With(am.Registry).NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "s3_file_uploader",
			Name:      "config",
			Help:      "App config info",
		},
		[]string{"version"},
	)

	// Send file metrics
	am.FileSendCount = promauto.With(am.Registry).NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "s3_file_uploader",
			Subsystem: "uploads",
			Name:      "total",
			Help:      "The total number of objects sent to s3 endpoint",
		},
		[]string{},
	)

	am.FileSendBytesSum = promauto.With(am.Registry).NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "s3_file_uploader",
			Subsystem: "uploads",
			Name:      "bytes_sum",
			Help:      "The total number of bytes sent to s3 endpoint",
		},
		[]string{},
	)

	am.FileOrigBytesSum = promauto.With(am.Registry).NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "s3_file_uploader",
			Subsystem: "files",
			Name:      "bytes_sum",
			Help:      "The total number of bytes of processed files before packing and encryption",
		},
		[]string{},
	)

	am.FileSendSuccess = promauto.With(am.Registry).NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "s3_file_uploader",
			Subsystem: "uploads",
			Name:      "success_total",
			Help:      "The total number of requests successfully sent to remote endpoint",
		},
		[]string{},
	)

	am.FileSendErrors = promauto.With(am.Registry).NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "s3_file_uploader",
			Subsystem: "uploads",
			Name:      "errors_total",
			Help:      "The total number of errors when sending requests",
		},
		[]string{},
	)

	am.HistFileSendDuration = promauto.With(am.Registry).NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "s3_file_uploader",
			Subsystem: "uploads",
			Name:      "hist_duration_seconds",
			Help:      "Histogram distribution of request durations, in seconds",
			Buckets:   secondsDurationBuckets,
		},
		[]string{},
	)

	// App health metrics
	am.ConfigWorkers = promauto.With(am.Registry).NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "s3_file_uploader",
			Subsystem: "config",
			Name:      "workers",
			Help:      "Number of workers",
		},
		[]string{},
	)

	am.ChannelLength = promauto.With(am.Registry).NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "s3_file_uploader",
			Subsystem: "runtime",
			Name:      "channel_length",
			Help:      "Number of messages in the main channel",
		},
		[]string{},
	)

	am.ChannelConfigLength = promauto.With(am.Registry).NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "s3_file_uploader",
			Subsystem: "config",
			Name:      "channel_length",
			Help:      "Max channel length",
		},
		[]string{},
	)

	am.ChannelFullEvents = promauto.With(am.Registry).NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "s3_file_uploader",
			Subsystem: "runtime",
			Name:      "channel_full_events",
			Help:      "Number of events when worker channel was full",
		},
		[]string{},
	)

	am.Config.WithLabelValues(version).Set(1)
	am.ChannelConfigLength.WithLabelValues().Set(float64(workersCannelSize))
	am.ChannelLength.WithLabelValues().Set(float64(0))
	am.ChannelFullEvents.WithLabelValues().Add(0)

	am.FileSendCount.WithLabelValues().Add(0)
	am.FileSendBytesSum.WithLabelValues().Add(0)
	am.FileSendErrors.WithLabelValues().Add(0)
	am.FileSendSuccess.WithLabelValues().Add(0)

	am.Registry.MustRegister()

	return am
}
