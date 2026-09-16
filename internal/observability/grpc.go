package observability

import (
	"context"
	"log/slog"
	"strings"
	"time"

	marketanalyzerv1 "github.com/imbpp123/market-analyzer/api/go/marketanalyzer/v1"
	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func UnaryServerInterceptor(metrics *Registry, logger *slog.Logger) grpcgo.UnaryServerInterceptor {
	return func(ctx context.Context, request any, info *grpcgo.UnaryServerInfo, handler grpcgo.UnaryHandler) (any, error) {
		started := time.Now()
		metrics.RequestStarted()

		response, err := handler(ctx, request)
		responseBytes := -1
		if message, ok := response.(proto.Message); ok && message != nil {
			responseBytes = proto.Size(message)
		}
		code := status.Code(err).String()
		metrics.RequestFinished(methodName(info.FullMethod), code, time.Since(started), responseBytes)

		attributes := []any{"rpc", methodName(info.FullMethod), "duration_ms", time.Since(started).Milliseconds(), "status", code}
		if selection := requestSelection(request); selection != nil {
			attributes = append(attributes, "exchange", selection.GetExchange(), "market", selection.GetMarket(), "symbol", selection.GetSymbol(),
				"interval", selection.GetInterval(), "candle_count", selection.GetCandleCount())
			if selection.GetTo() != nil {
				attributes = append(attributes, "to", selection.GetTo().AsTime())
			}
		}
		if metadata := responseMetadata(response); metadata != nil {
			attributes = append(attributes, "source_from", metadata.GetSourceFrom().AsTime(), "source_to", metadata.GetSourceTo().AsTime(),
				"algorithm_ids", strings.Join(metadata.GetAlgorithmIds(), ","))
		}
		if reason := errorReason(err); reason != "" {
			attributes = append(attributes, "reason", reason)
		}
		logger.InfoContext(ctx, "analysis request completed", attributes...)

		return response, err
	}
}

type selectionRequest interface {
	GetSelection() *marketanalyzerv1.Selection
}

func requestSelection(request any) *marketanalyzerv1.Selection {
	value, ok := request.(selectionRequest)
	if !ok {
		return nil
	}
	return value.GetSelection()
}

type metadataResponse interface {
	GetMetadata() *marketanalyzerv1.Metadata
}

func responseMetadata(response any) *marketanalyzerv1.Metadata {
	value, ok := response.(metadataResponse)
	if !ok {
		return nil
	}
	return value.GetMetadata()
}

func errorReason(err error) string {
	if err == nil {
		return ""
	}
	for _, detail := range status.Convert(err).Details() {
		if value, ok := detail.(*marketanalyzerv1.ErrorDetail); ok {
			return value.GetReason()
		}
	}
	return ""
}

func methodName(fullMethod string) string {
	if index := strings.LastIndexByte(fullMethod, '/'); index >= 0 {
		return fullMethod[index+1:]
	}
	return fullMethod
}
