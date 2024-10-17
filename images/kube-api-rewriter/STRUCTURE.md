# kube-api-rewriter structure

The idea of the rewriter proxy is simple: make controller connect to the local 
proxy in the sidecar, so proxy will pass requests to real Kubernetes API Server.
Proxy may rewrite JSON payloads for different purposes, e.g. resources renaming.

Kube-api-rewriter contains 2 proxy instances:
- "api" proxy to handle usual API requests from the proxied controller to the Kubernetes API Server.
- "webhook" proxy to handle webhook requests from the Kubernetes API Server to the proxied controller.


Example setup: rename resources for Kubevirt.
```mermaid
%%{init: {"flowchart": {"htmlLabels": false}} }%%
flowchart TB
    NoProxy-.->WithProxy

    subgraph NoProxy ["`**Original Kubevirt setup**`"]
        direction TB

        subgraph np-virt-operator-deploy ["`Deploy/virt-operator`"]
            np-virt-operator("`container
            name: virt-operator`")
        end
        
        subgraph np-virt-controller-deploy ["`Deploy/virt-controller`"]
            np-virt-controller("`container
            name: virt-controller`")
        end
        
        np-kube-api["`Kubernetes API Server
        with resources in apiGroup
        *.kubevirt.io*`"]

        np-virt-operator <-- "Original resources
        in API calls" --> np-kube-api
        np-virt-controller <-- "Original resources
        in API calls" --> np-kube-api
    end
    subgraph WithProxy ["`**Kubevirt with proxy**`"]
        direction TB
        
        subgraph p-virt-operator-deploy ["`Deploy/virt-operator`"]
            p-virt-operator("`container
            name: virt-operator`")
            p-virt-operator-proxy{{"container
            name: proxy"}}
            p-virt-operator -- "Original resources
            in API calls" --> p-virt-operator-proxy
            p-virt-operator-proxy -- "Restored resources
            in API responses" --> p-virt-operator
        end
        
        subgraph p-virt-controller-deploy ["`Deploy/virt-controller`"]
            p-virt-controller("`container
            name: virt-controller`")
            p-virt-controller-proxy{{"container
            name: proxy"}}
            p-virt-controller -- "Original resources
in API calls" --> p-virt-controller-proxy
            p-virt-controller-proxy -- "Restored resources
            in API responses" --> p-virt-controller
        end
        
        p-kube-api["`Kubernetes API Server
        with resources in apiGroup
        *.x.virtualization.deckhouse.io*`"]

        p-virt-operator-proxy <-- "Renamed resources in
        API calls" --> p-kube-api
        p-virt-controller-proxy <-- "Renamed resources in
        API calls" --> p-kube-api
    end
```

