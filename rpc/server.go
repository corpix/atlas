package rpc

import (
	"context"
	"crypto/tls"
	"net"
	"os"
	"path/filepath"

	grpclog "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"git.tatikoma.dev/corpix/atlas/errors"
	"git.tatikoma.dev/corpix/atlas/log"
	"git.tatikoma.dev/corpix/atlas/rpc/auth"
)

func Listen(ctx context.Context, url string) (net.Listener, func(), error) {
	ep, err := ParseEndpoint(url)
	if err != nil {
		return nil, nil, err
	}

	if ep.Scheme != "unix" {
		lc := net.ListenConfig{}
		l, err := lc.Listen(ctx, ep.Network(), ep.Address())
		if err != nil {
			return nil, nil, errors.Wrapf(err, "failed to listen on %q", url)
		}
		return l, func() {}, nil
	}

	socket := ep.Path
	err = os.MkdirAll(filepath.Dir(socket), 0o755)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to create socket dir for %q", socket)
	}
	err = os.Remove(socket)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, nil, errors.Wrapf(err, "failed to remove stale socket %q", socket)
	}
	lc := net.ListenConfig{}
	l, err := lc.Listen(ctx, "unix", socket)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to listen on unix socket %q", socket)
	}
	err = os.Chmod(socket, 0o660)
	if err != nil {
		errors.LogCallErrCtx(ctx, l.Close, "failed to close socket after chmod failure")
		return nil, nil, errors.Wrapf(err, "failed to chmod socket %q", socket)
	}
	cleanup := func() {
		err := os.Remove(socket)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			log.Ctx(ctx).Error().Err(err).Str("socket", socket).Msg("failed to remove socket")
		}
	}
	return l, cleanup, nil
}

func NewServer(tlsCfg *tls.Config, a *auth.Auth, l log.Logger) *grpc.Server {
	return NewServerWithOptions(tlsCfg, a, l)
}

type serverOptions struct {
	validator   Validator
	transformer Transformer
	unaryChain  []grpc.UnaryServerInterceptor
	streamChain []grpc.StreamServerInterceptor
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

func WithUnaryServerInterceptors(is ...grpc.UnaryServerInterceptor) ServerOption {
	return func(opts *serverOptions) {
		opts.unaryChain = append(opts.unaryChain, is...)
	}
}

func WithStreamServerInterceptors(is ...grpc.StreamServerInterceptor) ServerOption {
	return func(opts *serverOptions) {
		opts.streamChain = append(opts.streamChain, is...)
	}
}

func NewServerWithOptions(tlsCfg *tls.Config, a *auth.Auth, l log.Logger, options ...ServerOption) *grpc.Server {
	logger := LoggerInterceptor(l)
	opts := serverOptions{
		validator:   validator{},
		transformer: DefaultsTransformer{},
	}
	for _, option := range options {
		option(&opts)
	}
	unary := append(
		append([]grpc.UnaryServerInterceptor{}, opts.unaryChain...),
		grpclog.UnaryServerInterceptor(logger),
		a.GRPC().UnaryInterceptor(),
		UnaryServerInterceptorWithValidator(opts.validator),
		UnaryServerInterceptorWithTransformer(opts.transformer),
	)
	stream := append(
		append([]grpc.StreamServerInterceptor{}, opts.streamChain...),
		grpclog.StreamServerInterceptor(logger),
		a.GRPC().StreamInterceptor(),
		StreamServerInterceptorWithValidator(opts.validator),
		StreamServerInterceptorWithTransformer(opts.transformer),
	)

	return grpc.NewServer(
		grpc.Creds(credentials.NewTLS(tlsCfg)),
		grpc.ChainUnaryInterceptor(unary...),
		grpc.ChainStreamInterceptor(stream...),
	)
}
