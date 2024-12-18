package logger

import (
	"io"
	stdlog "log"
	"os"

	console "github.com/mattn/go-isatty"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func init() {
	var w io.Writer = os.Stderr
	if console.IsTerminal(os.Stderr.Fd()) {
		w = zerolog.ConsoleWriter{Out: os.Stderr}
	}
	log.Logger = zerolog.New(w).With().Timestamp().Logger()
	stdlog.SetFlags(0)
	stdlog.SetOutput(log.Logger)
}
