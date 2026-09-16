package marketdata

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/imbpp123/market-analyzer/internal/application"
	"github.com/imbpp123/market-analyzer/internal/domain"
	marketdatav1 "github.com/imbpp123/market-data/api/go/marketdata/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const DefaultMaxResponseBytes = 16 << 20

type Reader struct {
	client marketdatav1.MarketDataServiceClient
}

func NewReader(client marketdatav1.MarketDataServiceClient) (*Reader, error) {
	if client == nil {
		return nil, errors.New("market data client is required")
	}

	return &Reader{client: client}, nil
}

func (r *Reader) ReadCandles(ctx context.Context, instrument domain.Instrument, interval domain.Interval, candleRange domain.CandleRange) (application.SourceSeries, error) {
	request := &marketdatav1.GetKlinesRequest{
		Exchange: pointer(instrument.Exchange),
		Market:   pointer(instrument.Market),
		Symbol:   pointer(instrument.Symbol),
		Interval: pointer(string(interval)),
		From:     timestamppb.New(candleRange.From),
		To:       timestamppb.New(candleRange.To),
	}

	response, err := r.client.GetKlines(ctx, request)
	if err != nil {
		return application.SourceSeries{}, mapError(err)
	}

	candles := make([]application.SourceCandle, len(response.GetKlines()))
	for index, kline := range response.GetKlines() {
		if kline == nil {
			candles[index] = application.SourceCandle{}
			continue
		}

		candles[index] = application.SourceCandle{
			OpenTime:    timestamp(kline.GetOpenTime()),
			CloseTime:   timestamp(kline.GetCloseTime()),
			Open:        kline.GetOpen(),
			High:        kline.GetHigh(),
			Low:         kline.GetLow(),
			Close:       kline.GetClose(),
			Volume:      kline.GetVolume(),
			Turnover:    kline.GetTurnover(),
			TradesCount: cloneInt64(kline.TradesCount),
			FetchedAt:   timestamp(kline.GetFetchedAt()),
		}
	}

	return application.SourceSeries{
		Instrument: domain.Instrument{Exchange: response.GetExchange(), Market: response.GetMarket(), Symbol: response.GetSymbol()},
		Interval:   domain.Interval(response.GetInterval()),
		Range:      candleRange,
		Candles:    candles,
	}, nil
}

func mapError(err error) error {
	grpcStatus := status.Convert(err)
	reason := upstreamReason(grpcStatus)

	kind := application.MarketDataFailure
	switch grpcStatus.Code() {
	case codes.InvalidArgument:
		kind = application.MarketDataRejectedRequest
	case codes.NotFound:
		kind = application.SymbolNotFound
	case codes.FailedPrecondition:
		kind = application.IncompleteData
	case codes.DataLoss:
		kind = application.InvalidMarketData
	case codes.Unavailable:
		kind = application.MarketDataUnavailable
	case codes.ResourceExhausted:
		kind = application.MarketDataResourceExhausted
	case codes.Canceled:
		kind = application.RequestCanceled
	case codes.DeadlineExceeded:
		kind = application.RequestTimeout
	case codes.Unimplemented:
		kind = application.MarketDataContractMismatch
	}

	return &application.Error{
		Kind:           kind,
		UpstreamCode:   grpcStatus.Code().String(),
		UpstreamReason: reason,
		Err:            fmt.Errorf("market data GetKlines: %w", err),
	}
}

func upstreamReason(grpcStatus *status.Status) string {
	for _, detail := range grpcStatus.Details() {
		if errorDetail, ok := detail.(*marketdatav1.ErrorDetail); ok {
			return errorDetail.GetReason()
		}
	}

	return ""
}

func timestamp(value *timestamppb.Timestamp) time.Time {
	if value == nil {
		return time.Time{}
	}

	return value.AsTime()
}

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}

	result := *value
	return &result
}

func pointer[T any](value T) *T { return &value }
