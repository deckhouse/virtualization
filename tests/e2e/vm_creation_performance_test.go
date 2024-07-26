package e2e

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/deckhouse/virtualization/api/client/kubeclient"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func CVMI(client kubeclient.Client, name string, action string) (*v1alpha2.ClusterVirtualMachineImage, error) {
	cvmi := v1alpha2.ClusterVirtualMachineImage{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: v1alpha2.ClusterVirtualMachineImageSpec{
			DataSource: v1alpha2.CVMIDataSource{
				Type: "HTTP", HTTP: &v1alpha2.DataSourceHTTP{
					URL: "https://cloud-images.ubuntu.com/jammy/current/jammy-server-cloudimg-amd64.img",
				},
			},
		},
	}

	if action == "create" {
		res, err := client.ClusterVirtualMachineImages().Get(context.TODO(), name, metav1.GetOptions{})
		if res != nil && err == nil {
			return res, err
		}

		res, err = client.ClusterVirtualMachineImages().Create(context.TODO(), &cvmi, metav1.CreateOptions{})
		if err != nil {
			log.Fatal(err)
		}
		return res, err
	}

	if action == "delete" {
		res, err := client.ClusterVirtualMachineImages().Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			log.Fatal(err)
		}

		err = client.ClusterVirtualMachineImages().Delete(context.TODO(), res.Name, metav1.DeleteOptions{})
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(res.Name, "was deleted")
		return res, err
	}

	err := errors.New("support only create or delete for CVMI resource")
	log.Fatal(err)
	return nil, err
}

func VM(namespace, name string, vmdName v1alpha2.VirtualMachineDisk) v1alpha2.VirtualMachine {
	vmUserData := fmt.Sprintf(`
     #cloud-config
     package_update: true
     packages:
     - qemu-guest-agent
     runcmd:
     - [ hostnamectl, set-hostname, %s ]
     - [ systemctl, daemon-reload ]
     - [ systemctl, enable, --now, qemu-guest-agent.service ]
     user: ubuntu
     password: ubuntu
     chpasswd: { expire: False }
     ssh_pwauth: True
     users:
     - name: cloud
       passwd: $6$VZitgOHHow4fx7aT$BXbg/QL4n/dYbjxFuNQlfFmRaTvtxApWn2Qwo7r1BxXIANtaJQNyJMtvu5A.mp2hxT59aTjnsiOYMVfYbyd0j.
       shell: /bin/bash
       sudo: ALL=(ALL) NOPASSWD:ALL
       chpasswd: { expire: False }
       lock_passwd: false
       ssh_authorized_keys:
       - ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIFxcXHmwaGnJ8scJaEN5RzklBPZpVSic4GdaAsKjQoeA your_email@example.com
`, name)

	vm := v1alpha2.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    map[string]string{"vm": "linux", "service": "v1"},
		},
		Spec: v1alpha2.VirtualMachineSpec{
			EnableParavirtualization: true,
			RunPolicy:                v1alpha2.RunPolicy("AlwaysOn"),
			OsType:                   v1alpha2.OsType("Generic"),
			Bootloader:               v1alpha2.BootloaderType("BIOS"),
			CPU: v1alpha2.CPUSpec{
				Cores:        1,
				CoreFraction: "25%",
				ModelName:    "generic-v1",
			},
			Memory: v1alpha2.MemorySpec{Size: "1Gi"},
			BlockDevices: []v1alpha2.BlockDeviceSpec{
				{
					Type: v1alpha2.DiskDevice,
					VirtualMachineDisk: &v1alpha2.DiskDeviceSpec{
						Name: vmdName.Name,
					},
				},
			},
			Provisioning: &v1alpha2.Provisioning{
				Type:     v1alpha2.ProvisioningType("UserData"),
				UserData: vmUserData,
			},
		},
	}
	return vm
}

func VMD(namespace, name string, cvmiName v1alpha2.ClusterVirtualMachineImage) v1alpha2.VirtualMachineDisk {
	vmd := v1alpha2.VirtualMachineDisk{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: v1alpha2.VirtualMachineDiskSpec{
			DataSource: &v1alpha2.VMDDataSource{
				Type: v1alpha2.DataSourceTypeClusterVirtualMachineImage,
				ClusterVirtualMachineImage: &v1alpha2.DataSourceNamedRef{
					Name: cvmiName.Name,
				},
			},
			PersistentVolumeClaim: v1alpha2.VMDPersistentVolumeClaim{
				Size: resource.NewQuantity(6*1024*1024*1024, resource.BinarySI),
			},
		},
	}
	return vmd
}

