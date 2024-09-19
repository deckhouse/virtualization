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
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	v1alpha2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// vmClassValues stores mock values for VirtualMachineClass objects.
var vmClassValues map[string]client.Object

var _ = Describe("SizePolicyService", func() {
	var mock *service.ClientMock
	var ctx context.Context

	BeforeEach(func() {
		mock = &service.ClientMock{}
		mock.GetFunc = func(_ context.Context, key client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
			val, ok := vmClassValues[key.Name]
			if !ok {
				return fmt.Errorf("object not found")
			}

			// Populate the incoming object's SizingPolicies from the mock data.
			obj.(*v1alpha2.VirtualMachineClass).Spec.SizingPolicies = val.(*v1alpha2.VirtualMachineClass).Spec.SizingPolicies

			return nil
		}

		// Initialize the mock data map.
		vmClassValues = make(map[string]client.Object)
	})

	Context("when no value is provided", func() {
		It("should return an error on Get()", func() {
			val := v1alpha2.VirtualMachineClass{}
			err := mock.Get(ctx, types.NamespacedName{Name: "vmclasstest"}, &val)
			Expect(err).ShouldNot(BeNil())
		})
	})

	Context("when value is provided", func() {
		BeforeEach(func() {
			// Set mock VM class data.
			vmClassValues["vmclasstest"] = &v1alpha2.VirtualMachineClass{
				Spec: v1alpha2.VirtualMachineClassSpec{
					SizingPolicies: []v1alpha2.SizingPolicy{
						{
							Cores: &v1alpha2.SizingPolicyCores{Min: 1, Max: 4},
							Memory: &v1alpha2.SizingPolicyMemory{
								MemoryMinMax: v1alpha2.MemoryMinMax{
									Min: resource.MustParse("1Gi"),
									Max: resource.MustParse("4Gi"),
								},
							},
						},
					},
				},
			}
		})

		It("should successfully find the value", func() {
			val := v1alpha2.VirtualMachineClass{}
			err := mock.Get(ctx, types.NamespacedName{Name: "vmclasstest"}, &val)
			Expect(err).Should(BeNil())
			Expect(len(val.Spec.SizingPolicies)).Should(Equal(1))
		})
	})

	Context("when VM has no class", func() {
		vm := &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{VirtualMachineClassName: ""},
		}

		BeforeEach(func() {
			// Set a default mock VM class.
			vmClassValues["vmclasstest"] = &v1alpha2.VirtualMachineClass{
				Spec: v1alpha2.VirtualMachineClassSpec{SizingPolicies: []v1alpha2.SizingPolicy{}},
			}
		})

		It("should fail validation due to empty class name", func() {
			service := service.NewSizePolicyService(mock)
			err := service.CheckVMMatchedSizePolicy(ctx, vm)
			Expect(err).ShouldNot(BeNil())
		})
	})

	Context("when VM has a non-existent class", func() {
		vm := &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{VirtualMachineClassName: "notexists"},
		}

		BeforeEach(func() {
			// Set a default mock VM class.
			vmClassValues["vmclasstest"] = &v1alpha2.VirtualMachineClass{
				Spec: v1alpha2.VirtualMachineClassSpec{SizingPolicies: []v1alpha2.SizingPolicy{}},
			}
		})

		It("should fail validation due to non-existent class", func() {
			service := service.NewSizePolicyService(mock)
			err := service.CheckVMMatchedSizePolicy(ctx, vm)
			Expect(err).ShouldNot(BeNil())
		})
	})

	Context("when VM's class has no valid size policy", func() {
		vm := &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{
				VirtualMachineClassName: "vmclasstest",
				CPU:                     v1alpha2.CPUSpec{Cores: 5, CoreFraction: "10%"},
			},
		}

		BeforeEach(func() {
			// Set mock VM class data with invalid policies for the VM.
			vmClassValues["vmclasstest"] = &v1alpha2.VirtualMachineClass{
				Spec: v1alpha2.VirtualMachineClassSpec{
					SizingPolicies: []v1alpha2.SizingPolicy{
						{
							Cores: &v1alpha2.SizingPolicyCores{Min: 1, Max: 4},
						},
					},
				},
			}
		})

		It("should fail validation due to invalid size policy", func() {
			service := service.NewSizePolicyService(mock)
			err := service.CheckVMMatchedSizePolicy(ctx, vm)
			Expect(err).ShouldNot(BeNil())
		})
	})

	Context("when VM's class has correct policy without memory requirements", func() {
		vm := &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{
				VirtualMachineClassName: "vmclasstest",
				CPU:                     v1alpha2.CPUSpec{Cores: 1, CoreFraction: "10%"},
			},
		}

		BeforeEach(func() {
			// Set mock VM class data with valid policies for the VM without memory requirements.
			vmClassValues["vmclasstest"] = &v1alpha2.VirtualMachineClass{
				Spec: v1alpha2.VirtualMachineClassSpec{
					SizingPolicies: []v1alpha2.SizingPolicy{
						{
							Cores: &v1alpha2.SizingPolicyCores{Min: 1, Max: 4},
						},
					},
				},
			}
		})

		It("should pass validation due to lack of memory requirements", func() {
			service := service.NewSizePolicyService(mock)
			err := service.CheckVMMatchedSizePolicy(ctx, vm)
			Expect(err).Should(BeNil())
		})
	})

	Context("when VM's memory does not matched with policy", func() {
		vm := &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{
				VirtualMachineClassName: "vmclasstest",
				CPU:                     v1alpha2.CPUSpec{Cores: 1, CoreFraction: "10%"},
				Memory:                  v1alpha2.MemorySpec{Size: resource.MustParse("1Gi")},
			},
		}

		BeforeEach(func() {
			// Set mock VM class data with valid memory policies for the VM.
			vmClassValues["vmclasstest"] = &v1alpha2.VirtualMachineClass{
				Spec: v1alpha2.VirtualMachineClassSpec{
					SizingPolicies: []v1alpha2.SizingPolicy{
						{
							Cores: &v1alpha2.SizingPolicyCores{Min: 1, Max: 4},
							Memory: &v1alpha2.SizingPolicyMemory{
								MemoryMinMax: v1alpha2.MemoryMinMax{
									Min: resource.MustParse("512Mi"),
									Max: resource.MustParse("2Gi"),
								},
							},
						},
					},
				},
			}
		})

		It("should pass validation due to match memory size", func() {
			service := service.NewSizePolicyService(mock)
			err := service.CheckVMMatchedSizePolicy(ctx, vm)
			Expect(err).Should(BeNil())
		})
	})

	Context("when VM's memory matches the policy", func() {
		vm := &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{
				VirtualMachineClassName: "vmclasstest",
				CPU:                     v1alpha2.CPUSpec{Cores: 1, CoreFraction: "10%"},
				Memory:                  v1alpha2.MemorySpec{Size: resource.MustParse("2Gi")},
			},
		}

		BeforeEach(func() {
			// Set mock VM class data with valid memory policies for the VM.
			vmClassValues["vmclasstest"] = &v1alpha2.VirtualMachineClass{
				Spec: v1alpha2.VirtualMachineClassSpec{
					SizingPolicies: []v1alpha2.SizingPolicy{
						{
							Cores: &v1alpha2.SizingPolicyCores{Min: 1, Max: 4},
							Memory: &v1alpha2.SizingPolicyMemory{
								MemoryMinMax: v1alpha2.MemoryMinMax{
									Min: resource.MustParse("1Gi"),
									Max: resource.MustParse("3Gi"),
								},
							},
						},
					},
				},
			}
		})

		It("should pass validation due to matched memory size", func() {
			service := service.NewSizePolicyService(mock)
			err := service.CheckVMMatchedSizePolicy(ctx, vm)
			Expect(err).Should(BeNil())
		})
	})

	Context("when class policy has empty memory requirements", func() {
		vm := &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{
				VirtualMachineClassName: "vmclasstest",
				CPU:                     v1alpha2.CPUSpec{Cores: 1, CoreFraction: "10%"},
				Memory:                  v1alpha2.MemorySpec{Size: resource.MustParse("2Gi")},
			},
		}

		BeforeEach(func() {
			// Set mock VM class data with empty memory requirements.
			vmClassValues["vmclasstest"] = &v1alpha2.VirtualMachineClass{
				Spec: v1alpha2.VirtualMachineClassSpec{
					SizingPolicies: []v1alpha2.SizingPolicy{
						{
							Cores:  &v1alpha2.SizingPolicyCores{Min: 1, Max: 4},
							Memory: &v1alpha2.SizingPolicyMemory{},
						},
					},
				},
			}
		})

		It("should pass validation due to lack of memory requirements", func() {
			service := service.NewSizePolicyService(mock)
			err := service.CheckVMMatchedSizePolicy(ctx, vm)
			Expect(err).Should(BeNil())
		})
	})

	Context("when VM's memory is correct per core", func() {
		vm := &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{
				VirtualMachineClassName: "vmclasstest",
				CPU:                     v1alpha2.CPUSpec{Cores: 2, CoreFraction: "10%"},
				Memory:                  v1alpha2.MemorySpec{Size: resource.MustParse("4Gi")},
			},
		}

		BeforeEach(func() {
			// Set mock VM class data with valid per core memory policies for the VM.
			vmClassValues["vmclasstest"] = &v1alpha2.VirtualMachineClass{
				Spec: v1alpha2.VirtualMachineClassSpec{
					SizingPolicies: []v1alpha2.SizingPolicy{
						{
							Cores: &v1alpha2.SizingPolicyCores{Min: 1, Max: 4},
							Memory: &v1alpha2.SizingPolicyMemory{
								PerCore: v1alpha2.SizingPolicyMemoryPerCore{
									MemoryMinMax: v1alpha2.MemoryMinMax{
										Min: resource.MustParse("1Gi"),
										Max: resource.MustParse("3Gi"),
									},
								},
							},
						},
					},
				},
			}
		})

		It("should pass validation due to matched per core memory size", func() {
			service := service.NewSizePolicyService(mock)
			err := service.CheckVMMatchedSizePolicy(ctx, vm)
			Expect(err).Should(BeNil())
		})
	})

	Context("when VM's memory is incorrect per core", func() {
		vm := &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{
				VirtualMachineClassName: "vmclasstest",
				CPU:                     v1alpha2.CPUSpec{Cores: 4, CoreFraction: "10%"},
				Memory:                  v1alpha2.MemorySpec{Size: resource.MustParse("4Gi")},
			},
		}

		BeforeEach(func() {
			// Set mock VM class data with invalid per core memory policies for the VM.
			vmClassValues["vmclasstest"] = &v1alpha2.VirtualMachineClass{
				Spec: v1alpha2.VirtualMachineClassSpec{
					SizingPolicies: []v1alpha2.SizingPolicy{
						{
							Cores: &v1alpha2.SizingPolicyCores{Min: 1, Max: 4},
							Memory: &v1alpha2.SizingPolicyMemory{
								PerCore: v1alpha2.SizingPolicyMemoryPerCore{
									MemoryMinMax: v1alpha2.MemoryMinMax{
										Min: resource.MustParse("2Gi"),
										Max: resource.MustParse("3Gi"),
									},
								},
							},
						},
					},
				},
			}
		})

		It("should fail validation due to non-matching per core memory size", func() {
			service := service.NewSizePolicyService(mock)
			err := service.CheckVMMatchedSizePolicy(ctx, vm)
			Expect(err).ShouldNot(BeNil())
		})
	})

	Context("when VM's core fraction is correct", func() {
		vm := &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{
				VirtualMachineClassName: "vmclasstest",
				CPU:                     v1alpha2.CPUSpec{Cores: 1, CoreFraction: "10%"},
				Memory:                  v1alpha2.MemorySpec{Size: resource.MustParse("2Gi")},
			},
		}

		BeforeEach(func() {
			// Set mock VM class data with valid core fraction policies for the VM.
			vmClassValues["vmclasstest"] = &v1alpha2.VirtualMachineClass{
				Spec: v1alpha2.VirtualMachineClassSpec{
					SizingPolicies: []v1alpha2.SizingPolicy{
						{
							Cores:         &v1alpha2.SizingPolicyCores{Min: 1, Max: 4},
							CoreFractions: []v1alpha2.CoreFractionValue{10, 25, 50, 100},
						},
					},
				},
			}
		})

		It("should pass validation due to matching core fraction", func() {
			service := service.NewSizePolicyService(mock)
			err := service.CheckVMMatchedSizePolicy(ctx, vm)
			Expect(err).Should(BeNil())
		})
	})

	Context("when VM's core fraction is incorrect", func() {
		vm := &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{
				VirtualMachineClassName: "vmclasstest",
				CPU:                     v1alpha2.CPUSpec{Cores: 1, CoreFraction: "11%"},
				Memory:                  v1alpha2.MemorySpec{Size: resource.MustParse("2Gi")},
			},
		}

		BeforeEach(func() {
			// Set mock VM class data with valid core fraction policies for the VM.
			vmClassValues["vmclasstest"] = &v1alpha2.VirtualMachineClass{
				Spec: v1alpha2.VirtualMachineClassSpec{
					SizingPolicies: []v1alpha2.SizingPolicy{
						{
							Cores:         &v1alpha2.SizingPolicyCores{Min: 1, Max: 4},
							CoreFractions: []v1alpha2.CoreFractionValue{10, 25, 50, 100},
						},
					},
				},
			}
		})

		It("should fail validation due to non-matching core fraction", func() {
			service := service.NewSizePolicyService(mock)
			err := service.CheckVMMatchedSizePolicy(ctx, vm)
			Expect(err).ShouldNot(BeNil())
		})
	})

	Context("when VM's memory step is correct", func() {
		vm := &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{
				VirtualMachineClassName: "vmclasstest",
				CPU:                     v1alpha2.CPUSpec{Cores: 2, CoreFraction: "10%"},
				Memory:                  v1alpha2.MemorySpec{Size: resource.MustParse("2Gi")},
			},
		}

		BeforeEach(func() {
			// Set mock VM class data with valid memory step policies for the VM.
			vmClassValues["vmclasstest"] = &v1alpha2.VirtualMachineClass{
				Spec: v1alpha2.VirtualMachineClassSpec{
					SizingPolicies: []v1alpha2.SizingPolicy{
						{
							Cores: &v1alpha2.SizingPolicyCores{Min: 1, Max: 4},
							Memory: &v1alpha2.SizingPolicyMemory{
								Step: resource.MustParse("1Gi"),
								MemoryMinMax: v1alpha2.MemoryMinMax{
									Min: resource.MustParse("1Gi"),
									Max: resource.MustParse("3Gi"),
								},
							},
						},
					},
				},
			}
		})

		It("should pass validation due to match memory step", func() {
			service := service.NewSizePolicyService(mock)
			err := service.CheckVMMatchedSizePolicy(ctx, vm)
			Expect(err).Should(BeNil())
		})
	})

	Context("when VM's memory step is incorrect", func() {
		vm := &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{
				VirtualMachineClassName: "vmclasstest",
				CPU:                     v1alpha2.CPUSpec{Cores: 2, CoreFraction: "10%"},
				Memory:                  v1alpha2.MemorySpec{Size: resource.MustParse("2001Mi")},
			},
		}

		BeforeEach(func() {
			// Set mock VM class data with invalid memory step policies for the VM.
			vmClassValues["vmclasstest"] = &v1alpha2.VirtualMachineClass{
				Spec: v1alpha2.VirtualMachineClassSpec{
					SizingPolicies: []v1alpha2.SizingPolicy{
						{
							Cores: &v1alpha2.SizingPolicyCores{Min: 1, Max: 4},
							Memory: &v1alpha2.SizingPolicyMemory{
								Step: resource.MustParse("1Gi"),
								MemoryMinMax: v1alpha2.MemoryMinMax{
									Min: resource.MustParse("1Gi"),
									Max: resource.MustParse("3Gi"),
								},
							},
						},
					},
				},
			}
		})

		It("should fail validation due to non-matching memory step", func() {
			service := service.NewSizePolicyService(mock)
			err := service.CheckVMMatchedSizePolicy(ctx, vm)
			Expect(err).ShouldNot(BeNil())
		})
	})

	Context("when VM's per core memory step is correct", func() {
		vm := &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{
				VirtualMachineClassName: "vmclasstest",
				CPU:                     v1alpha2.CPUSpec{Cores: 2, CoreFraction: "10%"},
				Memory:                  v1alpha2.MemorySpec{Size: resource.MustParse("4Gi")},
			},
		}

		BeforeEach(func() {
			// Set mock VM class data with valid per core memory step policies for the VM.
			vmClassValues["vmclasstest"] = &v1alpha2.VirtualMachineClass{
				Spec: v1alpha2.VirtualMachineClassSpec{
					SizingPolicies: []v1alpha2.SizingPolicy{
						{
							Cores: &v1alpha2.SizingPolicyCores{Min: 1, Max: 4},
							Memory: &v1alpha2.SizingPolicyMemory{
								Step: resource.MustParse("1Gi"),
								PerCore: v1alpha2.SizingPolicyMemoryPerCore{
									MemoryMinMax: v1alpha2.MemoryMinMax{
										Min: resource.MustParse("1Gi"),
										Max: resource.MustParse("3Gi"),
									},
								},
							},
						},
					},
				},
			}
		})

		It("should pass validation due to match per core memory step", func() {
			service := service.NewSizePolicyService(mock)
			err := service.CheckVMMatchedSizePolicy(ctx, vm)
			Expect(err).Should(BeNil())
		})
	})

	Context("when VM's per core memory step is incorrect", func() {
		vm := &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{
				VirtualMachineClassName: "vmclasstest",
				CPU:                     v1alpha2.CPUSpec{Cores: 2, CoreFraction: "10%"},
				Memory:                  v1alpha2.MemorySpec{Size: resource.MustParse("4001Mi")},
			},
		}

		BeforeEach(func() {
			// Set mock VM class data with invalid per core memory step policies for the VM.
			vmClassValues["vmclasstest"] = &v1alpha2.VirtualMachineClass{
				Spec: v1alpha2.VirtualMachineClassSpec{
					SizingPolicies: []v1alpha2.SizingPolicy{
						{
							Cores: &v1alpha2.SizingPolicyCores{Min: 1, Max: 4},
							Memory: &v1alpha2.SizingPolicyMemory{
								Step: resource.MustParse("1Gi"),
								PerCore: v1alpha2.SizingPolicyMemoryPerCore{
									MemoryMinMax: v1alpha2.MemoryMinMax{
										Min: resource.MustParse("1Gi"),
										Max: resource.MustParse("3Gi"),
									},
								},
							},
						},
					},
				},
			}
		})

		It("should fail validation due to non-matching per core memory step", func() {
			service := service.NewSizePolicyService(mock)
			err := service.CheckVMMatchedSizePolicy(ctx, vm)
			Expect(err).ShouldNot(BeNil())
		})
	})
})
