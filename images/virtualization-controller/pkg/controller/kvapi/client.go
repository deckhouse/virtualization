package kvapi

import "k8s.io/client-go/rest"

const vmSubresourceURLFmt = "/apis/subresources.kubevirt.io/%s/namespaces/%s/virtualmachines/%s/%s"

func NewClient(restClient *rest.RESTClient) *Client {
	return &Client{
		restClient: restClient,
	}
}

type Client struct {
	restClient *rest.RESTClient
}
