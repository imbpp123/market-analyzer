package grpc

import (
	"context"
	"testing"
	"time"

	marketanalyzerv1 "github.com/imbpp123/market-analyzer/api/go/marketanalyzer/v1"
	"github.com/imbpp123/market-analyzer/internal/application"
	"github.com/imbpp123/market-analyzer/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type transportInstruments struct{}

func (transportInstruments) ReadInstruments(context.Context, string, string) ([]domain.Instrument, error) {
	return []domain.Instrument{{
		Exchange:   "binance",
		Market:     "spot",
		Symbol:     "BTCUSDT",
		BaseAsset:  "BTC",
		QuoteAsset: "USDT",
		Status:     domain.InstrumentStatusTrading,
	}}, nil
}

func (transportInstruments) ReadMarketStats(context.Context, string, string) ([]domain.MarketStats, error) {
	return nil, nil
}

func TestFindActiveInstrumentsGRPCRoundTrip(t *testing.T) {
	client := startClientWithInstruments(t, &transportReader{read: transportSource}, transportInstruments{}, DefaultMaxResponseBytes, 64<<10)

	response, err := client.FindActiveInstruments(t.Context(), &marketanalyzerv1.FindActiveInstrumentsRequest{
		Exchange: pointer("binance"), Market: pointer("spot"),
	})

	require.NoError(t, err)
	require.Len(t, response.Instruments, 1)
	assert.Equal(t, "BTCUSDT", response.Instruments[0].Symbol)
	assert.Equal(t, "BTC", response.Instruments[0].BaseAsset)
	assert.Equal(t, "USDT", response.Instruments[0].QuoteAsset)
	assert.Nil(t, response.Instruments[0].Volume_24H)
	assert.Nil(t, response.Instruments[0].Trades_24H)
	assert.Nil(t, response.Instruments[0].StatsFetchedAt)
}

func TestFindActiveInstrumentsGRPCRejectsMissingScope(t *testing.T) {
	client := startClientWithInstruments(t, &transportReader{read: transportSource}, transportInstruments{}, DefaultMaxResponseBytes, 64<<10)

	_, err := client.FindActiveInstruments(t.Context(), &marketanalyzerv1.FindActiveInstrumentsRequest{Exchange: pointer("binance")})

	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestFindActiveInstrumentsMappingPreservesOptionalThresholdsAndEvidence(t *testing.T) {
	request := &marketanalyzerv1.FindActiveInstrumentsRequest{Exchange: pointer("binance"), Market: pointer("spot"),
		MinVolume_24H: pointer("100.00"), MinTrades_24H: pointer(int64(0)), MinNatr: pointer("4"), NatrPeriod: pointer(uint32(2))}

	input, err := mapFindActiveInstrumentsRequest(request)

	require.NoError(t, err)
	assert.Equal(t, "100.00", *input.MinVolume24h)
	assert.Equal(t, int64(0), *input.MinTrades24h)
	assert.Equal(t, "4", *input.MinNATR)
	assert.Equal(t, uint32(2), *input.NATRPeriod)

	now := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	output := mapFindActiveInstrumentsResponse(application.FindActiveInstrumentsResponse{
		Instruments: []domain.ActiveInstrument{{
			Instrument: domain.Instrument{
				Exchange:   "binance",
				Market:     "spot",
				Symbol:     "BTCUSDT",
				BaseAsset:  "BTC",
				QuoteAsset: "USDT",
			},
			MarketStats: &domain.MarketStats{
				Exchange:   "binance",
				Market:     "spot",
				Symbol:     "BTCUSDT",
				Volume:     "100.00",
				TradeCount: pointer(int64(0)),
				FetchedAt:  now,
			},
			NATR:          pointer("4"),
			NATRValueTime: now,
		}},
	})

	encoded, err := proto.Marshal(output)
	require.NoError(t, err)
	decoded := &marketanalyzerv1.FindActiveInstrumentsResponse{}
	require.NoError(t, proto.Unmarshal(encoded, decoded))
	require.Len(t, decoded.Instruments, 1)
	assert.Equal(t, "BTCUSDT", decoded.Instruments[0].Symbol)
	assert.Equal(t, "100.00", decoded.Instruments[0].GetVolume_24H())
	assert.Equal(t, int64(0), decoded.Instruments[0].GetTrades_24H())
	assert.Equal(t, "4", decoded.Instruments[0].GetNatr())
	assert.Equal(t, now, decoded.Instruments[0].GetStatsFetchedAt().AsTime())
	assert.Equal(t, now, decoded.Instruments[0].GetNatrValueTime().AsTime())
}

func TestFindActiveInstrumentsMappingRejectsMissingScope(t *testing.T) {
	_, err := mapFindActiveInstrumentsRequest(&marketanalyzerv1.FindActiveInstrumentsRequest{Exchange: pointer("binance")})
	require.Error(t, err)
	assert.Equal(t, "request", err.(*application.Error).Field)
}
