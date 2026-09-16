package releasevalidation

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

type MetricsSnapshot struct {
	ActiveRequests             float64 `json:"active_requests"`
	RequestCount               float64 `json:"request_count"`
	RequestDurationSeconds     float64 `json:"request_duration_seconds"`
	MarketDataCount            float64 `json:"market_data_count"`
	MarketDataDurationSeconds  float64 `json:"market_data_duration_seconds"`
	CalculationCount           float64 `json:"calculation_count"`
	CalculationDurationSeconds float64 `json:"calculation_duration_seconds"`
	SourceCandleCount          float64 `json:"source_candle_count"`
	SourceCandles              float64 `json:"source_candles"`
	ResponseCount              float64 `json:"response_count"`
	ResponseBytes              float64 `json:"response_bytes"`
}

type MetricsDelta struct {
	MetricsSnapshot
	RequestAverageDurationSeconds    float64 `json:"request_average_duration_seconds"`
	MarketDataAverageDurationSeconds float64 `json:"market_data_average_duration_seconds"`
	CalculationAverageSeconds        float64 `json:"calculation_average_duration_seconds"`
	AverageResponseBytes             float64 `json:"average_response_bytes"`
}

func ParseMetrics(reader io.Reader) (MetricsSnapshot, error) {
	var result MetricsSnapshot
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			return MetricsSnapshot{}, fmt.Errorf("invalid metrics line %q", line)
		}
		value, err := strconv.ParseFloat(fields[1], 64)
		if err != nil {
			return MetricsSnapshot{}, fmt.Errorf("parse metric value in %q: %w", line, err)
		}
		name := fields[0]
		if index := strings.IndexByte(name, '{'); index >= 0 {
			name = name[:index]
		}
		switch name {
		case "market_analyzer_active_requests":
			result.ActiveRequests = value
		case "market_analyzer_requests_count":
			result.RequestCount += value
		case "market_analyzer_requests_sum":
			result.RequestDurationSeconds += value
		case "market_analyzer_market_data_count":
			result.MarketDataCount += value
		case "market_analyzer_market_data_sum":
			result.MarketDataDurationSeconds += value
		case "market_analyzer_calculation_count":
			result.CalculationCount += value
		case "market_analyzer_calculation_sum":
			result.CalculationDurationSeconds += value
		case "market_analyzer_source_candles_count":
			result.SourceCandleCount += value
		case "market_analyzer_source_candles_sum":
			result.SourceCandles += value
		case "market_analyzer_response_bytes_count":
			result.ResponseCount += value
		case "market_analyzer_response_bytes_sum":
			result.ResponseBytes += value
		}
	}
	if err := scanner.Err(); err != nil {
		return MetricsSnapshot{}, fmt.Errorf("read metrics: %w", err)
	}

	return result, nil
}

func DifferenceMetrics(before, after MetricsSnapshot) MetricsDelta {
	delta := MetricsDelta{MetricsSnapshot: MetricsSnapshot{
		ActiveRequests:             after.ActiveRequests,
		RequestCount:               after.RequestCount - before.RequestCount,
		RequestDurationSeconds:     after.RequestDurationSeconds - before.RequestDurationSeconds,
		MarketDataCount:            after.MarketDataCount - before.MarketDataCount,
		MarketDataDurationSeconds:  after.MarketDataDurationSeconds - before.MarketDataDurationSeconds,
		CalculationCount:           after.CalculationCount - before.CalculationCount,
		CalculationDurationSeconds: after.CalculationDurationSeconds - before.CalculationDurationSeconds,
		SourceCandleCount:          after.SourceCandleCount - before.SourceCandleCount,
		SourceCandles:              after.SourceCandles - before.SourceCandles,
		ResponseCount:              after.ResponseCount - before.ResponseCount,
		ResponseBytes:              after.ResponseBytes - before.ResponseBytes,
	}}
	if delta.RequestCount > 0 {
		delta.RequestAverageDurationSeconds = delta.RequestDurationSeconds / delta.RequestCount
	}
	if delta.MarketDataCount > 0 {
		delta.MarketDataAverageDurationSeconds = delta.MarketDataDurationSeconds / delta.MarketDataCount
	}
	if delta.CalculationCount > 0 {
		delta.CalculationAverageSeconds = delta.CalculationDurationSeconds / delta.CalculationCount
	}
	if delta.ResponseCount > 0 {
		delta.AverageResponseBytes = delta.ResponseBytes / delta.ResponseCount
	}
	return delta
}
