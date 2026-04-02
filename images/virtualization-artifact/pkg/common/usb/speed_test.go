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

package usb

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
)

func TestSpeed(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "USB speed package tests")
}

var _ = Describe("ResolveSpeed", func() {
	DescribeTable("returns correct hub type for speed",
		func(speed int, expectedHS, expectedSS bool) {
			isHS, isSS := ResolveSpeed(speed)
			Expect(isHS).To(Equal(expectedHS))
			Expect(isSS).To(Equal(expectedSS))
		},
		Entry("low speed 1.0", 1, false, false),
		Entry("full speed 1.1", 12, false, false),
		Entry("high speed 2.0", 480, true, false),
		Entry("wireless 2.5", 2500, false, false),
		Entry("super speed 3.0", 5000, false, true),
		Entry("super speed 3.1", 10000, false, true),
		Entry("super speed 3.2", 20000, false, true),
		Entry("super speed plus 3.1", 10000, false, true),
	)
})

var _ = Describe("CheckFreePort", func() {
	DescribeTable("returns correct availability",
		func(speed int, nodeAnnotations map[string]string, expectedResult, expectError bool) {
			result, err := CheckFreePort(nodeAnnotations, speed)
			if expectError {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(expectedResult))
			}
		},
		Entry("HS speed, enough ports",
			480,
			map[string]string{
				annotations.AnnUSBIPTotalPorts:             "8",
				annotations.AnnUSBIPHighSpeedHubUsedPorts:  "3",
				annotations.AnnUSBIPSuperSpeedHubUsedPorts: "0",
			},
			true, false,
		),
		Entry("HS speed, no ports left",
			480,
			map[string]string{
				annotations.AnnUSBIPTotalPorts:             "8",
				annotations.AnnUSBIPHighSpeedHubUsedPorts:  "4",
				annotations.AnnUSBIPSuperSpeedHubUsedPorts: "0",
			},
			false, false,
		),
		Entry("SS speed, enough ports",
			5000,
			map[string]string{
				annotations.AnnUSBIPTotalPorts:             "8",
				annotations.AnnUSBIPHighSpeedHubUsedPorts:  "4",
				annotations.AnnUSBIPSuperSpeedHubUsedPorts: "2",
			},
			true, false,
		),
		Entry("SS speed, no ports left",
			10000,
			map[string]string{
				annotations.AnnUSBIPTotalPorts:             "8",
				annotations.AnnUSBIPHighSpeedHubUsedPorts:  "4",
				annotations.AnnUSBIPSuperSpeedHubUsedPorts: "4",
			},
			false, false,
		),
		Entry("unsupported speed",
			12,
			map[string]string{
				annotations.AnnUSBIPTotalPorts:             "8",
				annotations.AnnUSBIPHighSpeedHubUsedPorts:  "0",
				annotations.AnnUSBIPSuperSpeedHubUsedPorts: "0",
			},
			false, true,
		),
		Entry("missing total ports annotation",
			480,
			map[string]string{
				annotations.AnnUSBIPHighSpeedHubUsedPorts:  "0",
				annotations.AnnUSBIPSuperSpeedHubUsedPorts: "0",
			},
			false, true,
		),
		Entry("missing HS hub annotation",
			480,
			map[string]string{
				annotations.AnnUSBIPTotalPorts:             "8",
				annotations.AnnUSBIPSuperSpeedHubUsedPorts: "0",
			},
			false, true,
		),
		Entry("missing SS hub annotation",
			5000,
			map[string]string{
				annotations.AnnUSBIPTotalPorts:            "8",
				annotations.AnnUSBIPHighSpeedHubUsedPorts: "0",
			},
			false, true,
		),
	)
})

var _ = Describe("CheckFreePortForRequest", func() {
	DescribeTable("returns correct availability for request",
		func(speed int, nodeAnnotations map[string]string, requestedCount int, expectedResult, expectError bool) {
			result, err := CheckFreePortForRequest(nodeAnnotations, speed, requestedCount)
			if expectError {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(expectedResult))
			}
		},
		Entry("HS speed, request 1, enough ports",
			480,
			map[string]string{
				annotations.AnnUSBIPTotalPorts:             "8",
				annotations.AnnUSBIPHighSpeedHubUsedPorts:  "3",
				annotations.AnnUSBIPSuperSpeedHubUsedPorts: "0",
			},
			1, true, false,
		),
		Entry("HS speed, request 2, no ports",
			480,
			map[string]string{
				annotations.AnnUSBIPTotalPorts:             "8",
				annotations.AnnUSBIPHighSpeedHubUsedPorts:  "3",
				annotations.AnnUSBIPSuperSpeedHubUsedPorts: "0",
			},
			2, false, false,
		),
		Entry("HS speed, request 2, exactly at limit",
			480,
			map[string]string{
				annotations.AnnUSBIPTotalPorts:             "8",
				annotations.AnnUSBIPHighSpeedHubUsedPorts:  "2",
				annotations.AnnUSBIPSuperSpeedHubUsedPorts: "0",
			},
			2, true, false,
		),
	)
})

var _ = Describe("GetTotalPortsPerHub", func() {
	DescribeTable("returns correct total ports per hub",
		func(nodeAnnotations map[string]string, expectedResult int, expectError bool) {
			result, err := GetTotalPortsPerHub(nodeAnnotations)
			if expectError {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(expectedResult))
			}
		},
		Entry("total 8 ports",
			map[string]string{annotations.AnnUSBIPTotalPorts: "8"},
			4, false,
		),
		Entry("total 4 ports",
			map[string]string{annotations.AnnUSBIPTotalPorts: "4"},
			2, false,
		),
		Entry("missing annotation",
			map[string]string{},
			0, true,
		),
		Entry("invalid value",
			map[string]string{annotations.AnnUSBIPTotalPorts: "invalid"},
			0, true,
		),
	)
})

var _ = Describe("GetUsedPorts", func() {
	DescribeTable("returns correct used ports",
		func(nodeAnnotations map[string]string, hubAnnotation string, expectedResult int, expectError bool) {
			result, err := GetUsedPorts(nodeAnnotations, hubAnnotation)
			if expectError {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(expectedResult))
			}
		},
		Entry("HS hub, 3 used",
			map[string]string{annotations.AnnUSBIPHighSpeedHubUsedPorts: "3"},
			annotations.AnnUSBIPHighSpeedHubUsedPorts,
			3, false,
		),
		Entry("SS hub, 0 used",
			map[string]string{annotations.AnnUSBIPSuperSpeedHubUsedPorts: "0"},
			annotations.AnnUSBIPSuperSpeedHubUsedPorts,
			0, false,
		),
		Entry("missing annotation",
			map[string]string{},
			annotations.AnnUSBIPHighSpeedHubUsedPorts,
			0, true,
		),
		Entry("invalid value",
			map[string]string{annotations.AnnUSBIPHighSpeedHubUsedPorts: "invalid"},
			annotations.AnnUSBIPHighSpeedHubUsedPorts,
			0, true,
		),
	)
})
