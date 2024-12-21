package auth

import (
	"context"

	"google.golang.org/grpc"
)

type (
	authTokenContextKey       void
	authTokenClaimsContextKey void
)

const (
	AuthTokenMetadataKey = "authorization"
)

var (
	AuthTokenContextKey       authTokenContextKey
	AuthTokenClaimsContextKey authTokenClaimsContextKey
)

type streamWithCtx struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *streamWithCtx) Context() context.Context {
	return w.ctx
}