All DVP components:
```mermaid
%%{init: {"flowchart": {"htmlLabels": false}} }%%
flowchart
    subgraph kubevirt ["Kubevirt"]
  subgraph virt-operator-deploy ["`Deploy/virt-operator`"]
      virt-operator("`container:
        virt-operator`")
      virt-operator-proxy{{"container:
        proxy"}}
      virt-operator --> virt-operator-proxy
      virt-operator-proxy --> virt-operator
  end
  
  subgraph p-virt-controller-deploy ["`Deploy/virt-controller`"]
      virt-controller("`container:
        virt-controller`")
      virt-controller-proxy{{"container:
        proxy"}}
      virt-controller --> virt-controller-proxy
      virt-controller-proxy --> virt-controller
  end
  subgraph p-virt-api-deploy ["`Deploy/virt-api`"]
      virt-api("`container:
        virt-api`")
      virt-api-proxy{{"container:
        proxy"}}
      virt-api --> virt-api-proxy
      virt-api-proxy --> virt-api
  end
  
  subgraph p-virt-handler-deploy ["`DaemonSet/virt-handler`"]
      virt-handler("`container:
        virt-handler`")
      virt-handler-proxy{{"container:
        proxy"}}
      virt-handler --> virt-handler-proxy
      virt-handler-proxy --> virt-handler
  end
  end
  
  subgraph kubeapi ["control-plane"]
    kube-api["`Kubernetes API Server`"]
  end

  virt-operator-proxy <----> kube-api
  virt-controller-proxy <----> kube-api
  virt-api-proxy <----> kube-api
  virt-handler-proxy <----> kube-api

  subgraph cdi ["CDI"]
    subgraph cdi-operator-deploy ["`Deploy/cdi-operator`"]
      cdi-operator-proxy{{"container:
        proxy"}}
      cdi-operator("`container:
        virt-handler`")
      cdi-operator --> cdi-operator-proxy
      cdi-operator-proxy --> cdi-operator
    end
  
    subgraph cdi-deployment-deploy ["`Deploy/cdi-deployment`"]
      cdi-deployment-proxy{{"container:
        proxy"}}
      cdi-deployment("`container:
        cdi-eployment`")
      cdi-deployment --> cdi-deployment-proxy
      cdi-deployment-proxy --> cdi-deployment
    end
  
    subgraph cdi-api-deploy ["`Deploy/cdi-api`"]
      cdi-api-proxy{{"container:
        proxy"}}
      cdi-api("`container:
        cdi-api`")
      cdi-api --> cdi-api-proxy
      cdi-api-proxy --> cdi-api
    end
  
    subgraph cdi-exportproxy-deploy ["`Deploy/cdi-exportproxy`"]
      cdi-exportproxy-proxy{{"container:
          proxy"}}
      cdi-exportproxy("`container:
          cdi-exportproxy`")
      cdi-exportproxy --> cdi-exportproxy-proxy
      cdi-exportproxy-proxy --> cdi-exportproxy
    end
  end
  kube-api <----> cdi-operator-proxy
  kube-api <----> cdi-deployment-proxy
  kube-api <----> cdi-api-proxy
  kube-api <----> cdi-exportproxy-proxy
  

  subgraph d8virt ["D8 API"]
    subgraph d8-virt-deploy ["Deploy/virtualization-controller"]
        d8-virt-controller-proxy("`container:
            proxy`")
        d8-virt-controller("`container: 
            virtualization-controller`")
        d8-virt-controller --> d8-virt-controller-proxy
        d8-virt-controller-proxy --> d8-virt-controller
    end
  end
    
  kube-api <----> d8-virt-controller-proxy
```

Variation (block diagram seems not so powerful as flowchart)
```mermaid
block-beta
    columns 5
    
    %% Main containers in kubevirt Pods
    virtoperator["virt-operator"]
    virtapi["virt-api"]
    virtcontroller["virt-controller"]
    virthandler["virt-handler"]
    virtexportproxy["virt-exportproxy"]

    %% Space for links.
    space:5
    %% Links between containers.
    virtoperator --> virtoperatorproxy
    %%virtoperatorproxy --> virtoperator
    virtapi --> virtapiproxy
    virtcontroller --> virtcontrollerproxy
    virthandler --> virthandlerproxy
    virtexportproxy --> virtexportproxyproxy
    
    %% Proxies in kubevirt Pods.
    virtoperatorproxy(["proxy"])
    virtapiproxy(["proxy"])
    virtcontrollerproxy(["proxy"])
    virthandlerproxy(["proxy"])
    virtexportproxyproxy(["proxy"])

    space:5

    space
    kubeapiserver{{"Kubernetes API Server"}}:3
    space

    virtoperatorproxy --> kubeapiserver
    %%kubeapiserver --> virtoperatorproxy
    virtapiproxy --> kubeapiserver
    virtcontrollerproxy --> kubeapiserver
    virthandlerproxy --> kubeapiserver
    virtexportproxyproxy --> kubeapiserver

    space:5
    cdioperatorproxy --> kubeapiserver
    cdiapiproxy --> kubeapiserver
    cdideploymentproxy --> kubeapiserver
    cdiuploadproxyproxy --> kubeapiserver
    virtualizationcontrollerproxy --> kubeapiserver

    %% Proxies in CDI Pods.
    cdioperatorproxy(["proxy"])
    cdiapiproxy(["proxy"])
    cdideploymentproxy(["proxy"])
    cdiuploadproxyproxy(["proxy"])
    virtualizationcontrollerproxy(["proxy"])

    %% Links inside CDI Pods.
    space:5
    cdioperator --> cdioperatorproxy
    cdiapi--> cdiapiproxy
    cdideployment --> cdideploymentproxy
    cdiuploadproxy --> cdiuploadproxyproxy
    virtualizationcontroller --> virtualizationcontrollerproxy
    
    cdioperator["cdi-operator"]
    cdiapi["cdi-api"]
    cdideployment["cdi-deployment"]
    cdiuploadproxy["cdi-uploadproxy"]
    virtualizationcontroller["virtualization-
    controller"]
```

