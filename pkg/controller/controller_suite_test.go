package controller

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/deckhouse/virtualization-controller/api/v2alpha1"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/two_phase_reconciler"
)

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))
})

func TestController(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Controller Suite")
}

var vmdControllerLog = logf.Log.WithName("vmd-controller-test")

func NewVMDReconciler(objects ...runtime.Object) *two_phase_reconciler.ReconcilerCore[*VMDReconcilerState] {
	objs := []runtime.Object{}
	objs = append(objs, objects...)

	s := scheme.Scheme
	_ = cdiv1.AddToScheme(s)
	_ = metav1.AddMetaToScheme(s)
	_ = v2alpha1.AddToScheme(s)

	builder := fake.NewClientBuilder().
		WithScheme(s).
		WithRuntimeObjects(objs...)

	cl := builder.Build()
	rec := record.NewFakeRecorder(10)

	reconciler := &VMDReconciler{}
	return two_phase_reconciler.NewReconcilerCore[*VMDReconcilerState](
		reconciler,
		NewVMDReconcilerState,
		two_phase_reconciler.ReconcilerOptions{
			Client:   cl,
			Recorder: rec,
			Scheme:   s,
			Log:      vmdControllerLog,
		})
}
