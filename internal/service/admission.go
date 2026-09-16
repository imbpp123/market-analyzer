package service

import (
	"context"
	"sync"

	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Admission struct {
	mu        sync.Mutex
	accepting bool
}

func (a *Admission) Open() {
	a.mu.Lock()
	a.accepting = true
	a.mu.Unlock()
}

func (a *Admission) Close() {
	a.mu.Lock()
	a.accepting = false
	a.mu.Unlock()
}

func (a *Admission) Ready() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.accepting
}

func (a *Admission) UnaryServerInterceptor() grpcgo.UnaryServerInterceptor {
	return func(ctx context.Context, request any, info *grpcgo.UnaryServerInfo, handler grpcgo.UnaryHandler) (any, error) {
		if !a.Ready() {
			return nil, status.Error(codes.Unavailable, "service is not accepting analysis requests")
		}
		return handler(ctx, request)
	}
}
