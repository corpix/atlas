package errors

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	ErrBeginTx    = "failed to start transaction"
	ErrRollbackTx = "failed to rollback transaction"
	ErrCommitTx   = "failed to commit transaction"
)

var (
	Is     = errors.Is
	As     = errors.As
	Wrap   = errors.Wrap
	Wrapf  = errors.Wrapf
	Errorf = fmt.Errorf
	New    = errors.New
)

func Log(err error, fmt string, args ...any) {
	if err != nil {
		log.Error().Err(err).Msgf(fmt, args...)
	}
}

func LogCtx(ctx context.Context, err error, fmt string, args ...any) {
	if err != nil {
		log.Ctx(ctx).Error().Err(err).Msgf(fmt, args...)
	}
}

func LogCallErr(fn func() error, fmt string, args ...any) {
	Log(fn(), fmt, args...)
}

func LogCallErrCtx(ctx context.Context, fn func() error, fmt string, args ...any) {
	LogCtx(ctx, fn(), fmt, args...)
}

func Chain(err error, cause error) error {
	return Errorf("%w: %w", err, cause)
}

func RpcCode(err error, code codes.Code, fmt string, args ...any) error {
	if err == nil {
		return nil
	}

	log.Error().Err(err).Msgf(fmt, args...)
	return status.Errorf(code, fmt, args...)
}

func RpcCodeCtx(ctx context.Context, err error, code codes.Code, fmt string, args ...any) error {
	if err == nil {
		return nil
	}

	log.Ctx(ctx).Error().Err(err).Msgf(fmt, args...)
	return status.Errorf(code, fmt, args...)
}

func Rpc(err error, fmt string, args ...any) error {
	return RpcCode(err, codes.Internal, fmt, args...)
}
