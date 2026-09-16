package releasevalidation

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseAndDifferenceMetrics(t *testing.T) {
	before, err := ParseMetrics(strings.NewReader(`
market_analyzer_active_requests 1
market_analyzer_requests_count{rpc="GetATR",status="OK"} 2
market_analyzer_requests_sum{rpc="GetATR",status="OK"} 0.4
market_analyzer_market_data_count{status="ok"} 2
market_analyzer_market_data_sum{status="ok"} 0.2
market_analyzer_calculation_count{calculation="atr"} 2
market_analyzer_calculation_sum{calculation="atr"} 0.1
market_analyzer_source_candles_count 2
market_analyzer_source_candles_sum 120
market_analyzer_response_bytes_count 2
market_analyzer_response_bytes_sum 1000
`))
	require.NoError(t, err)
	after, err := ParseMetrics(strings.NewReader(`
market_analyzer_active_requests 0
market_analyzer_requests_count{rpc="GetATR",status="OK"} 5
market_analyzer_requests_count{rpc="GetLevels",status="OK"} 1
market_analyzer_requests_sum{rpc="GetATR",status="OK"} 1.0
market_analyzer_requests_sum{rpc="GetLevels",status="OK"} 0.2
market_analyzer_market_data_count{status="ok"} 6
market_analyzer_market_data_sum{status="ok"} 0.6
market_analyzer_calculation_count{calculation="atr"} 6
market_analyzer_calculation_sum{calculation="atr"} 0.3
market_analyzer_source_candles_count 6
market_analyzer_source_candles_sum 360
market_analyzer_response_bytes_count 6
market_analyzer_response_bytes_sum 3000
`))
	require.NoError(t, err)

	delta := DifferenceMetrics(before, after)

	assert.Zero(t, delta.ActiveRequests)
	assert.Equal(t, 4.0, delta.RequestCount)
	assert.InDelta(t, 0.8, delta.RequestDurationSeconds, 0.0001)
	assert.InDelta(t, 0.2, delta.RequestAverageDurationSeconds, 0.0001)
	assert.Equal(t, 4.0, delta.MarketDataCount)
	assert.InDelta(t, 0.1, delta.MarketDataAverageDurationSeconds, 0.0001)
	assert.Equal(t, 240.0, delta.SourceCandles)
	assert.Equal(t, 2000.0, delta.ResponseBytes)
	assert.Equal(t, 500.0, delta.AverageResponseBytes)
}

func TestParseMetricsRejectsMalformedLine(t *testing.T) {
	_, err := ParseMetrics(strings.NewReader("broken\n"))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid metrics line")
}
