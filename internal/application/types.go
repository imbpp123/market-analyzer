package application

import (
	"context"
	"fmt"
	"time"

	"github.com/imbpp123/market-analyzer/internal/domain"
)

const DefaultRequestTimeout = 30 * time.Second

type Clock interface {
	Now() time.Time
}

type CandleReader interface {
	ReadCandles(context.Context, domain.Instrument, domain.Interval, domain.CandleRange) (SourceSeries, error)
}

type SourceSeries struct {
	Instrument domain.Instrument
	Interval   domain.Interval
	Range      domain.CandleRange
	Candles    []SourceCandle
}

type SourceCandle struct {
	OpenTime    time.Time
	CloseTime   time.Time
	Open        string
	High        string
	Low         string
	Close       string
	Volume      string
	Turnover    string
	TradesCount *int64
	FetchedAt   time.Time
}

type Selection struct {
	Exchange    string
	Market      string
	Symbol      string
	To          time.Time
	CandleCount uint32
	Interval    string
}

type ATRSettings struct {
	Period uint32
}

type ExtremaMethod interface{ applicationExtremaMethod() }

type LocalExtremaSettings struct{ PivotSpan uint32 }

func (LocalExtremaSettings) applicationExtremaMethod() {}

type PercentReversalSettings struct{ ReversalPct string }

func (PercentReversalSettings) applicationExtremaMethod() {}

type ATRReversalSettings struct {
	ATRPeriod     uint32
	ATRMultiplier string
}

func (ATRReversalSettings) applicationExtremaMethod() {}

type ExtremaSettings struct {
	PriceSource domain.PriceSource
	Method      ExtremaMethod
}

type TrendSettings struct {
	Extrema              *ExtremaSettings
	EqualityTolerancePct string
}

type LevelSettings struct {
	Extrema                *ExtremaSettings
	ATRPeriod              uint32
	ZoneWidthATR           string
	MinTouches             uint32
	MinTouchSeparationBars uint32
}

type ATRRequest struct {
	Selection *Selection
	Settings  *ATRSettings
}

type NATRRequest = ATRRequest

type ExtremaRequest struct {
	Selection *Selection
	Settings  *ExtremaSettings
}

type TrendRequest struct {
	Selection *Selection
	Settings  *TrendSettings
}

type LevelsRequest struct {
	Selection *Selection
	Settings  *LevelSettings
}

type Metadata struct {
	Selection     Selection
	EvaluatedAt   time.Time
	SourceFrom    time.Time
	SourceTo      time.Time
	Algorithms    []string
	NumericPolicy string
}

type ATRResponse struct {
	Metadata Metadata
	Candles  []SourceCandle
	Settings ATRSettings
	Result   domain.ATRResult
}

type NATRResponse struct {
	Metadata Metadata
	Candles  []SourceCandle
	Settings ATRSettings
	Result   domain.NATRResult
}

type ExtremaResponse struct {
	Metadata Metadata
	Candles  []SourceCandle
	Settings ExtremaSettings
	Result   domain.ExtremaResult
}

type TrendResponse struct {
	Metadata Metadata
	Candles  []SourceCandle
	Settings TrendSettings
	Result   domain.TrendResult
}

type LevelsResponse struct {
	Metadata Metadata
	Candles  []SourceCandle
	Settings LevelSettings
	Result   domain.LevelsResult
}

type ErrorKind string

const (
	InvalidParameter            ErrorKind = "invalid_parameter"
	MarketDataRejectedRequest   ErrorKind = "market_data_rejected_request"
	SymbolNotFound              ErrorKind = "symbol_not_found"
	IncompleteData              ErrorKind = "incomplete_data"
	InvalidMarketData           ErrorKind = "invalid_market_data"
	MarketDataUnavailable       ErrorKind = "market_data_unavailable"
	MarketDataResourceExhausted ErrorKind = "market_data_resource_exhausted"
	ResponseTooLarge            ErrorKind = "response_too_large"
	RequestCanceled             ErrorKind = "request_canceled"
	RequestTimeout              ErrorKind = "request_timeout"
	MarketDataContractMismatch  ErrorKind = "market_data_contract_mismatch"
	MarketDataFailure           ErrorKind = "market_data_failure"
	InternalError               ErrorKind = "internal_error"
)

type Error struct {
	Kind           ErrorKind
	Field          string
	UpstreamCode   string
	UpstreamReason string
	Err            error
}

func (e *Error) Error() string {
	if e.Err == nil {
		return string(e.Kind)
	}

	return fmt.Sprintf("%s: %v", e.Kind, e.Err)
}

func (e *Error) Unwrap() error { return e.Err }

type parsedExtremaSettings struct {
	application ExtremaSettings
	domain      domain.ExtremaSettings
}

type parsedTrendSettings struct {
	application TrendSettings
	domain      domain.TrendSettings
}

type parsedLevelSettings struct {
	application LevelSettings
	domain      domain.LevelSettings
}
