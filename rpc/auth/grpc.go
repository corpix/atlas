package auth

import (
	"context"
	"crypto/x509"
	"encoding/asn1"
	"encoding/json"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	"git.tatikoma.dev/corpix/atlas/errors"
	"git.tatikoma.dev/corpix/protoc-gen-grpc-capabilities/capabilities"
)

type GRPC struct {
	auth *Auth
}

func (g *GRPC) TransportCredentials() credentials.TransportCredentials {
	return credentials.NewTLS(g.auth.TLSConfig())
}

func (g *GRPC) DialOption() grpc.DialOption {
	return grpc.WithTransportCredentials(g.TransportCredentials())
}

func (g *GRPC) ServerOption() grpc.ServerOption {
	return grpc.Creds(g.TransportCredentials())
}

func (g *GRPC) UnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		handlerCtx, err := g.authenticateGrpcContext(ctx)
		if err != nil {
			return nil, err
		}
		handlerCtx, err = g.authorizeGrpcContext(handlerCtx, info.FullMethod)
		if err != nil {
			return nil, err
		}
		return handler(handlerCtx, req)
	}
}

func (g *GRPC) StreamInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		handlerCtx, err := g.authenticateGrpcContext(ss.Context())
		if err != nil {
			return err
		}
		handlerCtx, err = g.authorizeGrpcContext(handlerCtx, info.FullMethod)
		if err != nil {
			return err
		}
		return handler(srv, &streamWithCtx{
			ServerStream: ss,
			ctx:          handlerCtx,
		})
	}
}

func (g *GRPC) tokenFromGrpcCtx(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", status.Errorf(codes.Unauthenticated, "missing metadata")
	}
	values := md[TokenMetadataKey]
	if len(values) == 0 {
		return "", status.Errorf(codes.Unauthenticated, "missing authorization token")
	}
	token := values[0]
	return token, nil
}

func (g *GRPC) authenticateGrpcContext(ctx context.Context) (context.Context, error) {
	var verified bool
	p, ok := peer.FromContext(ctx)
	if ok {
		tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo)
		if ok && len(tlsInfo.State.VerifiedChains) > 0 {
			verified = true
		}
	}

	if g.auth.token == nil {
		// note: client may be verified by client cert only, token may remain unconfigured
		if verified {
			return ctx, nil
		}
		return nil, status.Errorf(codes.Unauthenticated, "no valid client certificate providen")
	}

	token, err := g.tokenFromGrpcCtx(ctx)
	if err != nil {
		if verified {
			return ctx, nil
		}
		return nil, err
	}
	claims, err := g.auth.tokenClaims(ctx, token)
	if err != nil {
		return nil, err
	}
	return context.WithValue(ctx, TokenClaimsContextKey, claims), nil
}

func (g *GRPC) authorizeGrpcContext(ctx context.Context, method string) (context.Context, error) {
	var (
		caps       = capabilities.Capabilities{}
		err        error
		authorized bool
	)
	p, ok := peer.FromContext(ctx)
	if ok {
		tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo)
		if ok && len(tlsInfo.State.VerifiedChains) > 0 {
			caps, err = g.capabilitiesFromCertificate(tlsInfo.State.VerifiedChains[0][0])
			if err != nil {
				return nil, status.Errorf(
					codes.Internal,
					"failed to extract capabilities from client certificate: %v", err,
				)
			}
			authorized = true
		}
	}

	if claims, ok := ctx.Value(TokenClaimsContextKey).(*Claims); ok {
		claimsCaps := g.parseCapabilities(claims.Groups)
		for k, v := range claimsCaps {
			caps[k] = v
		}
		authorized = true
	}

	if !authorized {
		return nil, status.Errorf(codes.Unauthenticated, "no valid authorization sources providen (expected client certificate or token)")
	}

	rule, matched := g.auth.acl.Match(caps, method)
	if !matched {
		return nil, status.Errorf(
			codes.InvalidArgument,
			"required client capability set for %q not satisfied, has: %s, want: %s",
			method, caps.String(), rule.String(),
		)
	}
	return context.WithValue(ctx, capabilities.CapabilitiesContextKey, caps), nil
}

func (g *GRPC) capabilitiesFromCertificate(cert *x509.Certificate) (capabilities.Capabilities, error) {
	if !isClientCertificate(cert) {
		return nil, errors.New("certificate is not valid for client auth")
	}
	for _, ext := range cert.Extensions {
		if !ext.Id.Equal(CapabilitiesCertificateOID) {
			continue
		}
		var rawValue string
		_, err := asn1.Unmarshal(ext.Value, &rawValue)
		if err != nil {
			return nil, errors.Wrap(err, "failed to unmarshal capabilities from x509 cert")
		}
		var capSlice []string
		err = json.Unmarshal([]byte(rawValue), &capSlice)
		if err != nil {
			return nil, errors.Wrap(err, "failed to parse capabilities list")
		}

		return g.parseCapabilities(capSlice), nil
	}
	return capabilities.Capabilities{}, nil
}

func (g *GRPC) parseCapabilities(capStrs []string) capabilities.Capabilities {
	caps := make(capabilities.Capabilities, len(capStrs))
	for _, capStr := range capStrs {
		capWithParams := strings.Split(capStr, ":")
		cap := capabilities.NewCapability(
			capabilities.CapabilityLiteral(capWithParams[0]),
			capWithParams[1:]...,
		)
		caps[cap.ID] = cap
	}
	return caps
}
