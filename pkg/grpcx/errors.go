// Package grpcx maps domain errors to gRPC status codes so every service
// translates failures the same way.
package grpcx

import (
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/ank/e-proc/pkg/apperr"
)

// Error converts a domain error into a gRPC status error.
func Error(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, apperr.ErrNotFound) {
		return status.Error(codes.NotFound, "resource not found")
	}
	if v, ok := apperr.AsValidation(err); ok {
		st := status.New(codes.InvalidArgument, "validation failed")
		withDetails, detailErr := st.WithDetails()
		if detailErr != nil {
			return st.Err()
		}
		_ = v
		return withDetails.Err()
	}
	if c, ok := apperr.AsConflict(err); ok {
		return status.Error(codes.AlreadyExists, c.Message)
	}
	if f, ok := apperr.AsForbidden(err); ok {
		return status.Error(codes.PermissionDenied, f.Message)
	}
	return status.Error(codes.Internal, "internal error")
}
