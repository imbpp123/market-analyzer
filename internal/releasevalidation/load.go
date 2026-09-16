package releasevalidation

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	marketanalyzerv1 "github.com/imbpp123/market-analyzer/api/go/marketanalyzer/v1"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

const LoadReportSchemaVersion = "market-analyzer-load-validation/v1"

type LoadOptions struct {
	Concurrency int `json:"concurrency"`
	Requests    int `json:"requests"`
}

type LoadReport struct {
	SchemaVersion  string        `json:"schema_version"`
	StartedAt      time.Time     `json:"started_at"`
	CompletedAt    time.Time     `json:"completed_at"`
	Configuration  ReportConfig  `json:"configuration"`
	Load           LoadOptions   `json:"load"`
	Results        []LoadResult  `json:"results"`
	Summary        LoadSummary   `json:"summary"`
	ServiceMetrics *MetricsDelta `json:"service_metrics,omitempty"`
}

type LoadResult struct {
	Index           int    `json:"index"`
	Name            string `json:"name"`
	RPC             string `json:"rpc"`
	Duration        string `json:"duration"`
	Status          string `json:"status"`
	ResponseBytes   int    `json:"response_bytes,omitempty"`
	Error           string `json:"error,omitempty"`
	ValidationError bool   `json:"validation_error,omitempty"`

	duration time.Duration
}

type LoadSummary struct {
	Total                int            `json:"total"`
	Passed               int            `json:"passed"`
	Failed               int            `json:"failed"`
	Elapsed              string         `json:"elapsed"`
	RequestsPerSecond    float64        `json:"requests_per_second"`
	ResponseBytes        int            `json:"response_bytes"`
	LatencyP50           string         `json:"latency_p50"`
	LatencyP95           string         `json:"latency_p95"`
	LatencyP99           string         `json:"latency_p99"`
	LatencyMaximum       string         `json:"latency_maximum"`
	Statuses             map[string]int `json:"statuses"`
	RPCStatuses          map[string]int `json:"rpc_statuses"`
	ValidationErrorCount int            `json:"validation_error_count"`
}

func RunLoad(ctx context.Context, client marketanalyzerv1.MarketAnalyzerServiceClient, config Config, options LoadOptions, now func() time.Time) (LoadReport, error) {
	if client == nil {
		return LoadReport{}, errors.New("analyzer client is required")
	}
	if now == nil {
		return LoadReport{}, errors.New("clock is required")
	}
	if err := config.Validate(); err != nil {
		return LoadReport{}, fmt.Errorf("validate config: %w", err)
	}
	if options.Concurrency <= 0 {
		return LoadReport{}, errors.New("concurrency must be positive")
	}
	if options.Requests <= 0 {
		return LoadReport{}, errors.New("requests must be positive")
	}

	report := LoadReport{
		SchemaVersion: LoadReportSchemaVersion,
		StartedAt:     now().UTC(),
		Configuration: reportConfig(config),
		Load:          options,
		Results:       make([]LoadResult, 0, options.Requests),
	}
	cases := validationCases(client, config)
	jobs := make(chan int, options.Requests)
	for index := range options.Requests {
		jobs <- index
	}
	close(jobs)

	started := time.Now()
	var waitGroup sync.WaitGroup
	var resultsMu sync.Mutex
	for range options.Concurrency {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			for index := range jobs {
				result := runLoadCase(ctx, config, cases[index%len(cases)], index)
				resultsMu.Lock()
				report.Results = append(report.Results, result)
				resultsMu.Unlock()
			}
		}()
	}
	waitGroup.Wait()
	elapsed := time.Since(started)
	sort.Slice(report.Results, func(left, right int) bool { return report.Results[left].Index < report.Results[right].Index })
	report.CompletedAt = now().UTC()
	report.Summary = summarizeLoad(report.Results, elapsed)

	if report.Summary.Failed > 0 {
		return report, fmt.Errorf("%d of %d load requests failed", report.Summary.Failed, report.Summary.Total)
	}

	return report, nil
}

func runLoadCase(ctx context.Context, config Config, testCase validationCase, index int) LoadResult {
	callCtx, cancel := context.WithTimeout(ctx, config.Timeout)
	defer cancel()

	started := time.Now()
	view, err := testCase.call(callCtx)
	duration := time.Since(started)
	validationFailed := false
	if err == nil {
		if validationErr := validateResult(config.Selection, view); validationErr != nil {
			err = validationErr
			validationFailed = true
		}
	}

	result := LoadResult{Index: index, Name: testCase.name, RPC: testCase.rpc, Duration: duration.String(), duration: duration}
	if err != nil {
		result.Status = status.Code(err).String()
		result.Error = err.Error()
		result.ValidationError = validationFailed
		return result
	}

	result.Status = "OK"
	result.ResponseBytes = proto.Size(view.message)
	return result
}

func summarizeLoad(results []LoadResult, elapsed time.Duration) LoadSummary {
	summary := LoadSummary{Total: len(results), Elapsed: elapsed.String(), Statuses: make(map[string]int), RPCStatuses: make(map[string]int)}
	latencies := make([]time.Duration, 0, len(results))
	for _, result := range results {
		latencies = append(latencies, result.duration)
		summary.Statuses[result.Status]++
		summary.RPCStatuses[result.RPC+"/"+result.Status]++
		if result.Status == "OK" {
			summary.Passed++
			summary.ResponseBytes += result.ResponseBytes
		} else {
			summary.Failed++
			if result.ValidationError {
				summary.ValidationErrorCount++
			}
		}
	}
	if elapsed > 0 {
		summary.RequestsPerSecond = float64(len(results)) / elapsed.Seconds()
	}
	if len(latencies) == 0 {
		return summary
	}

	sort.Slice(latencies, func(left, right int) bool { return latencies[left] < latencies[right] })
	summary.LatencyP50 = percentile(latencies, 50).String()
	summary.LatencyP95 = percentile(latencies, 95).String()
	summary.LatencyP99 = percentile(latencies, 99).String()
	summary.LatencyMaximum = latencies[len(latencies)-1].String()
	return summary
}

func percentile(sorted []time.Duration, percentage int) time.Duration {
	index := (len(sorted)*percentage + 99) / 100
	if index < 1 {
		index = 1
	}
	return sorted[index-1]
}

func reportConfig(config Config) ReportConfig {
	return ReportConfig{
		AnalyzerEndpoint: config.AnalyzerEndpoint, Timeout: config.Timeout.String(), MaxResponseBytes: config.MaxResponseBytes,
		BuildRevision: config.BuildRevision, UpstreamContractVersion: config.UpstreamContractVersion,
		Selection: config.Selection, Settings: config.Settings, RuntimeSettings: config.RuntimeSettings,
	}
}
