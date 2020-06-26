# promlog

[![GoDoc Widget]][GoDoc]

---
[forked klog](https://github.com/practo/klog) hook to expose the number of log messages as Prometheus metrics:
```
log_messages_total{severity="ERROR"} 0
log_messages_total{severity="INFO"} 42
log_messages_total{severity="WARNING"} 0
```

## Usage

Sample code:
```go
package main

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/practo/klog/v2"
	"github.com/practo/promlog"
)

func main() {
	// Create the Prometheus hook:
	hook := promlog.MustNewPrometheusHook("", klog.InfoSeverityLevel)

	// Configure klog to use the Prometheus hook:
	klog.AddHook(hook)

	// Expose Prometheus metrics via HTTP, as you usually would:
	go http.ListenAndServe(":8080", promhttp.Handler())

	// Log with klog, as you usually would.
	// Every time the program generates a log message,
	// a Prometheus counter is incremented for the corresponding level.
	for {
		klog.Infof("foo")
		time.Sleep(1 * time.Second)
	}
}
```

Run the above program:
```
$ cd example && go run main.go
I0624 13:26:39.035027   51136 main.go:26] foo
I0624 13:26:40.035543   51136 main.go:26] foo
I0624 13:26:41.039174   51136 main.go:26] foo
I0624 13:26:42.039930   51136 main.go:26] foo
I0624 13:26:43.041310   51136 main.go:26] foo
```

Scrape the Prometheus metrics exposed by the hook:
```
$ curl -fsS localhost:8080 | grep log_messages
# HELP log_messages_total Total number of log messages.
# TYPE log_messages_total counter
log_messages_total{severity="ERROR"} 0
log_messages_total{severity="INFO"} 42
log_messages_total{severity="WARNING"} 0
```

## Compile
```
$ go build
```

## Test
```
$ go test
I0624 13:27:29.696765   51237 promlog_test.go:49] this is at info level!
W0624 13:27:29.697785   51237 promlog_test.go:55] this is at warning level!
E0624 13:27:29.698964   51237 promlog_test.go:61] this is at error level!
PASS
ok  	github.com/practo/promlog	0.237s
```

[GoDoc]: https://godoc.org/github.com/practo/promlog
[GoDoc Widget]: https://godoc.org/github.com/practo/k8s-worker-pod-autoscaler?status.svg
