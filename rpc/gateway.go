package rpc

import (
	"context"
	"net"
	"net/http"
	"net/textproto"
	"strings"
	"time"

	gruntime "github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"git.tatikoma.dev/corpix/atlas/log"
	"git.tatikoma.dev/corpix/atlas/rpc/auth"
)

const (
	DefaultGatewayMaxHeaderBytes    = http.DefaultMaxHeaderBytes
	DefaultGatewayReadHeaderTimeout = 5 * time.Second
)

type (
	GatewayServiceRegistry func(context.Context, *gruntime.ServeMux, string, []grpc.DialOption) error

	GatewayHooks interface {
		HeaderMatcher(key string) (string, bool)
		ErrorHandler(ctx context.Context, mux *gruntime.ServeMux, marshaler gruntime.Marshaler, w http.ResponseWriter, r *http.Request, err error)
	}
	GatewayMux = gruntime.ServeMux

	DefaultGatewayHooks void
)

func (DefaultGatewayHooks) HeaderMatcher(key string) (string, bool) {
	return DefaultGatewayHeaderMatcher(key)
}

func (DefaultGatewayHooks) ErrorHandler(ctx context.Context, mux *gruntime.ServeMux, marshaler gruntime.Marshaler, w http.ResponseWriter, r *http.Request, err error) {
	DefaultGatewayErrorHandler(ctx, mux, marshaler, w, r, err)
}

type GatewayConfig struct {
	Hooks             GatewayHooks
	Prefix            string
	Services          []GatewayServiceRegistry
	DialOptions       []grpc.DialOption
	ReadHeaderTimeout time.Duration
	MaxHeaderBytes    int
}

type Gateway struct {
	mux         http.Handler
	auth        *auth.Auth
	server      *http.Server
	rpcEndpoint string
	prefix      string
}

// DefaultGatewayHeaderMatcher picks headers which will be passed into gRPC context as metadata.
func DefaultGatewayHeaderMatcher(key string) (string, bool) {
	key = textproto.CanonicalMIMEHeaderKey(key)
	switch key {
	case "Date":
	case "From":
	case "Host":
	case "Origin":
	case "Via":
	default:
		return "", false
	}

	return gruntime.MetadataPrefix + key, true
}

func DefaultGatewayErrorHandler(ctx context.Context, mux *gruntime.ServeMux, marshaler gruntime.Marshaler, w http.ResponseWriter, r *http.Request, err error) {
	log.Ctx(ctx).Error().
		Str("path", r.URL.Path).
		Err(err).
		Msg("gateway error")

	var respErr error
	st, ok := status.FromError(err)
	if ok {
		code := st.Code()
		switch code {
		case codes.Unavailable:
			respErr = status.Errorf(code, "rpc backend unavailable")
		case codes.NotFound:
			respErr = status.Errorf(code, "not found %q", r.URL.Path)
		}
	}
	if respErr == nil {
		respErr = status.Errorf(codes.Internal, "internal error")
	}

	gruntime.DefaultHTTPErrorHandler(ctx, mux, marshaler, w, r, respErr)
}

func (g *Gateway) Register(mux *http.ServeMux) {
	prefix := g.prefix
	mux.Handle(prefix, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		trimmed := strings.TrimPrefix(r.URL.Path, prefix)
		r.URL.Path = "/" + strings.TrimPrefix(trimmed, "/")
		g.mux.ServeHTTP(w, r)
	}))
}

func (g *Gateway) Serve(l net.Listener) error {
	return g.server.Serve(l)
}

func (g *Gateway) Close() error {
	return g.server.Close()
}

func NewGateway(ctx context.Context, a *auth.Auth, rpcEndpoint string, cfg GatewayConfig) (*Gateway, error) {
	cfg = cfg.Defaults()
	return NewGatewayWithMux(ctx, a, rpcEndpoint, NewGatewayMux(a, cfg), cfg)
}

func NewGatewayWithMux(ctx context.Context, a *auth.Auth, rpcEndpoint string, mux *gruntime.ServeMux, cfg GatewayConfig) (*Gateway, error) {
	cfg = cfg.Defaults()

	opts := make([]grpc.DialOption, 0, 1+len(cfg.DialOptions))
	opts = append(opts, a.DialOption())
	opts = append(opts, cfg.DialOptions...)

	for _, srv := range cfg.Services {
		err := srv(ctx, mux, rpcEndpoint, opts)
		if err != nil {
			return nil, err
		}
	}

	return &Gateway{
		mux:         mux,
		rpcEndpoint: rpcEndpoint,
		auth:        a,
		prefix:      cfg.Prefix,
		server: &http.Server{
			ReadHeaderTimeout: cfg.ReadHeaderTimeout,
			MaxHeaderBytes:    cfg.MaxHeaderBytes,
			Handler:           mux,
		},
	}, nil
}

func NewGatewayMux(a *auth.Auth, cfg GatewayConfig) *gruntime.ServeMux {
	opts := []gruntime.ServeMuxOption{
		gruntime.WithIncomingHeaderMatcher(cfg.Hooks.HeaderMatcher),
		gruntime.WithMetadata(a.MetadataAnnotator),
		gruntime.WithErrorHandler(cfg.Hooks.ErrorHandler),
	}

	return gruntime.NewServeMux(opts...)
}

func (cfg GatewayConfig) Defaults() GatewayConfig {
	if cfg.Prefix == "" {
		cfg.Prefix = "/"
	}
	if cfg.ReadHeaderTimeout == 0 {
		cfg.ReadHeaderTimeout = DefaultGatewayReadHeaderTimeout
	}
	if cfg.MaxHeaderBytes == 0 {
		cfg.MaxHeaderBytes = DefaultGatewayMaxHeaderBytes
	}
	if cfg.Hooks == nil {
		cfg.Hooks = DefaultGatewayHooks{}
	}
	return cfg
}
