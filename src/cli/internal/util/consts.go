/*
Copyright 2018 The KubeVirt Authors
Copyright 2024 Flant JSC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

Initially copied from https://github.com/kubevirt/kubevirt/blob/main/pkg/virtctl/utils/utils.go
*/

package util

const (
	CloseGoingAwayMessage       = "\r\nYou were disconnected from the console. This has one of the following reasons:\r\n - another user connected to the console of the target vm\r\n"
	CloseAbnormalClosureMessage = "\r\nYou were disconnected from the console. This has one of the following reasons:\r\n - network issues\r\n - machine restart\r\n"
)
