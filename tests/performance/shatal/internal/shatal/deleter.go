package shatal

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/shatal/internal/api"
)

// Deleter deletes virtual machines.
type Deleter struct {
	api    *api.Client
	logger *slog.Logger
}

func NewDeleter(api *api.Client, log *slog.Logger) *Deleter {
	return &Deleter{
		api:    api,
		logger: log.With("type", "deleter"),
	}
}

func (s *Deleter) Do(ctx context.Context, vm v1alpha2.VirtualMachine) {
	s.logger.With("node", vm.Status.Node).Info(fmt.Sprintf("Delete: %s", vm.Name))

	err := s.api.DeleteVM(ctx, vm)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}

		s.logger.Error(err.Error())
		return
	}
}
