//go:build !cgo

package usbredir

import (
	"context"
	"errors"
)

func Run(ctx context.Context, config Config) error {
	if err := config.Validate(); err != nil {
		return err
	}

	return errors.New("usbredir functionality is not available: build with CGO_ENABLED=1 to enable native USB redirection support")
}
