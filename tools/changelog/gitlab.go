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
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// GitLab is the REST API of the project the pipeline runs in.
type GitLab struct {
	BaseURL   string
	Token     string
	ProjectID string

	client *http.Client
}

func NewGitLab(baseURL, token, projectID string) *GitLab {
	return &GitLab{
		BaseURL:   strings.TrimRight(baseURL, "/"),
		Token:     token,
		ProjectID: projectID,
		client:    &http.Client{Timeout: 60 * time.Second},
	}
}

// MergeRequest is what the changelog needs to know about a merge request.
type MergeRequest struct {
	IID         int    `json:"iid"`
	Title       string `json:"title"`
	Description string `json:"description"`
	WebURL      string `json:"web_url"`
	Labels      Labels `json:"labels"`
}

// Milestone is a release the changelog is generated for.
type Milestone struct {
	ID    int    `json:"id"`
	IID   int    `json:"iid"`
	Title string `json:"title"`
}

// Labels reads the labels of a merge request in all three shapes the API uses:
// a list of names, a list of label objects (when label details were asked for),
// and the comma-separated string a merge request is created with, which some
// answers echo back.
type Labels []string

func (l *Labels) UnmarshalJSON(data []byte) error {
	var names []string
	if err := json.Unmarshal(data, &names); err == nil {
		*l = names
		return nil
	}
	var joined string
	if err := json.Unmarshal(data, &joined); err == nil {
		*l = nil
		for _, name := range strings.Split(joined, ",") {
			if name = strings.TrimSpace(name); name != "" {
				*l = append(*l, name)
			}
		}
		return nil
	}
	var details []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(data, &details); err != nil {
		return err
	}
	*l = nil
	for _, detail := range details {
		*l = append(*l, detail.Name)
	}
	return nil
}

func (l Labels) Has(name string) bool {
	for _, label := range l {
		if label == name {
			return true
		}
	}
	return false
}

// MergedMergeRequests returns every merged merge request of a milestone, oldest
// first.
func (g *GitLab) MergedMergeRequests(milestone string) ([]MergeRequest, error) {
	return listAll[MergeRequest](g, g.projectPath("merge_requests"), url.Values{
		"state":     {"merged"},
		"milestone": {milestone},
		"per_page":  {"100"},
		"order_by":  {"created_at"},
		"sort":      {"asc"},
	})
}

// OpenMergeRequests returns the open merge requests between two branches.
func (g *GitLab) OpenMergeRequests(sourceBranch, targetBranch string) ([]MergeRequest, error) {
	return listAll[MergeRequest](g, g.projectPath("merge_requests"), url.Values{
		"source_branch": {sourceBranch},
		"target_branch": {targetBranch},
		"state":         {"opened"},
	})
}

// ActiveMilestones returns the milestones that are still open.
func (g *GitLab) ActiveMilestones() ([]Milestone, error) {
	return listAll[Milestone](g, g.projectPath("milestones"), url.Values{
		"state":    {"active"},
		"per_page": {"100"},
	})
}

// MergeRequestDescription reads the description of one merge request.
func (g *GitLab) MergeRequestDescription(iid string) (string, error) {
	body, err := g.do(http.MethodGet, g.BaseURL+g.projectPath("merge_requests/"+iid), nil)
	if err != nil {
		return "", err
	}
	var mr MergeRequest
	if err := json.Unmarshal(body, &mr); err != nil {
		return "", err
	}
	return strings.TrimSpace(mr.Description), nil
}

// CreateMergeRequest opens a merge request and returns it.
func (g *GitLab) CreateMergeRequest(payload map[string]any) (MergeRequest, error) {
	var created MergeRequest
	body, err := g.do(http.MethodPost, g.BaseURL+g.projectPath("merge_requests"), payload)
	if err != nil {
		return created, err
	}
	return created, json.Unmarshal(body, &created)
}

// UpdateMergeRequestDescription rewrites the description of a merge request.
func (g *GitLab) UpdateMergeRequestDescription(iid int, description string) error {
	path := fmt.Sprintf("%s%s", g.BaseURL, g.projectPath(fmt.Sprintf("merge_requests/%d", iid)))
	_, err := g.do(http.MethodPut, path, map[string]any{"description": description})
	return err
}

func (g *GitLab) projectPath(suffix string) string {
	return fmt.Sprintf("/projects/%s/%s", url.PathEscape(g.ProjectID), suffix)
}

// listAll walks every page of a list endpoint. GitLab points at the next one
// through the Link header (RFC 5988), so the caller never counts pages.
func listAll[T any](g *GitLab, path string, params url.Values) ([]T, error) {
	next := g.BaseURL + path
	if encoded := params.Encode(); encoded != "" {
		next += "?" + encoded
	}

	var all []T
	for next != "" {
		request, err := http.NewRequest(http.MethodGet, next, nil)
		if err != nil {
			return nil, err
		}
		request.Header.Set("PRIVATE-TOKEN", g.Token)
		request.Header.Set("Accept", "application/json")

		response, err := g.client.Do(request)
		if err != nil {
			return nil, err
		}
		body, err := readResponse(response)
		_ = response.Body.Close()
		if err != nil {
			return nil, err
		}
		var page []T
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, err
		}
		all = append(all, page...)
		next = nextLink(response.Header.Get("Link"))
	}
	return all, nil
}

func (g *GitLab) do(method, url string, payload any) ([]byte, error) {
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	request.Header.Set("PRIVATE-TOKEN", g.Token)
	request.Header.Set("Accept", "application/json")
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := g.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()
	return readResponse(response)
}

// readResponse reads the body of a response the caller closes.
func readResponse(response *http.Response) ([]byte, error) {
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	if response.StatusCode >= 300 {
		// The body of a GitLab error says which field it refused; the status
		// alone leaves a job log with nothing to act on.
		return nil, fmt.Errorf("%s %s: HTTP %d: %s",
			response.Request.Method, response.Request.URL.Path, response.StatusCode,
			strings.TrimSpace(string(body)))
	}
	return body, nil
}

var linkRE = regexp.MustCompile(`<([^>]+)>;\s*rel="([^"]+)"`)

func nextLink(header string) string {
	for _, part := range strings.Split(header, ",") {
		match := linkRE.FindStringSubmatch(strings.TrimSpace(part))
		if match != nil && match[2] == "next" {
			return match[1]
		}
	}
	return ""
}