### Changes to add proxy to the Pod
- Add a ConfigMap with a simple kubeconfig points to the local proxy.
    ```
    ...
    clusters:
    - cluster:
        server: http://127.0.0.1:23915
    ...
    ```
- Add a volume and a volumeMount to pass new kubeconfig as file to the main container.
- Set KUBECONFIG variable in the main container. File should contain configuration to connect to proxy port.
  - Note: kubevirt containers use --kubeconfig flag, cdi containers use KUBECONFIG env variable.
- Add a new sidecar container with the proxy.
  - Set WEBHOOK_ADDRESS if webhook proxying is required.
  - Add volumeMount with a certificate and set WEBHOOK_CERT_FILE and WEBHOOK_KEY_FILE to use the certificate.
  - Add port 24192 to the webhook Service to use the certificate without issuing new one with changed ServerName.

## API client proxying

Implemented rewrites:
- apiGroup, kind, metadata.ownerReferences for Kubevirt and CDI Custom Resources.
- metadata.ownerReferences for Pod
- rules for Role, ClusterRole
- webhooks[].rules for ValidatingWebhookConfiguration, MutatingWebhookConfiguration
- metadata.name, spec.group, spec.names for CustomResourceDefinition.
- patch /spec for CustomResourceDefinition.
- fieldSelector=metadata.name=&watch=true for CRD.
- request.resource, request.object, request.kind, etc. for AdmissionReview.

TODO:
- labels and annotations for Kubevirt and CDI CRs and all kubevirt related resources, Nodes and Pods.
- patches in general.
- SubjectAccessReview https://dev-k8sref-io.web.app/docs/authorization/subjectaccessreview-v1/

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

```mermaid
---
config:
  htmlLabels: false
---

sequenceDiagram

    box Pod with controller
    participant ctrl as container<br>name: controller
    participant proxy as container<br>name: proxy
    end

    Note over ctrl: Listen on 0.0.0.0:9443
    Note over proxy: Listen on 0.0.0.0:24192<br>and pass requests to<br>127.0.0.1:9443
    
    box Control plane
    participant kubeapi as Kubernetes<br>API Server
    end
    note over kubeapi: Request webhook with AdmissionReview

    kubeapi --> ctrl: Webhook handling
    
    kubeapi ->>+ proxy: Send AdmissionReview with<br>renamed resources<br>apiVersion: x.virtualization.deckhouse.io<br>PrefixedVirtualMachine
    
    proxy ->>+ ctrl: Proxy restores resource:<br>apiGroup, kind, ownerReferences<br>apiVersion: kubevirt.io<br>kind: VirtualMachine
    
    ctrl ->>- proxy:  AdmissionReview<br>with webhook response

    alt Validating webhook response
    proxy ->> kubeapi: No rewrite, pass as-is
    else Mutating webhook response
    proxy ->>- kubeapi: Rewrite patch if<br>ownerReferences is modified
    end
    
    

    %%participant Bob
    %%  ctrl->>John: "`This **is** _Markdown_`"
    %%loop HealthCheck
    %%    John->>John: Fight against hypochondria
    %%end
    %%Note right of John: Rational thoughts <br/>prevail!
    %%John-->>ctrl: Great!
    %%John->>Bob: How about you?
    %%Bob-->>John: Jolly good!
```
