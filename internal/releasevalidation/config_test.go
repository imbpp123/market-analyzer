package releasevalidation

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const validConfigJSON = `{
  "analyzer_endpoint": "localhost:9091",
  "timeout": "30s",
  "max_response_bytes": 33554432,
  "selection": {
    "exchange": "binance",
    "market": "spot",
    "symbol": "BTCUSDT",
    "to": "2026-09-15T14:02:30Z",
    "candle_count": 300,
    "interval": "1m"
  },
  "settings": {
    "atr_period": 14,
    "pivot_span": 3,
    "reversal_pct": "2",
    "reversal_atr_period": 14,
    "reversal_atr_multiplier": "1.5",
    "equality_tolerance_pct": "0.1",
    "level_atr_period": 14,
    "zone_width_atr": "0.5",
    "min_touches": 2,
    "min_touch_separation_bars": 5
  },
  "runtime_settings": {
    "request_timeout": "30s",
    "max_request_bytes": 65536,
    "market_data_max_response_bytes": 16777216,
    "max_response_bytes": 33554432
  }
}`

func TestParseConfig(t *testing.T) {
	config, err := ParseConfig(strings.NewReader(validConfigJSON))

	require.NoError(t, err)
	assert.Equal(t, "localhost:9091", config.AnalyzerEndpoint)
	assert.Equal(t, uint32(300), config.Selection.CandleCount)
	assert.Equal(t, "2026-09-15T14:02:30Z", config.Selection.To.Format("2006-01-02T15:04:05Z07:00"))
	assert.Equal(t, uint32(14), config.Settings.ReversalATRPeriod)
}

func TestParseConfigRejectsInvalidInput(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"unknown field", strings.Replace(validConfigJSON, `"timeout": "30s"`, `"timeout": "30s", "extra": true`, 1), "unknown field"},
		{"invalid timestamp", strings.Replace(validConfigJSON, "2026-09-15T14:02:30Z", "tomorrow", 1), "selection.to"},
		{"missing endpoint", strings.Replace(validConfigJSON, "localhost:9091", "", 1), "analyzer endpoint"},
		{"invalid settings", strings.Replace(validConfigJSON, `"min_touches": 2`, `"min_touches": 1`, 1), "min_touches"},
		{"second value", validConfigJSON + `{}`, "one JSON value"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := ParseConfig(strings.NewReader(testCase.input))

			require.Error(t, err)
			assert.Contains(t, err.Error(), testCase.want)
		})
	}
}
