package kubevirt

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/tidwall/gjson"
	"kube-api-proxy/pkg/rewriter"
)

func TestRewriteRequestAPIResourceList(t *testing.T) {
	// Request APIResourcesList of original, non-renamed resources.
	origGroup := "kubevirt.io"
	reqURL, err := url.Parse(`http://127.0.0.1:121/apis/kubevirt.io/v1`)
	if err != nil {
		t.Fatalf("should parse hardcoded url: %v", err)
	}
	req := &http.Request{
		Method: http.MethodGet,
		URL:    reqURL,
	}

	// Response body with renamed APIResourcesList
	ProxyRewriteRules.KindPrefix = "KV"
	ProxyRewriteRules.URLPrefix = "kv"
	resp := `
{
  "kind": "APIResourceList",
  "apiVersion": "v1",
  "groupVersion": "x.virtualization.deckhouse.io/v1",
  "resources": [
    {"name":"kvvirtualmachines","singularName":"kvvirtualmachine",
    "namespaced":true,"kind":"KVVirtualMachine",
    "verbs":["delete","deletecollection","get","list","patch","create","update","watch"],
    "shortNames":["xvm","xvms"],"categories":["kubevirt"],"storageVersionHash":"1qIJ90Mhvd8="},
    {"name":"kvvirtualmachines/status","singularName":"","namespaced":true,
     "kind":"KVVirtualMachine","verbs":["get","patch","update"]},

    {"name":"kvdatavolumes","singularName":"kvdatavolume",
     "namespaced":true,"kind":"KVDataVolume",
     "verbs":["delete","deletecollection","get","list","patch","create","update","watch"],
     "shortNames":["xdv","xdvs"],"categories":["kubevirt"],"storageVersionHash":"Nwlto9QquX0="},
   
    {"name":"kvdatavolumes/status","singularName":"","namespaced":true,
     "kind":"KVDataVolume","verbs":["get","patch","update"]}
]
}`

	rwr := NewKubevirtRewriter()
	var reqResult *rewriter.RewriteRequestResult

	// Test rewriting request parameters.
	{
		reqResult, err = rwr.RewriteInbound(req)
		if err != nil {
			t.Fatalf("should rewrite request: %v", err)
		}
		if reqResult == nil {
			t.Fatalf("should return non nil result: %v", err)
		}
		if reqResult.TargetPath == "" {
			t.Fatalf("should set target path: %v", err)
		}
		fmt.Printf("path: %+v\n", reqResult.TargetPath)

		if reqResult.OrigGroup != origGroup {
			t.Fatalf("should set OrigGroup to %s, got: %s", origGroup, reqResult.OrigGroup)
		}

		fmt.Printf("res: %+v\n", reqResult)
	}

	// Test response rewriting
	{
		bodyReader := io.NopCloser(bytes.NewBuffer([]byte(resp)))

		bodyBytes, err := rwr.RewriteOutbound(reqResult, "application/json", bodyReader)
		if err != nil {
			t.Fatalf("should rewrite body with renamed resources: %v", err)
		}

		bodyStr := string(bodyBytes)

		// Should contain resources from original group.
		if !strings.Contains(bodyStr, `"virtualmachines"`) {
			t.Fatalf("should contain virtualmachines resource, body: %s", bodyStr)
		}

		if !strings.Contains(bodyStr, `"virtualmachines/status"`) {
			t.Fatalf("should contain virtualmachines/status resource, body: %s", bodyStr)
		}

		// Should filter out resources from another group (i.e. cdi.kubevirt.io)
		if strings.Contains(bodyStr, `"datavolumes"`) {
			t.Fatalf("should not contain datavolumes resource, body: %s", bodyStr)
		}

		if strings.Contains(bodyStr, `"datavolumes/status"`) {
			t.Fatalf("should not contain datavolumes/status resource, body: %s", bodyStr)
		}

		fmt.Printf("body: %s\n", bodyStr)
	}
}

