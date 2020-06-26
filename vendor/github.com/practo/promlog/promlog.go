package promlog

import (
	"fmt"
	"github.com/practo/klog/v2"
	"github.com/prometheus/client_golang/prometheus"
)

// supportedSeverityLevels are the level of severities supported
var supportedSeverityLevels = []string{
	klog.InfoSeverityLevel,
	klog.WarningSeverityLevel,
	klog.ErrorSeverityLevel,
}

// PrometheusHook exposes Prometheus counters for each of klog severity levels.
type PrometheusHook struct {
	// severityLevel specifies the required level for the log metrics
	// that must be emitted as prometheus metric
	severityLevel string
	counterVec    *prometheus.CounterVec
}

// NewPrometheusHook creates a new instance of PrometheusHook
// which exposes Prometheus counters for various severity.
// Contrarily to MustNewPrometheusHook, it returns an error to the
// caller in case of issue.
// Use NewPrometheusHook if you want more control.
// Use MustNewPrometheusHook if you want a less verbose hook creation.
func NewPrometheusHook(
	metricPrefix string, severityLevel string) (*PrometheusHook, error) {

	counterVec := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: metricPrefix + "log_messages_total",
		Help: "Total number of log messages.",
	}, []string{"severity"})

	var severityLevels []string

	switch severityLevel {
	case klog.InfoSeverityLevel:
		severityLevels = []string{
			klog.InfoSeverityLevel,
			klog.WarningSeverityLevel,
			klog.ErrorSeverityLevel,
		}
	case klog.WarningSeverityLevel:
		severityLevels = []string{
			klog.WarningSeverityLevel,
			klog.ErrorSeverityLevel,
		}
	case klog.ErrorSeverityLevel:
		severityLevels = []string{
			klog.ErrorSeverityLevel,
		}
	default:
		return nil, fmt.Errorf(
			"only following severity levels are supported: %v",
			supportedSeverityLevels)
	}

	// Initialise counters for all supported severity:
	for _, severity := range severityLevels {
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
		severityLevel: severityLevel,
		counterVec:    counterVec,
	}, nil
}

// MustNewPrometheusHook creates a new instance of PrometheusHook
// which exposes Prometheus counters for various log levels.
// Contrarily to NewPrometheusHook, it does not return
// any error to the caller, but panics instead.
// Use MustNewPrometheusHook if you want a less verbose
// hook creation. Use NewPrometheusHook if you want more control.
func MustNewPrometheusHook(
	metricPrefix string, severityLevel string) *PrometheusHook {

	hook, err := NewPrometheusHook(metricPrefix, severityLevel)
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
	return hook.severityLevel
}
