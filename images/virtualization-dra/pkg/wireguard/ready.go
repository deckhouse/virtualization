package wireguard

import "context"

func (c *Controller) ReadyForUse(_ context.Context) (bool, error) {
	return c.ready.Load(), nil
}
