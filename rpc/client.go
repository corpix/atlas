package rpc

import (
	"context"
	"fmt"
	"time"

	grpclog "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"

	"git.tatikoma.dev/corpix/atlas/log"
	"git.tatikoma.dev/corpix/atlas/rpc/auth"
)

func NewClientConn(a *auth.Auth, l log.Logger, host string, port int) (*grpc.ClientConn, error) {
	return grpc.NewClient(
		fmt.Sprintf("%s:%d", host, port),
		a.GRPC().DialOption(),
		grpc.WithDisableServiceConfig(),
		grpc.WithChainUnaryInterceptor(grpclog.UnaryClientInterceptor(
			LoggerInterceptor(l),
			grpclog.WithLogOnEvents(grpclog.StartCall, grpclog.FinishCall),
		)),
		grpc.WithDefaultCallOptions(grpc.WaitForReady(true)),
		grpc.WithConnectParams(grpc.ConnectParams{
			Backoff: backoff.Config{
				BaseDelay:  1 * time.Second,
				Multiplier: 1.5,
				Jitter:     0.2,
				MaxDelay:   10 * time.Second,
			},
			MinConnectTimeout: 20 * time.Second,
		}),
	)
}

// TimeoutUnaryClientInterceptor bounds every unary call which does not already
// carry a tighter deadline, so an unresponsive server never pins a caller.
func TimeoutUnaryClientInterceptor(timeout time.Duration) grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply any,
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		deadline, ok := ctx.Deadline()
		if ok && time.Until(deadline) <= timeout {
			return invoker(ctx, method, req, reply, cc, opts...)
		}
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}
