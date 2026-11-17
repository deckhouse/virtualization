# Changelog v1.13

## v1.13.0

### ci

- **chore** (low): Consolidate the license and validation helper tools into a single modtools CLI and drop the CI check already covered by DMT. ([!42](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/42))
- **fix** (low): Fuzz image build restores the corpus with AWS CLI, no longer depending on the removed MinIO Client download. ([!451](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/451))
- **chore** (low): The USB e2e specs run on the NFS and Ceph nightly clusters. ([!472](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/472))
- **fix** (low): Stabilize nightly e2e storage setup by enabling storage module dependencies in order and waiting for StorageProfile readiness. ([!507](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/507))
- **chore** (low): Migrate fuzz builds and corpus replay to shared CI templates and images, and save verbose replay logs as artifacts. ([!508](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/508))
- **fix** (low): Nightly E2E cleanup now removes old nested clusters instead of leaving them to drain the resource pool. ([!524](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/524))
- **fix** (low): Nightly E2E resource cleanup no longer stalls on VMs that cannot be stopped. ([!524](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/524))
- **fix** (low): Nightly E2E report is now posted to the channel even when a run produces no artifacts. ([!524](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/524))
- **chore** (low): remove nikita.korolev from CODEOWNERS ([!532](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/532))
- **fix** (low): Approve manual-approval-gated module releases (state-snapshotter, storage-foundation) before e2e storage setup so nightly and release pipelines roll out the latest versions. ([!540](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/540))
- **fix** (low): Wait for the nfs StorageProfile in the configure-virtualization stage instead of configure-storage, where the virtualization module is not yet installed. ([!540](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/540))
- **chore** (low): build_dev_manual can build tags predating the GitLab CI migration. ([!544](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/544))
- **chore** (low): Migrate Svace static analysis to the shared Svace.gitlab-ci.yml template (adds svacer autoimport and LLM autoreview). ([!559](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/559))
- **chore** (low): Svace projects are named after the bundle image the binary ships in, so the SSDLC static-analysis report covers all components. ([!578](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/578))
- **fix** (low): Nightly e2e on nested clusters no longer fails at kubeconfig generation on DKP 1.77. ([!579](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/579))
- **fix** (low): Pin the TRDL binary during fuzz build setup to avoid failures when concurrent jobs replace the shared current symlink. ([!582](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/582))
- **fix** (low): Nightly e2e no longer fails the SDN setup step on a brief nested API outage. ([!585](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/585))
- **chore** (low): The e2e jobs of a merge request run in a pinned toolchain image on the docker-executor runner. ([!593](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/593))
- **chore** (low): The nightly e2e cleanup deletes all previous nested clusters on every run instead of keeping the ones younger than a day. ([!600](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/600))
- **fix** (low): The lint:go CI job pulls the golangci-lint image again after the pinned digest disappeared from the registry. ([!601](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/601))
- **chore** (low): Module components can be profiled with Pyroscope and Alloy deployed into a dev cluster via tools/pprof. ([!605](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/605))
- **fix** (low): Nightly and release e2e workflows install the Go version required by the repository again. ([!629](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/629))
- **feature** (low): The nightly e2e Loop report now shows which storage class each OS stand ran on. ([!636](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/636))
- **feature** (low): The nightly e2e storage rotation now spreads the OS stands so they do not run the same class on the same day (never two on replicated). ([!636](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/636))
- **fix** (low): The nightly e2e Loop report links now point at the GitLab pipeline instead of a 404. ([!636](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/636))
- **chore** (low): A tag can be built into the dev registry through the API, not only from the Run pipeline form. ([!658](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/658))
- **chore** (low): Module releases are built into the stage registry and reach production only through Promote. ([!661](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/661))
- **fix** (low): A hung e2e suite is stopped before the job timeout, so the cluster is cleaned up and the logs are kept. ([!675](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/675))
- **chore** (low): The nightly e2e suite runs on 24 processes and fits into the 1h timeout. ([!684](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/684))
- **chore** (low): The e2e job log shows why suite processes die or hang on the suite timeout. ([!721](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/721))
- **fix** (low): The e2e check of the peering access policy sends an object that reaches the policy. ([!770](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/770))
- **chore** (low): The Go code is now checked by the wsl_v5 linter for consistent blank lines and grouped variable declarations. ([!791](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/791))
- **chore** (low): The requirements check of the release pipeline reads module.yaml from the release tag in git instead of GitHub. ([!806](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/806))
- **fix** (low): The changelog job no longer fails when two milestones of one minor version are open. ([!811](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/811))

