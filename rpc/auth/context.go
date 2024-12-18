package auth

import (
	"context"

	"google.golang.org/grpc"
)

type (
	authTokenContextKey        void
	authTokenClaimsContextKey  void
	authCapabilitiesContextKey void
)

const (
	AuthTokenMetadataKey = "authorization"
)

var (
	AuthTokenContextKey        authTokenContextKey
	AuthTokenClaimsContextKey  authTokenClaimsContextKey
	AuthCapabilitiesContextKey authCapabilitiesContextKey
)

type streamWithCtx struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *streamWithCtx) Context() context.Context {
	return w.ctx
}
