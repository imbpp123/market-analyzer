package application

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/imbpp123/market-analyzer/internal/domain"
	"github.com/shopspring/decimal"
)

const defaultActiveNATRPeriod uint32 = 14
const maxActiveNATRPeriod uint32 = 999

type activeFilters struct {
	minVolume24h *decimal.Decimal
	minTrades24h *int64
	minNATR      *decimal.Decimal
	natrPeriod   uint32
}

func (f activeFilters) needsMarketStats() bool {
	return f.minVolume24h != nil || f.minTrades24h != nil
}

func (a *Analyzer) FindActiveInstruments(ctx context.Context, request FindActiveInstrumentsRequest) (FindActiveInstrumentsResponse, error) {
	if a.instruments == nil {
		return FindActiveInstrumentsResponse{}, &Error{
			Kind: InternalError,
			Err:  errors.New("instrument reader is required"),
		}
	}

	filters, err := validateActiveRequest(request)
	if err != nil {
		return FindActiveInstrumentsResponse{}, err
	}

	requestCtx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	evaluatedAt := a.clock.Now().UTC()

	instruments, err := a.instruments.ReadInstruments(requestCtx, request.Exchange, request.Market)
	if err != nil {
		return FindActiveInstrumentsResponse{}, normalizeError(err)
	}
	if err := requestCtx.Err(); err != nil {
		return FindActiveInstrumentsResponse{}, calculationError(err)
	}

	statsBySymbol, err := a.readActiveStats(requestCtx, request, filters)
	if err != nil {
		return FindActiveInstrumentsResponse{}, err
	}

	result := make([]domain.ActiveInstrument, 0)
	seen := make(map[string]struct{}, len(instruments))
	for _, instrument := range instruments {
		if err := requestCtx.Err(); err != nil {
			return FindActiveInstrumentsResponse{}, calculationError(err)
		}
		if err := validateActiveInstrument(instrument, request, seen); err != nil {
			return FindActiveInstrumentsResponse{}, err
		}

		active, matched, err := a.evaluateActiveInstrument(requestCtx, instrument, statsBySymbol, filters, evaluatedAt)
		if err != nil {
			return FindActiveInstrumentsResponse{}, err
		}
		if matched {
			result = append(result, active)
		}
	}

	sort.Slice(result, func(i, j int) bool { return result[i].Symbol < result[j].Symbol })
	return FindActiveInstrumentsResponse{Instruments: result}, nil
}

func (a *Analyzer) readActiveStats(ctx context.Context, request FindActiveInstrumentsRequest, filters activeFilters) (map[string]domain.MarketStats, error) {
	statsBySymbol := map[string]domain.MarketStats{}
	if !filters.needsMarketStats() {
		return statsBySymbol, nil
	}

	stats, err := a.instruments.ReadMarketStats(ctx, request.Exchange, request.Market)
	if err != nil {
		return nil, normalizeError(err)
	}
	if err := ctx.Err(); err != nil {
		return nil, calculationError(err)
	}

	for _, stat := range stats {
		if err := stat.Validate(); err != nil {
			return nil, &Error{
				Kind: InvalidMarketData,
				Err:  fmt.Errorf("invalid market stats: %w", err),
			}
		}

		if stat.Exchange != request.Exchange || stat.Market != request.Market {
			return nil, invalidActiveSource("market stats identity does not match request")
		}
		if _, exists := statsBySymbol[stat.Symbol]; exists {
			return nil, invalidActiveSource("duplicate market stats symbol")
		}
		stat.TradeCount = cloneInt64(stat.TradeCount)
		statsBySymbol[stat.Symbol] = stat
	}

	return statsBySymbol, nil
}

