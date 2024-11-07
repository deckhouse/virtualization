package vm

import (
	"encoding/json"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"

	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type Template interface {
	WithOptions(options ...Option)
	Object() *virtv2.VirtualMachine
	KVVM() *virtv1.VirtualMachine
	Render() ([]byte, error)
}

func New(namespacedName types.NamespacedName) Template {
	return &virtualMachineTemplate{
		vm: newBasicVirtualMachine(namespacedName),
	}
}

type virtualMachineTemplate struct {
	vm   *virtv2.VirtualMachine
	kvvm *virtv1.VirtualMachine
}

func (t virtualMachineTemplate) KVVM() *virtv1.VirtualMachine {
	//if t.kvvm == nil {
	//	t.kvvm = kvbuilder.NewEmptyKVVM(common.NamespacedName(t.vm), kvbuilder.NewKVVMOptions(t.vm.Spec))
	//
	//}
	//TODO implement me
	panic("implement me")
}

func (t virtualMachineTemplate) WithOptions(options ...Option) {
	for _, o := range options {
		o.Apply(t.vm)
	}
}

func (t virtualMachineTemplate) Object() *virtv2.VirtualMachine {
	return t.vm
}

func (t virtualMachineTemplate) Render() ([]byte, error) {
	return json.Marshal(t.vm)
}

func newBasicVirtualMachine(name types.NamespacedName) *virtv2.VirtualMachine {
	return &virtv2.VirtualMachine{
		TypeMeta: metav1.TypeMeta{
			APIVersion: virtv2.SchemeGroupVersion.String(),
			Kind:       virtv2.VirtualMachineKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name.Name,
			Namespace: name.Namespace,
		},
	}
}

type Option interface {
	Apply(vm *virtv2.VirtualMachine)
}

type metadataOption struct {
	metadata metav1.ObjectMeta
}

func (o *metadataOption) Apply(vm *virtv2.VirtualMachine) {
	if vm != nil {
		vm.ObjectMeta = o.metadata
	}
}

type namespacedNameOption types.NamespacedName

func (o *namespacedNameOption) Apply(vm *virtv2.VirtualMachine) {
	if vm != nil {
		vm.Name = o.Name
		vm.Namespace = o.Namespace
	}
}

type labelsOption struct {
	labels map[string]string
}

func (l *labelsOption) Apply(vm *virtv2.VirtualMachine) {
	if vm != nil {
		vm.Labels = l.labels
	}
}

func WithLabels(labels map[string]string) Option {
	return &labelsOption{labels}
}

type annotationsOption struct {
	annotations map[string]string
}

func (a *annotationsOption) Apply(vm *virtv2.VirtualMachine) {
	if vm != nil {
		vm.Annotations = a.annotations
	}
}

func WithAnnotations(annotations map[string]string) Option {
	return &annotationsOption{annotations}
}

type specOption struct {
	spec virtv2.VirtualMachineSpec
}

func (o *specOption) Apply(vm *virtv2.VirtualMachine) {
	if vm != nil {
		vm.Spec = o.spec
	}
}

func WithSpec(spec virtv2.VirtualMachineSpec) Option {
	return &specOption{spec}
}

type runPolicyOption virtv2.RunPolicy

func (o runPolicyOption) Apply(vm *virtv2.VirtualMachine) {
	if vm != nil {
		vm.Spec.RunPolicy = virtv2.RunPolicy(o)
	}
}

func WithRunPolicy(policy virtv2.RunPolicy) Option {
	return runPolicyOption(policy)
}

type ipAddressOption string

func (o ipAddressOption) Apply(vm *virtv2.VirtualMachine) {
	if vm != nil {
		vm.Spec.VirtualMachineIPAddress = string(o)
	}
}

func WithIPAddress(address string) Option {
	return ipAddressOption(address)
}

type topologyOption []corev1.TopologySpreadConstraint

func (o topologyOption) Apply(vm *virtv2.VirtualMachine) {
	if vm != nil {
		vm.Spec.TopologySpreadConstraints = o
	}
}

func WithTopologySpreadConstraints(constraints ...corev1.TopologySpreadConstraint) Option {
	return topologyOption(constraints)
}

type affinityOption struct {
	affinity *virtv2.VMAffinity
}

func (o affinityOption) Apply(vm *virtv2.VirtualMachine) {
	if vm != nil {
		vm.Spec.Affinity = o.affinity
	}
}

func WithAffinity(affinity *virtv2.VMAffinity) Option {
	return affinityOption{affinity: affinity}
}

type nodeSelectorOption struct {
	nodeSelector map[string]string
}

func (o *nodeSelectorOption) Apply(vm *virtv2.VirtualMachine) {
	if vm != nil {
		vm.Spec.NodeSelector = o.nodeSelector
	}
}

func WithNodeSelector(selector map[string]string) Option {
	return &nodeSelectorOption{nodeSelector: selector}
}

type priorityClassOption string

func (o priorityClassOption) Apply(vm *virtv2.VirtualMachine) {
	if vm != nil {
		vm.Spec.PriorityClassName = string(o)
	}
}

func WithPriorityClassName(name string) Option {
	return priorityClassOption(name)
}

type tolerationsOption []corev1.Toleration

func (o tolerationsOption) Apply(vm *virtv2.VirtualMachine) {
	if vm != nil {
		vm.Spec.Tolerations = o
	}
}

func WithTolerations(tolerations ...corev1.Toleration) Option {
	return tolerationsOption(tolerations)
}

type disruptionOption struct {
	disruptions *virtv2.Disruptions
}

func (o *disruptionOption) Apply(vm *virtv2.VirtualMachine) {
	if vm != nil {
		vm.Spec.Disruptions = o.disruptions
	}
}

func WithDisruptions(disruptions *virtv2.Disruptions) Option {
	return &disruptionOption{disruptions: disruptions}
}

type gracePeriodOption struct {
	gracePeriod *int64
}

func (o *gracePeriodOption) Apply(vm *virtv2.VirtualMachine) {
	if vm != nil {
		vm.Spec.TerminationGracePeriodSeconds = o.gracePeriod
	}
}

func WithTerminationGracePeriod(gracePeriod int64) Option {
	return &gracePeriodOption{gracePeriod: &gracePeriod}
}

type enableParaVirtualizationOption bool

func (o enableParaVirtualizationOption) Apply(vm *virtv2.VirtualMachine) {
	if vm != nil {
		vm.Spec.EnableParavirtualization = bool(o)
	}
}

func WithEnableParaVirtualization(enableParaVirtualization bool) Option {
	return enableParaVirtualizationOption(enableParaVirtualization)
}

type osTypeOption virtv2.OsType

func (o osTypeOption) Apply(vm *virtv2.VirtualMachine) {
	if vm != nil {
		vm.Spec.OsType = virtv2.OsType(o)
	}
}

func WithOsType(osType virtv2.OsType) Option {
	return osTypeOption(osType)
}

type bootLoaderOption virtv2.BootloaderType

func (o bootLoaderOption) Apply(vm *virtv2.VirtualMachine) {
	if vm != nil {
		vm.Spec.Bootloader = virtv2.BootloaderType(o)
	}
}

func WithBootLoaderType(bootloader virtv2.BootloaderType) Option {
	return bootLoaderOption(bootloader)
}

type classOption string

func (o classOption) Apply(vm *virtv2.VirtualMachine) {
	if vm != nil {
		vm.Spec.VirtualMachineClassName = string(o)
	}
}

func WithVirtualMachineClassName(name string) Option {
	return classOption(name)
}

type cpuSpecOption struct {
	cpuSpec virtv2.CPUSpec
}

func (o *cpuSpecOption) Apply(vm *virtv2.VirtualMachine) {
	if vm != nil {
		vm.Spec.CPU = o.cpuSpec
	}
}

func WithCPUSpec(cpu virtv2.CPUSpec) Option {
	return &cpuSpecOption{cpuSpec: cpu}
}

type memorySpecOption struct {
	memorySpec virtv2.MemorySpec
}

func (o *memorySpecOption) Apply(vm *virtv2.VirtualMachine) {
	if vm != nil {
		vm.Spec.Memory = o.memorySpec
	}
}

func WithMemorySpec(memory virtv2.MemorySpec) Option {
	return &memorySpecOption{memorySpec: memory}
}

type blockDeviceRefsOption struct {
	blockDeviceRefs []virtv2.BlockDeviceSpecRef
}

func (o *blockDeviceRefsOption) Apply(vm *virtv2.VirtualMachine) {
	if vm != nil {
		vm.Spec.BlockDeviceRefs = o.blockDeviceRefs
	}
}

func WithBlockDevicesRefs(refs []virtv2.BlockDeviceSpecRef) Option {
	return &blockDeviceRefsOption{blockDeviceRefs: refs}
}

type provisioningOption struct {
	provisioning *virtv2.Provisioning
}

func (o *provisioningOption) Apply(vm *virtv2.VirtualMachine) {
	if vm != nil {
		vm.Spec.Provisioning = o.provisioning
	}
}

func WithProvisioning(provisioning *virtv2.Provisioning) Option {
	return &provisioningOption{provisioning: provisioning}
}

type statusOption struct {
	status virtv2.VirtualMachineStatus
}

func (o *statusOption) Apply(vm *virtv2.VirtualMachine) {
	if vm != nil {
		vm.Status = o.status
	}
}

func WithStatus(status virtv2.VirtualMachineStatus) Option {
	return &statusOption{status: status}
}
