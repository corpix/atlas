package auth

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/asn1"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"hash/fnv"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"git.tatikoma.dev/corpix/protoc-gen-grpc-capabilities/capabilities"
)

var (
	TokenStateCookieName = "token_state_" + hex.EncodeToString(fnv.New64a().Sum([]byte(fmt.Sprintf("%T", token{}))))[:8]
	TokenCookieName      = "token_" + hex.EncodeToString(fnv.New64a().Sum([]byte(fmt.Sprintf("%T", token{}))))[:8]

	// well_known_private_prefix + [ord(x) for x in "atlas"]
	CapabilitiesCertificateOID = asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 97, 116, 108, 97, 115}
)

type (
	void struct{}

	Claims struct {
		Email  string   `json:"email"`
		Groups []string `json:"groups"`
	}

	Config struct {
		URL *url.URL
		ACL capabilities.CapabilityRuleMap

		Certificate *CertificateConfig
		Token       *TokenConfig
	}

	CertificateConfig struct {
		CA   string
		Cert string
		Key  string
	}

	TokenConfig struct {
		Issuer string
		Client string
		Secret string
	}

	token struct {
		Provider     *oidc.Provider
		Verifier     *oidc.IDTokenVerifier
		OAuth2Config oauth2.Config
	}

	Auth struct {
		config     *Config
		tls        *tls.Config
		tlsManager *TLSConfigCertificateManager
		token      *token
		acl        capabilities.CapabilityRuleMap
	}

	Option func(*Auth)
)

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

func (a *Auth) TLSConfig() *tls.Config {
	return a.tls.Clone()
}

func (a *Auth) GRPC() *GRPC {
	return &GRPC{auth: a}
}

func (a *Auth) HTTP() *HTTP {
	return &HTTP{auth: a}
}

func (a *Auth) CertificateManager() *TLSConfigCertificateManager {
	return a.tlsManager
}

func (a *Auth) tokenClaims(ctx context.Context, token string) (*Claims, error) {
	idToken, err := a.token.Verifier.Verify(ctx, token)
	if err != nil {
		return nil, status.Errorf(codes.Unauthenticated, "invalid token: %v", err)
	}
	var claims Claims
	err = idToken.Claims(&claims)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to parse claims: %v", err)
	}

	return &claims, nil
}

func WithClientCert() Option {
	return func(a *Auth) {
		a.tls.ClientAuth = tls.VerifyClientCertIfGiven
		a.tls.ClientCAs = a.tls.RootCAs
	}
}

func New(cfg Config, opts ...Option) (*Auth, error) {
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
