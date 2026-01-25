package auth

import (
	"context"

	"google.golang.org/grpc"
)

type (
	tokenContextKey       void
	tokenClaimsContextKey void
)

const (
	TokenMetadataKey = "authorization"
)

var (
	TokenContextKey       tokenContextKey
	TokenClaimsContextKey tokenClaimsContextKey
)

type streamWithCtx struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *streamWithCtx) Context() context.Context {
	return w.ctx
}
