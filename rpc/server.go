package rpc

import (
	"crypto/tls"

	grpclog "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"git.tatikoma.dev/corpix/atlas/log"
	"git.tatikoma.dev/corpix/atlas/rpc/auth"
)

func NewServer(tlsCfg *tls.Config, a *auth.Auth, l log.Logger) *grpc.Server {
	return NewServerWithOptions(tlsCfg, a, l)
}

type serverOptions struct {
	validator   Validator
	transformer Transformer
}

type ServerOption func(*serverOptions)

func WithValidator(v Validator) ServerOption {
	return func(opts *serverOptions) {
		opts.validator = v
	}
}

func WithTransformer(t Transformer) ServerOption {
	return func(opts *serverOptions) {
		opts.transformer = t
	}
}

func NewServerWithOptions(tlsCfg *tls.Config, a *auth.Auth, l log.Logger, options ...ServerOption) *grpc.Server {
	logger := LoggerInterceptor(l)
	opts := serverOptions{
		validator:   ValidatorMethod{},
		transformer: DefaultsTransformer{},
	}
	for _, option := range options {
		option(&opts)
	}
	return grpc.NewServer(
		grpc.Creds(credentials.NewTLS(tlsCfg)),
		grpc.ChainUnaryInterceptor(
			grpclog.UnaryServerInterceptor(logger),
			a.GRPC().UnaryInterceptor(),
			UnaryServerInterceptorWithValidator(opts.validator),
			UnaryServerInterceptorWithTransformer(opts.transformer),
		),
		grpc.ChainStreamInterceptor(
			grpclog.StreamServerInterceptor(logger),
			a.GRPC().StreamInterceptor(),
			StreamServerInterceptorWithValidator(opts.validator),
			StreamServerInterceptorWithTransformer(opts.transformer),
		),
	)
}
