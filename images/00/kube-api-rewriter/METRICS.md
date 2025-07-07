# Metrics

## Custom metrics

These metrics describe proxy instances performance.

### kube_api_rewriter_client_requests_total

Total number of received client requests.

Type: counter

Labels:
- name - Proxy instance name (kube-api or webhook).
- resource - Kubernetes resource type from url path.
- method - HTTP method of the request.
- watch - Is watch stream requested? (watch=true in the url query).
- decision - proxy decision: pass request Body as-is or rewrite its content.

### kube_api_rewriter_target_responses_total

Total number of responses from the target.

Type: counter

Labels:
- name - Proxy instance name (kube-api or webhook).
- resource - Kubernetes resource type from url path.
- method - HTTP method of the request.
- watch - Is watch stream requested? (watch=true in the url query).
- decision - proxy decision: pass request Body as-is or rewrite its content.
- status - HTTP status of the target response.
- error - 0 if no error, 1 if error occurred.

### kube_api_rewriter_target_response_invalid_json_total

Total target responses with invalid JSON. Can be used to catch accidental Protobuf responses.

Type: counter

Labels:
- name - Proxy instance name (kube-api or webhook).
- resource - Kubernetes resource type from url path.
- method - HTTP method of the request.
- watch - Is watch stream requested? (watch=true in the url query).
- status - HTTP status of the target response.

### kube_api_rewriter_requests_handled_total

Total number of requests handled by the proxy instance.

Type: counter

Labels:
- name - Proxy instance name (kube-api or webhook).
- resource - Kubernetes resource type from url path.
- method - HTTP method of the request.
- watch - Is watch stream requested? (watch=true in the url query).
- decision - proxy decision: `pass` target response Body as-is or `rewrite` its content.
- status - HTTP status of the target response.
- error - 0 if no error, 1 if error occurred.


### kube_api_rewriter_request_handling_duration_seconds

Duration of request handling for non-watching and watch event handling for watch requests

Type: histogram

Buckets: 1, 2, 5 ms, 10, 20, 50 ms, 100, 200, 500 ms, 1, 2, 5 s

Labels:
- name - Proxy instance name (kube-api or webhook).
- resource - Kubernetes resource type from url path.
- method - HTTP method of the request.
- watch - Is watch stream requested? (watch=true in the url query).
- decision - proxy decision: `pass` target response Body as-is or `rewrite` its content.
- status - HTTP status of the target response.

### kube_api_rewriter_rewrites_total

Total rewrites executed by the proxy instance.

Type: counter

Labels:
- name - Proxy instance name (kube-api or webhook).
- resource - Kubernetes resource type from url path.
- method - HTTP method of the request.
- watch - Is watch stream requested? (watch=true in the url query).
- side - What was rewritten: `client` request or `target` response.
- operation - Rewrite operation: restore or rename.
- error - 0 if no error, 1 if error occurred.

### kube_api_rewriter_rewrite_duration_seconds

Duration of rewrite operations.

Type: histogram

Buckets: 1, 2, 5 ms, 10, 20, 50 ms, 100, 200, 500 ms, 1, 2, 5 s

Labels:
- name - Proxy instance name (kube-api or webhook).
- resource - Kubernetes resource type from url path.
- method - HTTP method of the request.
- watch - Is watch stream requested? (watch=true in the url query).
- side - What was rewritten: `client` request or `target` response.
- operation - Rewrite operation: restore or rename.

### kube_api_rewriter_from_client_bytes_total

Total bytes received from the client.

Type: counter

Labels:

- name - Proxy instance name (kube-api or webhook).
- resource - Kubernetes resource type from url path.
- method - HTTP method of the request.
- watch - Is watch stream requested? (watch=true in the url query).
- decision - proxy decision: `pass` client request Body as-is or `rewrite` its content.

### kube_api_rewriter_to_target_bytes_total

Total bytes transferred to the target.

Type: counter

Labels:

- name - Proxy instance name (kube-api or webhook).
- resource - Kubernetes resource type from url path.
- method - HTTP method of the request.
- watch - Is watch stream requested? (watch=true in the url query).
- decision - proxy decision: `pass` client request Body as-is or `rewrite` its content.

### kube_api_rewriter_from_target_bytes_total

Total bytes received from the target.

Type: counter

Labels:

- name - Proxy instance name (kube-api or webhook).
- resource - Kubernetes resource type from url path.
- method - HTTP method of the request.
- watch - Is watch stream requested? (watch=true in the url query).
- decision - proxy decision: `pass` target response Body as-is or `rewrite` its content.

### kube_api_rewriter_to_client_bytes_total

Total bytes transferred back to the client.

Type: counter

Labels:

- name - Proxy instance name (kube-api or webhook).
- resource - Kubernetes resource type from url path.
- method - HTTP method of the request.
- watch - Is watch stream requested? (watch=true in the url query).
- decision - proxy decision: `pass` target response Body as-is or `rewrite` its content.