func TestRewriteAdmissionReviewRequestForResource(t *testing.T) {
	// Request APIResourcesList of original, non-renamed resources.
	origGroup := "kubevirt.io"
	reqURL, err := url.Parse(`http://127.0.0.1:121/validate-x-virtualization-deckhouse-io-v1-virtualmachine`)
	if err != nil {
		t.Fatalf("should parse hardcoded url: %v", err)
	}
	req := &http.Request{
		Method: http.MethodPost,
		URL:    reqURL,
	}

	// Response body with renamed APIResourcesList
	ProxyRewriteRules.KindPrefix = "KV"
	ProxyRewriteRules.URLPrefix = "kv"
	rwr := NewKubevirtRewriter()

	admissionReview := `
{
"kind":"AdmissionReview",
"apiVersion":"admission.k8s.io/v1",
  "request":{
    "uid":"389cfe15-34a1-4829-ad4d-de2576385711",
    "kind":{"group":"x.virtualization.deckhouse.io","version":"v1","kind":"KVVirtualMachine"},
    "resource":{"group":"x.virtualization.deckhouse.io","version":"v1","resource":"kvvirtualmachines"},
    "requestKind":{"group":"x.virtualization.deckhouse.io","version":"v1","kind":"KVVirtualMachine"},
    "requestResource":{"group":"x.virtualization.deckhouse.io","version":"v1","resource":"kvvirtualmachines"},
    "name":"cloud-alpine",
    "namespace":"vm",
    "operation":"UPDATE",
    "userInfo":{"username":"kubernetes-admin","groups":["system:masters","system:authenticated"]},
    "object":{
      "apiVersion":"x.virtualization.deckhouse.io/v1",
      "kind":"KVVirtualMachine",
      "metadata":{
        "annotations":{
          "kubevirt.io/latest-observed-api-version":"v1",
          "kubevirt.io/storage-observed-api-version":"v1"
        },
        "creationTimestamp":"2024-02-05T12:42:32Z",
        "finalizers":["virtualization.deckhouse.io/kvvm-protection","kubevirt.io/virtualMachineControllerFinalize"],
        "generation":5,"labels":{"vm":"cloud-alpine"},
        "managedFields":[
          {"apiVersion":"x.virtualization.deckhouse.io/v1",
            "fieldsType":"FieldsV1",
            "fieldsV1":{"f:status":{".":{},"f:conditions":{},"f:created":{},"f:desiredGeneration":{},"f:observedGeneration":{},"f:printableStatus":{},"f:ready":{},"f:volumeSnapshotStatuses":{}}},
            "manager":"Go-http-client",
            "operation":"Update","subresource":"status","time":"2024-03-06T14:38:39Z"},
          {"apiVersion":"x.virtualization.deckhouse.io/v1",
            "fieldsType":"FieldsV1",
            "fieldsV1":{"f:metadata":{"f:annotations":{".":{},"f:custom-anno":{},"f:kubevirt.io/latest-observed-api-version":{},"f:kubevirt.io/storage-observed-api-version":{},"f:virt.deckhouse.io/last-propagated-vm-annotations":{},"f:virt.deckhouse.io/last-propagated-vm-labels":{},"f:virt.deckhouse.io/vm.last-applied-spec":{}},
              "f:finalizers":{".":{},"v:\"kubevirt.io/virtualMachineControllerFinalize\"":{},"v:\"virtualization.deckhouse.io/kvvm-protection\"":{}},
              "f:labels":{".":{},"f:vm":{}},"f:ownerReferences":{".":{},
                "k:{\"uid\":\"904cfea9-c9d6-4d3a-82f7-5790b1a1b3e0\"}":{}}},
              "f:spec":{".":{},"f:runStrategy":{},
                "f:template":{".":{},"f:metadata":{".":{},"f:annotations":{
                  ".":{},"f:cni.cilium.io/ipAddress":{},"f:kubevirt.io/allow-pod-bridge-network-live-migration":{}},"f:creationTimestamp":{}},
                  "f:spec":{".":{},"f:domain":{".":{},"f:cpu":{".":{},"f:model":{}},"f:devices":{".":{},"f:autoattachInputDevice":{},"f:disks":{},"f:inputs":{},"f:interfaces":{},"f:rng":{}},"f:features":{".":{},"f:acpi":{".":{},"f:enabled":{}},"f:smm":{".":{},"f:enabled":{}}},"f:firmware":{},"f:machine":{".":{},"f:type":{}},"f:resources":{".":{},"f:limits":{".":{},"f:cpu":{},"f:memory":{}},"f:requests":{".":{},"f:cpu":{},"f:memory":{}}}},"f:networks":{},"f:terminationGracePeriodSeconds":{},"f:volumes":{}}}}},
            "manager":"Go-http-client","operation":"Update","time":"2024-03-07T10:56:32Z"},
          {"apiVersion":"x.virtualization.deckhouse.io/v1",
            "fieldsType":"FieldsV1","fieldsV1":{"f:metadata":{"f:annotations":{"f:qwe":{}}}},"manager":"kubectl-edit","operation":"Update","time":"2024-03-08T14:49:07Z"}],
        "name":"cloud-alpine","namespace":"vm",
        "ownerReferences":[
          {"apiVersion":"virtualization.deckhouse.io/v1alpha2",
            "blockOwnerDeletion":true,"controller":true,"kind":"VirtualMachine","name":"cloud-alpine","uid":"904cfea9-c9d6-4d3a-82f7-5790b1a1b3e0"}
        ],"resourceVersion":"265111919","uid":"4c74c3ff-2199-4f20-a71c-3b0e5fb505ca"},
      "spec":{"runStrategy":"Manual","template":{"metadata":{"annotations":{"cni.cilium.io/ipAddress":"10.66.10.1","kubevirt.io/allow-pod-bridge-network-live-migration":"true"},"creationTimestamp":null},"spec":{"architecture":"amd64","domain":{"cpu":{"model":"Nehalem"},"devices":{"autoattachInputDevice":true,"disks":[{"disk":{"bus":"scsi"},"name":"vmd-cloud-alpine","serial":"vmd-cloud-alpine"},{"disk":{"bus":"scsi"},"name":"vmd-cloud-alpine-data","serial":"vmd-cloud-alpine-data"},{"disk":{"bus":"scsi"},"name":"cloudinit"}],"inputs":[{"bus":"usb","name":"default-0","type":"tablet"}],"interfaces":[{"bridge":{},"model":"virtio","name":"default"}],"rng":{}},"features":{"acpi":{"enabled":true},"smm":{"enabled":true}},"firmware":{},"machine":{"type":"q35"},"resources":{"limits":{"cpu":"4","memory":"4Gi"},"requests":{"cpu":"4","memory":"4Gi"}}},"networks":[{"name":"default","pod":{}}],"terminationGracePeriodSeconds":60,"volumes":[{"name":"vmd-cloud-alpine","persistentVolumeClaim":{"claimName":"vmd-cloud-alpine-f5366231-3d3d-4536-b455-50156a6a2bbf"}},{"name":"vmd-cloud-alpine-data","persistentVolumeClaim":{"claimName":"vmd-cloud-alpine-data-77febda4-2857-4fba-937c-ae94abc900b4"}},{"cloudInitNoCloud":{"userData":"#cloud-config\nuser: ubuntu\npassword: ubuntu\nchpasswd: { expire: False }\n"},"name":"cloudinit"}]}}},
      "status":{
         "conditions":[
           {"lastProbeTime":null,"lastTransitionTime":"2024-03-06T14:38:39Z","status":"True","type":"Ready"},
           {"lastProbeTime":null,"lastTransitionTime":null,"status":"True","type":"LiveMigratable"},
           {"lastProbeTime":"2024-02-29T14:11:05Z","lastTransitionTime":null,"status":"True","type":"AgentConnected"}],
        "created":true,"desiredGeneration":5,"observedGeneration":5,
        "printableStatus":"Running","ready":true,
        "volumeSnapshotStatuses":[
          {"enabled":false,"name":"vmd-cloud-alpine","reason":"No VolumeSnapshotClass: Volume snapshots are not configured for this StorageClass [linstor-thick-data-r1] [vmd-cloud-alpine]"},
          {"enabled":false,"name":"vmd-cloud-alpine-data","reason":"No VolumeSnapshotClass: Volume snapshots are not configured for this StorageClass [linstor-thick-data-r1] [vmd-cloud-alpine-data]"},
          {"enabled":false,"name":"cloudinit","reason":"Snapshot is not supported for this volumeSource type [cloudinit]"}]}},

    "oldObject":{
       "apiVersion":"x.virtualization.deckhouse.io/v1",
       "kind":"KVVirtualMachine",
       "metadata":{"annotations":{
           "kubevirt.io/latest-observed-api-version":"v1",
           "kubevirt.io/storage-observed-api-version":"v1"
         },
       "creationTimestamp":"2024-02-05T12:42:32Z",
       "finalizers":["virtualization.deckhouse.io/kvvm-protection","kubevirt.io/virtualMachineControllerFinalize"],
       "generation":5,
       "labels":{"vm":"cloud-alpine"},
       "managedFields":[{"apiVersion":"x.virtualization.deckhouse.io/v1","fieldsType":"FieldsV1","fieldsV1":{"f:status":{".":{},"f:conditions":{},"f:created":{},"f:desiredGeneration":{},"f:observedGeneration":{},"f:printableStatus":{},"f:ready":{},"f:volumeSnapshotStatuses":{}}},"manager":"Go-http-client","operation":"Update","subresource":"status","time":"2024-03-06T14:38:39Z"},{"apiVersion":"x.virtualization.deckhouse.io/v1","fieldsType":"FieldsV1","fieldsV1":{"f:metadata":{"f:annotations":{"f:asd":{}}}},"manager":"kubectl-replace","operation":"Update","time":"2024-03-07T10:55:59Z"},{"apiVersion":"x.virtualization.deckhouse.io/v1","fieldsType":"FieldsV1","fieldsV1":{"f:metadata":{"f:annotations":{".":{},"f:custom-anno":{},"f:kubevirt.io/latest-observed-api-version":{},"f:kubevirt.io/storage-observed-api-version":{},"f:virt.deckhouse.io/last-propagated-vm-annotations":{},"f:virt.deckhouse.io/last-propagated-vm-labels":{},"f:virt.deckhouse.io/vm.last-applied-spec":{}},"f:finalizers":{".":{},"v:\"kubevirt.io/virtualMachineControllerFinalize\"":{},"v:\"virtualization.deckhouse.io/kvvm-protection\"":{}},"f:labels":{".":{},"f:vm":{}},"f:ownerReferences":{".":{},"k:{\"uid\":\"904cfea9-c9d6-4d3a-82f7-5790b1a1b3e0\"}":{}}},"f:spec":{".":{},"f:runStrategy":{},"f:template":{".":{},"f:metadata":{".":{},"f:annotations":{".":{},"f:cni.cilium.io/ipAddress":{},"f:kubevirt.io/allow-pod-bridge-network-live-migration":{}},"f:creationTimestamp":{}},"f:spec":{".":{},"f:domain":{".":{},"f:cpu":{".":{},"f:model":{}},"f:devices":{".":{},"f:autoattachInputDevice":{},"f:disks":{},"f:inputs":{},"f:interfaces":{},"f:rng":{}},"f:features":{".":{},"f:acpi":{".":{},"f:enabled":{}},"f:smm":{".":{},"f:enabled":{}}},"f:firmware":{},"f:machine":{".":{},"f:type":{}},"f:resources":{".":{},"f:limits":{".":{},"f:cpu":{},"f:memory":{}},"f:requests":{".":{},"f:cpu":{},"f:memory":{}}}},"f:networks":{},"f:terminationGracePeriodSeconds":{},"f:volumes":{}}}}},"manager":"Go-http-client","operation":"Update","time":"2024-03-07T10:56:32Z"},{"apiVersion":"x.virtualization.deckhouse.io/v1","fieldsType":"FieldsV1","fieldsV1":{"f:metadata":{"f:annotations":{"f:qwe":{}}}},"manager":"kubectl-edit","operation":"Update","time":"2024-03-08T14:49:07Z"}],"name":"cloud-alpine","namespace":"vm","ownerReferences":[{"apiVersion":"virtualization.deckhouse.io/v1alpha2","blockOwnerDeletion":true,"controller":true,"kind":"VirtualMachine","name":"cloud-alpine","uid":"904cfea9-c9d6-4d3a-82f7-5790b1a1b3e0"}],"resourceVersion":"265111919","uid":"4c74c3ff-2199-4f20-a71c-3b0e5fb505ca"},"spec":{"runStrategy":"Manual","template":{"metadata":{"annotations":{"cni.cilium.io/ipAddress":"10.66.10.1","kubevirt.io/allow-pod-bridge-network-live-migration":"true"},"creationTimestamp":null},"spec":{"architecture":"amd64","domain":{"cpu":{"model":"Nehalem"},"devices":{"autoattachInputDevice":true,"disks":[{"disk":{"bus":"scsi"},"name":"vmd-cloud-alpine","serial":"vmd-cloud-alpine"},{"disk":{"bus":"scsi"},"name":"vmd-cloud-alpine-data","serial":"vmd-cloud-alpine-data"},{"disk":{"bus":"scsi"},"name":"cloudinit"}],"inputs":[{"bus":"usb","name":"default-0","type":"tablet"}],"interfaces":[{"bridge":{},"model":"virtio","name":"default"}],"rng":{}},"features":{"acpi":{"enabled":true},"smm":{"enabled":true}},"firmware":{},"machine":{"type":"q35"},"resources":{"limits":{"cpu":"4","memory":"4Gi"},"requests":{"cpu":"4","memory":"4Gi"}}},"networks":[{"name":"default","pod":{}}],"terminationGracePeriodSeconds":60,"volumes":[{"name":"vmd-cloud-alpine","persistentVolumeClaim":{"claimName":"vmd-cloud-alpine-f5366231-3d3d-4536-b455-50156a6a2bbf"}},{"name":"vmd-cloud-alpine-data","persistentVolumeClaim":{"claimName":"vmd-cloud-alpine-data-77febda4-2857-4fba-937c-ae94abc900b4"}},{"cloudInitNoCloud":{"userData":"#cloud-config\nuser: ubuntu\npassword: ubuntu\nchpasswd: { expire: False }\n"},"name":"cloudinit"}]}}},"status":{"conditions":[{"lastProbeTime":null,"lastTransitionTime":"2024-03-06T14:38:39Z","status":"True","type":"Ready"},{"lastProbeTime":null,"lastTransitionTime":null,"status":"True","type":"LiveMigratable"},{"lastProbeTime":"2024-02-29T14:11:05Z","lastTransitionTime":null,"status":"True","type":"AgentConnected"}],"created":true,"desiredGeneration":5,"observedGeneration":5,"printableStatus":"Running","ready":true,"volumeSnapshotStatuses":[{"enabled":false,"name":"vmd-cloud-alpine","reason":"No VolumeSnapshotClass: Volume snapshots are not configured for this StorageClass [linstor-thick-data-r1] [vmd-cloud-alpine]"},{"enabled":false,"name":"vmd-cloud-alpine-data","reason":"No VolumeSnapshotClass: Volume snapshots are not configured for this StorageClass [linstor-thick-data-r1] [vmd-cloud-alpine-data]"},{"enabled":false,"name":"cloudinit","reason":"Snapshot is not supported for this volumeSource type [cloudinit]"}]}},"dryRun":false,"options":{"kind":"UpdateOptions","apiVersion":"meta.k8s.io/v1","fieldManager":"kubectl-edit","fieldValidation":"Strict"}}}
}
`
	req.Body = io.NopCloser(bytes.NewBuffer([]byte(admissionReview)))

	var reqResult *rewriter.RewriteRequestResult

	// Test rewriting request parameters.
	{
		reqResult, err = rwr.RewriteInbound(req)
		if err != nil {
			t.Fatalf("should rewrite request: %v", err)
		}
		if reqResult == nil {
			t.Fatalf("should return non nil result: %v", err)
		}
		if reqResult.TargetPath == "" {
			t.Fatalf("should set target path: %v", err)
		}
		fmt.Printf("res.TargetPath: %+v\n", reqResult.TargetPath)

		//if reqResult.OrigGroup != origGroup {
		//	t.Fatalf("should set OrigGroup to %s, got: %s", origGroup, reqResult.OrigGroup)
		//}

		fmt.Printf("res: %+v\n", reqResult)
		fmt.Printf("res.Body: %+v\n", string(reqResult.Body))
	}

	tests := []struct {
		path     string
		expected string
	}{
		{"request.kind.group", origGroup},
		{"request.kind.kind", "VirtualMachine"},
		{"request.requestKind.group", origGroup},
		{"request.requestKind.kind", "VirtualMachine"},
		{"request.resource.group", origGroup},
		{"request.resource.resource", "virtualmachines"},
		{"request.requestResource.group", origGroup},
		{"request.requestResource.resource", "virtualmachines"},
		{"request.object.apiVersion", "kubevirt.io/v1"},
		{"request.object.kind", "VirtualMachine"},
		{"request.oldObject.apiVersion", "kubevirt.io/v1"},
		{"request.oldObject.kind", "VirtualMachine"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			actual := gjson.GetBytes(reqResult.Body, tt.path).String()
			if actual != tt.expected {
				t.Fatalf("%s value should be %s, got %s", tt.path, tt.expected, actual)
			}
		})
	}

	// Test response rewriting
	//{
	//	//bodyReader := io.NopCloser(bytes.NewBuffer([]byte(resp)))
	//
	//	bodyBytes, err := rwr.RewriteFromTarget(reqResult, "application/json", bodyReader)
	//	if err != nil {
	//		t.Fatalf("should rewrite body with renamed resources: %v", err)
	//	}
	//
	//	bodyStr := string(bodyBytes)
	//
	//	// Should contain resources from original group.
	//	if !strings.Contains(bodyStr, `"virtualmachines"`) {
	//		t.Fatalf("should contain virtualmachines resource, body: %s", bodyStr)
	//	}
	//
	//	if !strings.Contains(bodyStr, `"virtualmachines/status"`) {
	//		t.Fatalf("should contain virtualmachines/status resource, body: %s", bodyStr)
	//	}
	//
	//	// Should filter out resources from another group (i.e. cdi.kubevirt.io)
	//	if strings.Contains(bodyStr, `"datavolumes"`) {
	//		t.Fatalf("should not contain datavolumes resource, body: %s", bodyStr)
	//	}
	//
	//	if strings.Contains(bodyStr, `"datavolumes/status"`) {
	//		t.Fatalf("should not contain datavolumes/status resource, body: %s", bodyStr)
	//	}
	//
	//	fmt.Printf("body: %s\n", bodyStr)
	//}
}