func createVM(client kubeclient.Client, vm v1alpha2.VirtualMachine, vmd v1alpha2.VirtualMachineDisk, namespace string) {
	GinkgoHelper()
	resVMD, err := client.VirtualMachineDisks(namespace).Create(context.TODO(), &vmd, metav1.CreateOptions{})
	Expect(err).NotTo(HaveOccurred(), "cannot create vmd - %s", &vmd.Name)
	resVM, err := client.VirtualMachines(namespace).Create(context.TODO(), &vm, metav1.CreateOptions{})
	Expect(err).NotTo(HaveOccurred(), "cannot create vm - %s", &vm.Name)
	fmt.Println("VM", resVM.Name, "with VMD", resVMD.Name, "created")
}

var _ = Describe("Performance test 26 vm creation", Label("performance"), Ordered, ContinueOnFailure, func() {
	const (
		vmName                         = "perf-test-vm"
		vmdName                        = "perf-test-vmd"
		cvmiName                       = "ubuntu-22.04"
		vmCount                        = 26
		overallTimeout                 = 12 * time.Minute
		deleteGracePeriodSeconds int64 = 30
	)
	// TODO:
	// switch to new api ++
	var cvmi *v1alpha2.ClusterVirtualMachineImage
	// vmMap := make(map[string]string)

	clientConfig := kubeclient.DefaultClientConfig(&pflag.FlagSet{})
	client, err := kubeclient.GetClientFromClientConfig(clientConfig)
	if err != nil {
		Expect(err).NotTo(HaveOccurred(), ("Cannot obtain Virtualization client"))
	}

	AfterAll(func() {
		By("Delete all resources")
		vmList, err := client.VirtualMachines(conf.Namespace).List(context.TODO(), metav1.ListOptions{})
		for _, vm := range vmList.Items {
			err = client.VirtualMachines(conf.Namespace).Delete(context.TODO(), vm.Name, metav1.DeleteOptions{
				GracePeriodSeconds: ptr.To(deleteGracePeriodSeconds),
			})
			Expect(err).NotTo(HaveOccurred())
			if err != nil {
				fmt.Println(err)
			}
		}
	})

	Context("VM", func() {
		cvmi, err = CVMI(client, cvmiName, "create")
		Expect(err).NotTo(HaveOccurred(), "should create CVMI %s", cvmiName)

		It("Create", func() {
			for i := 1; i <= vmCount; i++ {
				vmd := VMD(conf.Namespace, fmt.Sprintf("%s-%d", vmdName, i), *cvmi)
				vm := VM(conf.Namespace, fmt.Sprintf("%s-%d", vmName, i), vmd)
				createVM(client, vm, vmd, conf.Namespace)
			}
		})
		It("Wait until all virtual machines are running", func() {
			Eventually(func() (int, error) {
				vmList, err := client.VirtualMachines(conf.Namespace).List(context.TODO(), metav1.ListOptions{})
				if err != nil {
					return 0, err
				}
				runningCount := 0
				for _, vm := range vmList.Items {
					if string(vm.Status.Phase) == "Running" {
						runningCount++
					}
				}
				return runningCount, nil
			}).WithTimeout(overallTimeout).Should(Equal(vmCount))

			// vmList, err := client.VirtualMachines(conf.Namespace).List(context.TODO(), metav1.ListOptions{})
			// Expect(err).NotTo(HaveOccurred())

			// start := time.Now()
			// fmt.Println("Starting at", start)

			// for len(vmMap) != vmCount {
			// 	vmList, err = client.VirtualMachines(conf.Namespace).List(context.TODO(), metav1.ListOptions{})
			// 	Expect(err).NotTo(HaveOccurred())

			// 	now := time.Now()
			// 	elapsed := now.Sub(start)

			// 	for _, vm := range vmList.Items {
			// 		if vmMap[vm.Name] != vm.Name && string(vm.Status.Phase) == "Running" {
			// 			vmMap[vm.Name] = string(vm.Status.Phase)
			// 		}
			// 	}

			// 	if elapsed > overallTimeout {
			// 		break
			// 	}

			// 	time.Sleep(1 * time.Second)
			// }
			// fmt.Println("Finished at", time.Now())
			// fmt.Println("Duration", time.Now().Sub(start))
			// Expect(len(vmList.Items)).To(Equal(vmCount))
		})
	})
})
