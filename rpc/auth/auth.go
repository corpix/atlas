package auth

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/asn1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	"golang.org/x/oauth2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

var (
	AuthTokenStateCookieName = "token_state_" + hex.EncodeToString(fnv.New64a().Sum([]byte(fmt.Sprintf("%T", token{}))))[:8]
	AuthTokenCookieName      = "token_" + hex.EncodeToString(fnv.New64a().Sum([]byte(fmt.Sprintf("%T", token{}))))[:8]

	// well_known_private_prefix + [ord(x) for x in "atlas"]
	AuthCapabilitiesCertificateOID = asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 97, 116, 108, 97, 115}
)

type void struct{}

type AuthClaims struct {
	Email  string   `json:"email"`
	Groups []string `json:"groups"`
}

type AuthConfig struct {
	URL *url.URL
	ACL CapabilityRuleMap

	Certificate *AuthCertificateConfig
	Token       *AuthTokenConfig
}

type AuthCertificateConfig struct {
	CA   string
	Cert string
	Key  string
}

type AuthTokenConfig struct {
	Issuer string
	Client string
	Secret string
}

type AuthOption func(*Auth)

type token struct {
	Provider     *oidc.Provider
	Verifier     *oidc.IDTokenVerifier
	OAuth2Config oauth2.Config
}

func (token) rand(n int) (string, error) {
	b := make([]byte, n)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func (token) setCookie(w http.ResponseWriter, r *http.Request, name, value string, age time.Duration) {
	c := &http.Cookie{
		Name:     name,
		Value:    value,
		MaxAge:   int(age.Seconds()),
		Secure:   r.TLS != nil,
		HttpOnly: true,
		Path:     "/",
	}
	http.SetCookie(w, c)
}

type Auth struct {
	config     *AuthConfig
	tls        *tls.Config
	tlsManager *TLSConfigCertificateManager
	token      *token
	acl        CapabilityRuleMap
}

func (a *Auth) TLS() *tls.Config {
	return a.tls.Clone()
}

func (a *Auth) TransportCredentials() credentials.TransportCredentials {
	return credentials.NewTLS(a.tls)
}

func (a *Auth) DialOption() grpc.DialOption {
	return grpc.WithTransportCredentials(a.TransportCredentials())
}

func (a *Auth) ServerOption() grpc.ServerOption {
	return grpc.Creds(a.TransportCredentials())
}

func (a *Auth) UnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		handlerCtx, err := a.authenticateGrpcContext(ctx)
		if err != nil {
			return nil, err
		}
		handlerCtx, err = a.authorizeGrpcContext(handlerCtx, info.FullMethod)
		if err != nil {
			return nil, err
		}
		return handler(handlerCtx, req)
	}
}

func (a *Auth) StreamInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		handlerCtx, err := a.authenticateGrpcContext(ss.Context())
		if err != nil {
			return err
		}
		handlerCtx, err = a.authorizeGrpcContext(handlerCtx, info.FullMethod)
		if err != nil {
			return err
		}
		return handler(srv, &streamWithCtx{
			ServerStream: ss,
			ctx:          handlerCtx,
		})
	}
}

