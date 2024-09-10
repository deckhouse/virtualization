package validators_test

import (
	"context"
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"

	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/validators"
	v1alpha2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

func TestValidators(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Validators Suite")
}

var _ = Describe("Spec policy comlience validator", func() {
	var mock ClientMock
	var ctx context.Context
	mem512m := resource.MustParse("512M")
	mem1g := resource.MustParse("1G")
	mem2g := resource.MustParse("2G")
	mem3g := resource.MustParse("3G")
	mem4g := resource.MustParse("4G")

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
							Min: mem1g,
							Max: mem4g,
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
		validator := validators.NewSizingPolicyCompliencyValidator(&mock)

		BeforeEach(func() {
			mock.Values["vmclasstest"] = &v1alpha2.VirtualMachineClass{
				Spec: v1alpha2.VirtualMachineClassSpec{
					SizingPolicies: make([]v1alpha2.SizingPolicy, 0),
				},
			}
		})

		It("Should fail validate because cannot find empty classname", func() {
			err := validator.CheckVMCompliedSizePolicy(ctx, vm)
			Expect(err).ShouldNot(BeNil())
		})
	})

	Context("Vm with not exists class", func() {
		vm := &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{
				VirtualMachineClassName: "notexists",
			},
		}
		validator := validators.NewSizingPolicyCompliencyValidator(&mock)

		BeforeEach(func() {
			mock.Values["vmclasstest"] = &v1alpha2.VirtualMachineClass{
				Spec: v1alpha2.VirtualMachineClassSpec{
					SizingPolicies: make([]v1alpha2.SizingPolicy, 0),
				},
			}
		})

		It("Should fail validate because cannot find empty classname", func() {
			err := validator.CheckVMCompliedSizePolicy(ctx, vm)
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
		validator := validators.NewSizingPolicyCompliencyValidator(&mock)

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
			err := validator.CheckVMCompliedSizePolicy(ctx, vm)
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
		validator := validators.NewSizingPolicyCompliencyValidator(&mock)

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
			err := validator.CheckVMCompliedSizePolicy(ctx, vm)
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
					Size: mem1g,
				},
			},
		}
		validator := validators.NewSizingPolicyCompliencyValidator(&mock)

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
							Min: mem512m,
							Max: mem2g,
						},
					},
				},
			)
		})

		It("Should not fail validate because memory compliency", func() {
			err := validator.CheckVMCompliedSizePolicy(ctx, vm)
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
					Size: mem2g,
				},
			},
		}
		validator := validators.NewSizingPolicyCompliencyValidator(&mock)

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
							Min: mem1g,
							Max: mem3g,
						},
					},
				},
			)
		})

		It("Should not fail validate because memory not compliency", func() {
			err := validator.CheckVMCompliedSizePolicy(ctx, vm)
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
					Size: mem2g,
				},
			},
		}
		validator := validators.NewSizingPolicyCompliencyValidator(&mock)

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
			err := validator.CheckVMCompliedSizePolicy(ctx, vm)
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
					Size: mem4g,
				},
			},
		}
		validator := validators.NewSizingPolicyCompliencyValidator(&mock)

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
								Min: mem1g,
								Max: mem3g,
							},
						},
					},
				},
			)
		})

		It("Should not fail validate because memory compliency", func() {
			err := validator.CheckVMCompliedSizePolicy(ctx, vm)
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
					Size: mem4g,
				},
			},
		}
		validator := validators.NewSizingPolicyCompliencyValidator(&mock)

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
								Min: mem2g,
								Max: mem3g,
							},
						},
					},
				},
			)
		})

		It("Should fail validate because not memory compliency", func() {
			err := validator.CheckVMCompliedSizePolicy(ctx, vm)
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
					Size: mem2g,
				},
			},
		}
		validator := validators.NewSizingPolicyCompliencyValidator(&mock)

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
			err := validator.CheckVMCompliedSizePolicy(ctx, vm)
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
					Size: mem2g,
				},
			},
		}
		validator := validators.NewSizingPolicyCompliencyValidator(&mock)

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
			err := validator.CheckVMCompliedSizePolicy(ctx, vm)
			Expect(err).ShouldNot(BeNil())
		})
	})
})
