package rpc

import (
	"context"
	"fmt"

	grpc_logging "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"github.com/rs/zerolog"

	"git.tatikoma.dev/corpix/protoc-gen-grpc-capabilities/capabilities"
)

func LoggerInterceptor(l zerolog.Logger) grpc_logging.Logger {
	return grpc_logging.LoggerFunc(func(ctx context.Context, lvl grpc_logging.Level, msg string, fields ...any) {
		evt := l.With().Fields(fields)
		caps := capabilities.CapabilitiesFromContext(ctx)
		if caps != nil {
			// fixme: for some reason it is nil on debug level of "finished call"
			// looks like it uses different context
			evt = evt.Str("capabilities", caps.String())
		}
		l := evt.Logger()

		switch lvl {
		case grpc_logging.LevelDebug:
			l.Debug().Msg(msg)
		case grpc_logging.LevelInfo:
			l.Info().Msg(msg)
		case grpc_logging.LevelWarn:
			l.Warn().Msg(msg)
		case grpc_logging.LevelError:
			l.Error().Msg(msg)
		default:
			panic(fmt.Sprintf("unknown level %v", lvl))
		}
	})
}
