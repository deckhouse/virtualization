package shatal

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// Nothing - use it to do nothing with virtual machines compared other operations.
// Maybe you want 10% of virtual machines to be updated, 10% deleted, and 80% to continue working as usual.
type Nothing struct {
	logger *slog.Logger
}

func NewNothing(log *slog.Logger) *Nothing {
	return &Nothing{
		logger: log.With("type", "nothing"),
	}
}

func (s *Nothing) Do(_ context.Context, vm v1alpha2.VirtualMachine) {
	s.logger.With("node", vm.Status.Node).Debug(fmt.Sprintf("Nothing: %s", vm.Name))
}
