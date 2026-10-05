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
package vi

import (
	"context"
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus/testutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/common"
	"github.com/deckhouse/virtualization-controller/pkg/monitoring/metrics"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func fqName(metric string) string {
	return metrics.MetricNamespace + "_" + metric
}

type stubIterator struct {
	images []*v1alpha2.VirtualImage
}

func (s stubIterator) Iter(_ context.Context, h handler) error {
	for _, vi := range s.images {
		if stop := h(newDataMetric(vi)); stop {
			return nil
		}
	}
	return nil
}

func collectorOf(images ...*v1alpha2.VirtualImage) Collector {
	return Collector{
		log:      log.NewNop(),
		iterator: stubIterator{images: images},
	}
}

func newVI(name string, phase v1alpha2.ImagePhase) *v1alpha2.VirtualImage {
	return &v1alpha2.VirtualImage{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "team-a", UID: types.UID("uid-" + name)},
		Status:     v1alpha2.VirtualImageStatus{Phase: phase},
	}
}

// imagePhases is every phase the metric must export a series for. A phase missing from the scraper
// zeroes out the image in the phase picture, which is what the table below catches.
var imagePhases = []v1alpha2.ImagePhase{
	v1alpha2.ImagePending,
	v1alpha2.ImageWaitForUserUpload,
	v1alpha2.ImageProvisioning,
	v1alpha2.ImageReady,
	v1alpha2.ImageFailed,
	v1alpha2.ImageTerminating,
	v1alpha2.ImageLost,
	v1alpha2.ImagePVCLost,
}

// phaseSeries renders the status_phase block of one image with the current phase marked with 1.
func phaseSeries(name string, current v1alpha2.ImagePhase) string {
	var b strings.Builder
	b.WriteString("# HELP d8_virtualization_virtualimage_status_phase The virtualimage current phase.\n")
	b.WriteString("# TYPE d8_virtualization_virtualimage_status_phase gauge\n")
	for _, p := range imagePhases {
		fmt.Fprintf(&b, "d8_virtualization_virtualimage_status_phase{name=%q,namespace=\"team-a\",phase=%q,uid=%q} %v\n",
			name, string(p), "uid-"+name, common.BoolFloat64(p == current))
	}
	return b.String()
}

var _ = Describe("Collector", func() {
	// The whole snapshot rather than one metric: an accidental rename or label change fails here.
	It("reports every metric of an image that is ready", func() {
		c := collectorOf(newVI("ubuntu", v1alpha2.ImageReady))

		expected := `
# HELP d8_virtualization_virtualimage_status_phase The virtualimage current phase.
# TYPE d8_virtualization_virtualimage_status_phase gauge
d8_virtualization_virtualimage_status_phase{name="ubuntu",namespace="team-a",phase="Failed",uid="uid-ubuntu"} 0
d8_virtualization_virtualimage_status_phase{name="ubuntu",namespace="team-a",phase="ImageLost",uid="uid-ubuntu"} 0
d8_virtualization_virtualimage_status_phase{name="ubuntu",namespace="team-a",phase="PVCLost",uid="uid-ubuntu"} 0
d8_virtualization_virtualimage_status_phase{name="ubuntu",namespace="team-a",phase="Pending",uid="uid-ubuntu"} 0
d8_virtualization_virtualimage_status_phase{name="ubuntu",namespace="team-a",phase="Provisioning",uid="uid-ubuntu"} 0
d8_virtualization_virtualimage_status_phase{name="ubuntu",namespace="team-a",phase="Ready",uid="uid-ubuntu"} 1
d8_virtualization_virtualimage_status_phase{name="ubuntu",namespace="team-a",phase="Terminating",uid="uid-ubuntu"} 0
d8_virtualization_virtualimage_status_phase{name="ubuntu",namespace="team-a",phase="WaitForUserUpload",uid="uid-ubuntu"} 0
`
		Expect(testutil.CollectAndCompare(c, strings.NewReader(expected))).To(Succeed())
	})

	DescribeTable("marks exactly the current phase with 1",
		func(phase, marked v1alpha2.ImagePhase) {
			c := collectorOf(newVI("image", phase))

			Expect(testutil.CollectAndCompare(c, strings.NewReader(phaseSeries("image", marked)),
				fqName(MetricVirtualImageStatusPhase))).To(Succeed())
		},
		Entry("Pending", v1alpha2.ImagePending, v1alpha2.ImagePending),
		Entry("WaitForUserUpload", v1alpha2.ImageWaitForUserUpload, v1alpha2.ImageWaitForUserUpload),
		Entry("Provisioning", v1alpha2.ImageProvisioning, v1alpha2.ImageProvisioning),
		Entry("Ready", v1alpha2.ImageReady, v1alpha2.ImageReady),
		Entry("Failed", v1alpha2.ImageFailed, v1alpha2.ImageFailed),
		Entry("Terminating", v1alpha2.ImageTerminating, v1alpha2.ImageTerminating),
		Entry("ImageLost", v1alpha2.ImageLost, v1alpha2.ImageLost),
		// The image is namespaced and lives on a PVC, which is why it has one more phase than
		// its cluster-wide counterpart.
		Entry("PVCLost", v1alpha2.ImagePVCLost, v1alpha2.ImagePVCLost),
		// An image the controller has not reached yet carries an empty phase. Reporting it as
		// Pending keeps the image visible: no series at all looks like a broken exporter.
		Entry("an empty phase counts as Pending", v1alpha2.ImagePhase(""), v1alpha2.ImagePending),
	)

	It("keeps the images apart when several are reported", func() {
		c := collectorOf(
			newVI("first", v1alpha2.ImageReady),
			newVI("second", v1alpha2.ImageProvisioning),
		)

		Expect(testutil.CollectAndCount(c, fqName(MetricVirtualImageStatusPhase))).To(Equal(2 * len(imagePhases)))
		Expect(testutil.CollectAndCompare(c, strings.NewReader(
			phaseSeries("first", v1alpha2.ImageReady)+
				strings.Join(strings.Split(phaseSeries("second", v1alpha2.ImageProvisioning), "\n")[2:], "\n"),
		))).To(Succeed())
	})
})
