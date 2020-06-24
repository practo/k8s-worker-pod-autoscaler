# promlog
[forked klog](https://github.com/practo/klog) hook to expose the number of log messages as Prometheus metrics:
```
log_messages{level="INFO"}
log_messages{level="WARNING"}
log_messages{level="ERROR"}
```

## Usage

Sample code:
```go
package main

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/practo/klog/v2"
	"github.com/practo/promlog"
)

func main() {
	// Create the Prometheus hook:
	hook := promlog.MustNewPrometheusHook()

	// Configure klog to use the Prometheus hook:
	log.AddHook(hook)

	// Expose Prometheus metrics via HTTP, as you usually would:
	go http.ListenAndServe(":8080", promhttp.Handler())

	// Log with klog, as you usually would.
	// Every time the program generates a log message, a Prometheus counter is incremented for the corresponding level.
	for {
		log.Infof("foo")
		time.Sleep(1 * time.Second)
	}
}
```

Run the above program:
```
$ cd example && go run main.go
I0624 12:24:55.951218   46000 main.go:25] foo
I0624 12:24:56.952746   46000 main.go:25] foo
I0624 12:24:57.953113   46000 main.go:25] foo
I0624 12:24:58.954579   46000 main.go:25] foo
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
I0624 12:28:12.049327   46304 promlog_test.go:48] this is at info level!
W0624 12:28:12.050440   46304 promlog_test.go:54] this is at warning level!
E0624 12:28:12.052603   46304 promlog_test.go:60] this is at error level!
PASS
ok  	github.com/practo/promlog	0.336s

```
