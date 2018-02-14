package internal

import (
	"log"
	"os"
	"time"

	metrics "github.com/armon/go-metrics"
	"github.com/armon/go-metrics/datadog"
	"github.com/armon/go-metrics/prometheus"
)

var (
	telemetry = metrics.NewInmemSink(10*time.Second, time.Minute)
)

// ConfigureTelemetry will configure telemetry sinks
func ConfigureTelemetry() {
	metricsConf := metrics.DefaultConfig("metadataproxy")
	metricsConf.EnableHostname = false
	metricsConf.EnableRuntimeMetrics = true
	metricsConf.EnableServiceLabel = true

	metrics.DefaultInmemSignal(telemetry)

	var fanout metrics.FanoutSink

	if addr := os.Getenv("STATSITE_ADDR"); addr != "" {
		sink, err := metrics.NewStatsiteSink(addr)
		if err != nil {
			log.Fatal(err)
		}

		fanout = append(fanout, sink)
	}

	if addr := os.Getenv("STATSD_ADDR"); addr != "" {
		sink, err := metrics.NewStatsdSink(addr)
		if err != nil {
			log.Fatal(err)
		}

		fanout = append(fanout, sink)
	}

	if addr := os.Getenv("DATADOG_ADDR"); addr != "" {
		sink, err := datadog.NewDogStatsdSink(addr, "")
		if err != nil {
			log.Fatal(err)
		}

		fanout = append(fanout, sink)
	}

	if enabled := os.Getenv("ENABLE_PROMETHEUS"); enabled != "" {
		sink, err := prometheus.NewPrometheusSink()
		if err != nil {
			log.Fatal(err)
		}

		fanout = append(fanout, sink)
	}

	if len(fanout) > 0 {
		fanout = append(fanout, telemetry)
		metrics.NewGlobal(metricsConf, fanout)
	} else {
		metrics.NewGlobal(metricsConf, telemetry)
	}
}
