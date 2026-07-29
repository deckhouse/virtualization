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

package step

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/dvcr/registrytoken"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func TestSteps(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "VirtualDisk Source Steps")
}

func noopEvent(client.Object, string, string, string) {}

func newTestRecorder() *eventrecord.EventRecorderLoggerMock {
	var recorder *eventrecord.EventRecorderLoggerMock
	recorder = &eventrecord.EventRecorderLoggerMock{
		EventFunc: noopEvent,
		WithLoggingFunc: func(eventrecord.InfoLogger) eventrecord.EventRecorderLogger {
			return recorder
		},
	}
	return recorder
}

func newTestSigner() *registrytoken.Signer {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	Expect(err).ToNot(HaveOccurred())
	der, err := x509.MarshalPKCS8PrivateKey(key)
	Expect(err).ToNot(HaveOccurred())
	signer, err := registrytoken.NewSignerFromPEM(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	Expect(err).ToNot(HaveOccurred())
	return signer
}

func newStepScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	Expect(corev1.AddToScheme(scheme)).To(Succeed())
	Expect(netv1.AddToScheme(scheme)).To(Succeed())
	Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())
	return scheme
}

var (
	testVMToleration = corev1.Toleration{
		Key:      "dedicated.deckhouse.io",
		Operator: corev1.TolerationOpEqual,
		Value:    "vm-workloads",
		Effect:   corev1.TaintEffectNoSchedule,
	}
	testVMClassToleration = corev1.Toleration{
		Key:      "dedicated.deckhouse.io",
		Operator: corev1.TolerationOpEqual,
		Value:    "vm-class",
		Effect:   corev1.TaintEffectNoSchedule,
	}
	testSystemToleration = corev1.Toleration{
		Key:      "dedicated.deckhouse.io",
		Operator: corev1.TolerationOpEqual,
		Value:    "system",
	}
)

func newTestVM(tolerations ...corev1.Toleration) *v1alpha2.VirtualMachine {
	return &v1alpha2.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "default"},
		Spec: v1alpha2.VirtualMachineSpec{
			VirtualMachineClassName: "vmclass",
			CPU:                     v1alpha2.CPUSpec{Cores: 2},
			Tolerations:             tolerations,
		},
	}
}

func newTestVMClass(tolerations ...corev1.Toleration) *v1alpha2.VirtualMachineClass {
	return &v1alpha2.VirtualMachineClass{
		ObjectMeta: metav1.ObjectMeta{Name: "vmclass"},
		Spec:       v1alpha2.VirtualMachineClassSpec{Tolerations: tolerations},
	}
}

func newTestVD(ds *v1alpha2.VirtualDiskDataSource, attachedVMs ...string) *v1alpha2.VirtualDisk {
	vd := &v1alpha2.VirtualDisk{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1alpha2.SchemeGroupVersion.String(),
			Kind:       v1alpha2.VirtualDiskKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vd",
			Namespace: "default",
			UID:       types.UID("vd-uid"),
		},
		Spec: v1alpha2.VirtualDiskSpec{DataSource: ds},
	}
	for _, name := range attachedVMs {
		vd.Status.AttachedToVirtualMachines = append(vd.Status.AttachedToVirtualMachines, v1alpha2.AttachedVirtualMachine{Name: name})
	}
	return vd
}
