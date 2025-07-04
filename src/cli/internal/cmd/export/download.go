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

package export

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization/src/cli/internal/clientconfig"
	"github.com/deckhouse/virtualization/src/cli/internal/templates"
)

const (
	downloadExample = `  # Download tar for VirtualDataExport 'myvdexport':
  {{ProgramName}} export download myvdexport
  {{ProgramName}} export download myvdexport -n mynamespace
  {{ProgramName}} export download myvdexport -f myvdexport.tar.gz`
)

type download struct {
	file     string
	insecure bool
}

func newExportDownloadCommand() *cobra.Command {
	c := &download{}
	cmd := &cobra.Command{
		Use:     "download (VirtualDataExport)",
		Short:   "Download an export.",
		Example: downloadExample,
		Args:    templates.ExactArgs("download", 1),
		RunE:    c.Run,
	}

	cmd.Flags().StringVarP(&c.file, "file", "f", "", "File to download to")
	cmd.Flags().BoolVarP(&c.insecure, "insecure", "i", false, "Skip TLS certificate verification")
	cmd.SetUsageTemplate(templates.UsageTemplate())
	return cmd
}

func (c *download) Run(cmd *cobra.Command, args []string) error {
	client, namespace, _, err := clientconfig.ClientAndNamespaceFromContext(cmd.Context())
	if err != nil {
		return err
	}
	name := args[0]
	export, err := client.VirtualDataExports(namespace).Get(cmd.Context(), name, metav1.GetOptions{})
	if err != nil {
		return err
	}

	url := export.Status.URL
	if url == "" {
		return fmt.Errorf("export %s is not ready", name)
	}
	file := c.file
	if file == "" {
		file = fmt.Sprintf("%s.%s.tar.gz", export.Name, export.Namespace)
	}

	out, err := os.Create(file)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %v", file, err)
	}
	defer out.Close()

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: c.insecure},
	}
	httpClient := &http.Client{Transport: tr}

	resp, err := httpClient.Get(url)
	if err != nil {
		return fmt.Errorf("failed to download from %s: %v", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read response body: %v", err)
		}
		return fmt.Errorf("bad status: %s : %s", resp.Status, msg)
	}

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to save file: %v", err)
	}

	cmd.Printf("File successfully downloaded to %s\n", file)

	return nil
}
