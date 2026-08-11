package metrics

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"

	"git.tatikoma.dev/corpix/atlas/errors"
	"git.tatikoma.dev/corpix/atlas/log"
	"git.tatikoma.dev/corpix/atlas/rpc"
)

const (
	DefaultPath              = "/metrics"
	DefaultReadHeaderTimeout = 5 * time.Second
	DefaultMaxHeaderBytes    = 1 << 20

	authorizationScheme = "Bearer "
)

type (
	Config struct {
		URL   string
		Path  string
		Token string
	}

	Server struct {
		config   Config
		gatherer prometheus.Gatherer
	}
)

func (s *Server) authorized(r *http.Request) bool {
	if s.config.Token == "" {
		return true
	}
	token, ok := strings.CutPrefix(r.Header.Get("authorization"), authorizationScheme)
	if !ok {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(s.config.Token)) == 1
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(r) {
		w.Header().Set("www-authenticate", strings.TrimSpace(authorizationScheme))
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	families, err := s.gatherer.Gather()
	if err != nil && len(families) == 0 {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	format := expfmt.NegotiateIncludingOpenMetrics(r.Header)
	w.Header().Set("content-type", string(format))

	enc := expfmt.NewEncoder(w, format)
	for _, family := range families {
		err = enc.Encode(family)
		if err != nil {
			log.Ctx(r.Context()).
				Error().
				Err(err).
				Msg("failed to encode metric family")
			return
		}
	}
	if closer, ok := enc.(expfmt.Closer); ok {
		errors.LogCallErrCtx(r.Context(), closer.Close, "failed to close metrics encoder")
	}
}

func (s *Server) Run(ctx context.Context) error {
	l, cleanup, err := rpc.Listen(ctx, s.config.URL)
	if err != nil {
		return errors.Wrapf(err, "failed to listen on %q", s.config.URL)
	}
	defer cleanup()
	defer errors.LogCallErrCtx(ctx, l.Close, "failed to close metrics listener %q", s.config.URL)

	mux := http.NewServeMux()
	mux.HandleFunc(s.config.Path, s.handle)

	srv := &http.Server{
		ReadHeaderTimeout: DefaultReadHeaderTimeout,
		MaxHeaderBytes:    DefaultMaxHeaderBytes,
		Handler:           mux,
	}

	go func() {
		<-ctx.Done()
		err := srv.Shutdown(context.Background())
		if err != nil {
			log.Ctx(ctx).
				Error().
				Err(err).
				Msg("failed to shutdown metrics server")
		}
	}()

	log.Ctx(ctx).
		Info().
		Str("url", s.config.URL).
		Str("path", s.config.Path).
		Msg("serving metrics")

	err = srv.Serve(l)
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func NewServer(c Config, g prometheus.Gatherer) *Server {
	if c.Path == "" {
		c.Path = DefaultPath
	}
	return &Server{config: c, gatherer: g}
}
