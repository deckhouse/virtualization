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

package api

type BlockJobsStatus struct {
	Return []BlockJobStatus `json:"return"`
	ID     string           `json:"id"`
}

type BlockJobStatus struct {
	AutoFinalize   bool   `json:"auto-finalize"`
	IOStatus       string `json:"io-status"`
	Device         string `json:"device"`
	AutoDismiss    bool   `json:"auto-dismiss"`
	Busy           bool   `json:"busy"`
	Len            int64  `json:"len"`
	Offset         int64  `json:"offset"`
	Status         string `json:"status"`
	Paused         bool   `json:"paused"`
	Speed          int64  `json:"speed"`
	Ready          bool   `json:"ready"`
	Type           string `json:"type"`
	ActivelySynced bool   `json:"actively-synced"`
	Error          string `json:"error"`
}

type JobsStatus struct {
	Return []JobStatus `json:"return"`
	ID     string      `json:"id"`
}

type JobStatus struct {
	CurrentProgress int64  `json:"current-progress"`
	Status          string `json:"status"`
	TotalProgress   int64  `json:"total-progress"`
	Type            string `json:"type"`
	ID              string `json:"id"`
	Error           string `json:"error"`
}
