package rpc

import (
	"context"
	"fmt"
	"strings"

	protovalidate "github.com/bufbuild/protovalidate-go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"

	"git.tatikoma.dev/corpix/atlas/errors"
	atlasrpc "git.tatikoma.dev/corpix/atlas/rpc/pb"
)

type Validator interface {
	Validate(req any) error
}

type ValidatorFunc func(req any) error

func (f ValidatorFunc) Validate(req any) error {
	return f(req)
}

// Deprecated: use protovalidate annotations instead.
type ValidatorMethod interface {
	Validate() error
}

type validator struct{}

func (validator) Validate(req any) error {
	if v, ok := req.(ValidatorMethod); ok {
		return v.Validate()
	}
	msg, ok := req.(proto.Message)
	if !ok {
		return nil
	}
	return ValidateProtoMessage(msg)
}

type ValidationError struct {
	Field   string
	Rule    string
	Message string
}

type ErrValidation = ValidationError

func (e *ValidationError) Error() string {
	return e.Message
}

func (e *ValidationError) ErrorDetails() []proto.Message {
	return []proto.Message{
		&atlasrpc.ValidationError{
			Field:   e.Field,
			Rule:    e.Rule,
			Message: e.Message,
		},
	}
}

func ValidateProtoMessage(msg proto.Message) error {
	err := protovalidate.Validate(msg)
	if err == nil {
		return nil
	}

	var validationErr *protovalidate.ValidationError
	if errors.As(err, &validationErr) {
		field, rule, message := FormatValidationError(validationErr)
		return errors.RpcCode(&ValidationError{
			Field:   field,
			Rule:    rule,
			Message: message,
		}, codes.InvalidArgument, "validation error")
	}

	return err
}

func FormatValidationError(err *protovalidate.ValidationError) (string, string, string) {
	if err == nil {
		return "", "", ""
	}

	for _, violation := range err.Violations {
		if violation == nil || violation.Proto == nil {
			continue
		}
		field := protovalidate.FieldPathString(violation.Proto.GetField())
		rule := protovalidate.FieldPathString(violation.Proto.GetRule())
		message := violation.Proto.GetMessage()
		if field != "" && message != "" {
			return field, rule, fmt.Sprintf("%s: %s", field, message)
		}
		return field, rule, message
	}
	return "", "", strings.TrimPrefix(err.Error(), "validation error: ")
}

func ValidateRequest(req any) error {
	return ValidateRequestWithValidator(validator{}, req)
}

func ValidateRequestWithValidator(v Validator, req any) error {
	err := v.Validate(req)
	if err != nil {
		return err
	}
	return nil
}

func ValidationUnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return UnaryServerInterceptorWithValidator(validator{})
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
	return StreamServerInterceptorWithValidator(validator{})
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