### core

- **chore** (low): Fix fuzzer finding: conntrack sync message may lead to memory over-allocation. ([!421](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/421))
- **fix** (low): The KubeVirt install strategy job now runs with the same restricted security context as the rest of the module. ([!423](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/423))
- **fix** (default): Live migration of a virtual machine with additional networks no longer fails when the network interface on the target node is not ready yet. ([!441](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/441))
- **chore** (low): The UEFI boot screen of a virtual machine shows the current Deckhouse Platform logo. ([!443](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/443))
- **fix** (low): Virtualization components are no longer blocked by the restricted pod security policy. ([!461](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/461))
- **fix** (low): Live migration of a VirtualMachine with a USB device no longer fails on the target node. ([!474](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/474))
- **chore** (default): Fixed vulnerabilities:
- CVE-2026-81870
- CVE-2026-81871 ([!492](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/492))
- **chore** (default): Fixed vulnerabilities:
- CVE-2026-56742 ([!534](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/534))
- **fix** (default): A virtual machine no longer stays paused if virtualization components restart on the source node during its live migration. ([!538](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/538))
- **fix** (default): DVCR no longer floods its log with trace export errors. ([!549](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/549))
- **feature** (default): Mass live migrations no longer stall on a few nodes. The parallel migration limit now counts only the migrations that transfer memory. Previously, migrations waiting for their turn took it up too, so only a small part of the allowed migrations transferred data at the same time. ([!564](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/564))
- **feature** (default): A live migration now prefers a target node that is not receiving another migration, so migrations no longer pile up on one node. ([!564](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/564))
- **fix** (default): Live migration of a virtual machine running without a restart since a version earlier than 1.9 no longer fails. ([!569](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/569))
- **fix** (default): A virtual machine is no longer restarted if virtualization components restart on the target node shortly after its live migration. ([!576](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/576))
- **feature** (default): Added the ability to move a running virtual machine without stopping it to another site. The virtual machine keeps its IP and MAC addresses. The functionality is available in the commercial editions of the platform. To use it, pair the clusters through the `peering` module and run the `d8 v migrate --project-peer` command. ([!587](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/587))
- **fix** (default): Health checks of the device passthrough components are no longer reachable from other nodes. Port `4108` is no longer open on nodes, and port `4107` accepts only local connections. ([!602](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/602))
- **chore** (low): DRA USB and PCI plugins expose a pprof endpoint when the module log level is debug. ([!604](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/604))
- **fix** (default): Live migration of a virtual machine no longer fails if the previous migration of this virtual machine failed. ([!609](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/609))
- **chore** (default): Fixed vulnerabilities:
- CVE-2026-56851
- CVE-2026-56857
- CVE-2026-56866
- CVE-2026-61801
- CVE-2026-78659
- CVE-2026-78660
- CVE-2026-78663
- CVE-2026-78667
- CVE-2026-78669
- CVE-2026-94439
- CVE-2026-94440
- CVE-2026-94448
- CVE-2026-97030
- CVE-2026-97031
- CVE-2026-97032 ([!614](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/614))
- **fix** (default): A disk that cannot be expanded no longer blocks detaching other disks of the virtual machine. ([!641](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/641))
- **chore** (default): Fixed vulnerabilities:
- CVE-2026-85747 ([!642](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/642))
- **fix** (default): Virtual machines, disks and images now work in a cluster without the VolumeSnapshot CRDs. Previously, virtualization did not work in such a cluster at all. ([!653](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/653))
- **chore** (low): KubeVirt is updated to v1.6.2-v12n.126, whose changes belong to live migration between clusters. ([!665](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/665))
- **chore** (default): Fixed vulnerabilities:
- CVE-2026-63209
- CVE-2026-111943 ([!670](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/670))
- **chore** (low): Follow the rename of the federation module to peering. ([!682](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/682))
- **chore** (low): With the `Peering` feature gate, the synchronization controller of migrations between clusters listens on ports `4102` and `4103` instead of `9185` and `8443`. ([!722](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/722))
- **fix** (low): Running virtual machines no longer risk losing connectivity when the module hands VM routing over to sdn on an upgrade. ([!723](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/723))
- **fix** (low): virt-handler no longer crashes when a virtual machine that has never been migrated is stopped or deleted. ([!735](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/735))
- **chore** (default): Fixed vulnerabilities:
- CVE-2026-78661
- CVE-2026-97045
- CVE-2026-116585
- CVE-2026-116586
- CVE-2026-116587 ([!736](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/736))
- **chore** (default): Fixed vulnerabilities:
- CVE-2026-78661
- CVE-2026-97045
- CVE-2026-116585
- CVE-2026-116586
- CVE-2026-116587 ([!740](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/740))
- **fix** (low): The synchronization controller no longer crashes on start with an unknown flag error. ([!744](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/744))
- **fix** (low): Running virtual machines no longer risk losing connectivity when the module hands VM routing over to sdn on an upgrade. ([!749](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/749))
- **fix** (low): The USB passthrough component no longer fails to start on nodes where USB/IP is built into the kernel. ([!760](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/760))
- **fix** (low): Improve failure reporting and recovery when migrating a virtual machine to a peer cluster. ([!780](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/780))
- **fix** (default): The virtualization controller no longer fails to start when the `sdn` module is enabled in the editions where this module supports only the main network. ([!794](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/794))