func (a *Auth) Middleware(next http.Handler, httpRedirect func(http.ResponseWriter, *http.Request, string, int)) http.Handler {
	authRedirect := func(w http.ResponseWriter, r *http.Request) {
		httpRedirect(w, r, a.config.URL.Path+"/auth/token", http.StatusFound)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, a.config.URL.Path+"/auth/") {
			next.ServeHTTP(w, r)
			return
		}

		token, err := r.Cookie(AuthTokenCookieName)
		if err != nil {
			authRedirect(w, r)
			return
		}

		ctx := r.Context()
		claims, err := a.tokenClaims(ctx, token.Value)
		if err != nil {
			log.Error().Err(err).Msg("failed to verify token")
			authRedirect(w, r)
			return
		}

		ctx = context.WithValue(ctx, AuthTokenContextKey, token.Value)
		ctx = context.WithValue(ctx, AuthTokenClaimsContextKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (a *Auth) MetadataAnnotator(ctx context.Context, r *http.Request) metadata.MD {
	meta := map[string]string{}
	token, ok := ctx.Value(AuthTokenContextKey).(string)
	if ok {
		meta[AuthTokenMetadataKey] = token
	}

	return metadata.New(meta)
}

func (a *Auth) Register(mux *http.ServeMux, httpError func(http.ResponseWriter, any, int)) {
	if a.token == nil {
		return
	}
	prefix := a.config.URL.Path

	mux.HandleFunc(prefix+"/auth/token", func(w http.ResponseWriter, r *http.Request) {
		state, err := a.token.rand(16)
		if err != nil {
			httpError(w, "internal error", http.StatusInternalServerError)
			return
		}
		a.token.setCookie(w, r, AuthTokenStateCookieName, state, 5*time.Minute)

		http.Redirect(w, r, a.token.OAuth2Config.AuthCodeURL(state), http.StatusFound)
	})

	mux.HandleFunc(prefix+"/auth/token/callback", func(w http.ResponseWriter, r *http.Request) {
		state, err := r.Cookie(AuthTokenStateCookieName)
		if err != nil {
			httpError(w, "state not found", http.StatusBadRequest)
			return
		}
		qstate := r.URL.Query().Get("state")
		if qstate != state.Value {
			log.Warn().Str("stored", state.Value).Str("query_state", qstate).Msg("state did not match")
			httpError(w, "state did not match", http.StatusBadRequest)
			return
		}

		now := time.Now()

		ctx := r.Context()
		token, err := a.token.OAuth2Config.Exchange(ctx, r.URL.Query().Get("code"))
		if err != nil {
			log.Error().Err(err).Msg("failed to exchange code for token")
			httpError(w, "failed to exchange code for token", http.StatusInternalServerError)
			return
		}

		_, err = a.tokenClaims(ctx, token.AccessToken)
		if err != nil {
			log.Error().Err(err).Msg("failed to get token claims")
			httpError(w, "failed to get token claims", http.StatusUnauthorized)
			return
		}
		a.token.setCookie(w, r, AuthTokenCookieName, token.AccessToken, token.Expiry.Sub(now))
		http.Redirect(w, r, "/", http.StatusFound)
	})
}

func (a *Auth) tokenClaims(ctx context.Context, token string) (*AuthClaims, error) {
	idToken, err := a.token.Verifier.Verify(ctx, token)
	if err != nil {
		return nil, status.Errorf(codes.Unauthenticated, "invalid token: %v", err)
	}
	var claims AuthClaims
	err = idToken.Claims(&claims)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to parse claims: %v", err)
	}

	return &claims, nil
}

func (a *Auth) tokenFromGrpcCtx(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", status.Errorf(codes.Unauthenticated, "missing metadata")
	}
	values := md[AuthTokenMetadataKey]
	if len(values) == 0 {
		return "", status.Errorf(codes.Unauthenticated, "missing authorization token")
	}
	token := values[0]
	return token, nil
}

func (a *Auth) authenticateGrpcContext(ctx context.Context) (context.Context, error) {
	var verified bool
	p, ok := peer.FromContext(ctx)
	if ok {
		tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo)
		if ok && len(tlsInfo.State.VerifiedChains) > 0 {
			verified = true
		}
	}

	if a.token == nil {
		// note: client may be verified by client cert only, token may remain unconfigured
		if verified {
			return ctx, nil
		} else {
			return nil, status.Errorf(codes.Unauthenticated, "no valid client certificate providen")
		}
	}

	token, err := a.tokenFromGrpcCtx(ctx)
	if err != nil {
		if verified {
			return ctx, nil
		}
		return nil, err
	}
	claims, err := a.tokenClaims(ctx, token)
	if err != nil {
		return nil, err
	}
	return context.WithValue(ctx, AuthTokenClaimsContextKey, claims), nil
}

