package rpc

import (
	"context"

	"google.golang.org/grpc"
)

type validator interface {
	Validate() error
}

type ValidationError struct {
	Field   string
	Message string
}

func ValidateRequest(req interface{}) error {
	if v, ok := req.(validator); ok {
		err := v.Validate()
		if err != nil {
			return err
		}
	}
	return nil
}

func ValidationUnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if err := ValidateRequest(req); err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
}

func ValidationStreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		wrapper := &validationStreamWrapper{
			ServerStream: ss,
		}
		return handler(srv, wrapper)
	}
}

type validationStreamWrapper struct {
	grpc.ServerStream
}

func (s *validationStreamWrapper) RecvMsg(m interface{}) error {
	if err := s.ServerStream.RecvMsg(m); err != nil {
		return err
	}

	if err := ValidateRequest(m); err != nil {
		return err
	}

	return nil
}
