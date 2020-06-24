package promlog

import (
	"github.com/practo/klog/v2"
	"github.com/prometheus/client_golang/prometheus"
)

var supportedSeverityLevels = []string{
	klog.InfoSeverityLevel,
	klog.WarningSeverityLevel,
	klog.ErrorSeverityLevel,
}

// PrometheusHook exposes Prometheus counters for each of klog severity levels.
type PrometheusHook struct {
	counterVec *prometheus.CounterVec
}

// NewPrometheusHook creates a new instance of PrometheusHook
// which exposes Prometheus counters for various severity.
// Contrarily to MustNewPrometheusHook, it returns an error to the
// caller in case of issue.
// Use NewPrometheusHook if you want more control.
// Use MustNewPrometheusHook if you want a less verbose hook creation.
func NewPrometheusHook() (*PrometheusHook, error) {
	counterVec := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "log_messages_total",
		Help: "Total number of log messages.",
	}, []string{"severity"})
	// Initialise counters for all supported severity:
	for _, severity := range supportedSeverityLevels {
		counterVec.WithLabelValues(severity)
	}
	// Try to unregister the counter vector,
	// in case already registered for some reason,
	// e.g. double initialisation/configuration
	// done by mistake by the end-user.
	prometheus.Unregister(counterVec)
	// Try to register the counter vector:
	err := prometheus.Register(counterVec)
	if err != nil {
		return nil, err
	}
	return &PrometheusHook{
		counterVec: counterVec,
	}, nil
}

// MustNewPrometheusHook creates a new instance of PrometheusHook
// which exposes Prometheus counters for various log levels.
// Contrarily to NewPrometheusHook, it does not return
// any error to the caller, but panics instead.
// Use MustNewPrometheusHook if you want a less verbose
// hook creation. Use NewPrometheusHook if you want more control.
func MustNewPrometheusHook() *PrometheusHook {
	hook, err := NewPrometheusHook()
	if err != nil {
		panic(err)
	}
	return hook
}

// Fire increments the appropriate Prometheus counter
func (hook *PrometheusHook) Fire(s string, args ...interface{}) error {
	hook.counterVec.WithLabelValues(s).Inc()
	return nil
}

// SeverityLevel can be "INFO", "WARNING", "ERROR" or "FATAL"
// Hook will be fired in all the cases when severity is greater than
// or equal to the severity level
func (hook *PrometheusHook) SeverityLevel() string {
	return klog.InfoSeverityLevel
}
