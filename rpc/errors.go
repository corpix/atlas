package rpc

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func ErrIsNotFound(err error) bool {
	st, ok := status.FromError(err)
	return ok && st.Code() == codes.NotFound
}
