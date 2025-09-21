package logger

import (
	"context"
	"testing"

	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
)

func TestLogger(t *testing.T) {
	ctx := context.Background()
	logger := With().Str("key", "value").Logger()

	ctx = logger.WithContext(ctx)
	ctxLogger := Ctx(ctx)
	assert.NotEqual(t, log.Logger, ctxLogger)
	assert.Equal(t, &logger, ctxLogger)

	ctxEmpty := context.Background()
	ctxLogger = Ctx(ctxEmpty)
	assert.Equal(t, &log.Logger, ctxLogger)
}
