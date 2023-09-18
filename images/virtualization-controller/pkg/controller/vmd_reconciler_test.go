package controller_test

// TODO write tests for VMD and VMI.
// var _ = Describe("VMD", func() {
//	var reconciler *two_phase_reconciler.ReconcilerCore[*controller.VMDReconcilerState]
//	var reconcileExecutor *testutil.ReconcileExecutor
//
//	AfterEach(func() {
//		if reconcileExecutor != nil {
//			reconcileExecutor = nil
//		}
//	})
//
//	AfterEach(func() {
//		if reconcileExecutor != nil && reconciler.Recorder != nil {
//			close(reconciler.Recorder.(*record.FakeRecorder).Events)
//		}
//	})
//
//	It("Successfully imports image by HTTP source", func() {
//		ctx := context.Background()
//
//		var dvName string
//
//		{
//			vmd := &virtv2.VirtualMachineDisk{
//				ObjectMeta: metav1.ObjectMeta{
//					Name:        "test-vmd",
//					Namespace:   "test-ns",
//					Labels:      nil,
//					Annotations: nil,
//				},
//				Spec: virtv2.VirtualMachineDiskSpec{
//					DataSource: &virtv2.DataSource{
//						Type: virtv2.DataSourceTypeHTTP,
//						HTTP: &virtv2.DataSourceHTTP{
//							URL: "http://mydomain.org/image.img",
//						},
//					},
//					PersistentVolumeClaim: virtv2.VMDPersistentVolumeClaim{
//						Size:             "10Gi",
//						StorageClassName: "local-path",
//					},
//				},
//			}
//
//			reconciler = controller.NewVMDReconciler(controller.TestReconcilerOptions{
//				KnownObjects: []client.Object{
//					&virtv2.VirtualMachine{},
//					&virtv2.VirtualMachineDisk{},
//					&virtv2.ClusterVirtualMachineImage{},
//					&cdiv1.DataVolume{},
//				},
//				RuntimeObjects: []runtime.Object{vmd},
//			})
//			reconcileExecutor = testutil.NewReconcileExecutor(types.NamespacedName{Name: "test-vmd", Namespace: "test-ns"})
//		}
//
//		{
//			err := reconcileExecutor.Execute(ctx, reconciler)
//			Expect(err).NotTo(HaveOccurred())
//
//			vmd, err := helper.FetchObject(ctx, types.NamespacedName{Name: "test-vmd", Namespace: "test-ns"}, reconciler.Client, &virtv2.VirtualMachineDisk{})
//			Expect(err).NotTo(HaveOccurred())
//			Expect(vmd).NotTo(BeNil())
//			Expect(vmd.Status.Phase).To(Equal(virtv2.DiskPending))
//			Expect(vmd.Status.Progress).To(Equal("N/A"))
//			Expect(vmd.Status.Capacity).To(Equal(""))
//			Expect(vmd.Status.Target.PersistentVolumeClaimName).To(Equal(""))
//
//			// UUID suffix
//			Expect(strings.HasPrefix(vmd.Annotations[controller.AnnVMDDataVolume], "virtual-machine-disk-")).To(BeTrue(), fmt.Sprintf("unexpected DataVolume name %q", vmd.Annotations[controller.AnnVMDDataVolume]))
//			Expect(len(vmd.Annotations[controller.AnnVMDDataVolume])).To(Equal(21 + 36))
//		}
//
//		{
//			err := reconcileExecutor.Execute(ctx, reconciler)
//			Expect(err).NotTo(HaveOccurred())
//
//			vmd, err := helper.FetchObject(ctx, types.NamespacedName{Name: "test-vmd", Namespace: "test-ns"}, reconciler.Client, &virtv2.VirtualMachineDisk{})
//			Expect(err).NotTo(HaveOccurred())
//			Expect(vmd).NotTo(BeNil())
//			Expect(vmd.Status.Phase).To(Equal(virtv2.DiskPending))
//			Expect(vmd.Status.Progress).To(Equal("N/A"))
//			Expect(vmd.Status.Capacity).To(Equal(""))
//			Expect(vmd.Status.Target.PersistentVolumeClaimName).To(Equal(""))
//
//			dvName = vmd.Annotations[controller.AnnVMDDataVolume]
//			dv, err := helper.FetchObject(ctx, types.NamespacedName{Name: dvName, Namespace: "test-ns"}, reconciler.Client, &cdiv1.DataVolume{})
//			Expect(err).NotTo(HaveOccurred())
//			Expect(dv).NotTo(BeNil())
//		}
//
//		{
//			dv, err := helper.FetchObject(ctx, types.NamespacedName{Name: dvName, Namespace: "test-ns"}, reconciler.Client, &cdiv1.DataVolume{})
//			Expect(err).NotTo(HaveOccurred())
//			Expect(dv).NotTo(BeNil())
//			dv.Status.Phase = cdiv1.Pending
//			err = reconciler.Client.Status().Update(ctx, dv)
//			Expect(err).NotTo(HaveOccurred())
//
//			err = reconcileExecutor.Execute(ctx, reconciler)
//			Expect(err).NotTo(HaveOccurred())
//
//			vmd, err := helper.FetchObject(ctx, types.NamespacedName{Name: "test-vmd", Namespace: "test-ns"}, reconciler.Client, &virtv2.VirtualMachineDisk{})
//			Expect(err).NotTo(HaveOccurred())
//			Expect(vmd).NotTo(BeNil())
//			Expect(vmd.Status.Phase).To(Equal(virtv2.DiskPending))
//			Expect(vmd.Status.Progress).To(Equal("N/A"))
//			Expect(vmd.Status.Capacity).To(Equal(""))
//			Expect(vmd.Status.Target.PersistentVolumeClaimName).To(Equal(""))
//		}
//
//		{
//			pv := &corev1.PersistentVolume{
//				ObjectMeta: metav1.ObjectMeta{
//					Namespace:   "test-ns",
//					Name:        "test-pv",
//					Labels:      nil,
//					Annotations: nil,
//				},
//				Spec: corev1.PersistentVolumeSpec{},
//				Status: corev1.PersistentVolumeStatus{
//					Phase: corev1.VolumeBound,
//				},
//			}
//			err := reconciler.Client.Create(ctx, pv)
//			Expect(err).NotTo(HaveOccurred())
//
//			pvc := &corev1.PersistentVolumeClaim{
//				ObjectMeta: metav1.ObjectMeta{
//					Namespace:   "test-ns",
//					Name:        dvName,
//					Labels:      nil,
//					Annotations: nil,
//				},
//				Spec: corev1.PersistentVolumeClaimSpec{
//					StorageClassName: util.GetPointer("local-path"),
//					VolumeName:       pv.Name,
//				},
//				Status: corev1.PersistentVolumeClaimStatus{
//					Phase: corev1.ClaimBound,
//					Capacity: corev1.ResourceList{
//						corev1.ResourceStorage: resource.MustParse("15Gi"),
//					},
//				},
//			}
//			err = reconciler.Client.Create(ctx, pvc)
//			Expect(err).NotTo(HaveOccurred())
//
//			dv, err := helper.FetchObject(ctx, types.NamespacedName{Name: dvName, Namespace: "test-ns"}, reconciler.Client, &cdiv1.DataVolume{})
//			Expect(err).NotTo(HaveOccurred())
//			Expect(dv).NotTo(BeNil())
//			dv.Status.Phase = cdiv1.CloneInProgress
//			dv.Status.Progress = "50%"
//			err = reconciler.Client.Status().Update(ctx, dv)
//			Expect(err).NotTo(HaveOccurred())
//
//			err = reconcileExecutor.Execute(ctx, reconciler)
//			Expect(err).NotTo(HaveOccurred())
//
//			vmd, err := helper.FetchObject(ctx, types.NamespacedName{Name: "test-vmd", Namespace: "test-ns"}, reconciler.Client, &virtv2.VirtualMachineDisk{})
//			Expect(err).NotTo(HaveOccurred())
//			Expect(vmd).NotTo(BeNil())
//			Expect(vmd.Status.Phase).To(Equal(virtv2.DiskProvisioning))
//			Expect(vmd.Status.Progress).To(Equal("50%"))
//			Expect(vmd.Status.Capacity).To(Equal("15Gi"))
//			Expect(vmd.Status.Target.PersistentVolumeClaimName).To(Equal(""))
//		}
//
//		{
//			dv, err := helper.FetchObject(ctx, types.NamespacedName{Name: dvName, Namespace: "test-ns"}, reconciler.Client, &cdiv1.DataVolume{})
//			Expect(err).NotTo(HaveOccurred())
//			Expect(dv).NotTo(BeNil())
//			dv.Status.Phase = cdiv1.Succeeded
//			dv.Status.Progress = "100%"
//			err = reconciler.Client.Status().Update(ctx, dv)
//			Expect(err).NotTo(HaveOccurred())
//
//			err = reconcileExecutor.Execute(ctx, reconciler)
//			Expect(err).NotTo(HaveOccurred())
//
//			vmd, err := helper.FetchObject(ctx, types.NamespacedName{Name: "test-vmd", Namespace: "test-ns"}, reconciler.Client, &virtv2.VirtualMachineDisk{})
//			Expect(err).NotTo(HaveOccurred())
//			Expect(vmd).NotTo(BeNil())
//			Expect(vmd.Status.Phase).To(Equal(virtv2.DiskReady))
//			Expect(vmd.Status.Progress).To(Equal("100%"))
//			Expect(vmd.Status.Capacity).To(Equal("15Gi"))
//			Expect(vmd.Status.Target.PersistentVolumeClaimName).To(Equal(dvName))
//		}
//	})
// })
