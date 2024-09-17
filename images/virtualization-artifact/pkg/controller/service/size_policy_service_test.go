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

type ClientMock struct {
	Values map[string]client.Object
}

func (m *ClientMock) Get(_ context.Context, key client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
	val, ok := m.Values[key.Name]
	if !ok {
		return fmt.Errorf("Object not found")
	}

	obj.(*v1alpha2.VirtualMachineClass).Spec.SizingPolicies = val.(*v1alpha2.VirtualMachineClass).Spec.SizingPolicies

	return nil
}

func (m *ClientMock) List(_ context.Context, _ client.ObjectList, _ ...client.ListOption) error {
	return nil
}

var _ = Describe("Spec policy comlience service", func() {
	var mock ClientMock
	var ctx context.Context

	BeforeEach(func() {
		mock = ClientMock{}
		mock.Values = make(map[string]client.Object)
	})

	Context("testing mock no value", func() {
		It("Should not value", func() {
			val := v1alpha2.VirtualMachineClass{}
			err := mock.Get(ctx, types.NamespacedName{
				Name: "vmclasstest",
			}, &val)
			Expect(err).ShouldNot(BeNil())
		})
	})

	Context("testing mock value", func() {
		BeforeEach(func() {
			mock.Values["vmclasstest"] = &v1alpha2.VirtualMachineClass{
				Spec: v1alpha2.VirtualMachineClassSpec{
					SizingPolicies: make([]v1alpha2.SizingPolicy, 0),
				},
			}
			mock.Values["vmclasstest"].(*v1alpha2.VirtualMachineClass).Spec.SizingPolicies = append(
				mock.Values["vmclasstest"].(*v1alpha2.VirtualMachineClass).Spec.SizingPolicies,
				v1alpha2.SizingPolicy{
					Cores: &v1alpha2.SizingPolicyCores{
						Min: 1,
						Max: 4,
					},
					Memory: &v1alpha2.SizingPolicyMemory{
						MemoryMinMax: v1alpha2.MemoryMinMax{
							Min: resource.MustParse("1Gi"),
							Max: resource.MustParse("4Gi"),
						},
					},
				},
			)
		})

		It("Should value", func() {
			val := v1alpha2.VirtualMachineClass{}
			err := mock.Get(ctx, types.NamespacedName{
				Name: "vmclasstest",
			}, &val)
			Expect(err).Should(BeNil())
			Expect(len(val.Spec.SizingPolicies)).Should(Equal(1))
		})
	})

	Context("Vm with no class", func() {
		vm := &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{
				VirtualMachineClassName: "",
			},
		}

		BeforeEach(func() {
			mock.Values["vmclasstest"] = &v1alpha2.VirtualMachineClass{
				Spec: v1alpha2.VirtualMachineClassSpec{
					SizingPolicies: make([]v1alpha2.SizingPolicy, 0),
				},
			}
		})

		It("Should fail validate because cannot find empty classname", func() {
			service := service.NewSizePolicyService(&mock)
			err := service.CheckVMMatchedSizePolicy(ctx, vm)
			Expect(err).ShouldNot(BeNil())
		})
	})

	Context("Vm with not exists class", func() {
		vm := &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{
				VirtualMachineClassName: "notexists",
			},
		}

		BeforeEach(func() {
			mock.Values["vmclasstest"] = &v1alpha2.VirtualMachineClass{
				Spec: v1alpha2.VirtualMachineClassSpec{
					SizingPolicies: make([]v1alpha2.SizingPolicy, 0),
				},
			}
		})

		It("Should fail validate because cannot find empty classname", func() {
			service := service.NewSizePolicyService(&mock)
			err := service.CheckVMMatchedSizePolicy(ctx, vm)
			Expect(err).ShouldNot(BeNil())
		})
	})

	Context("Vm with correct class without correct policy", func() {
		vm := &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{
				VirtualMachineClassName: "vmclasstest",
				CPU: v1alpha2.CPUSpec{
					Cores:        5,
					CoreFraction: "10%",
				},
			},
		}

		BeforeEach(func() {
			mock.Values["vmclasstest"] = &v1alpha2.VirtualMachineClass{
				Spec: v1alpha2.VirtualMachineClassSpec{
					SizingPolicies: make([]v1alpha2.SizingPolicy, 0),
				},
			}
			mock.Values["vmclasstest"].(*v1alpha2.VirtualMachineClass).Spec.SizingPolicies = append(
				mock.Values["vmclasstest"].(*v1alpha2.VirtualMachineClass).Spec.SizingPolicies,
				v1alpha2.SizingPolicy{
					Cores: &v1alpha2.SizingPolicyCores{
						Min: 1,
						Max: 4,
					},
				},
			)
		})

		It("Should fail validate because has no valid size policy", func() {
			service := service.NewSizePolicyService(&mock)
			err := service.CheckVMMatchedSizePolicy(ctx, vm)
			Expect(err).ShouldNot(BeNil())
		})
	})

	Context("Vm with correct class with correct policy that has no memory requirements", func() {
		vm := &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{
				VirtualMachineClassName: "vmclasstest",
				CPU: v1alpha2.CPUSpec{
					Cores:        1,
					CoreFraction: "10%",
				},
			},
		}

		BeforeEach(func() {
			mock.Values["vmclasstest"] = &v1alpha2.VirtualMachineClass{
				Spec: v1alpha2.VirtualMachineClassSpec{
					SizingPolicies: make([]v1alpha2.SizingPolicy, 0),
				},
			}
			mock.Values["vmclasstest"].(*v1alpha2.VirtualMachineClass).Spec.SizingPolicies = append(
				mock.Values["vmclasstest"].(*v1alpha2.VirtualMachineClass).Spec.SizingPolicies,
				v1alpha2.SizingPolicy{
					Cores: &v1alpha2.SizingPolicyCores{
						Min: 1,
						Max: 4,
					},
				},
			)
		})

		It("Should not fail validate because no memory requirements", func() {
			service := service.NewSizePolicyService(&mock)
			err := service.CheckVMMatchedSizePolicy(ctx, vm)
			Expect(err).Should(BeNil())
		})
	})

	Context("Vm with correct class with incorrect memory", func() {
		vm := &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{
				VirtualMachineClassName: "vmclasstest",
				CPU: v1alpha2.CPUSpec{
					Cores:        1,
					CoreFraction: "10%",
				},
				Memory: v1alpha2.MemorySpec{
					Size: resource.MustParse("1Gi"),
				},
			},
		}

		BeforeEach(func() {
			mock.Values["vmclasstest"] = &v1alpha2.VirtualMachineClass{
				Spec: v1alpha2.VirtualMachineClassSpec{
					SizingPolicies: make([]v1alpha2.SizingPolicy, 0),
				},
			}
			mock.Values["vmclasstest"].(*v1alpha2.VirtualMachineClass).Spec.SizingPolicies = append(
				mock.Values["vmclasstest"].(*v1alpha2.VirtualMachineClass).Spec.SizingPolicies,
				v1alpha2.SizingPolicy{
					Cores: &v1alpha2.SizingPolicyCores{
						Min: 1,
						Max: 4,
					},
					Memory: &v1alpha2.SizingPolicyMemory{
						MemoryMinMax: v1alpha2.MemoryMinMax{
							Min: resource.MustParse("512Mi"),
							Max: resource.MustParse("2Gi"),
						},
					},
				},
			)
		})

		It("Should not fail validate because memory compliency", func() {
			service := service.NewSizePolicyService(&mock)
			err := service.CheckVMMatchedSizePolicy(ctx, vm)
			Expect(err).Should(BeNil())
		})
	})

	Context("Vm with correct class with correct memory", func() {
		vm := &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{
				VirtualMachineClassName: "vmclasstest",
				CPU: v1alpha2.CPUSpec{
					Cores:        1,
					CoreFraction: "10%",
				},
				Memory: v1alpha2.MemorySpec{
					Size: resource.MustParse("2Gi"),
				},
			},
		}

		BeforeEach(func() {
			mock.Values["vmclasstest"] = &v1alpha2.VirtualMachineClass{
				Spec: v1alpha2.VirtualMachineClassSpec{
					SizingPolicies: make([]v1alpha2.SizingPolicy, 0),
				},
			}
			mock.Values["vmclasstest"].(*v1alpha2.VirtualMachineClass).Spec.SizingPolicies = append(
				mock.Values["vmclasstest"].(*v1alpha2.VirtualMachineClass).Spec.SizingPolicies,
				v1alpha2.SizingPolicy{
					Cores: &v1alpha2.SizingPolicyCores{
						Min: 1,
						Max: 4,
					},
					Memory: &v1alpha2.SizingPolicyMemory{
						MemoryMinMax: v1alpha2.MemoryMinMax{
							Min: resource.MustParse("1Gi"),
							Max: resource.MustParse("3Gi"),
						},
					},
				},
			)
		})

		It("Should not fail validate because memory not compliency", func() {
			service := service.NewSizePolicyService(&mock)
			err := service.CheckVMMatchedSizePolicy(ctx, vm)
			Expect(err).Should(BeNil())
		})
	})

	Context("Vm class policy has empty memory policy", func() {
		vm := &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{
				VirtualMachineClassName: "vmclasstest",
				CPU: v1alpha2.CPUSpec{
					Cores:        1,
					CoreFraction: "10%",
				},
				Memory: v1alpha2.MemorySpec{
					Size: resource.MustParse("2Gi"),
				},
			},
		}

		BeforeEach(func() {
			mock.Values["vmclasstest"] = &v1alpha2.VirtualMachineClass{
				Spec: v1alpha2.VirtualMachineClassSpec{
					SizingPolicies: make([]v1alpha2.SizingPolicy, 0),
				},
			}
			mock.Values["vmclasstest"].(*v1alpha2.VirtualMachineClass).Spec.SizingPolicies = append(
				mock.Values["vmclasstest"].(*v1alpha2.VirtualMachineClass).Spec.SizingPolicies,
				v1alpha2.SizingPolicy{
					Cores: &v1alpha2.SizingPolicyCores{
						Min: 1,
						Max: 4,
					},
					Memory: &v1alpha2.SizingPolicyMemory{},
				},
			)
		})

		It("Should not fail validate because has no memory requirements", func() {
			service := service.NewSizePolicyService(&mock)
			err := service.CheckVMMatchedSizePolicy(ctx, vm)
			Expect(err).Should(BeNil())
		})
	})

	Context("Vm with correct class with correct memory by per core value", func() {
		vm := &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{
				VirtualMachineClassName: "vmclasstest",
				CPU: v1alpha2.CPUSpec{
					Cores:        2,
					CoreFraction: "10%",
				},
				Memory: v1alpha2.MemorySpec{
					Size: resource.MustParse("4Gi"),
				},
			},
		}

		BeforeEach(func() {
			mock.Values["vmclasstest"] = &v1alpha2.VirtualMachineClass{
				Spec: v1alpha2.VirtualMachineClassSpec{
					SizingPolicies: make([]v1alpha2.SizingPolicy, 0),
				},
			}
			mock.Values["vmclasstest"].(*v1alpha2.VirtualMachineClass).Spec.SizingPolicies = append(
				mock.Values["vmclasstest"].(*v1alpha2.VirtualMachineClass).Spec.SizingPolicies,
				v1alpha2.SizingPolicy{
					Cores: &v1alpha2.SizingPolicyCores{
						Min: 1,
						Max: 4,
					},
					Memory: &v1alpha2.SizingPolicyMemory{
						PerCore: v1alpha2.SizingPolicyMemoryPerCore{
							MemoryMinMax: v1alpha2.MemoryMinMax{
								Min: resource.MustParse("1Gi"),
								Max: resource.MustParse("3Gi"),
							},
						},
					},
				},
			)
		})

		It("Should not fail validate because memory compliency", func() {
			service := service.NewSizePolicyService(&mock)
			err := service.CheckVMMatchedSizePolicy(ctx, vm)
			Expect(err).Should(BeNil())
		})
	})

	Context("Vm with correct class with incorrect memory by per core value", func() {
		vm := &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{
				VirtualMachineClassName: "vmclasstest",
				CPU: v1alpha2.CPUSpec{
					Cores:        4,
					CoreFraction: "10%",
				},
				Memory: v1alpha2.MemorySpec{
					Size: resource.MustParse("4Gi"),
				},
			},
		}

		BeforeEach(func() {
			mock.Values["vmclasstest"] = &v1alpha2.VirtualMachineClass{
				Spec: v1alpha2.VirtualMachineClassSpec{
					SizingPolicies: make([]v1alpha2.SizingPolicy, 0),
				},
			}
			mock.Values["vmclasstest"].(*v1alpha2.VirtualMachineClass).Spec.SizingPolicies = append(
				mock.Values["vmclasstest"].(*v1alpha2.VirtualMachineClass).Spec.SizingPolicies,
				v1alpha2.SizingPolicy{
					Cores: &v1alpha2.SizingPolicyCores{
						Min: 1,
						Max: 4,
					},
					Memory: &v1alpha2.SizingPolicyMemory{
						PerCore: v1alpha2.SizingPolicyMemoryPerCore{
							MemoryMinMax: v1alpha2.MemoryMinMax{
								Min: resource.MustParse("2Gi"),
								Max: resource.MustParse("3Gi"),
							},
						},
					},
				},
			)
		})

		It("Should fail validate because not memory compliency", func() {
			service := service.NewSizePolicyService(&mock)
			err := service.CheckVMMatchedSizePolicy(ctx, vm)
			Expect(err).ShouldNot(BeNil())
		})
	})

	Context("Vm with correct core fraction", func() {
		vm := &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{
				VirtualMachineClassName: "vmclasstest",
				CPU: v1alpha2.CPUSpec{
					Cores:        1,
					CoreFraction: "10%",
				},
				Memory: v1alpha2.MemorySpec{
					Size: resource.MustParse("2Gi"),
				},
			},
		}

		BeforeEach(func() {
			mock.Values["vmclasstest"] = &v1alpha2.VirtualMachineClass{
				Spec: v1alpha2.VirtualMachineClassSpec{
					SizingPolicies: make([]v1alpha2.SizingPolicy, 0),
				},
			}
			mock.Values["vmclasstest"].(*v1alpha2.VirtualMachineClass).Spec.SizingPolicies = append(
				mock.Values["vmclasstest"].(*v1alpha2.VirtualMachineClass).Spec.SizingPolicies,
				v1alpha2.SizingPolicy{
					Cores: &v1alpha2.SizingPolicyCores{
						Min: 1,
						Max: 4,
					},
					CoreFractions: []v1alpha2.CoreFractionValue{
						10, 25, 50, 100,
					},
				},
			)
		})

		It("Should not fail validate because correct core fraction", func() {
			service := service.NewSizePolicyService(&mock)
			err := service.CheckVMMatchedSizePolicy(ctx, vm)
			Expect(err).Should(BeNil())
		})
	})

	Context("Vm with incorrect core fraction", func() {
		vm := &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{
				VirtualMachineClassName: "vmclasstest",
				CPU: v1alpha2.CPUSpec{
					Cores:        1,
					CoreFraction: "11%",
				},
				Memory: v1alpha2.MemorySpec{
					Size: resource.MustParse("2Gi"),
				},
			},
		}

		BeforeEach(func() {
			mock.Values["vmclasstest"] = &v1alpha2.VirtualMachineClass{
				Spec: v1alpha2.VirtualMachineClassSpec{
					SizingPolicies: make([]v1alpha2.SizingPolicy, 0),
				},
			}
			mock.Values["vmclasstest"].(*v1alpha2.VirtualMachineClass).Spec.SizingPolicies = append(
				mock.Values["vmclasstest"].(*v1alpha2.VirtualMachineClass).Spec.SizingPolicies,
				v1alpha2.SizingPolicy{
					Cores: &v1alpha2.SizingPolicyCores{
						Min: 1,
						Max: 4,
					},
					CoreFractions: []v1alpha2.CoreFractionValue{
						10, 25, 50, 100,
					},
				},
			)
		})

		It("Should fail validate because incorrect core fraction", func() {
			service := service.NewSizePolicyService(&mock)
			err := service.CheckVMMatchedSizePolicy(ctx, vm)
			Expect(err).ShouldNot(BeNil())
		})
	})

	Context("Vm with correct step requirements", func() {
		vm := &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{
				VirtualMachineClassName: "vmclasstest",
				CPU: v1alpha2.CPUSpec{
					Cores:        2,
					CoreFraction: "10%",
				},
				Memory: v1alpha2.MemorySpec{
					Size: resource.MustParse("2Gi"),
				},
			},
		}

		BeforeEach(func() {
			mock.Values["vmclasstest"] = &v1alpha2.VirtualMachineClass{
				Spec: v1alpha2.VirtualMachineClassSpec{
					SizingPolicies: make([]v1alpha2.SizingPolicy, 0),
				},
			}
			mock.Values["vmclasstest"].(*v1alpha2.VirtualMachineClass).Spec.SizingPolicies = append(
				mock.Values["vmclasstest"].(*v1alpha2.VirtualMachineClass).Spec.SizingPolicies,
				v1alpha2.SizingPolicy{
					Cores: &v1alpha2.SizingPolicyCores{
						Min: 1,
						Max: 4,
					},
					Memory: &v1alpha2.SizingPolicyMemory{
						Step: resource.MustParse("1Gi"),
						MemoryMinMax: v1alpha2.MemoryMinMax{
							Min: resource.MustParse("1Gi"),
							Max: resource.MustParse("3Gi"),
						},
					},
				},
			)
		})

		It("Should correct validate because correct step", func() {
			service := service.NewSizePolicyService(&mock)
			err := service.CheckVMMatchedSizePolicy(ctx, vm)
			Expect(err).Should(BeNil())
		})
	})

	Context("Vm with incorrect memory by step requirements", func() {
		vm := &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{
				VirtualMachineClassName: "vmclasstest",
				CPU: v1alpha2.CPUSpec{
					Cores:        2,
					CoreFraction: "10%",
				},
				Memory: v1alpha2.MemorySpec{
					Size: resource.MustParse("2001Mi"),
				},
			},
		}

		BeforeEach(func() {
			mock.Values["vmclasstest"] = &v1alpha2.VirtualMachineClass{
				Spec: v1alpha2.VirtualMachineClassSpec{
					SizingPolicies: make([]v1alpha2.SizingPolicy, 0),
				},
			}
			mock.Values["vmclasstest"].(*v1alpha2.VirtualMachineClass).Spec.SizingPolicies = append(
				mock.Values["vmclasstest"].(*v1alpha2.VirtualMachineClass).Spec.SizingPolicies,
				v1alpha2.SizingPolicy{
					Cores: &v1alpha2.SizingPolicyCores{
						Min: 1,
						Max: 4,
					},
					Memory: &v1alpha2.SizingPolicyMemory{
						Step: resource.MustParse("1Gi"),
						MemoryMinMax: v1alpha2.MemoryMinMax{
							Min: resource.MustParse("1Gi"),
							Max: resource.MustParse("3Gi"),
						},
					},
				},
			)
		})

		It("Should fail validate because memory incorrect", func() {
			service := service.NewSizePolicyService(&mock)
			err := service.CheckVMMatchedSizePolicy(ctx, vm)
			Expect(err).ShouldNot(BeNil())
		})
	})

	Context("Vm with correct per core step requirements", func() {
		vm := &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{
				VirtualMachineClassName: "vmclasstest",
				CPU: v1alpha2.CPUSpec{
					Cores:        2,
					CoreFraction: "10%",
				},
				Memory: v1alpha2.MemorySpec{
					Size: resource.MustParse("4Gi"),
				},
			},
		}

		BeforeEach(func() {
			mock.Values["vmclasstest"] = &v1alpha2.VirtualMachineClass{
				Spec: v1alpha2.VirtualMachineClassSpec{
					SizingPolicies: make([]v1alpha2.SizingPolicy, 0),
				},
			}
			mock.Values["vmclasstest"].(*v1alpha2.VirtualMachineClass).Spec.SizingPolicies = append(
				mock.Values["vmclasstest"].(*v1alpha2.VirtualMachineClass).Spec.SizingPolicies,
				v1alpha2.SizingPolicy{
					Cores: &v1alpha2.SizingPolicyCores{
						Min: 1,
						Max: 4,
					},
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
			)
		})

		It("Should correct validate because correct per core step", func() {
			service := service.NewSizePolicyService(&mock)
			err := service.CheckVMMatchedSizePolicy(ctx, vm)
			Expect(err).Should(BeNil())
		})
	})

	Context("Vm with incorrect per core memory by step requirements", func() {
		vm := &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{
				VirtualMachineClassName: "vmclasstest",
				CPU: v1alpha2.CPUSpec{
					Cores:        2,
					CoreFraction: "10%",
				},
				Memory: v1alpha2.MemorySpec{
					Size: resource.MustParse("4001Mi"),
				},
			},
		}

		BeforeEach(func() {
			mock.Values["vmclasstest"] = &v1alpha2.VirtualMachineClass{
				Spec: v1alpha2.VirtualMachineClassSpec{
					SizingPolicies: make([]v1alpha2.SizingPolicy, 0),
				},
			}
			mock.Values["vmclasstest"].(*v1alpha2.VirtualMachineClass).Spec.SizingPolicies = append(
				mock.Values["vmclasstest"].(*v1alpha2.VirtualMachineClass).Spec.SizingPolicies,
				v1alpha2.SizingPolicy{
					Cores: &v1alpha2.SizingPolicyCores{
						Min: 1,
						Max: 4,
					},
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
			)
		})

		It("Should fail validate because per core memory incorrect", func() {
			service := service.NewSizePolicyService(&mock)
			err := service.CheckVMMatchedSizePolicy(ctx, vm)
			Expect(err).ShouldNot(BeNil())
		})
	})
})
