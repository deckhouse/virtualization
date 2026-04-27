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

package precheck

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	. "github.com/onsi/ginkgo/v2"

	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
)

const (
	modulePhaseReady = "Ready"
)

const (
	LabelsFile = "/tmp/e2e-specs.json"
)

// specInfo represents a test spec with its location and labels.
type specInfo struct {
	Location string   `json:"location"`
	Labels   []string `json:"labels"`
}

// ginkgoReport represents the structure of Ginkgo JSON report.
type ginkgoReport struct {
	SpecReports []specReport `json:"SpecReports"`
}

// specReport represents a spec in Ginkgo JSON report.
type specReport struct {
	ContainerHierarchyTexts  []string   `json:"ContainerHierarchyTexts"`
	ContainerHierarchyLabels [][]string `json:"ContainerHierarchyLabels"`
	LeafNodeText             string     `json:"LeafNodeText"`
	LeafNodeType             string     `json:"LeafNodeType"`
}

// Precheck defines interface for precheck implementations.
type Precheck interface {
	// Label returns the precheck label that tests must use to require this precheck.
	Label() string

	// Run executes the precheck.
	// Returns error if precheck fails.
	Run(ctx context.Context, f *framework.Framework) error
}

// registeredPrechecks holds all registered precheck implementations.
var registeredPrechecks = make(map[string]Precheck)

// commonPrechecks are prechecks that run for all tests (no label required).
var commonPrechecks []Precheck

// specLabels stores labels collected from all specs (loaded from file).
var specLabels []string

// RegisterPrecheck registers a precheck implementation.
// If isCommon is true, the precheck runs for all tests regardless of labels.
func RegisterPrecheck(p Precheck, isCommon bool) {
	registeredPrechecks[p.Label()] = p
	if isCommon {
		commonPrechecks = append(commonPrechecks, p)
	}
}

// LoadSpecLabelsFromFile loads spec labels from file and filters by labelFilter.
// Called from SynchronizedBeforeSuite.
func LoadSpecLabelsFromFile(filename, labelFilter string) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return
	}

	// Parse Ginkgo JSON report
	var reports []ginkgoReport
	if err := json.Unmarshal(data, &reports); err != nil {
		return
	}

	// Convert to specInfo
	var allSpecs []specInfo
	for _, report := range reports {
		for _, r := range report.SpecReports {
			if r.LeafNodeType == "" || r.LeafNodeType == "ReportBeforeSuite" || r.LeafNodeType == "ReportAfterSuite" {
				continue
			}

			location := ""
			if len(r.ContainerHierarchyTexts) > 0 {
				location = r.ContainerHierarchyTexts[0]
				for i := 1; i < len(r.ContainerHierarchyTexts); i++ {
					location += " / " + r.ContainerHierarchyTexts[i]
				}
				if r.LeafNodeText != "" {
					location += " - " + r.LeafNodeText
				}
			}

			if location == "" {
				continue
			}

			var labels []string
			for _, levelLabels := range r.ContainerHierarchyLabels {
				labels = append(labels, levelLabels...)
			}

			allSpecs = append(allSpecs, specInfo{
				Location: location,
				Labels:   labels,
			})
		}
	}

	// Filter specs based on FOCUS or LABELS filter.
	// FOCUS filters by spec location (description), LABELS filters by labels.
	// Parameter labelFilter takes precedence over LABELS env var.
	focusRegex := os.Getenv("FOCUS")
	if labelFilter == "" {
		labelFilter = os.Getenv("LABELS")
	}

	filteredSpecs := allSpecs
	if focusRegex != "" || labelFilter != "" {
		filteredSpecs = filterSpecs(allSpecs, focusRegex, labelFilter)
	}

	// Collect precheck labels only from filtered specs
	labelSet := make(map[string]bool)
	for _, spec := range filteredSpecs {
		for _, label := range spec.Labels {
			if _, ok := registeredPrechecks[label]; ok {
				labelSet[label] = true
			}
		}
	}

	for label := range labelSet {
		specLabels = append(specLabels, label)
	}
}

