package releasevalidation

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

type Selection struct {
	Exchange    string    `json:"exchange"`
	Market      string    `json:"market"`
	Symbol      string    `json:"symbol"`
	To          time.Time `json:"to"`
	CandleCount uint32    `json:"candle_count"`
	Interval    string    `json:"interval"`
}

type Settings struct {
	ATRPeriod              uint32 `json:"atr_period"`
	PivotSpan              uint32 `json:"pivot_span"`
	ReversalPct            string `json:"reversal_pct"`
	ReversalATRPeriod      uint32 `json:"reversal_atr_period"`
	ReversalATRMultiplier  string `json:"reversal_atr_multiplier"`
	EqualityTolerancePct   string `json:"equality_tolerance_pct"`
	LevelATRPeriod         uint32 `json:"level_atr_period"`
	ZoneWidthATR           string `json:"zone_width_atr"`
	MinTouches             uint32 `json:"min_touches"`
	MinTouchSeparationBars uint32 `json:"min_touch_separation_bars"`
}

type RuntimeSettings struct {
	RequestTimeout             string `json:"request_timeout"`
	MaxRequestBytes            int    `json:"max_request_bytes"`
	MarketDataMaxResponseBytes int    `json:"market_data_max_response_bytes"`
	MaxResponseBytes           int    `json:"max_response_bytes"`
}

type Config struct {
	AnalyzerEndpoint        string          `json:"analyzer_endpoint"`
	Timeout                 time.Duration   `json:"-"`
	MaxResponseBytes        int             `json:"max_response_bytes"`
	BuildRevision           string          `json:"build_revision"`
	UpstreamContractVersion string          `json:"upstream_contract_version"`
	Selection               Selection       `json:"selection"`
	Settings                Settings        `json:"settings"`
	RuntimeSettings         RuntimeSettings `json:"runtime_settings"`
}

type configFile struct {
	AnalyzerEndpoint        string `json:"analyzer_endpoint"`
	Timeout                 string `json:"timeout"`
	MaxResponseBytes        int    `json:"max_response_bytes"`
	BuildRevision           string `json:"build_revision"`
	UpstreamContractVersion string `json:"upstream_contract_version"`
	Selection               struct {
		Exchange    string `json:"exchange"`
		Market      string `json:"market"`
		Symbol      string `json:"symbol"`
		To          string `json:"to"`
		CandleCount uint32 `json:"candle_count"`
		Interval    string `json:"interval"`
	} `json:"selection"`
	Settings        Settings        `json:"settings"`
	RuntimeSettings RuntimeSettings `json:"runtime_settings"`
}

func ParseConfig(reader io.Reader) (Config, error) {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()

	var input configFile
	if err := decoder.Decode(&input); err != nil {
		return Config{}, fmt.Errorf("decode validation config: %w", err)
	}

	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return Config{}, errors.New("validation config must contain one JSON value")
	}

	timeout, err := time.ParseDuration(input.Timeout)
	if err != nil || timeout <= 0 {
		return Config{}, errors.New("timeout must be a positive duration")
	}

	to, err := time.Parse(time.RFC3339Nano, input.Selection.To)
	if err != nil {
		return Config{}, errors.New("selection.to must be an RFC3339 timestamp")
	}

	config := Config{
		AnalyzerEndpoint:        strings.TrimSpace(input.AnalyzerEndpoint),
		Timeout:                 timeout,
		MaxResponseBytes:        input.MaxResponseBytes,
		BuildRevision:           strings.TrimSpace(input.BuildRevision),
		UpstreamContractVersion: strings.TrimSpace(input.UpstreamContractVersion),
		Selection: Selection{
			Exchange: strings.TrimSpace(input.Selection.Exchange), Market: strings.TrimSpace(input.Selection.Market),
			Symbol: input.Selection.Symbol, To: to.UTC(), CandleCount: input.Selection.CandleCount, Interval: input.Selection.Interval,
		},
		Settings:        input.Settings,
		RuntimeSettings: input.RuntimeSettings,
	}

	if err := config.Validate(); err != nil {
		return Config{}, err
	}

	return config, nil
}

func (c Config) Validate() error {
	runtimeTimeout, runtimeTimeoutErr := time.ParseDuration(c.RuntimeSettings.RequestTimeout)
	switch {
	case c.AnalyzerEndpoint == "":
		return errors.New("analyzer endpoint is required")
	case c.Timeout <= 0:
		return errors.New("timeout must be positive")
	case c.MaxResponseBytes <= 0:
		return errors.New("max response bytes must be positive")
	case c.Selection.Exchange == "":
		return errors.New("selection.exchange is required")
	case c.Selection.Market == "":
		return errors.New("selection.market is required")
	case c.Selection.Symbol == "":
		return errors.New("selection.symbol is required")
	case c.Selection.To.IsZero():
		return errors.New("selection.to is required")
	case c.Selection.CandleCount == 0:
		return errors.New("selection.candle_count must be positive")
	case c.Selection.Interval == "":
		return errors.New("selection.interval is required")
	case c.Settings.ATRPeriod == 0:
		return errors.New("settings.atr_period must be positive")
	case c.Settings.PivotSpan == 0:
		return errors.New("settings.pivot_span must be positive")
	case c.Settings.ReversalPct == "":
		return errors.New("settings.reversal_pct is required")
	case c.Settings.ReversalATRPeriod == 0:
		return errors.New("settings.reversal_atr_period must be positive")
	case c.Settings.ReversalATRMultiplier == "":
		return errors.New("settings.reversal_atr_multiplier is required")
	case c.Settings.EqualityTolerancePct == "":
		return errors.New("settings.equality_tolerance_pct is required")
	case c.Settings.LevelATRPeriod == 0:
		return errors.New("settings.level_atr_period must be positive")
	case c.Settings.ZoneWidthATR == "":
		return errors.New("settings.zone_width_atr is required")
	case c.Settings.MinTouches < 2:
		return errors.New("settings.min_touches must be at least 2")
	case c.Settings.MinTouchSeparationBars == 0:
		return errors.New("settings.min_touch_separation_bars must be positive")
	case runtimeTimeoutErr != nil || runtimeTimeout <= 0:
		return errors.New("runtime_settings.request_timeout must be a positive duration")
	case c.RuntimeSettings.MaxRequestBytes <= 0:
		return errors.New("runtime_settings.max_request_bytes must be positive")
	case c.RuntimeSettings.MarketDataMaxResponseBytes <= 0:
		return errors.New("runtime_settings.market_data_max_response_bytes must be positive")
	case c.RuntimeSettings.MaxResponseBytes <= 0:
		return errors.New("runtime_settings.max_response_bytes must be positive")
	}

	return nil
}