### disks

- **fix** (default): The external upload URL of an image or a disk now appears only when it is ready to accept data. Previously, an upload started right after the URL appeared could fail with the `503` error. ([!520](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/520))
- **chore** (low): Fix compilation of VirtualDisk migration unit tests. ([!542](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/542))

### docs

- **docs** (default): Removed the VIOMMU feature from the release notes v1.12.0. ([!491](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/491))
- **docs** (low): Update docs v1.13.0. ([!603](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/603))
- **chore** (low): The VirtualMachinePool documentation now states that a static IP address can be set for the Main network interface. ([!652](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/652))

### module

- **chore** (low): Set category virtualization-cluster for cluster-wide CRDs. ([!426](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/426))
- **fix** (low): PCIDevice and NodePCIDevice resources are now accessible through the legacy user-authz roles and protected by the device access admission policy, the same way as USB devices. ([!428](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/428))
- **fix** (low): Pod security policy checks are enabled again in the `d8-virtualization` namespace. ([!430](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/430))
- **fix** (low): Virtualization audit is no longer blocked by the restricted pod security policy. ([!464](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/464))
- **fix** (default): Invalid TLS certificates of virtualization components are now reissued automatically and no longer stop the delivery of audit events. ([!477](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/477))
- **fix** (low): Updating the module in the Community Edition no longer stalls the Deckhouse queue. ([!496](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/496))
- **fix** (default): Live migration over a dedicated migration network is no longer limited to 640 MiB/s by default. Over the node network, the limit still applies. ([!509](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/509))
- **feature** (default): The `virtualMachineCIDRs` list in the module settings can now be cleared entirely once no VirtualMachineIPAddress resources are left in the cluster. Previously, the list had to keep at least one subnet. ([!518](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/518))
- **chore** (low): Module docs, crds and openapi are now shipped in the release-channel-version image so the platform can read them for not-yet-enabled modules. ([!536](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/536))
- **fix** (low): Users with access granted through ClusterAuthorizationRule can now view VirtualMachinePool resources. ([!537](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/537))
- **feature** (high): Access rights now follow the role model of Deckhouse Platform 1.78, so project users keep access to virtual machines after the platform is upgraded to 1.78. ([!547](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/547))
- **feature** (default): Added the ability to restrict which VirtualMachineClass and ClusterVirtualImage resources each project can use with ClusterResourceGrantPolicy. Until an administrator creates a policy, all classes and images stay available to every project. To use it, the cluster must run Deckhouse Platform 1.78 or newer with the `multitenancy-manager` module enabled. ([!784](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/784))