// filterSpecs filters specs by FOCUS (regex on location) and LABELS (comma-separated labels).
func filterSpecs(specs []specInfo, focusRegex, labelFilter string) []specInfo {
	var filtered []specInfo

	var focus *regexp.Regexp
	var err error
	if focusRegex != "" {
		focus, err = regexp.Compile(focusRegex)
		if err != nil {
			// Invalid regex, skip focus filter
			focus = nil
		}
	}

	// Parse label filter (comma-separated, supports ~ for negated)
	requiredLabels := make(map[string]bool)
	negatedLabels := make(map[string]bool)
	if labelFilter != "" {
		for _, l := range strings.Split(labelFilter, ",") {
			l = strings.TrimSpace(l)
			if strings.HasPrefix(l, "~") {
				negatedLabels[l[1:]] = true
			} else {
				requiredLabels[l] = true
			}
		}
	}

	for _, spec := range specs {
		// Check FOCUS filter
		if focus != nil && !focus.MatchString(spec.Location) {
			continue
		}

		// Check LABELS filter
		if len(requiredLabels) > 0 {
			matched := false
			for _, sl := range spec.Labels {
				if requiredLabels[sl] {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}

		// Check negated labels (~label)
		hasNegated := false
		for _, sl := range spec.Labels {
			if negatedLabels[sl] {
				hasNegated = true
				break
			}
		}
		if hasNegated {
			continue
		}

		filtered = append(filtered, spec)
	}

	return filtered
}

// ValidateFromJSONFile validates specs from Ginkgo JSON report.
// This is used with ginkgo --json-report flag.
func ValidateFromJSONFile(filename string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read spec file: %w", err)
	}

	// Ginkgo JSON report is an array of suite reports
	var reports []ginkgoReport
	if err := json.Unmarshal(data, &reports); err != nil {
		// Debug: show first 200 chars of data
		dataStr := string(data)
		if len(dataStr) > 200 {
			dataStr = dataStr[:200]
		}
		return fmt.Errorf("failed to parse ginkgo report: %w, data: %s", err, dataStr)
	}

	// Convert ginkgo spec reports to our format
	var specs []specInfo
	for _, report := range reports {
		for _, r := range report.SpecReports {
			// Skip non-spec entries like ReportBeforeSuite, ReportAfterSuite
			if r.LeafNodeType == "" || r.LeafNodeType == "ReportBeforeSuite" || r.LeafNodeType == "ReportAfterSuite" {
				continue
			}

			location := ""
			if len(r.ContainerHierarchyTexts) > 0 {
				location = r.ContainerHierarchyTexts[0]
				for i := 1; i < len(r.ContainerHierarchyTexts); i++ {
					location += " / " + r.ContainerHierarchyTexts[i]
				}
				if r.LeafNodeText != "" {
					location += " - " + r.LeafNodeText
				}
			}

			if location == "" {
				continue
			}

			// Flatten labels from all hierarchy levels
			var labels []string
			for _, levelLabels := range r.ContainerHierarchyLabels {
				labels = append(labels, levelLabels...)
			}

			specs = append(specs, specInfo{
				Location: location,
				Labels:   labels,
			})
		}
	}

	return validateSpecs(specs)
}

func validateSpecs(specs []specInfo) error {
	var missingSpecs []string

	for _, spec := range specs {
		hasPrecheckLabel := false
		for _, label := range spec.Labels {
			if IsPrecheckLabel(label) {
				hasPrecheckLabel = true
				break
			}
		}
		if !hasPrecheckLabel {
			missingSpecs = append(missingSpecs, spec.Location)
		}
	}

	if len(missingSpecs) > 0 {
		display := missingSpecs
		if len(display) > 20 {
			display = append(display[:20], fmt.Sprintf("... and %d more", len(missingSpecs)-20))
		}
		return fmt.Errorf("found %d specs without precheck labels:\n%s\n\n"+
			"Add Label(precheck.NoPrecheck) or Label(precheck.PrecheckXXX) to your Describe/It",
			len(missingSpecs), strings.Join(display, "\n"))
	}

	fmt.Printf("PASS: all %d specs have precheck labels\n", len(specs))
	return nil
}

// Run executes prechecks based on loaded spec labels.
func Run(f *framework.Framework, labelFilter string) {
	// Run common prechecks first (always run)
	for _, p := range commonPrechecks {
		_, _ = GinkgoWriter.Write([]byte("Running common precheck: " + p.Label() + "\n"))
		if err := p.Run(NewContext(), f); err != nil {
			Fail("common precheck " + p.Label() + " failed: " + err.Error())
		}
	}

	// Run prechecks for loaded labels
	for _, label := range specLabels {
		p := registeredPrechecks[label]
		if p == nil {
			continue
		}
		_, _ = GinkgoWriter.Write([]byte("Running precheck: " + label + "\n"))
		if err := p.Run(NewContext(), f); err != nil {
			Fail("precheck " + label + " failed: " + err.Error())
		}
	}
}

// isCheckEnabled returns true if the precheck is not disabled.
func isCheckEnabled(envName string) bool {
	if os.Getenv("PRECHECK") == "no" {
		return false
	}
	return os.Getenv(envName) != "no"
}

// IsModuleEnabled checks if a Deckhouse module is enabled.
// Returns true if the module exists and is enabled (Spec.Enabled = true).
func IsModuleEnabled(ctx context.Context, f *framework.Framework, moduleName string) bool {
	module, err := f.GetModuleConfig(ctx, moduleName)
	if err != nil {
		_, _ = fmt.Fprintf(GinkgoWriter, "failed to get %s module config: %v\n", moduleName, err)
		return false
	}
	enabled := module.Spec.Enabled
	return enabled != nil && *enabled
}
