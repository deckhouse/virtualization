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

package service_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"

	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("SizePolicyService", func() {
	Context("when VM's class has no valid size policy", func() {
		// Virtual machine with non-matching CPU parameters
		vm := &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{
				VirtualMachineClassName: "vmclasstest",
				CPU:                     v1alpha2.CPUSpec{Cores: 5, CoreFraction: "10%"},
			},
		}

		// Initialize a virtual machine class with policies that do not match the VM's parameters
		vmClass := &v1alpha2.VirtualMachineClass{
			Spec: v1alpha2.VirtualMachineClassSpec{
				SizingPolicies: []v1alpha2.SizingPolicy{
					{
						Cores: &v1alpha2.SizingPolicyCores{Min: 1, Max: 4},
					},
				},
			},
		}

		It("should fail validation due to invalid size policy", func() {
			service := service.NewSizePolicyService()
			err := service.CheckVMMatchedSizePolicy(vm, vmClass)
			// Expect an error because the policies do not meet the VM's requirements
			Expect(err).ShouldNot(BeNil())
		})
	})

	Context("when VM's class has correct policy without memory requirements", func() {
		// Virtual machine with appropriate CPU parameters and no memory requirements
		vm := &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{
				VirtualMachineClassName: "vmclasstest",
				CPU:                     v1alpha2.CPUSpec{Cores: 1, CoreFraction: "10%"},
			},
		}

		// Set mock VM class data with valid policies for the VM without memory requirements
		vmClass := &v1alpha2.VirtualMachineClass{
			Spec: v1alpha2.VirtualMachineClassSpec{
				SizingPolicies: []v1alpha2.SizingPolicy{
					{
						Cores: &v1alpha2.SizingPolicyCores{Min: 1, Max: 4},
					},
				},
			},
		}

		It("should pass validation due to lack of memory requirements", func() {
			service := service.NewSizePolicyService()
			err := service.CheckVMMatchedSizePolicy(vm, vmClass)
			// Expect no errors since there are no memory requirements
			Expect(err).Should(BeNil())
		})
	})

	Context("when VM's memory does not match with policy", func() {
		// Virtual machine with non-matching memory parameters
		vm := &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{
				VirtualMachineClassName: "vmclasstest",
				CPU:                     v1alpha2.CPUSpec{Cores: 1, CoreFraction: "10%"},
				Memory:                  v1alpha2.MemorySpec{Size: resource.MustParse("1Gi")},
			},
		}

		// Set mock VM class data with policies that match memory requirements for the VM
		vmClass := &v1alpha2.VirtualMachineClass{
			Spec: v1alpha2.VirtualMachineClassSpec{
				SizingPolicies: []v1alpha2.SizingPolicy{
					{
						Cores: &v1alpha2.SizingPolicyCores{Min: 1, Max: 4},
						Memory: &v1alpha2.SizingPolicyMemory{
							MemoryMinMax: v1alpha2.MemoryMinMax{
								Min: ptr.To(resource.MustParse("512Mi")),
								Max: ptr.To(resource.MustParse("2Gi")),
							},
						},
					},
				},
			},
		}

		It("should pass validation due to matching memory size", func() {
			service := service.NewSizePolicyService()
			err := service.CheckVMMatchedSizePolicy(vm, vmClass)
			// Expect no errors because the memory size does not match the policy
			Expect(err).Should(BeNil())
		})
	})

	Context("when VM's memory matches the policy", func() {
		// Virtual machine with matching memory parameters
		vm := &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{
				VirtualMachineClassName: "vmclasstest",
				CPU:                     v1alpha2.CPUSpec{Cores: 1, CoreFraction: "10%"},
				Memory:                  v1alpha2.MemorySpec{Size: resource.MustParse("2Gi")},
			},
		}

		// Set mock VM class data with valid memory policies for the VM
		vmClass := &v1alpha2.VirtualMachineClass{
			Spec: v1alpha2.VirtualMachineClassSpec{
				SizingPolicies: []v1alpha2.SizingPolicy{
					{
						Cores: &v1alpha2.SizingPolicyCores{Min: 1, Max: 4},
						Memory: &v1alpha2.SizingPolicyMemory{
							MemoryMinMax: v1alpha2.MemoryMinMax{
								Min: ptr.To(resource.MustParse("1Gi")),
								Max: ptr.To(resource.MustParse("3Gi")),
							},
						},
					},
				},
			},
		}

		It("should pass validation due to matched memory size", func() {
			service := service.NewSizePolicyService()
			err := service.CheckVMMatchedSizePolicy(vm, vmClass)
			// Expect no errors because the memory size matches the policy
			Expect(err).Should(BeNil())
		})
	})

	Context("when class policy has empty memory requirements", func() {
		// Virtual machine with memory size that matches an empty memory requirement policy
		vm := &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{
				VirtualMachineClassName: "vmclasstest",
				CPU:                     v1alpha2.CPUSpec{Cores: 1, CoreFraction: "10%"},
				Memory:                  v1alpha2.MemorySpec{Size: resource.MustParse("2Gi")},
			},
		}

		vmClass := &v1alpha2.VirtualMachineClass{
			Spec: v1alpha2.VirtualMachineClassSpec{
				// No specific memory policies defined
				SizingPolicies: []v1alpha2.SizingPolicy{
					{
						Cores:  &v1alpha2.SizingPolicyCores{Min: 1, Max: 4},
						Memory: &v1alpha2.SizingPolicyMemory{},
					},
				},
			},
		}

		It("should pass validation due to lack of memory requirements", func() {
			service := service.NewSizePolicyService()
			err := service.CheckVMMatchedSizePolicy(vm, vmClass)
			// Expect no errors because there are no memory requirements
			Expect(err).Should(BeNil())
		})
	})

	Context("when VM's memory is correct per core", func() {
		// Virtual machine with memory size that adheres to per-core memory policies
		vm := &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{
				VirtualMachineClassName: "vmclasstest",
				CPU:                     v1alpha2.CPUSpec{Cores: 2, CoreFraction: "10%"},
				Memory:                  v1alpha2.MemorySpec{Size: resource.MustParse("4Gi")},
			},
		}

		vmClass := &v1alpha2.VirtualMachineClass{
			Spec: v1alpha2.VirtualMachineClassSpec{
				// Setting policies with per-core memory requirements
				SizingPolicies: []v1alpha2.SizingPolicy{
					{
						Cores: &v1alpha2.SizingPolicyCores{Min: 1, Max: 4},
						Memory: &v1alpha2.SizingPolicyMemory{
							PerCore: &v1alpha2.SizingPolicyMemoryPerCore{
								MemoryMinMax: v1alpha2.MemoryMinMax{
									Min: ptr.To(resource.MustParse("1Gi")),
									Max: ptr.To(resource.MustParse("3Gi")),
								},
							},
						},
					},
				},
			},
		}

		It("should pass validation due to matched per-core memory size", func() {
			service := service.NewSizePolicyService()
			err := service.CheckVMMatchedSizePolicy(vm, vmClass)
			// Expect no errors because the per-core memory size matches the policy
			Expect(err).Should(BeNil())
		})
	})

	Context("when VM's memory is incorrect per core", func() {
		// Virtual machine with incorrect per-core memory size
		vm := &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{
				VirtualMachineClassName: "vmclasstest",
				CPU:                     v1alpha2.CPUSpec{Cores: 4, CoreFraction: "10%"},
				Memory:                  v1alpha2.MemorySpec{Size: resource.MustParse("4Gi")},
			},
		}

		// Set mock VM class data with invalid per-core memory policies for the VM
		vmClass := &v1alpha2.VirtualMachineClass{
			Spec: v1alpha2.VirtualMachineClassSpec{
				SizingPolicies: []v1alpha2.SizingPolicy{
					{
						Cores: &v1alpha2.SizingPolicyCores{Min: 1, Max: 4},
						Memory: &v1alpha2.SizingPolicyMemory{
							PerCore: &v1alpha2.SizingPolicyMemoryPerCore{
								MemoryMinMax: v1alpha2.MemoryMinMax{
									Min: ptr.To(resource.MustParse("2Gi")),
									Max: ptr.To(resource.MustParse("3Gi")),
								},
							},
						},
					},
				},
			},
		}

		It("should fail validation due to non-matching per-core memory size", func() {
			service := service.NewSizePolicyService()
			err := service.CheckVMMatchedSizePolicy(vm, vmClass)
			// Expect an error because the per-core memory size does not match the policy
			Expect(err).ShouldNot(BeNil())
		})
	})

	Context("when VM's core fraction is correct", func() {
		// Virtual machine with a correct core fraction
		vm := &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{
				VirtualMachineClassName: "vmclasstest",
				CPU:                     v1alpha2.CPUSpec{Cores: 1, CoreFraction: "10%"},
				Memory:                  v1alpha2.MemorySpec{Size: resource.MustParse("2Gi")},
			},
		}

		// Set mock VM class data with valid core fraction policies for the VM
		vmClass := &v1alpha2.VirtualMachineClass{
			Spec: v1alpha2.VirtualMachineClassSpec{
				SizingPolicies: []v1alpha2.SizingPolicy{
					{
						Cores:         &v1alpha2.SizingPolicyCores{Min: 1, Max: 4},
						CoreFractions: []v1alpha2.CoreFractionValue{10, 25, 50, 100},
					},
				},
			},
		}

		It("should pass validation due to matching core fraction", func() {
			service := service.NewSizePolicyService()
			err := service.CheckVMMatchedSizePolicy(vm, vmClass)
			// Expect no errors because the core fraction matches the policy
			Expect(err).Should(BeNil())
		})
	})

	Context("when VM's core fraction is incorrect", func() {
		// Virtual machine with an incorrect core fraction
		vm := &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{
				VirtualMachineClassName: "vmclasstest",
				CPU:                     v1alpha2.CPUSpec{Cores: 1, CoreFraction: "11%"},
				Memory:                  v1alpha2.MemorySpec{Size: resource.MustParse("2Gi")},
			},
		}

		// Set mock VM class data with valid core fraction policies for the VM
		vmClass := &v1alpha2.VirtualMachineClass{
			Spec: v1alpha2.VirtualMachineClassSpec{
				SizingPolicies: []v1alpha2.SizingPolicy{
					{
						Cores:         &v1alpha2.SizingPolicyCores{Min: 1, Max: 4},
						CoreFractions: []v1alpha2.CoreFractionValue{10, 25, 50, 100},
					},
				},
			},
		}

		It("should fail validation due to non-matching core fraction", func() {
			service := service.NewSizePolicyService()
			err := service.CheckVMMatchedSizePolicy(vm, vmClass)
			// Expect an error because the core fraction does not match the policy
			Expect(err).ShouldNot(BeNil())
		})
	})

	Context("when VM's memory step is correct", func() {
		// Virtual machine with a correct memory step
		vm := &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{
				VirtualMachineClassName: "vmclasstest",
				CPU:                     v1alpha2.CPUSpec{Cores: 2, CoreFraction: "10%"},
				Memory:                  v1alpha2.MemorySpec{Size: resource.MustParse("2Gi")},
			},
		}

		// Set mock VM class data with valid memory step policies for the VM
		vmClass := &v1alpha2.VirtualMachineClass{
			Spec: v1alpha2.VirtualMachineClassSpec{
				SizingPolicies: []v1alpha2.SizingPolicy{
					{
						Cores: &v1alpha2.SizingPolicyCores{Min: 1, Max: 4},
						Memory: &v1alpha2.SizingPolicyMemory{
							Step: ptr.To(resource.MustParse("1Gi")),
							MemoryMinMax: v1alpha2.MemoryMinMax{
								Min: ptr.To(resource.MustParse("1Gi")),
								Max: ptr.To(resource.MustParse("3Gi")),
							},
						},
					},
				},
			},
		}

		It("should pass validation due to matched memory step", func() {
			service := service.NewSizePolicyService()
			err := service.CheckVMMatchedSizePolicy(vm, vmClass)
			// Expect no errors because the memory size matches the step policy
			Expect(err).Should(BeNil())
		})
	})

	Context("when VM's memory step is incorrect", func() {
		// Virtual machine with an incorrect memory step
		vm := &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{
				VirtualMachineClassName: "vmclasstest",
				CPU:                     v1alpha2.CPUSpec{Cores: 2, CoreFraction: "10%"},
				Memory:                  v1alpha2.MemorySpec{Size: resource.MustParse("2001Mi")},
			},
		}

		// Set mock VM class data with invalid memory step policies for the VM
		vmClass := &v1alpha2.VirtualMachineClass{
			Spec: v1alpha2.VirtualMachineClassSpec{
				SizingPolicies: []v1alpha2.SizingPolicy{
					{
						Cores: &v1alpha2.SizingPolicyCores{Min: 1, Max: 4},
						Memory: &v1alpha2.SizingPolicyMemory{
							Step: ptr.To(resource.MustParse("1Gi")),
							MemoryMinMax: v1alpha2.MemoryMinMax{
								Min: ptr.To(resource.MustParse("1Gi")),
								Max: ptr.To(resource.MustParse("3Gi")),
							},
						},
					},
				},
			},
		}

		It("should fail validation due to non-matching memory step", func() {
			service := service.NewSizePolicyService()
			err := service.CheckVMMatchedSizePolicy(vm, vmClass)
			// Expect an error because the memory size does not match the step policy
			Expect(err).ShouldNot(BeNil())
		})
	})

	Context("when VM's per core memory step is correct", func() {
		// Virtual machine with a correct per-core memory step
		vm := &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{
				VirtualMachineClassName: "vmclasstest",
				CPU:                     v1alpha2.CPUSpec{Cores: 2, CoreFraction: "10%"},
				Memory:                  v1alpha2.MemorySpec{Size: resource.MustParse("4Gi")},
			},
		}

		vmClass := &v1alpha2.VirtualMachineClass{
			Spec: v1alpha2.VirtualMachineClassSpec{
				SizingPolicies: []v1alpha2.SizingPolicy{
					{
						Cores: &v1alpha2.SizingPolicyCores{Min: 1, Max: 4},
						Memory: &v1alpha2.SizingPolicyMemory{
							Step: ptr.To(resource.MustParse("1Gi")),
							PerCore: &v1alpha2.SizingPolicyMemoryPerCore{
								MemoryMinMax: v1alpha2.MemoryMinMax{
									Min: ptr.To(resource.MustParse("1Gi")),
									Max: ptr.To(resource.MustParse("3Gi")),
								},
							},
						},
					},
				},
			},
		}

		It("should pass validation due to match per core memory step", func() {
			service := service.NewSizePolicyService()
			err := service.CheckVMMatchedSizePolicy(vm, vmClass)
			// Expect no errors because the per-core memory size matches the step policy
			Expect(err).Should(BeNil())
		})
	})

	Context("when VM's per core memory step is incorrect", func() {
		// Virtual machine with an incorrect per-core memory step
		vm := &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{
				VirtualMachineClassName: "vmclasstest",
				CPU:                     v1alpha2.CPUSpec{Cores: 2, CoreFraction: "10%"},
				Memory:                  v1alpha2.MemorySpec{Size: resource.MustParse("4001Mi")},
			},
		}

		// Set mock VM class data with invalid per-core memory step policies for the VM
		vmClass := &v1alpha2.VirtualMachineClass{
			Spec: v1alpha2.VirtualMachineClassSpec{
				SizingPolicies: []v1alpha2.SizingPolicy{
					{
						Cores: &v1alpha2.SizingPolicyCores{Min: 1, Max: 4},
						Memory: &v1alpha2.SizingPolicyMemory{
							Step: ptr.To(resource.MustParse("1Gi")),
							PerCore: &v1alpha2.SizingPolicyMemoryPerCore{
								MemoryMinMax: v1alpha2.MemoryMinMax{
									Min: ptr.To(resource.MustParse("1Gi")),
									Max: ptr.To(resource.MustParse("3Gi")),
								},
							},
						},
					},
				},
			},
		}

		It("should fail validation due to non-matching per core memory step", func() {
			service := service.NewSizePolicyService()
			err := service.CheckVMMatchedSizePolicy(vm, vmClass)
			// Expect an error because the per-core memory size does not match the step policy
			Expect(err).ShouldNot(BeNil())
		})
	})

	Context("When size policy not provided", func() {
		vm := &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{
				VirtualMachineClassName: "vmclasstest",
				CPU:                     v1alpha2.CPUSpec{Cores: 2, CoreFraction: "10%"},
				Memory:                  v1alpha2.MemorySpec{Size: resource.MustParse("4001Mi")},
			},
		}
		vmClass := &v1alpha2.VirtualMachineClass{}

		It("should pass validation cause no requirements", func() {
			service := service.NewSizePolicyService()
			err := service.CheckVMMatchedSizePolicy(vm, vmClass)
			Expect(err).Should(BeNil())
		})
	})

	// classWithCoresStep builds a class with a single policy that discretizes the number of cores.
	classWithCoresStep := func() *v1alpha2.VirtualMachineClass {
		return &v1alpha2.VirtualMachineClass{
			Spec: v1alpha2.VirtualMachineClassSpec{
				SizingPolicies: []v1alpha2.SizingPolicy{
					{Cores: &v1alpha2.SizingPolicyCores{Min: 1, Max: 9, Step: 4}}, // allowed: 1, 5, 9
				},
			},
		}
	}

	Context("when VM's number of cores matches the cores step", func() {
		vm := &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{
				VirtualMachineClassName: "vmclasstest",
				CPU:                     v1alpha2.CPUSpec{Cores: 5, CoreFraction: "10%"},
			},
		}

		It("should pass validation", func() {
			err := service.NewSizePolicyService().CheckVMMatchedSizePolicy(vm, classWithCoresStep())
			Expect(err).Should(BeNil())
		})
	})

	Context("when VM's number of cores does not match the cores step", func() {
		vm := &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{
				VirtualMachineClassName: "vmclasstest",
				CPU:                     v1alpha2.CPUSpec{Cores: 3, CoreFraction: "10%"},
			},
		}

		It("should fail and point at spec.cpu.cores with the nearest valid values", func() {
			err := service.NewSizePolicyService().CheckVMMatchedSizePolicy(vm, classWithCoresStep())
			Expect(err).ShouldNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("spec.cpu.cores"))
			Expect(err.Error()).To(ContainSubstring("1 or 5"))
		})
	})

	Context("when memory policy defines only min without max", func() {
		vmClass := &v1alpha2.VirtualMachineClass{
			Spec: v1alpha2.VirtualMachineClassSpec{
				SizingPolicies: []v1alpha2.SizingPolicy{
					{
						Cores: &v1alpha2.SizingPolicyCores{Min: 1, Max: 4},
						Memory: &v1alpha2.SizingPolicyMemory{
							MemoryMinMax: v1alpha2.MemoryMinMax{Min: ptr.To(resource.MustParse("2Gi"))},
						},
					},
				},
			},
		}

		It("should fail when memory is below the min (regression: min was ignored without max)", func() {
			vm := &v1alpha2.VirtualMachine{
				Spec: v1alpha2.VirtualMachineSpec{
					VirtualMachineClassName: "vmclasstest",
					CPU:                     v1alpha2.CPUSpec{Cores: 1, CoreFraction: "10%"},
					Memory:                  v1alpha2.MemorySpec{Size: resource.MustParse("512Mi")},
				},
			}
			err := service.NewSizePolicyService().CheckVMMatchedSizePolicy(vm, vmClass)
			Expect(err).ShouldNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("at least 2Gi"))
		})

		It("should pass when memory is at or above the min", func() {
			vm := &v1alpha2.VirtualMachine{
				Spec: v1alpha2.VirtualMachineSpec{
					VirtualMachineClassName: "vmclasstest",
					CPU:                     v1alpha2.CPUSpec{Cores: 1, CoreFraction: "10%"},
					Memory:                  v1alpha2.MemorySpec{Size: resource.MustParse("4Gi")},
				},
			}
			err := service.NewSizePolicyService().CheckVMMatchedSizePolicy(vm, vmClass)
			Expect(err).Should(BeNil())
		})
	})

	Context("when per-core memory policy defines only min without max", func() {
		vmClass := &v1alpha2.VirtualMachineClass{
			Spec: v1alpha2.VirtualMachineClassSpec{
				SizingPolicies: []v1alpha2.SizingPolicy{
					{
						Cores: &v1alpha2.SizingPolicyCores{Min: 1, Max: 4},
						Memory: &v1alpha2.SizingPolicyMemory{
							PerCore: &v1alpha2.SizingPolicyMemoryPerCore{
								MemoryMinMax: v1alpha2.MemoryMinMax{Min: ptr.To(resource.MustParse("2Gi"))},
							},
						},
					},
				},
			},
		}

		It("should fail when per-core memory is below the min", func() {
			vm := &v1alpha2.VirtualMachine{
				Spec: v1alpha2.VirtualMachineSpec{
					VirtualMachineClassName: "vmclasstest",
					CPU:                     v1alpha2.CPUSpec{Cores: 2, CoreFraction: "10%"},
					Memory:                  v1alpha2.MemorySpec{Size: resource.MustParse("2Gi")}, // 1Gi per core
				},
			}
			err := service.NewSizePolicyService().CheckVMMatchedSizePolicy(vm, vmClass)
			Expect(err).ShouldNot(BeNil())
		})
	})

	Context("when per-core memory step does not match", func() {
		vmClass := &v1alpha2.VirtualMachineClass{
			Spec: v1alpha2.VirtualMachineClassSpec{
				SizingPolicies: []v1alpha2.SizingPolicy{
					{
						Cores: &v1alpha2.SizingPolicyCores{Min: 1, Max: 4},
						Memory: &v1alpha2.SizingPolicyMemory{
							Step: ptr.To(resource.MustParse("1Gi")),
							PerCore: &v1alpha2.SizingPolicyMemoryPerCore{
								MemoryMinMax: v1alpha2.MemoryMinMax{
									Min: ptr.To(resource.MustParse("1Gi")),
									Max: ptr.To(resource.MustParse("3Gi")),
								},
							},
						},
					},
				},
			},
		}

		It("should report the nearest valid values as total memory, not per core", func() {
			vm := &v1alpha2.VirtualMachine{
				Spec: v1alpha2.VirtualMachineSpec{
					VirtualMachineClassName: "vmclasstest",
					CPU:                     v1alpha2.CPUSpec{Cores: 2, CoreFraction: "10%"},
					Memory:                  v1alpha2.MemorySpec{Size: resource.MustParse("3Gi")}, // 1.5Gi per core, off-grid
				},
			}
			err := service.NewSizePolicyService().CheckVMMatchedSizePolicy(vm, vmClass)
			Expect(err).ShouldNot(BeNil())
			// Per-core grid is 1Gi/2Gi/3Gi; for 2 cores that is 2Gi/4Gi total.
			Expect(err.Error()).To(ContainSubstring("2Gi or 4Gi"))
			Expect(err.Error()).To(ContainSubstring("spec.memory.size"))
		})
	})

	Context("when several parameters violate the policy at once", func() {
		vmClass := &v1alpha2.VirtualMachineClass{
			Spec: v1alpha2.VirtualMachineClassSpec{
				SizingPolicies: []v1alpha2.SizingPolicy{
					{
						Cores:         &v1alpha2.SizingPolicyCores{Min: 1, Max: 4},
						CoreFractions: []v1alpha2.CoreFractionValue{10, 25},
						Memory: &v1alpha2.SizingPolicyMemory{
							MemoryMinMax: v1alpha2.MemoryMinMax{
								Min: ptr.To(resource.MustParse("1Gi")),
								Max: ptr.To(resource.MustParse("2Gi")),
							},
						},
					},
				},
			},
		}

		It("should report every violation in a single message", func() {
			vm := &v1alpha2.VirtualMachine{
				Spec: v1alpha2.VirtualMachineSpec{
					VirtualMachineClassName: "vmclasstest",
					CPU:                     v1alpha2.CPUSpec{Cores: 1, CoreFraction: "33%"},
					Memory:                  v1alpha2.MemorySpec{Size: resource.MustParse("4Gi")},
				},
			}
			err := service.NewSizePolicyService().CheckVMMatchedSizePolicy(vm, vmClass)
			Expect(err).ShouldNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("several reasons"))
			Expect(err.Error()).To(ContainSubstring("spec.cpu.coreFraction"))
			Expect(err.Error()).To(ContainSubstring("spec.memory.size"))
		})
	})

	Context("when no sizing policy matches the number of cores", func() {
		vmClass := &v1alpha2.VirtualMachineClass{
			Spec: v1alpha2.VirtualMachineClassSpec{
				SizingPolicies: []v1alpha2.SizingPolicy{
					{Cores: &v1alpha2.SizingPolicyCores{Min: 1, Max: 4}},
					{Cores: &v1alpha2.SizingPolicyCores{Min: 9, Max: 16}},
				},
			},
		}

		It("should list the allowed core ranges", func() {
			vm := &v1alpha2.VirtualMachine{
				Spec: v1alpha2.VirtualMachineSpec{
					VirtualMachineClassName: "vmclasstest",
					CPU:                     v1alpha2.CPUSpec{Cores: 6, CoreFraction: "10%"},
				},
			}
			err := service.NewSizePolicyService().CheckVMMatchedSizePolicy(vm, vmClass)
			Expect(err).ShouldNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("1-4"))
			Expect(err.Error()).To(ContainSubstring("9-16"))
		})
	})
})
