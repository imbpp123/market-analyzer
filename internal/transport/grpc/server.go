package grpc

import (
	"context"
	"errors"

	marketanalyzerv1 "github.com/imbpp123/market-analyzer/api/go/marketanalyzer/v1"
	"github.com/imbpp123/market-analyzer/internal/application"
	"google.golang.org/protobuf/proto"
)

const DefaultMaxResponseBytes = 32 << 20

type Server struct {
	marketanalyzerv1.UnimplementedMarketAnalyzerServiceServer
	analyzer         *application.Analyzer
	maxResponseBytes int
}

func NewServer(analyzer *application.Analyzer, maxResponseBytes int) (*Server, error) {
	if analyzer == nil {
		return nil, errors.New("analyzer is required")
	}

	if maxResponseBytes <= 0 {
		return nil, errors.New("max response bytes must be positive")
	}

	return &Server{analyzer: analyzer, maxResponseBytes: maxResponseBytes}, nil
}

func (s *Server) GetATR(ctx context.Context, request *marketanalyzerv1.GetATRRequest) (*marketanalyzerv1.GetATRResponse, error) {
	input, err := mapATRRequest(request)
	if err != nil {
		return nil, publicError(err)
	}

	result, err := s.analyzer.GetATR(ctx, input)
	if err != nil {
		return nil, publicError(err)
	}

	response, err := mapATRResponse(result)
	if err != nil {
		return nil, publicError(err)
	}

	return checkSize(response, s.maxResponseBytes)
}

func (s *Server) GetNATR(ctx context.Context, request *marketanalyzerv1.GetNATRRequest) (*marketanalyzerv1.GetNATRResponse, error) {
	input, err := mapNATRRequest(request)
	if err != nil {
		return nil, publicError(err)
	}

	result, err := s.analyzer.GetNATR(ctx, input)
	if err != nil {
		return nil, publicError(err)
	}

	response, err := mapNATRResponse(result)
	if err != nil {
		return nil, publicError(err)
	}

	return checkSize(response, s.maxResponseBytes)
}

func (s *Server) GetExtrema(ctx context.Context, request *marketanalyzerv1.GetExtremaRequest) (*marketanalyzerv1.GetExtremaResponse, error) {
	input, err := mapExtremaRequest(request)
	if err != nil {
		return nil, publicError(err)
	}

	result, err := s.analyzer.GetExtrema(ctx, input)
	if err != nil {
		return nil, publicError(err)
	}

	response, err := mapExtremaResponse(result)
	if err != nil {
		return nil, publicError(err)
	}

	return checkSize(response, s.maxResponseBytes)
}

func (s *Server) GetTrend(ctx context.Context, request *marketanalyzerv1.GetTrendRequest) (*marketanalyzerv1.GetTrendResponse, error) {
	input, err := mapTrendRequest(request)
	if err != nil {
		return nil, publicError(err)
	}

	result, err := s.analyzer.GetTrend(ctx, input)
	if err != nil {
		return nil, publicError(err)
	}

	response, err := mapTrendResponse(result)
	if err != nil {
		return nil, publicError(err)
	}

	return checkSize(response, s.maxResponseBytes)
}

func (s *Server) GetLevels(ctx context.Context, request *marketanalyzerv1.GetLevelsRequest) (*marketanalyzerv1.GetLevelsResponse, error) {
	input, err := mapLevelsRequest(request)
	if err != nil {
		return nil, publicError(err)
	}

	result, err := s.analyzer.GetLevels(ctx, input)
	if err != nil {
		return nil, publicError(err)
	}

	response, err := mapLevelsResponse(result)
	if err != nil {
		return nil, publicError(err)
	}

	return checkSize(response, s.maxResponseBytes)
}

func checkSize[T proto.Message](response T, limit int) (T, error) {
	if proto.Size(response) > limit {
		var zero T
		return zero, publicError(&application.Error{Kind: application.ResponseTooLarge, Err: errors.New("serialized response exceeds configured limit")})
	}

	return response, nil
}