### observability

- **fix** (low): The hypervisor overhead on the CPU charts is one line per virtual machine again, without the dead negative line left by a migration. ([!331](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/331))
- **fix** (low): Memory, file system, disk and network figures of a virtual machine are no longer counted twice, or halved, while the machine is visible from two nodes. ([!331](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/331))
- **fix** (low): The packet rate, packet drops and interface errors of a virtual machine are shown in packets per second instead of eight times that. ([!331](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/331))
- **fix** (low): The Agent and Config tiles of a virtual machine show its actual state instead of always showing the good one. ([!331](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/331))
- **fix** (low): The dashboards of virtual machines count objects once in a cluster where the controller runs in several replicas. ([!331](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/331))
- **feature** (low): The dashboards of virtual machines show how fast a machine changes the contents of its memory, which tells in advance whether its live migration will be slow. ([!331](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/331))
- **feature** (low): The cluster overview shows the CPU stolen from virtual machines, by node and by project, both as a share of the vCPU given out and as the steal an average machine sees. ([!331](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/331))
- **fix** (low): The share of the CPU stolen from virtual machines is not diluted by the vCPU of the machines that are switched off. ([!331](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/331))
- **fix** (low): The snapshot column of the disk tables is filled again in a cluster where the controller runs in several replicas. ([!331](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/331))
- **chore** (low): Metric collectors are covered by unit tests that pin the names, labels and phases of every exported series. ([!589](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/589))
- **feature** (default): The Virtualization Overview dashboard now shows how many disks are created from each image and the CPU and memory overhead of virtual machines per namespace. ([!634](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/634))
- **feature** (default): The `d8_virtualization_virtualdisk_info` metric now shows the data source of each disk. ([!634](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/634))
- **fix** (default): The Virtualization Overview dashboard no longer leaves out the disks, images and snapshots of namespaces without virtual machines. ([!634](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/634))

### test

- **chore** (low): Re-enable skipped e2e tests related to snapshotting and restoring: VirtualMachineOperationRestore and VMSOP. ([!224](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/224))
- **fix** (low): The e2e precreated CVI precheck fails at once, naming the URL, when an image is unreachable, and recreates a CVI left in the Failed phase instead of waiting out the 20-minute timeout. ([!404](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/404))
- **chore** (low): The USB e2e suite is back and covers hotplug, restart and namespace reassignment. ([!472](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/472))
- **fix** (low): e2e stand pool no longer drains because the janitor readiness gate waits on a DRA driver that never runs on stands. ([!497](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/497))
- **chore** (low): The DataExports e2e spec is no longer skipped and runs in the nightly suite. ([!565](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/565))
- **chore** (low): Add an e2e spec that migrates a VM with 15 hotplugged disks twice in a row. ([!595](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/595))
- **fix** (low): The nested e2e clusters bootstrap again with two additional networks per node. ([!619](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/619))
- **chore** (low): The e2e suite runs without the Go race detector, so the nightly run fits on the CI runner. ([!762](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/762))
- **feature** (low): Run the module upgrade suite with `task run:upgrade`, one parallel spec per virtual machine, with the StorageClasses taken from STORAGE_CLASS_NAME and LOCAL_STORAGE_CLASS_NAME. ([!764](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/764))
- **fix** (low): The e2e readiness checks no longer fail on a deckhouse pod left after a node shutdown. ([!765](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/765))
- **fix** (low): A short network failure between CI and the cluster no longer fails e2e specs or removes the results of the whole run. ([!772](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/772))

