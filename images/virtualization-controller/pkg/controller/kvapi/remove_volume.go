package kvapi

import (
	"context"
	"encoding/json"
	"fmt"

	virtv1 "kubevirt.io/api/core/v1"
)

const vmSubresourceURLFmt = "/apis/subresources.kubevirt.io/%s/namespaces/%s/virtualmachines/%s/%s"

func (c *Client) RemoveVolume(ctx context.Context, namespace, name string, removeVolumeOptions *virtv1.RemoveVolumeOptions) error {
	uri := fmt.Sprintf(vmSubresourceURLFmt, virtv1.ApiStorageVersion, namespace, name, "removevolume")

	data, err := json.Marshal(removeVolumeOptions)
	if err != nil {
		return err
	}

	res, err := c.restClient.Put().AbsPath(uri).Body(data).Do(ctx).Raw()
	if err != nil {
		return fmt.Errorf("%w: %s", err, string(res))
	}

	return nil
}
