/*
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
*/

package route

// We save the generated code in this directory because bpf2go generates non-exported functions and structures.
//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -target amd64 -type route_event ebpf ../../../bpf/route_watcher.c
