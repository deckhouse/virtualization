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

package source

import (
	"context"
	"errors"

	vsv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/controller"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/importer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

var _ = Describe("Source validations and helpers", func() {
	newScheme := func() *runtime.Scheme {
		scheme := runtime.NewScheme()
		Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(netv1.AddToScheme(scheme)).To(Succeed())
		Expect(vsv1.AddToScheme(scheme)).To(Succeed())
		return scheme
	}

	newRecorder := func() *eventrecord.EventRecorderLoggerMock {
		return &eventrecord.EventRecorderLoggerMock{EventFunc: func(client.Object, string, string, string) {}}
	}

	newVI := func() *v1alpha2.VirtualImage {
		return &v1alpha2.VirtualImage{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "vi",
				Namespace: "default",
				UID:       "vi-uid",
			},
			Spec: v1alpha2.VirtualImageSpec{
				DataSource: v1alpha2.VirtualImageDataSource{
					Type: v1alpha2.DataSourceTypeObjectRef,
				},
			},
		}
	}

	DescribeTable(
		"builds typed source errors",
		func(factory func(string) error, expected string) {
			Expect(factory("test")).To(MatchError(expected))
		},
		Entry("image not ready", NewImageNotReadyError, "VirtualImage test not ready"),
		Entry("cluster image not ready", NewClusterImageNotReadyError, "ClusterVirtualImage test not ready"),
		Entry("virtual disk not ready", NewVirtualDiskNotReadyError, "VirtualDisk test not ready"),
		Entry("virtual disk not ready for use", NewVirtualDiskNotReadyForUseError, "the VirtualDisk test not ready for use"),
		Entry("virtual disk attached", NewVirtualDiskAttachedToVirtualMachineError, "the VirtualDisk test attached to VirtualMachine"),
		Entry("virtual disk snapshot not ready", NewVirtualDiskSnapshotNotReadyError, "VirtualDiskSnapshot test not ready"),
	)

	Describe("validateVirtualDiskSnapshot", func() {
		var (
			ctx    context.Context
			scheme *runtime.Scheme
			vi     *v1alpha2.VirtualImage
		)

		BeforeEach(func() {
			ctx = context.Background()
			scheme = newScheme()
			vi = newVI()
			vi.Spec.DataSource.ObjectRef = &v1alpha2.VirtualImageObjectRef{
				Kind: v1alpha2.VirtualImageObjectRefKindVirtualDiskSnapshot,
				Name: "snap",
			}
		})

		It("returns error when object ref is missing", func() {
			vi.Spec.DataSource.ObjectRef = nil
			client := fake.NewClientBuilder().WithScheme(scheme).Build()

			Expect(validateVirtualDiskSnapshot(ctx, vi, client)).To(MatchError("object ref missed for data source"))
		})

		It("returns not ready when snapshot is absent", func() {
			client := fake.NewClientBuilder().WithScheme(scheme).Build()

			Expect(validateVirtualDiskSnapshot(ctx, vi, client)).To(MatchError("VirtualDiskSnapshot snap not ready"))
		})

		It("returns not ready when volume snapshot is not ready to use", func() {
			vdSnapshot := &v1alpha2.VirtualDiskSnapshot{
				ObjectMeta: metav1.ObjectMeta{Name: "snap", Namespace: vi.Namespace},
				Status: v1alpha2.VirtualDiskSnapshotStatus{
					Phase:              v1alpha2.VirtualDiskSnapshotPhaseReady,
					VolumeSnapshotName: "vs",
				},
			}
			vs := &vsv1.VolumeSnapshot{
				ObjectMeta: metav1.ObjectMeta{Name: "vs", Namespace: vi.Namespace},
				Status:     &vsv1.VolumeSnapshotStatus{ReadyToUse: ptr.To(false)},
			}
			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vdSnapshot, vs).Build()

			Expect(validateVirtualDiskSnapshot(ctx, vi, client)).To(MatchError("VirtualDiskSnapshot snap not ready"))
		})

		It("succeeds for ready snapshot and ready volume snapshot", func() {
			vdSnapshot := &v1alpha2.VirtualDiskSnapshot{
				ObjectMeta: metav1.ObjectMeta{Name: "snap", Namespace: vi.Namespace},
				Status: v1alpha2.VirtualDiskSnapshotStatus{
					Phase:              v1alpha2.VirtualDiskSnapshotPhaseReady,
					VolumeSnapshotName: "vs",
				},
			}
			vs := &vsv1.VolumeSnapshot{
				ObjectMeta: metav1.ObjectMeta{Name: "vs", Namespace: vi.Namespace},
				Status:     &vsv1.VolumeSnapshotStatus{ReadyToUse: ptr.To(true)},
			}
			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vdSnapshot, vs).Build()
			syncerCR := NewObjectRefVirtualDiskSnapshotCR(nil, nil, nil, client, nil, newRecorder())
			syncerPVC := NewObjectRefVirtualDiskSnapshotPVC(nil, nil, nil, nil, client, nil, newRecorder())

			Expect(syncerCR.Validate(ctx, vi)).To(Succeed())
			Expect(syncerPVC.Validate(ctx, vi)).To(Succeed())
		})
	})

	Describe("ObjectRefVirtualDisk", func() {
		var (
			ctx      context.Context
			scheme   *runtime.Scheme
			settings *dvcr.Settings
			vi       *v1alpha2.VirtualImage
		)

		BeforeEach(func() {
			ctx = context.Background()
			scheme = newScheme()
			settings = &dvcr.Settings{AuthSecret: "dvcr-auth", RegistryURL: "registry.example"}
			vi = newVI()
			vi.Spec.DataSource.ObjectRef = &v1alpha2.VirtualImageObjectRef{
				Kind: v1alpha2.VirtualImageObjectRefKindVirtualDisk,
				Name: "vd",
			}
		})

		It("constructs syncer", func() {
			syncer := NewObjectRefVirtualDisk(newRecorder(), nil, fake.NewClientBuilder().WithScheme(scheme).Build(), nil, settings, nil)
			Expect(syncer).ToNot(BeNil())
		})

		It("returns error for non virtual disk source", func() {
			vi.Spec.DataSource.ObjectRef.Kind = v1alpha2.VirtualImageObjectRefKindVirtualImage
			syncer := NewObjectRefVirtualDisk(newRecorder(), nil, fake.NewClientBuilder().WithScheme(scheme).Build(), nil, settings, nil)

			Expect(syncer.Validate(ctx, vi)).To(MatchError("not a VirtualDisk data source"))
		})

		It("returns not ready when virtual disk is absent", func() {
			syncer := NewObjectRefVirtualDisk(newRecorder(), nil, fake.NewClientBuilder().WithScheme(scheme).Build(), nil, settings, nil)

			Expect(syncer.Validate(ctx, vi)).To(MatchError("VirtualDisk vd not ready"))
		})

		It("returns not ready for use when in-use condition is stale", func() {
			vd := &v1alpha2.VirtualDisk{
				ObjectMeta: metav1.ObjectMeta{Name: "vd", Namespace: vi.Namespace, Generation: 2},
				Status: v1alpha2.VirtualDiskStatus{
					Phase: v1alpha2.DiskReady,
					Conditions: []metav1.Condition{{
						Type:               vdcondition.InUseType.String(),
						Status:             metav1.ConditionTrue,
						Reason:             vdcondition.UsedForImageCreation.String(),
						ObservedGeneration: 1,
					}},
				},
			}
			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vd).Build()
			syncer := NewObjectRefVirtualDisk(newRecorder(), nil, client, nil, settings, nil)

			Expect(syncer.Validate(ctx, vi)).To(MatchError("the VirtualDisk vd not ready for use"))
		})

		It("allows virtual disk used for image creation", func() {
			vd := &v1alpha2.VirtualDisk{
				ObjectMeta: metav1.ObjectMeta{Name: "vd", Namespace: vi.Namespace, Generation: 2},
				Status: v1alpha2.VirtualDiskStatus{
					Phase: v1alpha2.DiskReady,
					Conditions: []metav1.Condition{{
						Type:               vdcondition.InUseType.String(),
						Status:             metav1.ConditionTrue,
						Reason:             vdcondition.UsedForImageCreation.String(),
						ObservedGeneration: 2,
					}},
				},
			}
			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vd).Build()
			syncer := NewObjectRefVirtualDisk(newRecorder(), nil, client, nil, settings, nil)

			Expect(syncer.Validate(ctx, vi)).To(Succeed())
		})

		It("returns attached to virtual machine error", func() {
			vd := &v1alpha2.VirtualDisk{
				ObjectMeta: metav1.ObjectMeta{Name: "vd", Namespace: vi.Namespace, Generation: 1},
				Status: v1alpha2.VirtualDiskStatus{
					Phase: v1alpha2.DiskReady,
					Conditions: []metav1.Condition{{
						Type:               vdcondition.InUseType.String(),
						Status:             metav1.ConditionTrue,
						Reason:             vdcondition.AttachedToVirtualMachine.String(),
						ObservedGeneration: 1,
					}},
				},
			}
			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vd).Build()
			syncer := NewObjectRefVirtualDisk(newRecorder(), nil, client, nil, settings, nil)

			Expect(syncer.Validate(ctx, vi)).To(MatchError("the VirtualDisk vd attached to VirtualMachine"))
		})

		It("skips in-use checks for ready image", func() {
			vi.Status.Phase = v1alpha2.ImageReady
			vd := &v1alpha2.VirtualDisk{
				ObjectMeta: metav1.ObjectMeta{Name: "vd", Namespace: vi.Namespace},
				Status:     v1alpha2.VirtualDiskStatus{Phase: v1alpha2.DiskReady},
			}
			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vd).Build()
			syncer := NewObjectRefVirtualDisk(newRecorder(), nil, client, nil, settings, nil)

			Expect(syncer.Validate(ctx, vi)).To(Succeed())
		})

		It("builds importer settings for filesystem and block pvc", func() {
			supgen := supplements.NewGenerator(annotations.VIShortName, vi.Name, vi.Namespace, vi.UID)
			syncer := NewObjectRefVirtualDisk(newRecorder(), nil, fake.NewClientBuilder().WithScheme(scheme).Build(), nil, settings, nil)

			fsSettings := syncer.getEnvSettings(vi, supgen, ptr.To(corev1.PersistentVolumeFilesystem))
			blockSettings := syncer.getEnvSettings(vi, supgen, ptr.To(corev1.PersistentVolumeBlock))

			Expect(fsSettings.Source).To(Equal(importer.SourceFilesystem))
			Expect(blockSettings.Source).To(Equal(importer.SourceBlockDevice))
			Expect(blockSettings.DestinationEndpoint).To(ContainSubstring("registry.example/vi/default/vi:vi-uid"))
		})
	})

	Describe("ObjectRefDataSource", func() {
		var (
			ctx      context.Context
			scheme   *runtime.Scheme
			settings *dvcr.Settings
			vi       *v1alpha2.VirtualImage
		)

		BeforeEach(func() {
			ctx = context.Background()
			scheme = newScheme()
			settings = &dvcr.Settings{AuthSecret: "dvcr-auth", RegistryURL: "registry.example"}
			vi = newVI()
		})

		It("constructs data source with nested syncers", func() {
			ds := NewObjectRefDataSource(newRecorder(), nil, nil, nil, settings, fake.NewClientBuilder().WithScheme(scheme).Build(), nil)
			Expect(ds).ToNot(BeNil())
			Expect(ds.vdSyncer).ToNot(BeNil())
			Expect(ds.vdSnapshotCRSyncer).ToNot(BeNil())
			Expect(ds.vdSnapshotPVCSyncer).ToNot(BeNil())
		})

		It("returns error when object ref is nil", func() {
			ds := NewObjectRefDataSource(newRecorder(), nil, nil, nil, settings, fake.NewClientBuilder().WithScheme(scheme).Build(), nil)
			Expect(ds.Validate(ctx, vi)).To(MatchError("nil object ref: ObjectRef"))
		})

		It("validates kubernetes virtual image references by phase", func() {
			vi.Spec.DataSource.ObjectRef = &v1alpha2.VirtualImageObjectRef{Kind: v1alpha2.VirtualImageObjectRefKindVirtualImage, Name: "ref"}
			ref := &v1alpha2.VirtualImage{
				ObjectMeta: metav1.ObjectMeta{Name: "ref", Namespace: vi.Namespace},
				Spec:       v1alpha2.VirtualImageSpec{Storage: v1alpha2.StorageKubernetes},
				Status:     v1alpha2.VirtualImageStatus{Phase: v1alpha2.ImageReady},
			}
			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ref).Build()
			ds := NewObjectRefDataSource(newRecorder(), nil, nil, nil, settings, client, nil)

			Expect(ds.Validate(ctx, vi)).To(Succeed())
			ref.Status.Phase = v1alpha2.ImagePending
			client = fake.NewClientBuilder().WithScheme(scheme).WithObjects(ref).Build()
			ds = NewObjectRefDataSource(newRecorder(), nil, nil, nil, settings, client, nil)
			Expect(ds.Validate(ctx, vi)).To(MatchError("VirtualImage ref not ready"))
		})

		It("validates cluster virtual image references through dvcr state", func() {
			vi.Spec.DataSource.ObjectRef = &v1alpha2.VirtualImageObjectRef{Kind: v1alpha2.VirtualImageObjectRefKindClusterVirtualImage, Name: "cvi"}
			cvi := &v1alpha2.ClusterVirtualImage{
				ObjectMeta: metav1.ObjectMeta{Name: "cvi"},
				Status:     v1alpha2.ClusterVirtualImageStatus{Phase: v1alpha2.ImageReady},
			}
			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cvi).Build()
			ds := NewObjectRefDataSource(newRecorder(), nil, nil, nil, settings, client, nil)

			Expect(ds.Validate(ctx, vi)).To(Succeed())
			cvi.Status.Phase = v1alpha2.ImagePending
			client = fake.NewClientBuilder().WithScheme(scheme).WithObjects(cvi).Build()
			ds = NewObjectRefDataSource(newRecorder(), nil, nil, nil, settings, client, nil)
			Expect(ds.Validate(ctx, vi)).To(MatchError("ClusterVirtualImage cvi not ready"))
		})

		It("delegates virtual disk validation", func() {
			vi.Spec.DataSource.ObjectRef = &v1alpha2.VirtualImageObjectRef{Kind: v1alpha2.VirtualImageObjectRefKindVirtualDisk, Name: "vd"}
			vd := &v1alpha2.VirtualDisk{
				ObjectMeta: metav1.ObjectMeta{Name: "vd", Namespace: vi.Namespace, Generation: 1},
				Status: v1alpha2.VirtualDiskStatus{
					Phase: v1alpha2.DiskReady,
					Conditions: []metav1.Condition{{
						Type:               vdcondition.InUseType.String(),
						Status:             metav1.ConditionTrue,
						Reason:             vdcondition.UsedForImageCreation.String(),
						ObservedGeneration: 1,
					}},
				},
			}
			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vd).Build()
			ds := NewObjectRefDataSource(newRecorder(), nil, nil, nil, settings, client, nil)

			Expect(ds.Validate(ctx, vi)).To(Succeed())
		})

		It("returns error for unexpected kind", func() {
			vi.Spec.DataSource.ObjectRef = &v1alpha2.VirtualImageObjectRef{Kind: "SomethingElse", Name: "bad"}
			ds := NewObjectRefDataSource(newRecorder(), nil, nil, nil, settings, fake.NewClientBuilder().WithScheme(scheme).Build(), nil)

			Expect(ds.Validate(ctx, vi)).To(MatchError("unexpected object ref kind: SomethingElse"))
		})

		It("builds settings and sources from ready dvcr source", func() {
			vi.Spec.DataSource.ObjectRef = &v1alpha2.VirtualImageObjectRef{Kind: v1alpha2.VirtualImageObjectRefKindClusterVirtualImage, Name: "cvi"}
			cvi := &v1alpha2.ClusterVirtualImage{
				ObjectMeta: metav1.ObjectMeta{Name: "cvi", UID: "cvi-uid"},
				Status: v1alpha2.ClusterVirtualImageStatus{
					Phase:  v1alpha2.ImageReady,
					Format: "qcow2",
					Size:   v1alpha2.ImageStatusSize{UnpackedBytes: "5Gi"},
					Target: v1alpha2.ClusterVirtualImageStatusTarget{RegistryURL: "registry.example/source"},
				},
			}
			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cvi).Build()
			ds := NewObjectRefDataSource(newRecorder(), nil, nil, nil, settings, client, nil)
			supgen := supplements.NewGenerator(annotations.VIShortName, vi.Name, vi.Namespace, vi.UID)
			dvcrSource, err := controller.NewDVCRDataSourcesForVMI(ctx, vi.Spec.DataSource, vi, client)
			Expect(err).ToNot(HaveOccurred())

			envSettings, err := ds.getEnvSettings(vi, supgen, dvcrSource)
			Expect(err).ToNot(HaveOccurred())
			Expect(envSettings.Source).To(Equal(importer.SourceDVCR))
			Expect(envSettings.Endpoint).To(Equal("registry.example/source"))

			pvcSize, err := ds.getPVCSize(dvcrSource)
			Expect(err).ToNot(HaveOccurred())
			Expect(pvcSize).To(Equal(resource.MustParse("5Gi")))

			source, err := ds.getSource(supgen, dvcrSource)
			Expect(err).ToNot(HaveOccurred())
			Expect(*source.Registry.URL).To(Equal("docker://registry.example/source"))
		})

		It("rejects not ready dvcr source in helper methods", func() {
			ds := NewObjectRefDataSource(newRecorder(), nil, nil, nil, settings, fake.NewClientBuilder().WithScheme(scheme).Build(), nil)
			notReady := controller.DVCRDataSource{}
			supgen := supplements.NewGenerator(annotations.VIShortName, vi.Name, vi.Namespace, vi.UID)

			_, err := ds.getEnvSettings(vi, supgen, notReady)
			Expect(err).To(MatchError("dvcr data source is not ready"))
			_, err = ds.getPVCSize(notReady)
			Expect(err).To(MatchError("dvcr data source is not ready"))
			_, err = ds.getSource(supgen, notReady)
			Expect(err).To(MatchError("dvcr data source is not ready"))
		})
	})

	Describe("Generic datasource helpers", func() {
		var (
			ctx      context.Context
			vi       *v1alpha2.VirtualImage
			settings *dvcr.Settings
		)

		BeforeEach(func() {
			ctx = context.Background()
			vi = newVI()
			vi.Spec.DataSource.ContainerImage = &v1alpha2.VirtualImageContainerImage{
				Image:           "docker.io/library/alpine:latest",
				ImagePullSecret: v1alpha2.ImagePullSecretName{Name: "pull-secret"},
				CABundle:        []byte("ca-data"),
			}
			vi.Spec.DataSource.HTTP = &v1alpha2.DataSourceHTTP{URL: "https://example.com/image.qcow2"}
			settings = &dvcr.Settings{AuthSecret: "dvcr-auth", RegistryURL: "registry.example"}
		})

		It("covers registry helpers", func() {
			scheme := newScheme()
			recorder := newRecorder()
			registry := NewRegistryDataSource(recorder, &StatMock{GetSizeFunc: func(*corev1.Pod) v1alpha2.ImageStatusSize {
				return v1alpha2.ImageStatusSize{UnpackedBytes: "2Gi"}
			}}, &ImporterMock{}, settings, fake.NewClientBuilder().WithScheme(scheme).Build(), nil)
			supgen := supplements.NewGenerator(annotations.VIShortName, vi.Name, vi.Namespace, vi.UID)

			Expect(registry.Validate(ctx, vi)).To(MatchError(ErrSecretNotFound))
			secretClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "pull-secret", Namespace: vi.Namespace}}).Build()
			registry.client = secretClient
			Expect(registry.Validate(ctx, vi)).To(Succeed())
			Expect(registry.getEnvSettings(vi, supgen).Source).To(Equal(importer.SourceRegistry))
			q, err := registry.getPVCSize(&corev1.Pod{})
			Expect(err).ToNot(HaveOccurred())
			Expect(q).To(Equal(resource.MustParse("2Gi")))
			Expect(*registry.getSource(supgen, "registry.example/image").Registry.URL).To(Equal("docker://registry.example/image"))
		})

		It("covers http helpers", func() {
			httpDS := NewHTTPDataSource(newRecorder(), &StatMock{GetSizeFunc: func(*corev1.Pod) v1alpha2.ImageStatusSize {
				return v1alpha2.ImageStatusSize{UnpackedBytes: "3Gi"}
			}}, &ImporterMock{}, settings, nil)
			supgen := supplements.NewGenerator(annotations.VIShortName, vi.Name, vi.Namespace, vi.UID)

			Expect(httpDS.Validate(ctx, vi)).To(Succeed())
			Expect(httpDS.getEnvSettings(vi, supgen).Source).To(Equal(importer.SourceHTTP))
			q, err := httpDS.getPVCSize(&corev1.Pod{})
			Expect(err).ToNot(HaveOccurred())
			Expect(q).To(Equal(resource.MustParse("3Gi")))
			Expect(*httpDS.getSource(supgen, "registry.example/image").Registry.URL).To(Equal("docker://registry.example/image"))
		})

		It("covers upload helpers", func() {
			upload := NewUploadDataSource(newRecorder(), &StatMock{GetSizeFunc: func(*corev1.Pod) v1alpha2.ImageStatusSize {
				return v1alpha2.ImageStatusSize{UnpackedBytes: "4Gi"}
			}}, &UploaderMock{}, settings, nil, fake.NewClientBuilder().WithScheme(newScheme()).Build())
			supgen := supplements.NewGenerator(annotations.VIShortName, vi.Name, vi.Namespace, vi.UID)

			Expect(upload.Validate(ctx, vi)).To(Succeed())
			Expect(upload.getEnvSettings(vi, supgen)).ToNot(BeNil())
			q, err := upload.getPVCSize(&corev1.Pod{})
			Expect(err).ToNot(HaveOccurred())
			Expect(q).To(Equal(resource.MustParse("4Gi")))
			Expect(*upload.getSource(supgen, "registry.example/image").Registry.URL).To(Equal("docker://registry.example/image"))
		})
	})

	Describe("Failure helpers", func() {
		It("sets failed phase condition", func() {
			cb := conditions.NewConditionBuilder(vicondition.ReadyType)
			phase := v1alpha2.ImagePhase("")
			setPhaseConditionToFailed(cb, &phase, errors.New("plain error"))
			Expect(phase).To(Equal(v1alpha2.ImageFailed))
			Expect(cb.Condition().Message).To(Equal("Plain error"))
		})
	})
})
