package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	marketanalyzerv1 "github.com/imbpp123/market-analyzer/api/go/marketanalyzer/v1"
	"github.com/imbpp123/market-analyzer/internal/application"
	marketdataadapter "github.com/imbpp123/market-analyzer/internal/infrastructure/marketdata"
	"github.com/imbpp123/market-analyzer/internal/observability"
	grpctransport "github.com/imbpp123/market-analyzer/internal/transport/grpc"
	"github.com/imbpp123/market-analyzer/internal/transport/httpops"
	marketdatav1 "github.com/imbpp123/market-data/api/go/marketdata/v1"
	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type App struct {
	config Config
	logger *slog.Logger

	grpcListener net.Listener
	httpListener net.Listener
	grpcServer   *grpcgo.Server
	httpServer   *http.Server
	connection   *grpcgo.ClientConn
	admission    *Admission
	errors       chan error

	startOnce    sync.Once
	shutdownOnce sync.Once
	shutdownErr  error
}

func New(config Config, logger *slog.Logger) (*App, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("validate configuration: %w", err)
	}
	if logger == nil {
		return nil, errors.New("logger is required")
	}

	grpcListener, err := net.Listen("tcp", config.GRPCAddress)
	if err != nil {
		return nil, fmt.Errorf("listen for gRPC on %q: %w", config.GRPCAddress, err)
	}
	fail := func(err error) (*App, error) {
		_ = grpcListener.Close()
		return nil, err
	}

	httpListener, err := net.Listen("tcp", config.HTTPAddress)
	if err != nil {
		return fail(fmt.Errorf("listen for HTTP on %q: %w", config.HTTPAddress, err))
	}
	failBoth := func(err error) (*App, error) {
		_ = httpListener.Close()
		return fail(err)
	}

	connection, err := grpcgo.NewClient(config.MarketDataEndpoint,
		grpcgo.WithTransportCredentials(insecure.NewCredentials()),
		grpcgo.WithDefaultCallOptions(grpcgo.MaxCallRecvMsgSize(config.MarketDataMaxResponseBytes)))
	if err != nil {
		return failBoth(fmt.Errorf("create Market Data client: %w", err))
	}
	failAll := func(err error) (*App, error) {
		_ = connection.Close()
		return failBoth(err)
	}

	reader, err := marketdataadapter.NewReader(marketdatav1.NewMarketDataServiceClient(connection))
	if err != nil {
		return failAll(fmt.Errorf("create Market Data reader: %w", err))
	}
	metrics := observability.NewRegistry()
	measuredReader := observability.NewReader(reader, metrics)
	analyzer, err := application.NewAnalyzerWithObserver(measuredReader, systemClock{}, config.RequestTimeout, metrics)
	if err != nil {
		return failAll(fmt.Errorf("create analyzer: %w", err))
	}
	service, err := grpctransport.NewServer(analyzer, config.MaxResponseBytes)
	if err != nil {
		return failAll(fmt.Errorf("create gRPC service: %w", err))
	}

	admission := &Admission{}
	grpcServer := grpcgo.NewServer(
		grpcgo.MaxRecvMsgSize(config.MaxRequestBytes),
		grpcgo.MaxSendMsgSize(config.MaxResponseBytes),
		grpcgo.ChainUnaryInterceptor(observability.UnaryServerInterceptor(metrics, logger), admission.UnaryServerInterceptor()),
	)
	marketanalyzerv1.RegisterMarketAnalyzerServiceServer(grpcServer, service)
	httpServer := &http.Server{
		Handler:           httpops.NewHandler(admission.Ready, metrics),
		ReadHeaderTimeout: 5 * time.Second,
	}

	return &App{config: config, logger: logger, grpcListener: grpcListener, httpListener: httpListener,
		grpcServer: grpcServer, httpServer: httpServer, connection: connection, admission: admission, errors: make(chan error, 2)}, nil
}

func (a *App) Start() {
	a.startOnce.Do(func() {
		go func() {
			if err := a.grpcServer.Serve(a.grpcListener); err != nil {
				a.errors <- fmt.Errorf("serve gRPC: %w", err)
			}
		}()
		go func() {
			if err := a.httpServer.Serve(a.httpListener); err != nil && !errors.Is(err, http.ErrServerClosed) {
				a.errors <- fmt.Errorf("serve HTTP: %w", err)
			}
		}()
		a.admission.Open()
	})
}

func (a *App) Errors() <-chan error { return a.errors }

func (a *App) GRPCAddress() string { return a.grpcListener.Addr().String() }

func (a *App) HTTPAddress() string { return a.httpListener.Addr().String() }

func (a *App) Shutdown(ctx context.Context) error {
	a.shutdownOnce.Do(func() {
		a.admission.Close()

		grpcDone := make(chan struct{})
		go func() {
			a.grpcServer.GracefulStop()
			close(grpcDone)
		}()

		httpDone := make(chan error, 1)
		go func() { httpDone <- a.httpServer.Shutdown(ctx) }()

		select {
		case <-grpcDone:
		case <-ctx.Done():
			a.grpcServer.Stop()
			<-grpcDone
			a.shutdownErr = ctx.Err()
		}

		if err := <-httpDone; err != nil && a.shutdownErr == nil {
			a.shutdownErr = err
		}
		if err := a.connection.Close(); err != nil && a.shutdownErr == nil {
			a.shutdownErr = fmt.Errorf("close Market Data connection: %w", err)
		}
	})
	return a.shutdownErr
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }
