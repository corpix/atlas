package rpc

import (
	"context"

	"google.golang.org/grpc"
)

type Validator interface {
	Validate(req any) error
}

type ValidatorFunc func(req any) error

func (f ValidatorFunc) Validate(req any) error {
	return f(req)
}

type ValidatorMethod struct{}

func (ValidatorMethod) Validate(req any) error {
	if v, ok := req.(interface{ Validate() error }); ok {
		return v.Validate()
	}
	return nil
}

type ValidationError struct {
	Field   string
	Message string
}

func ValidateRequest(req any) error {
	return ValidateRequestWithValidator(ValidatorMethod{}, req)
}

func ValidateRequestWithValidator(v Validator, req any) error {
	err := v.Validate(req)
	if err != nil {
		return err
	}
	return nil
}

func ValidationUnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return UnaryServerInterceptorWithValidator(ValidatorMethod{})
}

func UnaryServerInterceptorWithValidator(v Validator) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if err := ValidateRequestWithValidator(v, req); err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
}

func ValidationStreamServerInterceptor() grpc.StreamServerInterceptor {
	return StreamServerInterceptorWithValidator(ValidatorMethod{})
}

func StreamServerInterceptorWithValidator(v Validator) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		wrapper := &validationStreamWrapper{
			ServerStream: ss,
			validator:    v,
		}
		return handler(srv, wrapper)
	}
}

type validationStreamWrapper struct {
	grpc.ServerStream
	validator Validator
}

func (s *validationStreamWrapper) RecvMsg(m any) error {
	if err := s.ServerStream.RecvMsg(m); err != nil {
		return err
	}

	if err := ValidateRequestWithValidator(s.validator, m); err != nil {
		return err
	}

	return nil
}