### vd

- **fix** (default): Restoring a disk from a snapshot into a size smaller than the source disk is now rejected with a message about the minimum size. Previously, such a disk got the source disk size, and with the `WaitForFirstConsumer` binding mode it was not created at all. ([!427](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/427))
- **fix** (default): If virtualization components restart during a disk volume migration, the migration no longer deletes both volumes together with the disk data. ([!453](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/453))
- **fix** (default): A disk volume migration no longer hangs in `Migrating` when the namespace storage quota is almost used up. Disks already stuck finish the migration after the upgrade. ([!471](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/471))
- **fix** (default): A disk restored from a snapshot no longer gets a size too small to attach. Previously, on some storage such as NFS, such a disk could get a size of a few bytes, and a virtual machine with it did not start. ([!484](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/484))
- **fix** (default): A disk is now attached to another virtual machine only after it is detached from the previous one. Previously, two virtual machines could write to the same disk and corrupt its file system. ([!554](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/554))
- **fix** (low): Reject scientific-notation VirtualDisk sizes so a crafted size can no longer hang the admission webhook. ([!572](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/572))
- **fix** (default): A disk or an image can now be created from a snapshot without a storage class even if the default storage class uses another CSI driver. Such a resource is created in the storage class of the snapshot. ([!639](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/639))
- **fix** (default): If a virtual machine restarts while its disks are migrating, the disks no longer end up on different nodes, and the virtual machine no longer hangs in `Pending`. ([!655](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/655))

### vdsnapshot

- **fix** (default): A disk snapshot deleted during its creation no longer hangs in `Terminating` and no longer leaves the guest file system frozen. ([!597](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/597))
- **fix** (default): Snapshots taken before the `state-snapshotter` module is enabled are no longer taken again after it is enabled, and failed ones can now be deleted. ([!624](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/624))
- **fix** (low): A consistent snapshot of a disk of a running virtual machine is now taken with the guest filesystem frozen instead of failing when state-snapshotter module is enabled. ([!709](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/709))
- **fix** (low): A snapshot of a disk attached to a running virtual machine fails with DeadlineExceeded when it is not ready within 13 minutes. ([!709](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/709))
- **fix** (low): Without the state-snapshotter module, a disk snapshot is marked consistent only when the guest stayed frozen for the whole capture. ([!710](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/710))
- **fix** (low): Without the state-snapshotter module, a snapshot of a disk attached to a running virtual machine fails with DeadlineExceeded when it is not ready within 13 minutes. ([!710](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/710))
- **fix** (low): Without the state-snapshotter module, snapshots in progress during the virtualization module update fail with AbortedByModuleUpdate and unfreeze the virtual machine. Take them again. ([!710](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/710))

### vm

