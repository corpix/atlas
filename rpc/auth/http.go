package auth

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/metadata"
)

type HTTP struct {
	auth *Auth
}

func (h *HTTP) Middleware(next http.Handler, httpRedirect func(http.ResponseWriter, *http.Request, string, int)) http.Handler {
	authRedirect := func(w http.ResponseWriter, r *http.Request) {
		httpRedirect(w, r, h.auth.config.URL.Path+"/auth/token", http.StatusFound)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, h.auth.config.URL.Path+"/auth/") {
			next.ServeHTTP(w, r)
			return
		}

		token, err := r.Cookie(TokenCookieName)
		if err != nil {
			authRedirect(w, r)
			return
		}

		ctx := r.Context()
		claims, err := h.auth.tokenClaims(ctx, token.Value)
		if err != nil {
			log.Error().Err(err).Msg("failed to verify token")
			authRedirect(w, r)
			return
		}

		ctx = context.WithValue(ctx, TokenContextKey, token.Value)
		ctx = context.WithValue(ctx, TokenClaimsContextKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (h *HTTP) MetadataAnnotator(ctx context.Context, _ *http.Request) metadata.MD {
	meta := map[string]string{}
	token, ok := ctx.Value(TokenContextKey).(string)
	if ok {
		meta[TokenMetadataKey] = token
	}

	return metadata.New(meta)
}

func (h *HTTP) Register(mux *http.ServeMux, httpError func(http.ResponseWriter, any, int)) {
	if h.auth.token == nil {
		return
	}
	prefix := h.auth.config.URL.Path

	mux.HandleFunc(prefix+"/auth/token", func(w http.ResponseWriter, r *http.Request) {
		state, err := h.auth.token.rand(16)
		if err != nil {
			httpError(w, "internal error", http.StatusInternalServerError)
			return
		}
		h.auth.token.setCookie(w, r, TokenStateCookieName, state, 5*time.Minute)

		http.Redirect(w, r, h.auth.token.OAuth2Config.AuthCodeURL(state), http.StatusFound)
	})

	mux.HandleFunc(prefix+"/auth/token/callback", func(w http.ResponseWriter, r *http.Request) {
		state, err := r.Cookie(TokenStateCookieName)
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
		token, err := h.auth.token.OAuth2Config.Exchange(ctx, r.URL.Query().Get("code"))
		if err != nil {
			log.Error().Err(err).Msg("failed to exchange code for token")
			httpError(w, "failed to exchange code for token", http.StatusInternalServerError)
			return
		}

		_, err = h.auth.tokenClaims(ctx, token.AccessToken)
		if err != nil {
			log.Error().Err(err).Msg("failed to get token claims")
			httpError(w, "failed to get token claims", http.StatusUnauthorized)
			return
		}
		h.auth.token.setCookie(w, r, TokenCookieName, token.AccessToken, token.Expiry.Sub(now))
		http.Redirect(w, r, "/", http.StatusFound)
	})
}
