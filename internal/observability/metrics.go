package observability

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/imbpp123/market-analyzer/internal/application"
	"github.com/imbpp123/market-analyzer/internal/domain"
)

type measurement struct {
	count uint64
	sum   float64
}

type Registry struct {
	active atomic.Int64

	mu            sync.Mutex
	requests      map[string]measurement
	dependencies  map[string]measurement
	calculations  map[string]measurement
	sourceCandles measurement
	responseBytes measurement
}

func NewRegistry() *Registry {
	return &Registry{
		requests:     make(map[string]measurement),
		dependencies: make(map[string]measurement),
		calculations: make(map[string]measurement),
	}
}

func (r *Registry) RequestStarted() { r.active.Add(1) }

func (r *Registry) RequestFinished(rpc, status string, duration time.Duration, responseBytes int) {
	r.active.Add(-1)

	r.mu.Lock()
	defer r.mu.Unlock()
	add(r.requests, rpc+"\x00"+status, duration.Seconds())
	if responseBytes >= 0 {
		r.responseBytes.count++
		r.responseBytes.sum += float64(responseBytes)
	}
}

func (r *Registry) ObserveDependency(status string, duration time.Duration, sourceCount int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	add(r.dependencies, status, duration.Seconds())
	if sourceCount >= 0 {
		r.sourceCandles.count++
		r.sourceCandles.sum += float64(sourceCount)
	}
}

func (r *Registry) ObserveCalculation(name string, duration time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	add(r.calculations, name, duration.Seconds())
}

func (r *Registry) ServeHTTP(response http.ResponseWriter, _ *http.Request) {
	r.mu.Lock()
	requests := clone(r.requests)
	dependencies := clone(r.dependencies)
	calculations := clone(r.calculations)
	sourceCandles := r.sourceCandles
	responseBytes := r.responseBytes
	r.mu.Unlock()

	response.Header().Set("Content-Type", "text/plain; version=0.0.4")
	_, _ = fmt.Fprintln(response, "# TYPE market_analyzer_active_requests gauge")
	_, _ = fmt.Fprintf(response, "market_analyzer_active_requests %d\n", r.active.Load())
	writeMeasurements(response, "market_analyzer_requests", []string{"rpc", "status"}, requests)
	writeMeasurements(response, "market_analyzer_market_data", []string{"status"}, dependencies)
	writeMeasurements(response, "market_analyzer_calculation", []string{"calculation"}, calculations)
	writeSingle(response, "market_analyzer_source_candles", sourceCandles)
	writeSingle(response, "market_analyzer_response_bytes", responseBytes)
}

type Reader struct {
	next    application.CandleReader
	metrics *Registry
}

func NewReader(next application.CandleReader, metrics *Registry) *Reader {
	return &Reader{next: next, metrics: metrics}
}

func (r *Reader) ReadCandles(ctx context.Context, instrument domain.Instrument, interval domain.Interval, candleRange domain.CandleRange) (application.SourceSeries, error) {
	started := time.Now()
	series, err := r.next.ReadCandles(ctx, instrument, interval, candleRange)
	status := "ok"
	sourceCount := len(series.Candles)
	if err != nil {
		status = dependencyStatus(err)
		sourceCount = -1
	}
	r.metrics.ObserveDependency(status, time.Since(started), sourceCount)
	return series, err
}

func dependencyStatus(err error) string {
	var applicationError *application.Error
	if errors.As(err, &applicationError) {
		return string(applicationError.Kind)
	}
	return "internal_error"
}

func add(values map[string]measurement, key string, value float64) {
	item := values[key]
	item.count++
	item.sum += value
	values[key] = item
}

func clone(source map[string]measurement) map[string]measurement {
	result := make(map[string]measurement, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func writeMeasurements(writer io.Writer, name string, labels []string, values map[string]measurement) {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		parts := strings.Split(key, "\x00")
		labelText := make([]string, len(labels))
		for index, label := range labels {
			labelText[index] = label + "=" + strconv.Quote(parts[index])
		}
		writeLabeled(writer, name, strings.Join(labelText, ","), values[key])
	}
}

func writeSingle(writer io.Writer, name string, value measurement) {
	_, _ = fmt.Fprintf(writer, "# TYPE %s summary\n%s_count %d\n%s_sum %g\n", name, name, value.count, name, value.sum)
}

func writeLabeled(writer io.Writer, name, labels string, value measurement) {
	_, _ = fmt.Fprintf(writer, "%s_count{%s} %d\n%s_sum{%s} %g\n", name, labels, value.count, name, labels, value.sum)
}