- **feature** (default): Added the ability to connect a virtual machine to a physical network through SR-IOV with the `UnderlayNetwork` network type. The functionality is available in the commercial editions of the platform and is experimental. To use it, the cluster must run the `sdn` module and Kubernetes `1.34` or newer. A virtual machine with such an interface cannot be live migrated. ([!374](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/374))
- **fix** (default): A virtual machine with a USB device attached from another node no longer freezes when that node loses network connectivity. After about 15 seconds, the guest OS sees the device as disconnected, and the virtual machine keeps running. ([!419](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/419))
- **fix** (default): Live migration of a virtual machine with the `EFIWithSecureBoot` bootloader no longer stays `InProgress` for hours after the virtual machine has already moved to the target node. ([!425](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/425))
- **fix** (default): Attaching a disk to a running virtual machine that was live migrated with its local disks no longer restarts it. ([!429](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/429))
- **fix** (default): A stopped virtual machine no longer switches to `Pending` for a few seconds and rejects a start when its disk is replaced right after the stop. ([!454](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/454))
- **fix** (default): A virtual machine without the `Main` network no longer gets an address from the pod network while none of its additional networks is ready. ([!488](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/488))
- **chore** (low): The controller no longer logs an error while a USB device referenced by a VM has no ResourceClaimTemplate. ([!489](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/489))
- **fix** (default): Additional network interfaces of a virtual machine no longer move to other networks after a restart or a node failure. Previously, a guest with static network settings lost connectivity. ([!521](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/521))
- **fix** (low): The OperationInProgress condition of a virtual machine is shown only while an operation is being performed; a failed operation no longer leaves a stale error on the machine and is reported by its VirtualMachineOperation. ([!525](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/525))
- **fix** (default): A migration now fails at once with the reason if no other node can accept the virtual machine, for example because of `nodeSelector`. Previously, the migration waited for the scheduler to time out. ([!526](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/526))
- **feature** (default): Added the ability to get the `Main` network address of a virtual machine from the IPAddress resource of the `sdn` module. The switch starts automatically when the `sdn` module serves the main cluster network. ([!527](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/527))
- **feature** (high): The VirtualMachineIPAddress and VirtualMachineIPAddressLease resources are deprecated but keep working. ([!528](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/528))
- **feature** (low): The vm-route-forge component is no longer shipped where the sdn module programs the routes to virtual machines. ([!529](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/529))
- **fix** (default): Disks used by node processes, such as Ceph OSD disks, are no longer offered as PCI devices for virtual machines. Attaching them could break the node. ([!530](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/530))
- **fix** (default): System devices of a node, such as the security processor, integrated USB controllers and audio devices, are no longer offered as PCI devices for virtual machines. ([!531](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/531))
- **feature** (default): If the guest OS is paused, for example because of a disk I/O error or a failed live migration, the `Running` condition now reports it and names the cause. Previously, such a virtual machine looked running, because there is no separate phase for a pause and the phase stays `Running`. ([!538](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/538))
- **fix** (default): Attaching a disk to a running virtual machine no longer resets its uptime and no longer sets the `Running` condition to `False` for a few seconds. ([!543](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/543))
- **fix** (low): wait for a deferred in-place resize and re-enable HotplugCPU e2e ([!546](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/546))
- **fix** (default): After the upgrade, virtual machines started before it no longer require a restart for changes that do not need one. ([!548](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/548))
- **feature** (default): The VirtualMachinePool status now explains why the pool does not reach the requested number of virtual machines. ([!550](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/550))
- **fix** (default): A VirtualMachinePool no longer reports itself unavailable while its virtual machines migrate. ([!550](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/550))
- **fix** (default): A VirtualMachinePool no longer gives the same reusable disk to two virtual machines. If it has already happened, the disk stays with the virtual machine that uses it. ([!553](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/553))
- **fix** (default): A disk removed from a running virtual machine is now detached even if another of its disks is not ready. ([!554](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/554))
- **fix** (default): Disks attached to a running virtual machine no longer become inaccessible after systemd on the node reloads its configuration, for example after `systemctl daemon-reload`. Previously, live migration of the virtual machine or attaching another disk then failed. ([!557](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/557))
- **fix** (low): Enabling the HotplugMemoryWithLiveMigration feature gate no longer restarts running virtual machines. ([!566](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/566))
- **fix** (default): A disk can now be attached right after its previous VirtualMachineBlockDeviceAttachment is deleted. The new attachment waits until the disk is released instead of being rejected. ([!568](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/568))
- **fix** (default): A new VNC, serial console or SPICE connection that replaces an open one no longer drops all console sessions on the node. ([!577](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/577))
- **fix** (high): The `Main` network can now be added back to a virtual machine whose additional network already uses id `1`. ([!586](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/586))
- **fix** (default): PCI devices of a node no longer disappear after the container runtime and kubelet restart on the node. Previously, virtual machines that used them could not start without a manual intervention. ([!588](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/588))
- **fix** (default): After many disks are attached to a running virtual machine at once, it no longer falsely requires a restart and can still be live migrated. ([!595](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/595))
- **fix** (default): A CPU or memory change of a running virtual machine is no longer silently left unapplied if its live migration fails or is interrupted by another operation. The migration is now retried, and the `ConfigurationApplied` condition explains what blocks the change. ([!598](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/598))
- **fix** (default): Live migration to a node with an older Linux kernel now fails with a clear reason instead of an internal QEMU error. ([!610](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/610))
- **fix** (default): A stopped virtual machine now always starts when its run policy is switched from `AlwaysOff` to `AlwaysOnUnlessStoppedManually`. Previously, it started only in some cases. ([!612](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/612))
- **fix** (default): Replacing an additional network of a running virtual machine in one change no longer hangs or requires a restart. ([!613](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/613))
- **fix** (low): A virtual machine no longer loses the MAC address of an additional network and hangs in Pending. ([!620](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/620))
- **fix** (default): Virtual machines with additional networks no longer lose their MAC address while they use it. Previously, the platform could release such an address, and after a restart the virtual machine hung in `Pending` waiting for a new one. ([!623](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/623))
- **fix** (low): Fixed an Auto VirtualMachineIPAddress looping on an address the sdn module does not know. ([!625](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/625))
- **fix** (default): A virtual machine with `coreFraction: Auto` is now live migrated instead of restarted when its node cannot apply a new CPU fraction in place. ([!640](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/640))
- **docs** (low): The user guide explains what happens when a CPU or memory change without a restart cannot be applied. ([!640](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/640))
- **fix** (default): A virtual machine with a disk on NFS no longer hangs in deletion when the NFS export is removed first. ([!668](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/668))
- **fix** (low): With sdn serving the Main network, new virtual machines no longer hang in `Pending` without an IP address when sdn refuses an address of another VirtualMachineIPAddress. ([!711](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/711))
- **fix** (low): With sdn serving the Main network, a VirtualMachineIPAddress deleted and recreated under the same name no longer waits about 10 minutes for its address. ([!711](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/711))
- **fix** (low): The module upgrade no longer restarts running virtual machines of a VirtualMachineClass with the `Discovery` CPU type. ([!714](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/714))
- **fix** (low): The module upgrade no longer restarts running virtual machines of a VirtualMachineClass with the `Discovery` CPU type. ([!726](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/726))
- **fix** (low): With sdn serving the Main network, new virtual machines no longer hang in `Pending` without an IP address when sdn refuses an address of another VirtualMachineIPAddress. ([!748](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/748))
- **fix** (low): With sdn serving the Main network, a VirtualMachineIPAddress deleted and recreated under the same name no longer waits about 10 minutes for its address. ([!748](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/748))
- **fix** (low): A move of a virtual machine to a peer cluster no longer leaves labels or annotations of the module on the virtual machine and its disks, never leaves the virtual machine running in both clusters, and ends with the cause reported by the receiving cluster. ([!783](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/783))
- **fix** (low): A move of a VM to a peer cluster is shown and can be cancelled in the operations of the VM. ([!785](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/785))
- **fix** (low): Moving a virtual machine to another cluster now waits for an active snapshot session instead of rejecting the move. ([!795](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/795))

