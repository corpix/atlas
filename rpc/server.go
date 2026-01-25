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
	logger := LoggerInterceptor(l)
	return grpc.NewServer(
		grpc.Creds(credentials.NewTLS(tlsCfg)),
		grpc.ChainUnaryInterceptor(
			grpclog.UnaryServerInterceptor(logger),
			a.GRPC().UnaryInterceptor(),
			ValidationUnaryServerInterceptor(),
			DefaultsUnaryServerInterceptor(),
		),
		grpc.ChainStreamInterceptor(
			grpclog.StreamServerInterceptor(logger),
			a.GRPC().StreamInterceptor(),
			ValidationStreamServerInterceptor(),
			DefaultsStreamServerInterceptor(),
		),
	)
}
