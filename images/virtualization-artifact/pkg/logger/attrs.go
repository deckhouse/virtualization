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
	"log/slog"
)

const (
	errAttr        = "err"
	nameAttr       = "name"
	namespaceAttr  = "namespace"
	handlerAttr    = "handler"
	controllerAttr = "controller"
	collectorAttr  = "collector"
	stepAttr       = "step"
)

func SlogErr(err error) slog.Attr {
	return slog.String(errAttr, err.Error())
}

func SlogName(name string) slog.Attr {
	return slog.String(nameAttr, name)
}

func SlogNamespace(namespace string) slog.Attr {
	return slog.String(namespaceAttr, namespace)
}

func SlogHandler(handler string) slog.Attr {
	return slog.String(handlerAttr, handler)
}

func SlogController(controller string) slog.Attr {
	return slog.String(controllerAttr, controller)
}

func SlogCollector(collector string) slog.Attr {
	return slog.String(collectorAttr, collector)
}

func SlogStep(step string) slog.Attr {
	return slog.String(stepAttr, step)
}