### vmbda

- **fix** (default): If an attached disk is deleted and then its VirtualMachineBlockDeviceAttachment, both resources no longer hang in `Terminating`. ([!635](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/635))

### vmclass

- **fix** (low): The virtualization controller no longer stops working when a VirtualMachineClass was created through v1alpha3 before the module finished installing, for example by the upmeter module. ([!414](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/414))
- **feature** (default): Nested virtualization (`vmx`, `svm`) can now be turned off in a VirtualMachineClass. It stays enabled by default. ([!447](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/447))
- **fix** (low): Discovery VirtualMachineClass no longer strands VMs in Pending when a qemu upgrade drops arch-capabilities-derived CPU features on AMD nodes. ([!479](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/479))

### vmip

- **fix** (low): A VirtualMachineIPAddress is no longer reported as attached to a VirtualMachine that does not use its address. ([!628](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/628))
- **fix** (low): A Static VirtualMachineIPAddress whose address sdn still reserves now reports that instead of asking to delete a non-existent IPAddress. ([!711](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/711))
- **fix** (low): A Static VirtualMachineIPAddress whose address sdn still reserves now reports that instead of asking to delete a non-existent IPAddress. ([!748](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/748))

### vmop

- **fix** (default): If the target node is not ready for a migration within 5 minutes, the operation now fails only after the migration is actually canceled. Because of that, the next migration of the virtual machine is no longer blocked. ([!505](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/505))
- **fix** (default): Eviction and migration of a virtual machine now wait up to 15 minutes for a disk that is still being created or attached. Previously, they failed. ([!649](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/649))
- **feature** (default): A migration of a virtual machine, including one started by a node drain or a move to another site, now waits for a snapshot of the virtual machine in progress to finish. ([!763](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/763))

