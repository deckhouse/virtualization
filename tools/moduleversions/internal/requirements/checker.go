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

package requirements

import (
	"archive/tar"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"bytes"
	"time"

	"gopkg.in/yaml.v3"
	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/blang/semver/v4"
)

const (
	moduleFileLinkTemplate = "https://raw.githubusercontent.com/deckhouse/virtualization/refs/tags/%s/module.yaml"
	moduleImageURL = "registry.deckhouse.io/deckhouse/%s/modules/%s:%s"
	moduleVersionFile = "version.json"
	deckhouseImageURL = "registry.deckhouse.io/deckhouse/%s:%s"
	deckhouseVersionFile = "deckhouse/version"
	httpTimeout = 5 * time.Second
)

type SemVerRange string
type Modules map[string]SemVerRange
type Requirements struct {
	Deckhouse SemVerRange   `yaml:"deckhouse"`
	Modules Modules   		`yaml:"modules"`
}
type Config struct {
	Requirements Requirements `yaml:"requirements"`
}

type ModuleVersion struct {
	Version string `json:"version"`
}

func ExtractFileFromImage(image, targetFile string) (string, error) {
	ctx := context.Background() // Загружаем образ (аналог crane export)
	img, err := crane.Pull(image, crane.WithContext(ctx))
	if err != nil {
		return "", fmt.Errorf("pull failed for image %v: %v\n", image, err)
	}
	var buf bytes.Buffer
	err = crane.Export(img, &buf)
	if err != nil {
		return "", fmt.Errorf("export failed for image %v: %v\n", image, err)
	}

	tr := tar.NewReader(&buf)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return "", fmt.Errorf("there is no file %v in tar archive for %v", targetFile, image)
		}
		if err != nil {
		return "", fmt.Errorf("tar read error for image %v: %v\n", image, err)
		}

		if hdr.Name == targetFile && hdr.Typeflag == tar.TypeReg {
			var buf bytes.Buffer
			if _, err := io.Copy(&buf, tr); err != nil {
				return "", fmt.Errorf("copy file content: %w", err)
			}
			return buf.String(), nil
		}
	}
}

func VerifyModuleRequirements(module string, sv SemVerRange, edition, channel, tag string) error {
	fmt.Printf("semver range of module %s: %s\n", module, sv)
	prange, err := semver.ParseRange(string(sv))
	if err != nil {
		fmt.Printf("semver.ParseRange failed for module %s: range=%q error=%v\n", module, sv, err)
		return fmt.Errorf("failed to parse range for module %v: %w", module, err)
	}

	isDeckhouse := false
	if module == "deckhouse" {
		isDeckhouse = true
	}

	var image, tf string
	if isDeckhouse {
		image = fmt.Sprintf(deckhouseImageURL, edition, channel)
		tf = deckhouseVersionFile
	} else {
		image = fmt.Sprintf(moduleImageURL, edition, module, channel)
		tf = moduleVersionFile
	}

	file, err := ExtractFileFromImage(image, tf)
	if err != nil {
		return err
	}

	vs := file
	if !isDeckhouse {
		tmp := ModuleVersion{}
		err = json.Unmarshal([]byte(file), &tmp)
		if err != nil {
			return err
		}
		vs = tmp.Version
	}
	fmt.Printf("version of module %s: %s\n", module, file)
	version, err := semver.Parse(vs)
	if err != nil {
		return fmt.Errorf("can't parse module %s version %s", module, version )
	}
	if !prange(version) {
		return fmt.Errorf("module %s version %s is not in range %s", module, version, sv)
	}
	return nil
}

func CheckVersionWithRetries(channel, version, moduleName string, attempts int) error {
	client := &http.Client{
		Timeout: httpTimeout,
	}

	moduleFileLink := fmt.Sprintf(moduleFileLinkTemplate, version)
	fmt.Printf("Fetching module requirements from %s\n", moduleFileLink)
	resp, err := client.Get(moduleFileLink)
	if err != nil {
		fmt.Printf("%v\n", err)
		return fmt.Errorf("failed to fetch module file from %s: %w", moduleFileLink, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("unexpected status code %d for URL %s\n", resp.StatusCode, moduleFileLink)
		return fmt.Errorf("unexpected status code %d for URL %s\n", resp.StatusCode, moduleFileLink)
	}

	file, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("%v\n", err)
		return err
	}

	c := Config{}
	if err := yaml.Unmarshal(file, &c); err != nil {
		fmt.Printf("Failed to parse module.yaml: %v\n", err)
		return fmt.Errorf("failed to parse module.yaml: %w", err)
	}
	fmt.Printf("Parsed requirements: deckhouse=%q, modules=%d\n", c.Requirements.Deckhouse, len(c.Requirements.Modules))

	// skip module check for now
	// for k, v := range c.Requirements.Modules { fmt.Printf("Verifying module %s (range %q) on channel %s version %s\n", k, v, channel, version)
	// 	err = VerifyModuleRequirements(k, v, channel, version)
	// 	if err != nil {
	// 		fmt.Printf("verifying module %s on channel %s and version %s failed: %s\n", k, channel, version, err)
	// 		return err
	// 	}
	// }

	var supportedEditions = []string{"fe", "ee", "ce", "se-plus"}
	for _, e := range supportedEditions {
		fmt.Printf("Verifying deckhouse (range %q) on channel %s version %s\n", c.Requirements.Deckhouse, channel, version)
		err = VerifyModuleRequirements("deckhouse", c.Requirements.Deckhouse, e, channel, version)
		if err != nil {
			fmt.Printf("verifying module %s on channel %s and version %s failed: %s\n", "deckhouse", channel, version, err)
			return err
		}
	}

	return nil
}