func (a *Auth) authorizeGrpcContext(ctx context.Context, method string) (context.Context, error) {
	var (
		caps       = Capabilities{}
		err        error
		authorized bool
	)
	p, ok := peer.FromContext(ctx)
	if ok {
		tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo)
		if ok && len(tlsInfo.State.VerifiedChains) > 0 {
			caps, err = a.capabilitiesFromCertificate(tlsInfo.State.VerifiedChains[0][0])
			if err != nil {
				return nil, status.Errorf(
					codes.Internal,
					"failed to extract capabilities from client certificate: %v", err,
				)
			}
			authorized = true
		}
	}

	if claims, ok := ctx.Value(AuthTokenClaimsContextKey).(*AuthClaims); ok {
		claimsCaps := a.parseCapabilities(claims.Groups)
		for k, v := range claimsCaps {
			caps[k] = v
		}
		authorized = true
	}

	if !authorized {
		return nil, status.Errorf(codes.Unauthenticated, "no valid authorization sources providen (expected client certificate or token)")
	}

	rule, matched := a.acl.Match(caps, method)
	if !matched {
		return nil, status.Errorf(
			codes.InvalidArgument,
			"required client capability set for %q not satisfied, has: %s, want: %s",
			method, caps.String(), rule.String(),
		)
	}
	return context.WithValue(ctx, AuthCapabilitiesContextKey, caps), nil
}

func (a *Auth) capabilitiesFromCertificate(cert *x509.Certificate) (Capabilities, error) {
	for _, ext := range cert.Extensions {
		if !ext.Id.Equal(AuthCapabilitiesCertificateOID) {
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

		return a.parseCapabilities(capSlice), nil
	}
	return Capabilities{}, nil
}

func (a *Auth) parseCapabilities(caps []string) Capabilities {
	capabilities := make(Capabilities, len(caps))
	for _, capString := range caps {
		capWithParams := strings.Split(capString, ":")
		cap := NewCapability(CapabilityLiteral(capWithParams[0]), capWithParams[1:]...)
		capabilities[cap.ID] = cap
	}
	return capabilities
}

func (a *Auth) CertificateManager() *TLSConfigCertificateManager {
	return a.tlsManager
}

func WithClientCertAuth() AuthOption {
	return func(a *Auth) {
		a.tls.ClientAuth = tls.VerifyClientCertIfGiven
		a.tls.ClientCAs = a.tls.RootCAs
	}
}

func NewAuth(cfg AuthConfig, opts ...AuthOption) (*Auth, error) {
	ctx := context.Background()

	ca, err := os.ReadFile(cfg.Certificate.CA)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read CA cert %q", cfg.Certificate.CA)
	}
	certPool := x509.NewCertPool()
	if ok := certPool.AppendCertsFromPEM(ca); !ok {
		return nil, errors.New("failed to append CA certificate")
	}
	tccm := NewTLSConfigCertificateManager()
	err = tccm.LoadCertificate(cfg.Certificate.Cert, cfg.Certificate.Key)
	if err != nil {
		return nil, err
	}
	err = tccm.LoadClientCertificate(cfg.Certificate.Cert, cfg.Certificate.Key)
	if err != nil {
		return nil, err
	}

	tc := NewTLSConfig(cfg.URL.Hostname(), certPool, tccm)

	//

	var t *token
	if cfg.Token != nil {
		provider, err := oidc.NewProvider(ctx, cfg.Token.Issuer)
		if err != nil {
			return nil, err
		}
		t = &token{
			Provider: provider,
			Verifier: provider.Verifier(&oidc.Config{ClientID: cfg.Token.Client}),
			OAuth2Config: oauth2.Config{
				ClientID:     cfg.Token.Client,
				ClientSecret: cfg.Token.Secret,
				Endpoint:     provider.Endpoint(),
				RedirectURL:  cfg.URL.String() + "/auth/token/callback",
				Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
			},
		}
	}

	a := &Auth{
		config:     &cfg,
		tls:        tc,
		tlsManager: tccm,
		token:      t,
		acl:        cfg.ACL,
	}

	for _, opt := range opts {
		opt(a)
	}

	return a, nil
}
