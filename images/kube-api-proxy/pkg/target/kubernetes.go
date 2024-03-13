package target

import (
	"fmt"
	"net/http"
	"net/url"

	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

type Kubernetes struct {
	Config       *rest.Config
	Client       *http.Client
	APIServerURL *url.URL
}

func NewKubernetesTarget() (*Kubernetes, error) {
	var err error
	k := &Kubernetes{}

	k.Config, err = config.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("load Kubernetes client config: %w", err)
	}

	// Configure HTTP client to Kubernetes API server.
	k.Client, err = rest.HTTPClientFor(k.Config)
	if err != nil {
		return nil, fmt.Errorf("setup Kubernetes API http client: %w", err)
	}

	k.APIServerURL, err = url.Parse(k.Config.Host)
	if err != nil {
		return nil, fmt.Errorf("parse API server URL: %w", err)
	}

	return k, nil
}
