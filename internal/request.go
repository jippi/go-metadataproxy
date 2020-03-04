package internal

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	metrics "github.com/armon/go-metrics"
	"github.com/gorilla/mux"
	uuid "github.com/satori/go.uuid"
	"github.com/sirupsen/logrus"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

const (
	telemetryPrefix = "metadataproxy"
)

var (
	isDataDogEnabled = os.Getenv("DATADOG_SERVICE_NAME") != ""
)

type Request struct {
	request       *http.Request
	vars          map[string]string
	id            string
	log           *logrus.Entry
	metricsLabels []metrics.Label
	loggingLabels logrus.Fields
	datadogSpan   tracer.Span
}

func NewRequest(r *http.Request, name, path string) *Request {
	id := uuid.NewV4()

	// Create struct
	request := &Request{
		request:       r,
		vars:          mux.Vars(r),
		id:            id.String(),
		log:           logrus.WithField("request_id", id.String()),
		metricsLabels: make([]metrics.Label, 0),
		loggingLabels: logrus.Fields{},
	}

	// Setup tracing first
	span, found := tracer.SpanFromContext(r.Context())
	request.datadogSpan = span
	if found {
		request.setLogLabel("dd.trace_id", fmt.Sprintf("%d", span.Context().TraceID()))
		request.setLogLabel("dd.span_id", fmt.Sprintf("%d", span.Context().SpanID()))
	}
	request.datadogSpan.SetTag("request_id", request.id)

	request.setLabel("handler_name", name)
	request.setLabel("request_path", path)
	request.setLogLabel("remote_addr", remoteIP(r.RemoteAddr))

	request.setLabelsFromRequest()

	return request
}

// Set a log label (only)
func (r *Request) setLogLabel(key, value string) {
	r.log = r.log.WithField(key, value)
	r.setTraceTag(key, value)
}

// Set a metric label (only)
func (r *Request) setMetricsLabel(key, value string) {
	r.metricsLabels = append(r.metricsLabels, metrics.Label{Name: key, Value: value})
	r.setTraceTag(key, value)
}

// Set both a log label and metric label
func (r *Request) setLabel(key, value string) {
	r.setLogLabel(key, value)
	r.setMetricsLabel(key, value)
}

// Set both a log and metric label for each item
func (r *Request) setLabels(pairs map[string]string) {
	for key, value := range pairs {
		r.setLabel(key, value)
	}
}

// Set Trace tag details
func (r *Request) setTraceTag(key, value string) {
	if !isDataDogEnabled {
		return
	}

	// Don't add datadog own data to traces
	if strings.HasPrefix(key, "dd.") {
		return
	}

	r.datadogSpan.SetTag(key, value)
}

func (r *Request) incrCounterWithLabels(path []string, val float32) {
	path = append([]string{telemetryPrefix}, path...)
	metrics.IncrCounterWithLabels(path, val, r.metricsLabels)
}

func (r *Request) setGaugeWithLabels(path []string, val float32) {
	path = append([]string{telemetryPrefix}, path...)
	metrics.SetGaugeWithLabels(path, val, r.metricsLabels)
}

func (r *Request) setResponseHeaders(w http.ResponseWriter) {
	w.Header().Set("X-Powered-By", "go-metadataproxy")
	w.Header().Set("X-Request-ID", r.id)
}

func (r *Request) setLabelsFromRequest() {
	if version, ok := r.vars["api_version"]; ok {
		r.setLabel("aws_api_version", version)
	}

	if len(copyRequestHeaders) >= 0 {
		for _, label := range copyRequestHeaders {
			if v := r.request.Header.Get("label"); v != "" {
				r.setLabel(labelName("header", label), v)
			}
		}
	}
}

func (r *Request) HandleError(err error, code int, description string, w http.ResponseWriter) {
	r.datadogSpan.Finish(tracer.WithError(err))

	r.setLabels(map[string]string{
		"response_code":     fmt.Sprintf("%d", code),
		"error_description": description,
	})

	r.log.Error(err)
	http.NotFound(w, nil)
}
