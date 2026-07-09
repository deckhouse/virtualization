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

package networkpolicy

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
)

func TestPVCImporterIngressPeers(t *testing.T) {
	t.Run("allows CDI pods and the controller namespace", func(t *testing.T) {
		peers := PVCImporterIngressPeers("d8-virtualization")
		if len(peers) != 2 {
			t.Fatalf("expected 2 peers, got %d", len(peers))
		}
		cdiExpr := peers[0].PodSelector.MatchExpressions[0]
		if cdiExpr.Key != annotations.AppLabel ||
			cdiExpr.Operator != metav1.LabelSelectorOpIn ||
			len(cdiExpr.Values) != 1 || cdiExpr.Values[0] != annotations.CDILabelValue {
			t.Fatalf("unexpected CDI peer selector: %#v", cdiExpr)
		}
		wantNS := map[string]string{corev1.LabelMetadataName: "d8-virtualization"}
		if got := peers[1].NamespaceSelector.MatchLabels; got == nil || got[corev1.LabelMetadataName] != wantNS[corev1.LabelMetadataName] {
			t.Fatalf("unexpected controller namespace peer: %#v", got)
		}
	})

	t.Run("allows only CDI pods when controller namespace is empty", func(t *testing.T) {
		if len(PVCImporterIngressPeers("")) != 1 {
			t.Fatal("expected a single CDI peer when controller namespace is empty")
		}
	})
}