func (a *Analyzer) evaluateActiveInstrument(ctx context.Context, instrument domain.Instrument, statsBySymbol map[string]domain.MarketStats, filters activeFilters, evaluatedAt time.Time) (domain.ActiveInstrument, bool, error) {
	if !instrument.IsTrading() {
		return domain.ActiveInstrument{}, false, nil
	}

	active := domain.ActiveInstrument{Instrument: instrument}
	if filters.needsMarketStats() {
		stat, ok := statsBySymbol[instrument.Symbol]
		if !ok {
			return domain.ActiveInstrument{}, false, nil
		}
		volume, err := parseDecimal("market_stats.volume", stat.Volume)
		if err != nil {
			return domain.ActiveInstrument{}, false, invalidActiveSource("invalid market stats volume")
		}
		if filters.minVolume24h != nil && volume.LessThan(*filters.minVolume24h) {
			return domain.ActiveInstrument{}, false, nil
		}
		if filters.minTrades24h != nil && (stat.TradeCount == nil || *stat.TradeCount < *filters.minTrades24h) {
			return domain.ActiveInstrument{}, false, nil
		}
		active.MarketStats = &stat
	}

	if filters.minNATR != nil {
		selection := &Selection{
			Exchange:    instrument.Exchange,
			Market:      instrument.Market,
			Symbol:      instrument.Symbol,
			To:          evaluatedAt,
			CandleCount: filters.natrPeriod + 1,
			Interval:    "1d",
		}
		natr, err := a.GetNATR(ctx, NATRRequest{
			Selection: selection,
			Settings:  &ATRSettings{Period: filters.natrPeriod},
		})
		if err != nil {
			return domain.ActiveInstrument{}, false, fmt.Errorf("calculate NATR for %s: %w", instrument.Symbol, err)
		}
		if natr.Result.Value.LessThan(*filters.minNATR) {
			return domain.ActiveInstrument{}, false, nil
		}
		value := domain.FormatPercentage(natr.Result.Value)
		active.NATR = &value
		active.NATRValueTime = natr.Result.ValueTime
	}

	return active, true, nil
}

func validateActiveRequest(request FindActiveInstrumentsRequest) (activeFilters, error) {
	if err := domain.ValidateExchange(request.Exchange); err != nil {
		return activeFilters{}, domainValidationError(err, InvalidParameter)
	}
	if err := domain.ValidateMarket(request.Market); err != nil {
		return activeFilters{}, domainValidationError(err, InvalidParameter)
	}

	return parseActiveFilters(request)
}

func validateActiveInstrument(instrument domain.Instrument, request FindActiveInstrumentsRequest, seen map[string]struct{}) error {
	if instrument.Exchange != request.Exchange || instrument.Market != request.Market {
		return invalidActiveSource("instrument identity does not match request")
	}
	if err := domain.ValidateSymbol(instrument.Symbol); err != nil {
		return invalidActiveSource("invalid instrument symbol")
	}
	if _, exists := seen[instrument.Symbol]; exists {
		return invalidActiveSource("duplicate instrument symbol")
	}
	seen[instrument.Symbol] = struct{}{}

	return nil
}

func parseActiveFilters(request FindActiveInstrumentsRequest) (activeFilters, error) {
	var minVolume24h *decimal.Decimal
	if request.MinVolume24h != nil {
		value, err := parseDecimal("min_volume_24h", *request.MinVolume24h)
		if err != nil || value.IsNegative() {
			return activeFilters{}, invalidRequest("min_volume_24h", errors.New("must be a nonnegative decimal"))
		}
		minVolume24h = &value
	}

	if request.MinTrades24h != nil && *request.MinTrades24h < 0 {
		return activeFilters{}, invalidRequest("min_trades_24h", errors.New("must be nonnegative"))
	}
	minTrades24h := cloneInt64(request.MinTrades24h)

	var minNATR *decimal.Decimal
	if request.MinNATR != nil {
		value, err := parseDecimal("min_natr", *request.MinNATR)
		if err != nil || value.IsNegative() {
			return activeFilters{}, invalidRequest("min_natr", errors.New("must be a nonnegative decimal"))
		}
		minNATR = &value
	}

	natrPeriod := defaultActiveNATRPeriod
	if request.NATRPeriod != nil {
		if *request.NATRPeriod == 0 || *request.NATRPeriod > maxActiveNATRPeriod {
			return activeFilters{}, invalidRequest("natr_period", errors.New("must be between 1 and 999"))
		}

		if minNATR == nil {
			return activeFilters{}, invalidRequest("natr_period", errors.New("min_natr is required when natr_period is set"))
		}
		natrPeriod = *request.NATRPeriod
	}

	return activeFilters{
		minVolume24h: minVolume24h,
		minTrades24h: minTrades24h,
		minNATR:      minNATR,
		natrPeriod:   natrPeriod,
	}, nil
}

func invalidActiveSource(message string) *Error {
	return &Error{
		Kind: InvalidMarketData,
		Err:  errors.New(message),
	}
}
