package wireguard

import "context"

func (c *Controller) ReadyForUse(ctx context.Context) (bool, error) {
	return false, nil
}
