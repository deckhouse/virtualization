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

package kubeapi

import (
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestKubeApi(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "KubeApi Suite")
}

var _ = Describe("kubeapi", func() {
	It("HasDRAFeatureGates", func() {
		err := os.Setenv("KUBE_APISERVER_FEATURE_GATES", `["APIResponseCompression","APIServerIdentity","APIServerTracing","AggregatedDiscoveryRemoveBetaType","AllowParsingUserUIDFromCertAuth","AnonymousAuthConfigurableEndpoints","AnyVolumeDataSource","AuthorizeNodeWithSelectors","AuthorizeWithSelectors","BtreeWatchCache","CPUManagerPolicyBetaOptions","CPUManagerPolicyOptions","CRDValidationRatcheting","CSIMigrationPortworx","ComponentSLIs","ConsistentListFromCache","ContainerCheckpoint","ContextualLogging","CronJobsScheduledAnnotation","CustomResourceFieldSelectors","DRAAdminAccess","DRAConsumableCapacity","DRADeviceBindingConditions","DRAPartitionableDevices","DRAPrioritizedList","DRAResourceClaimDeviceStatus","DRASchedulerFilterTimeout","DeclarativeValidation","DetectCacheInconsistency","DisableAllocatorDualWrite","DisableCPUQuotaWithExclusiveCPUs","DisableNodeKubeProxyVersion","DynamicResourceAllocation","ExecProbeTimeout","ExternalServiceAccountTokenSigner","GracefulNodeShutdown","GracefulNodeShutdownBasedOnPodPriority","HonorPVReclaimPolicy","ImageMaximumGCAge","InOrderInformers","InPlacePodVerticalScaling","JobBackoffLimitPerIndex","JobManagedBy","JobPodReplacementPolicy","JobSuccessPolicy","KubeletCgroupDriverFromCRI","KubeletFineGrainedAuthz","KubeletPSI","KubeletPodResourcesDynamicResources","KubeletPodResourcesGet","KubeletPodResourcesListUseActivePods","KubeletSeparateDiskGC","KubeletServiceAccountTokenForCredentialProviders","KubeletTracing","ListFromCacheSnapshot","LoadBalancerIPMode","LogarithmicScaleDown","LoggingBetaOptions","MatchLabelKeysInPodAffinity","MatchLabelKeysInPodTopologySpread","MatchLabelKeysInPodTopologySpreadSelectorMerge","MemoryManager","MultiCIDRServiceAllocator","NFTablesProxyMode","NodeInclusionPolicyInPodTopologySpread","NodeSwap","OpenAPIEnums","OrderedNamespaceDeletion","PodDeletionCost","PodIndexLabel","PodLevelResources","PodLifecycleSleepAction","PodLifecycleSleepActionAllowZero","PodObservedGenerationTracking","PodReadyToStartContainersCondition","PodSchedulingReadiness","PortForwardWebsockets","PreferSameTrafficDistribution","PreventStaticPodAPIReferences","ProbeHostPodSecurityStandards","ProcMountType","RecoverVolumeExpansionFailure","RecursiveReadOnlyMounts","RelaxedDNSSearchValidation","RelaxedEnvironmentVariableValidation","ReloadKubeletServerCertificateFile","RemoteRequestHeaderUID","ResilientWatchCacheInitialization","RetryGenerateName","RotateKubeletServerCertificate","SELinuxChangePolicy","SELinuxMountReadWriteOncePod","SchedulerAsyncPreemption","SchedulerPopFromBackoffQ","SchedulerQueueingHints","SeparateTaintEvictionController","ServiceAccountNodeAudienceRestriction","ServiceAccountTokenJTI","ServiceAccountTokenNodeBinding","ServiceAccountTokenNodeBindingValidation","ServiceAccountTokenPodNodeInfo","ServiceTrafficDistribution","SidecarContainers","SizeBasedListCostEstimate","SizeMemoryBackedVolumes","StatefulSetAutoDeletePVC","StatefulSetSemanticRevisionComparison","StorageNamespaceIndex","StorageVersionHash","StreamingCollectionEncodingToJSON","StreamingCollectionEncodingToProtobuf","StrictCostEnforcementForVAP","StrictCostEnforcementForWebhooks","StructuredAuthenticationConfiguration","StructuredAuthenticationConfigurationEgressSelector","StructuredAuthorizationConfiguration","SupplementalGroupsPolicy","SystemdWatchdog","TokenRequestServiceAccountUIDValidation","TopologyAwareHints","TopologyManagerPolicyBetaOptions","TopologyManagerPolicyOptions","TranslateStreamCloseWebsocketRequests","UnauthenticatedHTTP2DOSMitigation","UserNamespacesSupport","VolumeAttributesClass","WatchList","WinDSR","WinOverlay","WindowsGracefulNodeShutdown"]`)
		Expect(err).NotTo(HaveOccurred())

		Expect(HasDRAFeatureGates()).To(BeTrue())
	})
})
