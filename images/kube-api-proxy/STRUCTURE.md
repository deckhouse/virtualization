# kube-api-proxy structure

## API client proxying

The idea is simple: make controller connect to local proxy, so proxy will pass 
requests to real Kubernetes API Server. Proxy may rewrite JSON and protobuf payloads
for different purposes, e.g. resources renaming.

Pod changes:
- Add a new container with the proxy.
- Set KUBECONFIG variable in the controller container. File should contain configuration to connect to proxy port.

```plantuml
@startuml
box "Pod with Controller" #fff
participant "container\nname: controller" as ctrl
note over ctrl
Use KUBECONFIG file to connect
to local proxy instead of
directly using API server:
""clusters:""
""- cluster:""
""    server: http://127.0.0.1:23915""
endnote
queue "additional container\nname: proxy" as proxy
/ note over proxy
Listen on ""127.0.0.1:23915""
and pass requests to
Kubernetes API Server
endnote
endbox
box "Control Plane" #fff
participant "Kubernetes\nAPI Server" as kube_api
endbox

== Get, List, Delete operations ==

ctrl -> proxy : Request operation via endpoint:\n\n/apis/kubevirt.io/v1/virtualmachines
proxy -> kube_api : Rewrite endpoint, pass request to:\n\n/apis/x.virtualization.deckhouse.io↩︎\n/v1/prefixedvirtualmachines

kube_api -> proxy : Response with renamed resources:\n\napiVersion: x.virtualization.deckhouse.io/v1\nkind: PrefixedVirtualMachine
proxy -> ctrl : Rewrite payload, pass\nresponse with restored resources:\n\napiVersion: kubevirt.io/v1\nkind: VirtualMachine

== Create, Update, Patch operations ==

ctrl -> proxy : Request operation via endpoint:\n\n/apis/kubevirt.io/v1/virtualmachines\n\nA payload contains original resources:\n\napiVersion: kubevirt.io/v1\nkind: VirtualMachine
proxy -> kube_api : Rewrite endpoint and payload,\npass request with renamed resources:\n\n/apis/x.virtualization.deckhouse.io↩︎\n/v1/prefixedvirtualmachines\n\napiVersion: x.virtualization.deckhouse.io/v1\nkind: PrefixedVirtualMachine

kube_api -> proxy : Response with renamed resources:\n\napiVersion: x.virtualization.deckhouse.io/v1\nkind: PrefixedVirtualMachine
proxy -> ctrl : Rewrite payload, pass\nresponse with restored resources:\n\napiVersion: kubevirt.io/v1\nkind: VirtualMachine

== Watch operation ==

ctrl -> proxy : Request WATCH operation via endpoint:\n\n/apis/kubevirt.io↩︎\n/v1/virtualmachines?watch=true
activate proxy
proxy -> kube_api : Rewrite endpoint, pass request to:\n\n/apis/x.virtualization.deckhouse.io↩︎\n/v1/prefixedvirtualmachines?watch=true
activate kube_api

kube_api -> kube_api : Generate\nWATCH\nevents

kube_api -> proxy : ADDED, MODIFIED or DELETED\nevent with renamed resource:\n\napiVersion: x.virtualization.deckhouse.io/v1\nkind: PrefixedVirtualMachine
activate proxy
proxy -> ctrl : Rewrite payload, pass\nevent with restored resource:\n\napiVersion: kubevirt.io/v1\nkind: VirtualMachine
deactivate proxy

kube_api -> proxy : BOOKMARK event with renamed resource:\n\napiVersion: x.virtualization.deckhouse.io/v1\nkind: PrefixedVirtualMachine
activate proxy
proxy -> ctrl : Rewrite payload, pass\nevent with restored resource:\n\napiVersion: kubevirt.io/v1\nkind: VirtualMachine
deactivate proxy

kube_api -> proxy : Stop WATCH operation
deactivate kube_api
proxy -> ctrl : Stop WATCH operation
deactivate proxy

@endplantuml
```

## Webhook proxying

Kubernetes API Server connects to proxy, so proxy will pass AdmissionReview to real webhook.  Proxy may rewrite JSON payloads
for different purposes, e.g. resources renaming.

Additional changes:

- A targetPort in the webhook Service should point to proxy container.
- A proxy container should mount secret with certificates.

```plantuml
@startuml
box "Pod with Controller" #fff
participant "container\nname: controller" as ctrl
queue "additional container\nname: proxy" as proxy
endbox
box "Control Plane" #fff
participant "Kubernetes\nAPI Server" as kube_api
endbox

note over ctrl
Listen on ""0.0.0.0:9443""
endnote
/ note over proxy
Listen on ""0.0.0.0:24192""
and pass requests to
the controller ""127.0.0.1:9443""
endnote
/ note over kube_api
Pass AdmissionReview to Pod
endnote

== Webhook handling ==

kube_api -> proxy : Request admission review via\nconfigured endpoint:\n\n/validate-x-virtualization-↩︎\ndeckhouse-io-prefixed-virtualmachines\n\nA payload contains renamed resource:\n\napiVersion: x.virtualization.deckhouse.io/v1\nkind: PrefixedVirtualMachine
proxy -> ctrl : Rewrite admission review, pass\nrequest with restored resource:\n\napiVersion: kubevirt.io/v1\nkind: VirtualMachine

... Validating webhook response ...
ctrl -> proxy : AdmissionReview response
proxy -> kube_api : No rewrite, pass as-is.

... Mutating webhook response ...
ctrl -> proxy : AdmissionReview response\nwith the patch
proxy -> kube_api : Rewrite ownerRef patch if\nresponse.patchType == JSONPatch\nand patch operates on the ownerRef content


@enduml
```

