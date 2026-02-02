package rpc

import (
	"context"

	"google.golang.org/grpc"
)

type Transformer interface {
	Transform(req any)
}

type TransformerFunc func(req any)

func (f TransformerFunc) Transform(req any) {
	f(req)
}

type DefaultsTransformer struct{}

func (DefaultsTransformer) Transform(req any) {
	if defaultable, ok := req.(interface{ Default() }); ok {
		defaultable.Default()
	}
}

func TransformUnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return UnaryServerInterceptorWithTransformer(DefaultsTransformer{})
}

func UnaryServerInterceptorWithTransformer(transformer Transformer) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		transformer.Transform(req)
		return handler(ctx, req)
	}
}

func TransformStreamServerInterceptor() grpc.StreamServerInterceptor {
	return StreamServerInterceptorWithTransformer(DefaultsTransformer{})
}

func StreamServerInterceptorWithTransformer(transformer Transformer) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		wrapper := &transformStreamWrapper{
			ServerStream: ss,
			transformer:  transformer,
		}
		return handler(srv, wrapper)
	}
}

type transformStreamWrapper struct {
	grpc.ServerStream
	transformer Transformer
}

func (s *transformStreamWrapper) RecvMsg(m any) error {
	if err := s.ServerStream.RecvMsg(m); err != nil {
		return err
	}

	s.transformer.Transform(m)

	return nil
}
