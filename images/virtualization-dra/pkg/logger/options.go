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

package logger

import (
	"flag"

	"github.com/spf13/pflag"
	"k8s.io/klog/v2"

	"github.com/deckhouse/deckhouse/pkg/log"
)

type Options struct {
	Level          string
	Output         string
	DebugVerbosity int
}

func (o *Options) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&o.Level, "log-level", o.Level, "Log level")
	fs.StringVar(&o.Output, "log-output", o.Output, "Log output")
	fs.IntVar(&o.DebugVerbosity, "log-debug-verbosity", o.DebugVerbosity, "Log debug verbosity")

	var klogFlags flag.FlagSet
	klog.InitFlags(&klogFlags)
	fs.AddGoFlagSet(&klogFlags)
}

func (o *Options) Complete() *log.Logger {
	return NewLogger(o.Level, o.Output, o.DebugVerbosity)
}
