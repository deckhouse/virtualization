package e2e

import (
	"context"
	"fmt"
	kubeclient "github.com/deckhouse/virtualization/api/client/kubeclient"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"log"
	"time"
)

func CVMI(client kubeclient.Client, name string, action string) *v1alpha2.ClusterVirtualMachineImage {

	cvmi := v1alpha2.ClusterVirtualMachineImage{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: v1alpha2.ClusterVirtualMachineImageSpec{
			DataSource: v1alpha2.CVMIDataSource{Type: "HTTP", HTTP: &v1alpha2.DataSourceHTTP{
				URL: "https://cloud-images.ubuntu.com/jammy/current/jammy-server-cloudimg-amd64.img"},
			},
		},
	}

	if action == "create" {
		res, err := client.ClusterVirtualMachineImages().Get(context.TODO(), name, metav1.GetOptions{})
		if res != nil && err == nil {
			return res
		}

		res, err = client.ClusterVirtualMachineImages().Create(context.TODO(), &cvmi, metav1.CreateOptions{})
		if err != nil {
			log.Fatal(err)
		}
		return res
	} else if action == "delete" {
		res, err := client.ClusterVirtualMachineImages().Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			log.Fatal(err)
		}

		err = client.ClusterVirtualMachineImages().Delete(context.TODO(), res.Name, metav1.DeleteOptions{})
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println("Deleted")
		return res
	} else {
		fmt.Println("None")
	}
	return nil
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
	resVMD, err := client.VirtualMachineDisks(namespace).Create(context.TODO(), &vmd, metav1.CreateOptions{})
	Expect(err).NotTo(HaveOccurred(), "cannot create vmd - %s", &vmd.Name)
	resVM, err := client.VirtualMachines(namespace).Create(context.TODO(), &vm, metav1.CreateOptions{})
	Expect(err).NotTo(HaveOccurred(), "cannot create vm - %s", &vm.Name)
	fmt.Println("VM", resVM.Name, "with VMD", resVMD.Name, "created")
}

var _ = Describe("Performance test 20 vm creation", Ordered, ContinueOnFailure, func() {
	const (
		vmName                         = "perf-test-vm"
		vmdName                        = "perf-test-vmd"
		cvmiName                       = "ubuntu-22.04"
		vmCount                        = 26
		notRunningVMCount              = 0
		minutesLimits                  = 12 * time.Minute
		deleteGracePeriodSeconds int64 = 30
	)

	var cvmi *v1alpha2.ClusterVirtualMachineImage
	var vmdList []string
	var vmMap []string

	clientConfig := kubeclient.DefaultClientConfig(&pflag.FlagSet{})
	client, err := kubeclient.GetClientFromClientConfig(clientConfig)
	if err != nil {
		log.Fatalf("Cannot obtain Virtualization client: %v\n", err)
	}

	AfterAll(func() {
		By("Delete all resources")
		for _, name := range vmMap {
			err = client.VirtualMachines(conf.Namespace).Delete(context.TODO(), name, metav1.DeleteOptions{
				GracePeriodSeconds: func(i int64) *int64 { return &i }(deleteGracePeriodSeconds)})
			//Expect(err).NotTo(HaveOccurred())
			if err != nil {
				fmt.Println(err)
			}
		}
		//for _, disk := range vmdList {
		//	err = client.VirtualMachineDisks(conf.Namespace).Delete(context.TODO(), disk, metav1.DeleteOptions{
		//		GracePeriodSeconds: func(i int64) *int64 { return &i }(deleteGracePeriodSeconds)})
		//	//Expect(err).NotTo(HaveOccurred())
		//	if err != nil {
		//		fmt.Println(err)
		//	}
		//}
	})

	Context("VM", func() {
		cvmi = CVMI(client, cvmiName, "create")
		It("Create", func() {
			for i := 1; i <= vmCount; i++ {
				vmd := VMD(conf.Namespace, fmt.Sprintf("%s-%d", vmdName, i), *cvmi)
				vm := VM(conf.Namespace, fmt.Sprintf("%s-%d", vmName, i), vmd)
				createVM(client, vm, vmd, conf.Namespace)
				vmMap = append(vmMap, vm.Name)
				vmdList = append(vmdList, vmd.Name)
			}
		})
		It("Wait until all virtual machines are running", func() {
			runningVM := 0

			start := time.Now()
			fmt.Println("Starting at", start)

			for len(vmMap) != notRunningVMCount {

				for i, name := range vmMap {

					vm, err := client.VirtualMachines(conf.Namespace).Get(context.TODO(), name, metav1.GetOptions{})
					Expect(err).NotTo(HaveOccurred())

					if string(vm.Status.Phase) == "Running" {
						runningVM += 1
						vmMap = append(vmMap[:i], vmMap[i+1:]...)
					}
				}

				now := time.Now()
				elapsed := now.Sub(start)

				if elapsed > overallTimeout {
					break
				}
				time.Sleep(1 * time.Second)
			}
			fmt.Println("Finished at", time.Now())
			fmt.Println("Duration", time.Now().Sub(start))
			Expect(len(vmMap)).To(Equal(notRunningVMCount))

		})
	})
})
