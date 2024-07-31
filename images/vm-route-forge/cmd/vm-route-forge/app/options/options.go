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

package options

import (
	"os"
	"strconv"

	"github.com/spf13/pflag"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"vm-route-forge/internal/netutil"
)

type Options struct {
	ZapOptions zap.Options

	Verbosity int
	Cidrs     netutil.CIDRSet
	DryRun    bool
	ProbeAddr string
	NodeName  string
	TableID   string
}

const (
	flagCidr, flagCidrShort           = "cidr", "c"
	flagDryRun, flagDryRunShort       = "dry-run", "d"
	flagProbeAddr                     = "health-probe-bind-address"
	flagVerbosity, flagVerbosityShort = "verbosity", "v"
	flagNodeName, flagNodeNameShort   = "nodeName", "n"
	flagTableId, flagTableIdShort     = "tableId", "t"
	defaultVerbosity                  = 1

	VerbosityEnv = "VERBOSITY"
	NodeNameEnv  = "NODE_NAME"
	TableIDEnv   = "TABLE_ID"
)

func NewOptions() Options {
	return Options{
		ZapOptions: zap.Options{
			Development: true,
		},
	}
}

func (o *Options) Flags(fs *pflag.FlagSet) {
	fs.StringSliceVarP((*[]string)(&o.Cidrs), flagCidr, flagCidrShort, []string{}, "CIDRs enabled to route (multiple flags allowed)")
	fs.BoolVarP(&o.DryRun, flagDryRun, flagDryRunShort, false, "Don't perform any changes on the node.")
	fs.StringVar(&o.ProbeAddr, flagProbeAddr, ":0", "The address the probe endpoint binds to.")
	fs.StringVarP(&o.NodeName, flagNodeName, flagNodeNameShort, os.Getenv(NodeNameEnv), "The name of the node.")
	fs.StringVarP(&o.TableID, flagTableId, flagTableIdShort, os.Getenv(TableIDEnv), "The id of the table.")
	fs.IntVarP(&o.Verbosity, flagVerbosity, flagVerbosityShort, getDefaultVerbosity(), "Verbosity of output")
}

func getDefaultVerbosity() int {
	if v, ok := os.LookupEnv(VerbosityEnv); ok {
		verbosity, err := strconv.Atoi(v)
		if err != nil {
			return defaultVerbosity
		}
		return verbosity
	}
	return defaultVerbosity
}
