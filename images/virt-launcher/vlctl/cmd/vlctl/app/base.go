/*
Copyright 2025 Flant JSC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package app

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"gopkg.in/yaml.v3"

	"vlctl/pkg/client"
)

const (
	outputFlag, outputFlagShort = "output", "o"
	socketFlag, socketFlagShort = "socket", "s"

	defaultSocket = "/run/kubevirt/sockets/launcher-sock"
)

const (
	outputJson = "json"
	outputYaml = "yaml"
	outputXml  = "xml"
)

type BaseOptions struct {
	Output string
	Socket string
}

func (o *BaseOptions) AddFlags(fs *pflag.FlagSet) {
	fs.StringVarP(&o.Output, outputFlag, outputFlagShort, outputXml, o.Output)
	fs.StringVarP(&o.Socket, socketFlag, socketFlagShort, defaultSocket, o.Socket)
}

func (o *BaseOptions) Validate() error {
	switch o.Output {
	case outputJson, outputYaml, outputXml:
	default:
		return fmt.Errorf("unsupported output: %s", o.Output)
	}
	if o.Socket == "" {
		return fmt.Errorf("socket cannot be empty")
	}

	return nil
}

func (o *BaseOptions) Client() (client.LauncherClient, error) {
	return client.NewClient(o.Socket)
}

func (o *BaseOptions) MarshalOutput(v interface{}) ([]byte, error) {
	switch o.Output {
	case outputJson:
		return json.MarshalIndent(v, "", "  ")
	case outputYaml:
		return yaml.Marshal(v)
	case outputXml:
		return xml.MarshalIndent(v, "", "  ")
	default:
		return nil, fmt.Errorf("unknown output format: %s", o.Output)
	}
}

func WithBaseOptions(ctx context.Context, opts BaseOptions) context.Context {
	return context.WithValue(ctx, "baseOpts", opts)
}

func BaseOptionsFromContext(ctx context.Context) BaseOptions {
	val := ctx.Value("baseOpts")
	opts, ok := val.(BaseOptions)
	if !ok {
		return BaseOptions{}
	}
	return opts
}

func BaseOptionsFromCommand(cmd *cobra.Command) BaseOptions {
	return BaseOptionsFromContext(cmd.Context())
}
