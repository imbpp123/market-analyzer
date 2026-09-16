package grpc

import (
	"errors"

	marketanalyzerv1 "github.com/imbpp123/market-analyzer/api/go/marketanalyzer/v1"
	"github.com/imbpp123/market-analyzer/internal/application"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func publicError(err error) error {
	var applicationError *application.Error
	if !errors.As(err, &applicationError) {
		applicationError = &application.Error{Kind: application.InternalError, Err: err}
	}

	base := status.New(codeFor(applicationError.Kind), applicationError.Error())
	detail := &marketanalyzerv1.ErrorDetail{Reason: string(applicationError.Kind)}
	if applicationError.Field != "" {
		detail.Field = &applicationError.Field
	}
	if applicationError.UpstreamCode != "" {
		detail.UpstreamCode = &applicationError.UpstreamCode
	}
	if applicationError.UpstreamReason != "" {
		detail.UpstreamReason = &applicationError.UpstreamReason
	}

	withDetail, detailError := base.WithDetails(detail)
	if detailError != nil {
		return base.Err()
	}

	return withDetail.Err()
}

func codeFor(kind application.ErrorKind) codes.Code {
	switch kind {
	case application.InvalidParameter, application.MarketDataRejectedRequest:
		return codes.InvalidArgument
	case application.SymbolNotFound:
		return codes.NotFound
	case application.IncompleteData:
		return codes.FailedPrecondition
	case application.InvalidMarketData:
		return codes.DataLoss
	case application.MarketDataUnavailable:
		return codes.Unavailable
	case application.MarketDataResourceExhausted, application.ResponseTooLarge:
		return codes.ResourceExhausted
	case application.RequestCanceled:
		return codes.Canceled
	case application.RequestTimeout:
		return codes.DeadlineExceeded
	case application.MarketDataContractMismatch:
		return codes.Unimplemented
	default:
		return codes.Internal
	}
}
