/*
Copyright 2024 Flant JSC

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

package logger

import (
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/deckhouse/pkg/log"
)

func NewConstructor(log *log.Logger) func(req *reconcile.Request) logr.Logger {
	return func(req *reconcile.Request) logr.Logger {
		log := log
		if req != nil {
			log = log.With(SlogNamespace(req.Namespace), SlogName(req.Name))
		}

		return logr.FromSlogHandler(log.Handler())
	}
}
