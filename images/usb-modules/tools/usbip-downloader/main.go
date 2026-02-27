/*
Copyright 2026 Flant JSC

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

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var kernel5Versions = []string{"5.0", "5.1", "5.2", "5.3", "5.4", "5.5", "5.6", "5.7", "5.8", "5.9", "5.10", "5.11", "5.12", "5.13", "5.14", "5.15", "5.16", "5.17", "5.18", "5.19"}
var kernel6Versions = []string{"6.0", "6.1", "6.2", "6.3", "6.4", "6.5", "6.6", "6.7", "6.8", "6.9", "6.10", "6.11", "6.12", "6.13", "6.14", "6.15", "6.16", "6.17", "6.18", "6.19"}

var kernelVersions = append(kernel5Versions, kernel6Versions...)

const gitLinuxUsbipContentUrlTmpl = "https://api.github.com/repos/torvalds/linux/contents/drivers/usb/usbip?ref=%s"

func main() {
	if err := NewDownloader().Execute(); err != nil {
		log.Fatal(err)
	}
}

func NewDownloader() *cobra.Command {
	o := options{}
	cmd := &cobra.Command{
		Use:           "usbip-downloader",
		Short:         "Downloads kernel modules for USBIP",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE:          o.Run,
	}

	o.AddFlags(cmd.Flags())

	return cmd
}

type options struct {
	timeout             time.Duration
	maxIdleConns        int
	maxIdleConnsPerHost int
	idleConnTimeout     time.Duration
	outputDir           string
}

func (o *options) AddFlags(fs *pflag.FlagSet) {
	fs.DurationVar(&o.timeout, "timeout", 5*time.Minute, "timeout per download")
	fs.IntVar(&o.maxIdleConns, "max-idle-conns", 20, "limit number of parallel downloads")
	fs.IntVar(&o.maxIdleConnsPerHost, "max-idle-conns-per-host", 10, "limit number of parallel downloads")
	fs.DurationVar(&o.idleConnTimeout, "idle-conn-timeout", 30*time.Second, "limit number of parallel downloads")
	fs.StringVar(&o.outputDir, "out-dir", ".out", "output directory")

}

func (o *options) Run(_ *cobra.Command, _ []string) error {
	client := &http.Client{
		Timeout: o.timeout,
		Transport: &http.Transport{
			MaxIdleConns:        o.maxIdleConns,
			MaxIdleConnsPerHost: o.maxIdleConnsPerHost,
			IdleConnTimeout:     o.idleConnTimeout,
		},
	}

	var wg sync.WaitGroup
	errCh := make(chan error, len(kernelVersions))

	for _, version := range kernelVersions {
		dest := o.dest(version)
		if _, err := os.Stat(dest); err == nil {
			log.Printf("Skipping %s, already exists\n", version)
			continue
		}

		wg.Add(1)
		go func() {
			defer wg.Done()

			if err := o.download(client, version); err != nil {
				errCh <- fmt.Errorf("%s: %w", version, err)
			}
		}()
	}

	wg.Wait()
	close(errCh)

	var resultErr error
	for err := range errCh {
		resultErr = errors.Join(resultErr, err)
	}

	if resultErr != nil {
		return fmt.Errorf("error downloading kernels: %w", resultErr)
	}

	return o.checkDownloads()
}

func (o *options) download(client *http.Client, version string) error {
	content, err := o.getContent(client, version)
	if err != nil {
		return err
	}

	for _, file := range content {
		if err := o.downloadFile(client, file, version); err != nil {
			return err
		}
	}

	log.Printf("Done %s\n", version)
	return nil
}

type fileInfo struct {
	DownloadURL string `json:"download_url"`
	Sha         string `json:"sha"`
	Path        string `json:"path"`
}

func (o *options) getContent(client *http.Client, version string) ([]fileInfo, error) {
	url := fmt.Sprintf(gitLinuxUsbipContentUrlTmpl, tag(version))

	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bad status: %s", resp.Status)
	}

	var files []fileInfo
	err = json.NewDecoder(resp.Body).Decode(&files)
	if err != nil {
		return nil, fmt.Errorf("error decoding json: %w", err)
	}

	return files, nil
}

func (o *options) downloadFile(client *http.Client, file fileInfo, version string) error {
	dest := o.dest(version)
	targetFile := filepath.Join(dest, file.Path)

	log.Printf("Downloading file. file: %s, downloadUrl: %s, sha: %s \n", targetFile, file.DownloadURL, file.Sha)

	resp, err := client.Get(file.DownloadURL)
	if err != nil {
		return fmt.Errorf("error downloading %s: %w", file.Path, err)
	}
	defer resp.Body.Close()

	dir := filepath.Dir(targetFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("error creating directory %s: %w", dir, err)
	}

	f, err := os.OpenFile(targetFile, os.O_CREATE|os.O_RDWR|os.O_TRUNC, os.FileMode(0644))
	if err != nil {
		return fmt.Errorf("create file failed: %w", err)
	}

	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		return fmt.Errorf("write file failed: %w", err)
	}

	if err := f.Close(); err != nil {
		return err
	}

	return nil
}

func (o *options) dest(version string) string {
	return filepath.Join(o.outputDir, version)
}

func (o *options) checkDownloads() error {
	for _, version := range kernelVersions {
		dest := o.dest(version)
		if _, err := os.Stat(dest); err != nil {
			return fmt.Errorf("missing download: %s: %w", version, err)
		}
	}
	return nil
}

func tag(version string) string {
	return "v" + version
}
