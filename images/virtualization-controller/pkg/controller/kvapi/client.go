package kvapi

import "k8s.io/client-go/rest"

func NewClient(restClient *rest.RESTClient) *Client {
	return &Client{
		restClient: restClient,
	}
}

type Client struct {
	restClient *rest.RESTClient
}
