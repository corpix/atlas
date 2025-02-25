package rpc

import (
	"context"

	"google.golang.org/grpc"
)

type defaults interface {
	Default()
}

func DefaultsUnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if defaultable, ok := req.(defaults); ok {
			defaultable.Default()
		}
		return handler(ctx, req)
	}
}

func DefaultsStreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		wrapper := &defaultsStreamWrapper{
			ServerStream: ss,
		}
		return handler(srv, wrapper)
	}
}

type defaultsStreamWrapper struct {
	grpc.ServerStream
}

func (s *defaultsStreamWrapper) RecvMsg(m interface{}) error {
	if err := s.ServerStream.RecvMsg(m); err != nil {
		return err
	}

	if defaultable, ok := m.(defaults); ok {
		defaultable.Default()
	}

	return nil
}
