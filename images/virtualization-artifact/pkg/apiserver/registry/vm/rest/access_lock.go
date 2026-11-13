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

package rest

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/client-go/metadata"

	"github.com/deckhouse/virtualization-controller/pkg/common/vmaccess"
	"github.com/deckhouse/virtualization/api/subresources"
	subv1alpha2 "github.com/deckhouse/virtualization/api/subresources/v1alpha2"
)

// AccessLockReader reads the lock the virtual machine access coordinator keeps on the internal virtual
// machine.
type AccessLockReader interface {
	ReadAccessLock(ctx context.Context, namespace, name string) (*vmaccess.Lock, error)
}

// NewAccessLockReader reads the lock through the metadata API, as the lock is an annotation.
func NewAccessLockReader(client metadata.Interface) AccessLockReader {
	return metadataAccessLockReader{client: client}
}

type metadataAccessLockReader struct {
	client metadata.Interface
}

func (r metadataAccessLockReader) ReadAccessLock(ctx context.Context, namespace, name string) (*vmaccess.Lock, error) {
	obj, err := r.client.Resource(vmaccess.InternalVirtualMachineGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		// A virtual machine without the internal one has no lock.
		if apierrors.IsNotFound(err) {
			return nil, nil
		}

		return nil, err
	}

	return vmaccess.ReadLock(obj)
}

// checkAccessLock rejects a freeze or unfreeze call that does not come from the claim holding the virtual
// machine, so nothing changes the guest filesystem state under a coordinator session. A malformed lock
// rejects every call until the coordinator drops it. The lock is read uncached, so a lock just taken is seen.
func (r *BaseREST) checkAccessLock(ctx context.Context, name, claimUID string) error {
	namespace, _ := request.NamespaceFrom(ctx)

	lock, err := r.accessLocks.ReadAccessLock(ctx, namespace, name)
	if errors.Is(err, vmaccess.ErrMalformedLock) {
		return apierrors.NewConflict(subresources.Resource("virtualmachines"), name, err)
	}

	if err != nil {
		return apierrors.NewInternalError(fmt.Errorf("read the access lock of the virtual machine %q: %w", name, err))
	}

	if err := vmaccess.CheckCall(lock, claimUID); err != nil {
		return apierrors.NewConflict(subresources.Resource("virtualmachines"), name, err)
	}

	return nil
}

// dropClaimUIDParam removes the claim UID from the request forwarded to KubeVirt, which has no use for it.
func dropClaimUIDParam(req *http.Request) error {
	query := req.URL.Query()
	query.Del(subv1alpha2.ClaimUIDParam)
	req.URL.RawQuery = query.Encode()

	return nil
}
