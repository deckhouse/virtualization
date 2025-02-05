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

package internal

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type nodeNamesDiffTestParams struct {
	prev    []string
	current []string
	added   []string
	removed []string
}

var _ = DescribeTable(
	"DiscoveryHandler NodeNamesDiff Test",
	func(params nodeNamesDiffTestParams) {
		calculatedAdded, calculatedRemoved := NodeNamesDiff(params.prev, params.current)
		Expect(calculatedAdded).Should(Equal(params.added))
		Expect(calculatedRemoved).Should(Equal(params.removed))
	},
	Entry(
		"Should be no diff",
		nodeNamesDiffTestParams{
			prev: []string{
				"node1",
				"node2",
			},
			current: []string{
				"node1",
				"node2",
			},
			added:   []string{},
			removed: []string{},
		},
	),
	Entry(
		"Should be added node",
		nodeNamesDiffTestParams{
			prev: []string{
				"node1",
				"node2",
			},
			current: []string{
				"node1",
				"node2",
				"node3",
			},
			added: []string{
				"node3",
			},
			removed: []string{},
		},
	),
	Entry(
		"Should be removed node",
		nodeNamesDiffTestParams{
			prev: []string{
				"node1",
				"node2",
				"node3",
			},
			current: []string{
				"node1",
				"node2",
			},
			added: []string{},
			removed: []string{
				"node3",
			},
		},
	),
	Entry(
		"Should be added and removed node",
		nodeNamesDiffTestParams{
			prev: []string{
				"node1",
				"node2",
			},
			current: []string{
				"node2",
				"node3",
			},
			added: []string{
				"node3",
			},
			removed: []string{
				"node1",
			},
		},
	),
	Entry(
		"Should be multiple added and removed node",
		nodeNamesDiffTestParams{
			prev: []string{
				"node3",
				"node4",
				"node5",
			},
			current: []string{
				"node1",
				"node2",
				"node3",
			},
			added: []string{
				"node1",
				"node2",
			},
			removed: []string{
				"node4",
				"node5",
			},
		},
	),
)
