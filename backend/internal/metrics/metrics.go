// Package metrics implements the two strictly-separated observability channels
// from SOFTWARE_DEV.md:
//
//	serviceMetric     -> aggregatable KPIs (latency, depth, throughput)
//	applicationMetric -> runtime health / errors, always with context
//
// Both write a fixed, grep-friendly line so a log shipper can parse them without
// a structured logger.
package metrics

import (
	"fmt"
	"log"
	"runtime/debug"
	"time"
)

// Severity levels for applicationMetric.
const (
	INFO  = "INFO"
	WARN  = "WARN"
	ERROR = "ERROR"
)

// Service emits a measurable KPI: `[SERVICE_METRIC] <key>=<value> <unit>`.
func Service(key string, value any, unit string) {
	log.Printf("[SERVICE_METRIC] %s=%v %s", key, value, unit)
}

// Latency emits an elapsed-time KPI in milliseconds from a start instant.
// Usage: defer metrics.Latency("api.add.latency_ms", time.Now()).
func Latency(key string, start time.Time) {
	Service(key, time.Since(start).Milliseconds(), "ms")
}

// App emits a health/error diagnostic:
// `[APP_METRIC] <severity> <message> | trace=<...>`.
// Never swallow errors silently — every error path routes through here.
func App(severity, message string, err error) {
	trace := ""
	if err != nil {
		trace = err.Error()
	}
	if severity == ERROR {
		// Include a stack on real errors so failures are debuggable from logs.
		trace = fmt.Sprintf("%s; stack=%s", trace, debug.Stack())
	}
	log.Printf("[APP_METRIC] %s %s | trace=%s", severity, message, trace)
}
