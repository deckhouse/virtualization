# Changelog

## [0.12.1](https://github.com/deckhouse/virtualization/compare/v0.12.0...v0.12.1) (2024-08-13)


### Bug Fixes

* **vmip:** changes to the resource name generation algorithm ([#276](https://github.com/deckhouse/virtualization/issues/276)) ([54c8b49](https://github.com/deckhouse/virtualization/commit/54c8b49a0df41d1ecc10cec645f5f561710b405b))
* **vm:** wait for virtual disk's target pvc to be created before start internal virtual machine ([9be8ab7](https://github.com/deckhouse/virtualization/commit/9be8ab74c8de88f57c553df821dd2e73e6cbdb06))

## [0.12.0](https://github.com/deckhouse/virtualization/compare/v0.11.0...v0.12.0) (2024-08-12)


### Features

* **api:** add importer's requests and limits for virtualization config ([#266](https://github.com/deckhouse/virtualization/issues/266)) ([363283d](https://github.com/deckhouse/virtualization/commit/363283de85856161d3f88970c2b6c867ee2db3dc))
* **api:** remove provisioner pod req and lim settings ([7f4e38a](https://github.com/deckhouse/virtualization/commit/7f4e38a2f91bd010907c4be0662900ae58fcb2a7))
* **api:** remove req/lim settings from virtualization mc ([7f4e38a](https://github.com/deckhouse/virtualization/commit/7f4e38a2f91bd010907c4be0662900ae58fcb2a7))
* **api:** set common logger slog ([7f62061](https://github.com/deckhouse/virtualization/commit/7f62061f65d0bce9e02e9bd4589db97fb88bd9e4))
* **vd:** add binding mode ([da65e56](https://github.com/deckhouse/virtualization/commit/da65e56a660bddcbc29f826f551bf1f45e5b1899))
* **vd:** set common logger slog for controller ([f37c5df](https://github.com/deckhouse/virtualization/commit/f37c5df0406364a1ffb9f988d94544f3ee757a1a))
* **vm-route-forge:** add route interface ([#268](https://github.com/deckhouse/virtualization/issues/268)) ([1343160](https://github.com/deckhouse/virtualization/commit/134316075f45b9624cd2e1c49d323fff89683473))


### Bug Fixes

* **module:** add 'need restart' and 'agent' status to brief output ([#262](https://github.com/deckhouse/virtualization/issues/262)) ([d4646a6](https://github.com/deckhouse/virtualization/commit/d4646a64d62b21f2d4b138b4d3627de7bb25053f))
* **module:** fix RBAC for Admin ([#259](https://github.com/deckhouse/virtualization/issues/259)) ([896073b](https://github.com/deckhouse/virtualization/commit/896073beca0563820e60f77966badc6480f80031))
* **module:** remove deprecated vmipCIDRs from module config ([#263](https://github.com/deckhouse/virtualization/issues/263)) ([dbb1181](https://github.com/deckhouse/virtualization/commit/dbb11815d8fb1b85f2493ae42b84c5048a0c2386))
* **vd:** revert degraded phase ([4db841b](https://github.com/deckhouse/virtualization/commit/4db841b0e2f6c8265135a07bb358dd3aa001ce7f))
* **vm:** add unittests for statistic handler ([#271](https://github.com/deckhouse/virtualization/issues/271)) ([767bb44](https://github.com/deckhouse/virtualization/commit/767bb4491029164516b15e75a85a78c8b02f3cc6))
* **vmip:** create double lease ([#261](https://github.com/deckhouse/virtualization/issues/261)) ([8bdf8c3](https://github.com/deckhouse/virtualization/commit/8bdf8c3ee5c4ad21df625cf0adc6d53c6caf250c))
* **vm:** remove pod finalizers after pod completion ([#265](https://github.com/deckhouse/virtualization/issues/265)) ([6de10fd](https://github.com/deckhouse/virtualization/commit/6de10fdd4f4d5d4ee9054c1690e0fe73b25892ff))

## [0.11.0](https://github.com/deckhouse/virtualization/compare/v0.10.1...v0.11.0) (2024-08-01)


### Features

* **api:** add object ref uid ([ab9c57d](https://github.com/deckhouse/virtualization/commit/ab9c57dd23e2aca77492fbe2b807b2fc9b54a569))
* **controller, vmop:** wait for the desired state of the vm ([#84](https://github.com/deckhouse/virtualization/issues/84)) ([94fac98](https://github.com/deckhouse/virtualization/commit/94fac9882b6adb23cf739291a382518844acd512))
* **controller:** add pprof ([#193](https://github.com/deckhouse/virtualization/issues/193)) ([5cf70c5](https://github.com/deckhouse/virtualization/commit/5cf70c54fc2856b7790ed3264579e048bcaaae41))
* **controller:** add recovery ([#249](https://github.com/deckhouse/virtualization/issues/249)) ([4d6bff1](https://github.com/deckhouse/virtualization/commit/4d6bff1bc6aca5d97ae8cc6c8b2a54d413725545))
* **core, dvcr:** generate htpasswd from hook ([#137](https://github.com/deckhouse/virtualization/issues/137)) ([bf009a0](https://github.com/deckhouse/virtualization/commit/bf009a0a4cac5884657db5562a4c4cc8a5b1cf8c))
* **cvi:** apply new controller design ([9e21de8](https://github.com/deckhouse/virtualization/commit/9e21de84c355b46c94933fd7fc2252358cc2052d))
* **dev:** added emulation of virtual machine movements ([677708b](https://github.com/deckhouse/virtualization/commit/677708b3359a1e8120659f606b50f1fa220d6f3b))
* **observability:** add logLevel option to module config ([#194](https://github.com/deckhouse/virtualization/issues/194)) ([d2e8cfc](https://github.com/deckhouse/virtualization/commit/d2e8cfcfec8a22aa3e8c8de27945cd967441d6d1))
* **proxy:** add rewriter for APIGroupDiscoveryList ([#99](https://github.com/deckhouse/virtualization/issues/99)) ([36712f3](https://github.com/deckhouse/virtualization/commit/36712f3e1fab9c3164160d7d3577e6c58b884409))
* **vd:** add dvcr duration to status ([d7c09b8](https://github.com/deckhouse/virtualization/commit/d7c09b8f61fa7945f9b0f7fc254b8f68ca4fcf03))
* **vd:** add dvcr import duration to status ([d7c09b8](https://github.com/deckhouse/virtualization/commit/d7c09b8f61fa7945f9b0f7fc254b8f68ca4fcf03))
* **vd:** add vd stats ([f2eb4ba](https://github.com/deckhouse/virtualization/commit/f2eb4bac2f723105bcd24c7c7b2fc587a1b15ecd))
* **vd:** apply new controller design ([e496da0](https://github.com/deckhouse/virtualization/commit/e496da057ba32e8d04772d1873ad1dba0232e925))
* **vi:** apply new controller design ([078b61d](https://github.com/deckhouse/virtualization/commit/078b61d97d640e6b21ee7e9d8ee27952dff3a4c7))
* **vi:** apply new design ([#142](https://github.com/deckhouse/virtualization/issues/142)) ([078b61d](https://github.com/deckhouse/virtualization/commit/078b61d97d640e6b21ee7e9d8ee27952dff3a4c7))
* **vm-route-forge:** add pprof server ([#244](https://github.com/deckhouse/virtualization/issues/244)) ([c61eb2e](https://github.com/deckhouse/virtualization/commit/c61eb2ebf682b7b8a381f9de55e18226606e7052))
* **vm-route-forge:** impl route reconciliation ([#242](https://github.com/deckhouse/virtualization/issues/242)) ([7f2f963](https://github.com/deckhouse/virtualization/commit/7f2f96375daaf2c153aa27fad7cfe1f0239cb4d3))
* **vm:** add pod handler ([#220](https://github.com/deckhouse/virtualization/issues/220)) ([f73174f](https://github.com/deckhouse/virtualization/commit/f73174fc0e744c2b4da7ecdbec13ec215c3258f6))
* **vm:** add statisticHandler ([#206](https://github.com/deckhouse/virtualization/issues/206)) ([b0b4540](https://github.com/deckhouse/virtualization/commit/b0b45406811db94df44e7da87ff72cfc1bfc17e6))
* **vm:** add the attached field to the status block device refs ([a9e4fc6](https://github.com/deckhouse/virtualization/commit/a9e4fc62e05c458f370211340e49b637931193e8))
* **vm:** apply new controller design ([#120](https://github.com/deckhouse/virtualization/issues/120)) ([ba12e49](https://github.com/deckhouse/virtualization/commit/ba12e492d37bd7e40a6c2566b191835948ec98ea))
* **vmbda:** apply new controller design ([2f489e4](https://github.com/deckhouse/virtualization/commit/2f489e4a1ae1a397e5a0ec00bc56e70b67f55ddc))
* **vmbda:** resolve conflicted requests ([ee2c91a](https://github.com/deckhouse/virtualization/commit/ee2c91a1f323b34a76d920d6e821f50253ab53cf))
* **vmclass:** first implementation ([#231](https://github.com/deckhouse/virtualization/issues/231)) ([a958bf3](https://github.com/deckhouse/virtualization/commit/a958bf38e4845d66560957151b833154eb511031))
* **vmip,vmipl:** apply new CRD design ([b73a1e2](https://github.com/deckhouse/virtualization/commit/b73a1e2b748496133d434524f3516583b981aba8))
* **vmip,vmipl:** apply new design ([#152](https://github.com/deckhouse/virtualization/issues/152)) ([4de51ab](https://github.com/deckhouse/virtualization/commit/4de51ab5d3d02d08916772679b6f6dad93266202))
* **vmip:** add validating ip address ([c1a3ce7](https://github.com/deckhouse/virtualization/commit/c1a3ce789272e5f2cf797d63f8c63ea648025eeb))
* **vmip:** apply new controller design ([d5ddb87](https://github.com/deckhouse/virtualization/commit/d5ddb8796c88f1ef5e13b0aad2ad83ac67e8263f))
* **vmipl:** apply new controller design ([84f2d25](https://github.com/deckhouse/virtualization/commit/84f2d25c3d703bd07471bb44964cb282a2af9d2e))
* **vm:** VD must be attached to only one virtual machine ([#221](https://github.com/deckhouse/virtualization/issues/221)) ([a6da25f](https://github.com/deckhouse/virtualization/commit/a6da25f686a0e43d503815d9dede88a1f1a1331c))


### Bug Fixes

* **api:** add name suffix ([#106](https://github.com/deckhouse/virtualization/issues/106)) ([7c7fb60](https://github.com/deckhouse/virtualization/commit/7c7fb607b9af147092ef19db0fd4208c6531c6d6))
* **core, dvcr:** configure dvcr creds before contatinerd config ([#128](https://github.com/deckhouse/virtualization/issues/128)) ([6cc4d26](https://github.com/deckhouse/virtualization/commit/6cc4d2695ecd3dd45ca4b3212a9f6089f1002772))
* **core, kubevirt:** add ability to configure burst for virt-api rate limiter ([e5c4605](https://github.com/deckhouse/virtualization/commit/e5c460570c93a626960cd38a37faf5305642081c))
* **core, kubevirt:** add ability to configure qps for virt-api rate l… ([#92](https://github.com/deckhouse/virtualization/issues/92)) ([03d5a21](https://github.com/deckhouse/virtualization/commit/03d5a21ffa5167f555e0cd8dca5cd21092fbcce1))
* **core, kubevirt:** add ability to configure qps for virt-api rate limiter ([03d5a21](https://github.com/deckhouse/virtualization/commit/03d5a21ffa5167f555e0cd8dca5cd21092fbcce1))
* **core:** fix virt-launcher's binaries ([#126](https://github.com/deckhouse/virtualization/issues/126)) ([9cab420](https://github.com/deckhouse/virtualization/commit/9cab420a314aa7fddfd4df87bd23ae59381c1b1d))
* **core:** rename exportproxy ([#145](https://github.com/deckhouse/virtualization/issues/145)) ([57eccea](https://github.com/deckhouse/virtualization/commit/57ecceacf14b67a45566600e2a103e4b14df0243))
* **cvi,vi:** add attachee handlers ([1689580](https://github.com/deckhouse/virtualization/commit/16895807f7ada6c18e5f1f7150cd1ddcf3577911))
* **kubevirt:** change boot logo in UEFI firmware ([#229](https://github.com/deckhouse/virtualization/issues/229)) ([0622c7b](https://github.com/deckhouse/virtualization/commit/0622c7b6b41e508f864fe0dbe42b2fcaacee7ae9))
* **kubevirt:** restructure edk2-ovmf files ([#232](https://github.com/deckhouse/virtualization/issues/232)) ([6ee978e](https://github.com/deckhouse/virtualization/commit/6ee978ef286d38ff4a7f22974d596dfa4a8b77f0))
* **module:** fix user API RBAC ([#116](https://github.com/deckhouse/virtualization/issues/116)) ([460f069](https://github.com/deckhouse/virtualization/commit/460f0692820ae2c028b59c077ff5e9499c18fd59))
* **observability:** fix dashboard title in tests ([#97](https://github.com/deckhouse/virtualization/issues/97)) ([ed9ea79](https://github.com/deckhouse/virtualization/commit/ed9ea79ebdd076763cdb3b98436dfa073fae32d1))
* **vd, vm:** fix sysprep and hotplug ([#225](https://github.com/deckhouse/virtualization/issues/225)) ([4a1a6d6](https://github.com/deckhouse/virtualization/commit/4a1a6d69dba3fb6c359e16764bf6d924b2ca0f2b))
* **vd,vi,cvi:** add object ref watchers ([af7e32c](https://github.com/deckhouse/virtualization/commit/af7e32cd843456566a886b1208570c00b418fdbd))
* **vd,vi,cvi:** fix capacity and cdrom ([73f929d](https://github.com/deckhouse/virtualization/commit/73f929d6f020006a7d8b2eca384e098f1fffe6e3))
* **vd,vi,cvi:** fix object ref datasource ([75b0a7d](https://github.com/deckhouse/virtualization/commit/75b0a7da07bdfd0c652b5c8a8b6b8fd7ec76bbbc))
* **vd:** add download status ([c43d895](https://github.com/deckhouse/virtualization/commit/c43d895fb77917744ff5c13a9b492ec3aa5036fa))
* **vd:** add phase collector ([e336c82](https://github.com/deckhouse/virtualization/commit/e336c82338bb122fd0836ea648615934f4ace7c7))
* **vd:** add stats reconciliation ([280a2fd](https://github.com/deckhouse/virtualization/commit/280a2fdc7b28684d359f8c81bfcd92b2f55251a6))
* **vd:** copy error from data volume ([aae4b4e](https://github.com/deckhouse/virtualization/commit/aae4b4e5aadd94e30aa7876008455b60e53ac07a))
* **vd:** fix fake pvc resizing ([6b4d431](https://github.com/deckhouse/virtualization/commit/6b4d43142a7c9d16526772fcccefac0d5552ff71))
* **vd:** fix fake pvc resizing ([6b4d431](https://github.com/deckhouse/virtualization/commit/6b4d43142a7c9d16526772fcccefac0d5552ff71))
* **vd:** fix panic if pvc is not exist ([#222](https://github.com/deckhouse/virtualization/issues/222)) ([23a0a7b](https://github.com/deckhouse/virtualization/commit/23a0a7bb54ee568e2c3f72ec9fef0f00e1cc67c1))
* **vd:** fix pvc watching ([cbf1a32](https://github.com/deckhouse/virtualization/commit/cbf1a3245b4c9d0fda2b470020e09ac502a5216c))
* **vd:** protection for deleted resource ([aefab1e](https://github.com/deckhouse/virtualization/commit/aefab1e9162935ce64168851c00116bc60f3586a))
* **vd:** set ready phase ([04f5479](https://github.com/deckhouse/virtualization/commit/04f5479f378dc69c5c7479c78004dc339e275929))
* **vd:** synchronize PVC status changes with VD status updates ([bb3e666](https://github.com/deckhouse/virtualization/commit/bb3e6668f7fc5876c515efff1eefb435276224c3))
* **vi,cvi:** fix pod errors handling ([21be7cd](https://github.com/deckhouse/virtualization/commit/21be7cd3a757cbf92f3ff9b1ba93d93629eecdbc))
* **vi:** fix status target ([296ebd7](https://github.com/deckhouse/virtualization/commit/296ebd74ac0abd012b73c0b31d047c7b6f0df85c))
* **vm:** add sync metadata handler ([#176](https://github.com/deckhouse/virtualization/issues/176)) ([c8660ac](https://github.com/deckhouse/virtualization/commit/c8660ac171251d4f822f74c43f46decbff41d388))
* **vm:** add value of the guest os info ([1ffcab7](https://github.com/deckhouse/virtualization/commit/1ffcab78de4fac975a477c14ad80467beb97f9d4))
* **vmbda:** fix hotplug api call ([0cce992](https://github.com/deckhouse/virtualization/commit/0cce99238408b670c0aee6ac6874ce83995c0d47))
* **vmbda:** fix panic ([#245](https://github.com/deckhouse/virtualization/issues/245)) ([61e4ab8](https://github.com/deckhouse/virtualization/commit/61e4ab85f1e43fabbbd6b336ad5abd87465cdc66))
* **vm:** check secret keys ([#187](https://github.com/deckhouse/virtualization/issues/187)) ([6e09877](https://github.com/deckhouse/virtualization/commit/6e098772e0c34dd15ee5f4bd09106f65197ad8c0))
* **vm:** clear annotations and labels from child resources after removing them from the vm ([#200](https://github.com/deckhouse/virtualization/issues/200)) ([df12d38](https://github.com/deckhouse/virtualization/commit/df12d382352c198983ec62a28900b8afe3563c97))
* **vm:** controller panic if using sysprep ([#184](https://github.com/deckhouse/virtualization/issues/184)) ([c03d0bc](https://github.com/deckhouse/virtualization/commit/c03d0bced13276d9f88409073c5ef0bdddf53b42))
* **vm:** do not check keys for sysprep secret ([#185](https://github.com/deckhouse/virtualization/issues/185)) ([0768d1e](https://github.com/deckhouse/virtualization/commit/0768d1ecbd90218445114ef3c9002bf9c74eb89a))
* **vm:** do not check keys for sysprep secret ([#186](https://github.com/deckhouse/virtualization/issues/186)) ([30c46c5](https://github.com/deckhouse/virtualization/commit/30c46c5bf88fb0a10b3eb1df158801f3abb2c72c))
* **vm:** fix blockdevices status and restartawaitingchanges ([#183](https://github.com/deckhouse/virtualization/issues/183)) ([9e08859](https://github.com/deckhouse/virtualization/commit/9e088592878f6aaff1dbb1ed21583aea8dd518df))
* **vm:** fix panic and virtClient ([#247](https://github.com/deckhouse/virtualization/issues/247)) ([41e43ae](https://github.com/deckhouse/virtualization/commit/41e43ae0c08113181156594af32c97724d900437))
* **vm:** fix vm-router panics when we delete a virtual machine.  ([#201](https://github.com/deckhouse/virtualization/issues/201)) ([8ebce8c](https://github.com/deckhouse/virtualization/commit/8ebce8c19a7213726a807215fe96a6edca4f8969))
* **vm:** force the startup of a VM with an AlwaysOnUnlessStoppedManually policy when creating ([#181](https://github.com/deckhouse/virtualization/issues/181)) ([a86590b](https://github.com/deckhouse/virtualization/commit/a86590b1ec18891f710b862c66f0aacfd6dd5073))
* **vmip:** double lease ([#173](https://github.com/deckhouse/virtualization/issues/173)) ([fad8e2a](https://github.com/deckhouse/virtualization/commit/fad8e2ac6f3510fdf56a0d3dbab8537715c9bed0))
* **vmipl:** fix frequent reconciles ([3e68faf](https://github.com/deckhouse/virtualization/commit/3e68faf9b2ad446f9d9548293e21901e882436a9))
* **vmip:** sticking in bound phase ([#240](https://github.com/deckhouse/virtualization/issues/240)) ([5790e28](https://github.com/deckhouse/virtualization/commit/5790e2813fb569969eeeecb1761006f5dce01cbf))
* **vm:** lifecycle vm ([#168](https://github.com/deckhouse/virtualization/issues/168)) ([2100e66](https://github.com/deckhouse/virtualization/commit/2100e661671f30f08dafc824fe73ffc6c5e5f97b))
* **vmop:** fix panic if VM is not exist ([#129](https://github.com/deckhouse/virtualization/issues/129)) ([9b90641](https://github.com/deckhouse/virtualization/commit/9b906410a0fd0c85983fa58cc2b3a079cdbb4403))
* **vm:** panic in cpu handler ([#171](https://github.com/deckhouse/virtualization/issues/171)) ([982d84e](https://github.com/deckhouse/virtualization/commit/982d84e15f015a2625a51c481c83abb978ee37cc))
* **vm:** proper boot from VirtualImage and ClusterVirtualImage ([#250](https://github.com/deckhouse/virtualization/issues/250)) ([01b4918](https://github.com/deckhouse/virtualization/commit/01b4918f172f072062cf73723bc4d7947152aa82))
* **vm:** set min max for blockdevicerefs list ([#134](https://github.com/deckhouse/virtualization/issues/134)) ([25f5f29](https://github.com/deckhouse/virtualization/commit/25f5f295ef865f50d1bc592fe9315bc856bff20e))

## [0.10.1](https://github.com/deckhouse/virtualization/compare/v0.10.0...v0.10.1) (2024-05-07)


### Miscellaneous Chores

* **main:** release 0.10.1 ([e06f682](https://github.com/deckhouse/virtualization/commit/e06f68264e78e1c8f4987bfb8e5e50109bb69b35))

## [0.10.0](https://github.com/deckhouse/virtualization/compare/v0.9.10...v0.10.0) (2024-05-07)


### Features

* added script to apply virtual machines ([5e57d5b](https://github.com/deckhouse/virtualization/commit/5e57d5b3c484a9ae780355f6c69a1c8c53c07db6))
* added script to apply virtual machines ([#43](https://github.com/deckhouse/virtualization/issues/43)) ([5e57d5b](https://github.com/deckhouse/virtualization/commit/5e57d5b3c484a9ae780355f6c69a1c8c53c07db6))
* **api:** 3rd party resource renaming ([9aaa05d](https://github.com/deckhouse/virtualization/commit/9aaa05d34e7050e8d261568c6ee162a33a04f59d))
* **api:** moved virtualization api structs to separated go mod ([#20](https://github.com/deckhouse/virtualization/issues/20)) ([215299c](https://github.com/deckhouse/virtualization/commit/215299c600a33307d972eb312646ada801a92edc))
* **api:** resource renaming ([966982d](https://github.com/deckhouse/virtualization/commit/966982d15f91fa24a5481ff146d540d1fa7819f9))
* **apiserver:** implement table-converter ([#64](https://github.com/deckhouse/virtualization/issues/64)) ([3433f91](https://github.com/deckhouse/virtualization/commit/3433f910adfb7488cc417d27145ce803e35887f6))
* **cicd:** add check yaml to workflow ([#26](https://github.com/deckhouse/virtualization/issues/26)) ([94fcb01](https://github.com/deckhouse/virtualization/commit/94fcb01862fd09924d647d1ae037ef43f1f2f7de))
* **e2e:** label and annotation ([#7](https://github.com/deckhouse/virtualization/issues/7)) ([1207c4b](https://github.com/deckhouse/virtualization/commit/1207c4bea92604eb0f2a120394c812bce4eb8890))
* **hooks:** add generation of root certificate and certificates for module apps ([#19](https://github.com/deckhouse/virtualization/issues/19)) ([f810c57](https://github.com/deckhouse/virtualization/commit/f810c57ebc5aa514f1b4cc163c2368c8bf84ee73))
* **metrics:** add node-exporter dashboard for virtual machine ([#60](https://github.com/deckhouse/virtualization/issues/60)) ([857eda0](https://github.com/deckhouse/virtualization/commit/857eda02c3a58fceb2424398b993dbbd95fa909a))
* **metrics:** add phase metrics for vm,disk,vmbda ([#45](https://github.com/deckhouse/virtualization/issues/45)) ([ec01110](https://github.com/deckhouse/virtualization/commit/ec011109ce80add8ffc6acccdc8bc24a30059a22))
* **tests:** add virtualization dashboard ([#47](https://github.com/deckhouse/virtualization/issues/47)) ([36599ee](https://github.com/deckhouse/virtualization/commit/36599eed5a92a197a7d20f536497c7e5856bf143))
* **virtualization-api:** first implementation ([#11](https://github.com/deckhouse/virtualization/issues/11)) ([a737b08](https://github.com/deckhouse/virtualization/commit/a737b088455ff8419b373c25c9e23ded446f418b))
* **vm:** always replace Pod on VM restart ([#3](https://github.com/deckhouse/virtualization/issues/3)) ([eeea94a](https://github.com/deckhouse/virtualization/commit/eeea94a6109939451f539f2d877cd3396f157274))
* **vmcpu:** added vmcpu resource and controller ([#24](https://github.com/deckhouse/virtualization/issues/24)) ([1576d18](https://github.com/deckhouse/virtualization/commit/1576d18b955cae74c952cb21e94a8847e1d22959))
* **vm:** provided sysprep ability ([#21](https://github.com/deckhouse/virtualization/issues/21)) ([552a06e](https://github.com/deckhouse/virtualization/commit/552a06eb1d2f50ca2d765d42c5ab4d701c92a555))


### Bug Fixes

* **client:** regenerate the lease and cpumodel with the nonNamespaced flag ([#32](https://github.com/deckhouse/virtualization/issues/32)) ([c329b69](https://github.com/deckhouse/virtualization/commit/c329b6953b3001fa315f0c8b60f3ebbc9a07534e))
* **client:** rename pkg kubecli to kubeclient ([#34](https://github.com/deckhouse/virtualization/issues/34)) ([2d49290](https://github.com/deckhouse/virtualization/commit/2d492906a8b2439e397e322af5d72456a0f0ddeb))
* **client:** rename pkg kubecli to kubeclient ([#36](https://github.com/deckhouse/virtualization/issues/36)) ([cd94269](https://github.com/deckhouse/virtualization/commit/cd942696adfba67ff6da62358922bf27a1cdc6e1))
* **crd:** corrects inaccuracies in documentation about CVMI, VMI, VMD ([#16](https://github.com/deckhouse/virtualization/issues/16)) ([9d7015c](https://github.com/deckhouse/virtualization/commit/9d7015c4335d89b3d7f4c92a100560cf3560d318))
* **crd:** increasing the grace period for virtual machine shutdown ([#12](https://github.com/deckhouse/virtualization/issues/12)) ([ba34ffd](https://github.com/deckhouse/virtualization/commit/ba34ffd05ed19f7e62966138d3bb6636718cf4b6))
* **crd:** restore the original value of the terminationGracePeriodSeconds parameter ([#46](https://github.com/deckhouse/virtualization/issues/46)) ([0dccc5b](https://github.com/deckhouse/virtualization/commit/0dccc5b76166fbfadd73eb16c028e0b6560f0ba4))
* fixed ingress and service monitor ([#49](https://github.com/deckhouse/virtualization/issues/49)) ([6604843](https://github.com/deckhouse/virtualization/commit/660484364ccf43ef558334ce34c1f3196ae315bc))
* fixed storage class and count processing ([#51](https://github.com/deckhouse/virtualization/issues/51)) ([984f2b2](https://github.com/deckhouse/virtualization/commit/984f2b298da2ae5316f3deedf7fd99a6cad032d6))
* force legacy discovery for Kubernetes 1.27+ ([#82](https://github.com/deckhouse/virtualization/issues/82)) ([501f4dc](https://github.com/deckhouse/virtualization/commit/501f4dccdc68f58dca7b90351ea2928dbd77e70e))
* free some space on Github-hosted runners ([#53](https://github.com/deckhouse/virtualization/issues/53)) ([ce6a38f](https://github.com/deckhouse/virtualization/commit/ce6a38fb5268981dd62a152774824b59705eb4d6))
* **hooks:** change python image ([#13](https://github.com/deckhouse/virtualization/issues/13)) ([a7b7d5b](https://github.com/deckhouse/virtualization/commit/a7b7d5b7c4d6f159de49395054e91874ccfae5b8))
* **metrics:** remove duplicated metrics ([#55](https://github.com/deckhouse/virtualization/issues/55)) ([1ad3dea](https://github.com/deckhouse/virtualization/commit/1ad3dea434c66b60ff20f8a03477081e2d5c502b))
* mute noisy log messages by default ([#10](https://github.com/deckhouse/virtualization/issues/10)) ([03704aa](https://github.com/deckhouse/virtualization/commit/03704aa93cdd529dd28196507d6a7f59e162a59a))
* **performance-test:** change pod/service match label ([#65](https://github.com/deckhouse/virtualization/issues/65)) ([19c984b](https://github.com/deckhouse/virtualization/commit/19c984bb7e0f5d93bcfd9f790779af9362b597a5))
* **pre-delete-hook:** hook should not fail ([#71](https://github.com/deckhouse/virtualization/issues/71)) ([e6ee59b](https://github.com/deckhouse/virtualization/commit/e6ee59bcd24970b5a5ac720432169b720146b4c9))
* **tests:** move dashboard to tests ([#48](https://github.com/deckhouse/virtualization/issues/48)) ([ad99cff](https://github.com/deckhouse/virtualization/commit/ad99cff67e95edbf105b8747c7c161a0aefe5371))
* **vmcpur:** fixed reconciliation bugs and rbac ([#29](https://github.com/deckhouse/virtualization/issues/29)) ([11d83f1](https://github.com/deckhouse/virtualization/commit/11d83f17d82375648425dfcc3ce9d3bbe80bc754))
* **vm:** fixed annotations and labels propagation ([#5](https://github.com/deckhouse/virtualization/issues/5)) ([1a99cc5](https://github.com/deckhouse/virtualization/commit/1a99cc54ec09dcf6bc8cbdc9dbf70df3bdbb7b08))
* **vmi,cvmi:** normalize size format ([#52](https://github.com/deckhouse/virtualization/issues/52)) ([b892ddd](https://github.com/deckhouse/virtualization/commit/b892ddd03f9edf91dc30b4bc922fada1c2a996a7))
* **vm:** remove restartID and spec.restartApprovalID ([#27](https://github.com/deckhouse/virtualization/issues/27)) ([b9c0c4d](https://github.com/deckhouse/virtualization/commit/b9c0c4ddf41dc1bdc065691a968900e8c53f9e38))
* **vm:** set default for modelName in comparators ([979c4f4](https://github.com/deckhouse/virtualization/commit/979c4f4f17b9009db2b537bafc976bb7d0710d48))
* **vm:** set default for modelName in comparators ([#37](https://github.com/deckhouse/virtualization/issues/37)) ([979c4f4](https://github.com/deckhouse/virtualization/commit/979c4f4f17b9009db2b537bafc976bb7d0710d48))
