package internal

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
)

func TestHandlers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Live migration handlers suite")
}

func setupEnvironment(kvvmi *virtv1.VirtualMachineInstance, objs ...client.Object) client.WithWatch {
	GinkgoHelper()
	Expect(kvvmi).ToNot(BeNil(), "Should set KVVMI for setupEnvironment")
	allObjects := []client.Object{kvvmi}
	allObjects = append(allObjects, objs...)

	fakeClient, err := testutil.NewFakeClientWithObjects(allObjects...)
	Expect(err).NotTo(HaveOccurred())

	return fakeClient
}
