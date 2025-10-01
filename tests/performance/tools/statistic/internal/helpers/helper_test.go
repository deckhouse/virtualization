/*
Copyright 2025 Flant JSC

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

package helpers

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDurationToString(t *testing.T) {
	tests := []struct {
		name string
		d    *metav1.Duration
		want string
	}{
		{
			name: "nil duration",
			d:    nil,
			want: "",
		},
		{
			name: "zero duration",
			d:    &metav1.Duration{},
			want: "0:0:0",
		},
		{
			name: "one hour",
			d:    &metav1.Duration{Duration: time.Hour},
			want: "1:0:0",
		},
		{
			name: "24 hours",
			d:    &metav1.Duration{Duration: time.Hour * 24},
			want: "24:0:0",
		},
		{
			name: "one minute",
			d:    &metav1.Duration{Duration: time.Minute},
			want: "0:1:0",
		},
		{
			name: "one second",
			d:    &metav1.Duration{Duration: time.Second},
			want: "0:0:1",
		},
		{
			name: "complex duration - 2 hours 30 minutes 45 seconds",
			d:    &metav1.Duration{Duration: 2*time.Hour + 30*time.Minute + 45*time.Second},
			want: "2:30:45",
		},
		{
			name: "complex duration - 1 hour 59 minutes 59 seconds",
			d:    &metav1.Duration{Duration: time.Hour + 59*time.Minute + 59*time.Second},
			want: "1:59:59",
		},
		{
			name: "complex duration - 0 hours 0 minutes 30 seconds",
			d:    &metav1.Duration{Duration: 30 * time.Second},
			want: "0:0:30",
		},
		{
			name: "complex duration - 0 hours 5 minutes 0 seconds",
			d:    &metav1.Duration{Duration: 5 * time.Minute},
			want: "0:5:0",
		},
		{
			name: "large duration - 100 hours 30 minutes 15 seconds",
			d:    &metav1.Duration{Duration: 100*time.Hour + 30*time.Minute + 15*time.Second},
			want: "100:30:15",
		},
		{
			name: "microseconds precision - should round down",
			d:    &metav1.Duration{Duration: time.Hour + 30*time.Minute + 45*time.Second + 500*time.Millisecond},
			want: "1:30:45",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DurationToString(tt.d)
			assert.Equal(t, tt.want, result)
		})
	}
}
