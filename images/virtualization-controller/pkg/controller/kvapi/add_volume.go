package kvapi

import (
	"context"
	"encoding/json"
	"fmt"

	virtv1 "kubevirt.io/api/core/v1"
)

func (c *Client) AddVolume(ctx context.Context, namespace, name string, addVolumeOptions virtv1.AddVolumeOptions) error {
	uri := fmt.Sprintf(vmSubresourceURLFmt, virtv1.ApiStorageVersion, namespace, name, "addvolume")

	data, err := json.Marshal(addVolumeOptions)
	if err != nil {
		return err
	}

	res, err := c.restClient.Put().AbsPath(uri).Body(data).Do(ctx).Raw()
	if err != nil {
		return fmt.Errorf("%w: %s", err, string(res))
	}

	return nil
}
