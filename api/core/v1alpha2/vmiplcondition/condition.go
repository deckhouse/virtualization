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

package vmiplcondition

type Type string

const (
	// BoundType represents the condition type when a Virtual Machine IP is bound.
	BoundType Type = "Bound"
)

func (t Type) String() string {
	return string(t)
}

// BoundReason represents specific reasons for the 'Bound' condition type.
type BoundReason string

func (r BoundReason) String() string {
	return string(r)
}

const (
	BoundReasonUnknown BoundReason = "Unknown"

	// Released is a BoundReason indicating the IP address lease has been released.
	Released BoundReason = "Released"

	// Bound is a BoundReason indicating the IP address lease is successfully bound.
	Bound BoundReason = "Bound"
)