### vmsnapshot

- **chore** (low): More stable work of VMBDA, VMSnapshots, VDSnapshots controllers. ([!482](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/482))
- **fix** (low): A snapshot no longer leaves the guest filesystem frozen when the reported freeze state flaps. ([!562](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/562))
- **fix** (low): A snapshot waiting for the guest filesystem freeze no longer flood API server. ([!562](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/562))
- **fix** (default): A failed VirtualMachineSnapshot now reports why it failed, for example a storage error. Previously, it only reported waiting for the file system. ([!591](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/591))
- **fix** (default): A virtual machine snapshot no longer leaves the guest file system frozen when the unfreeze request fails. ([!608](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/608))
- **fix** (default): With the `state-snapshotter` module enabled, a failed snapshot now reports the storage error. ([!626](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/626))
- **chore** (low): Add internal VirtualMachineAccessClaim API for freeze coordination. ([!651](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/651))
- **chore** (low): Add an internal coordinator of guest filesystem freezes for snapshots; snapshots start using it in a later change. ([!662](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/662))
- **fix** (low): Snapshots of one virtual machine no longer leave the guest frozen or unfreeze it under each other, and a snapshot is marked consistent only when the guest stayed frozen for the whole capture. A second snapshot of a virtual machine being snapshotted fails at once. ([!709](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/709))
- **fix** (low): A snapshot without requiredConsistency fails with VirtualMachineStateChanged when the virtual machine is stopped, started, restarted or migrated during the capture. ([!709](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/709))
- **fix** (low): Snapshots in progress during the virtualization module update fail with AbortedByModuleUpdate and unfreeze the virtual machine. ([!709](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/709))
- **fix** (low): Without the state-snapshotter module, snapshots of one virtual machine no longer unfreeze the guest under each other or leave it frozen, and a second snapshot of a virtual machine being snapshotted fails at once. ([!710](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/710))
- **fix** (low): Without the state-snapshotter module, a snapshot of a running virtual machine fails with DeadlineExceeded when it is not ready within 13 minutes. ([!710](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/710))
- **fix** (low): Without the state-snapshotter module, a snapshot without requiredConsistency no longer freezes the guest filesystem and is marked consistent: false. Set requiredConsistency: true for a consistent snapshot. ([!710](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/710))
- **fix** (low): Without the state-snapshotter module, a snapshot without requiredConsistency fails with VirtualMachineStateChanged when the virtual machine is stopped, started, restarted or migrated during the capture. Take the snapshot again. ([!710](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/710))
- **docs** (low): The reference describes what `requiredConsistency: false` does for VirtualMachineSnapshot and VirtualDiskSnapshot. ([!769](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/769))
