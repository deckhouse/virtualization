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

package lifecycle

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/pflag"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/deckhouse/virtualization/api/client/kubeclient"

	"github.com/deckhouse/virtualization/src/pkg/cli/cmd/lifecycle/vmop"
	"github.com/deckhouse/virtualization/src/pkg/cli/templates"
)

type Command string

const (
	Stop    Command = "stop"
	Start   Command = "start"
	Restart Command = "restart"
	Evict   Command = "evict"
)

type Manager interface {
	Stop(ctx context.Context, name, namespace string) (msg string, err error)
	Start(ctx context.Context, name, namespace string) (msg string, err error)
	Restart(ctx context.Context, name, namespace string) (msg string, err error)
	Evict(ctx context.Context, name, namespace string) (msg string, err error)
}

func NewLifecycle(cmd Command, clientConfig clientcmd.ClientConfig) *Lifecycle {
	return &Lifecycle{
		cmd:          cmd,
		clientConfig: clientConfig,
		opts:         DefaultOptions(),
	}
}

type Lifecycle struct {
	cmd          Command
	clientConfig clientcmd.ClientConfig
	opts         Options
}

func DefaultOptions() Options {
	return Options{
		Force:        false,
		WaitComplete: false,
		CreateOnly:   false,
		Timeout:      5 * time.Minute,
	}
}

type Options struct {
	Force        bool
	WaitComplete bool
	CreateOnly   bool
	Timeout      time.Duration
}

func (l *Lifecycle) Run(args []string) error {
	name, namespace, err := l.getNameNamespace(args)
	key := types.NamespacedName{Namespace: namespace, Name: name}
	if err != nil {
		return err
	}
	mgr, err := l.getManager()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), l.opts.Timeout)
	defer cancel()
	writer := os.Stdout
	var msg string
	switch l.cmd {
	case Stop:
		fmt.Fprintf(writer, "Stopping virtual machine %q\n", key.String())
		msg, err = mgr.Stop(ctx, name, namespace)
	case Start:
		fmt.Fprintf(writer, "Starting virtual machine %q\n", key.String())
		msg, err = mgr.Start(ctx, name, namespace)
	case Restart:
		fmt.Fprintf(writer, "Restarting virtual machine %q\n", key.String())
		msg, err = mgr.Restart(ctx, name, namespace)
	case Evict:
		fmt.Fprintf(writer, "Evicting virtual machine %q\n", key.String())
		msg, err = mgr.Evict(ctx, name, namespace)
	default:
		return fmt.Errorf("invalid command %q", l.cmd)
	}
	if msg != "" {
		fmt.Fprint(os.Stdout, msg)
	}
	return err
}

func (l *Lifecycle) Usage() string {
	opts := DefaultOptions()
	usage := fmt.Sprintf(` # %s VirtualMachine 'myvm':`, cases.Title(language.English).String(string(l.cmd)))
	usage += strings.Replace(fmt.Sprintf(`
  {{ProgramName}} {{operation}} myvm
  {{ProgramName}} {{operation}} myvm.mynamespace
  {{ProgramName}} {{operation}} myvm -n mynamespace
  # Configure one minute timeout (default: timeout=%v)
  {{ProgramName}} {{operation}} --%s=1m myvm
  # Configure wait vm phase (default: wait=%v)
  {{ProgramName}} {{operation}} --%s myvm`, opts.Timeout, timeoutFlag, opts.WaitComplete, waitFlag), "{{operation}}", string(l.cmd), -1)
	if l.cmd != Start && l.cmd != Evict {
		usage += fmt.Sprintf(`
  # Configure shutdown policy (default: force=%v)
  {{ProgramName}} %s --%s myvm`, opts.Force, l.cmd, forceFlag)
	}
	return usage
}

func (l *Lifecycle) getNameNamespace(args []string) (string, string, error) {
	namespace, name, err := templates.ParseTarget(args[0])
	if err != nil {
		return "", "", err
	}
	if namespace == "" {
		namespace, _, err = l.clientConfig.Namespace()
		if err != nil {
			return "", "", err
		}
	}
	return name, namespace, nil
}

func (l *Lifecycle) getManager() (Manager, error) {
	virtCli, err := kubeclient.GetClientFromClientConfig(l.clientConfig)
	if err != nil {
		return nil, err
	}

	return vmop.New(
		virtCli,
		vmop.WithCreateOnly(l.opts.CreateOnly),
		vmop.WithWaitComplete(l.opts.WaitComplete),
		vmop.WithForce(l.opts.Force),
	), nil
}

const (
	forceFlag, forceFlagShort           = "force", "f"
	waitFlag, waitFlagShort             = "wait", "w"
	createOnlyFlag, createOnlyFlagShort = "create-only", "c"
	timeoutFlag, timeoutFlagShort       = "timeout", "t"
)

func AddCommandlineArgs(flagset *pflag.FlagSet, opts *Options) {
	flagset.BoolVarP(&opts.Force, forceFlag, forceFlagShort, opts.Force,
		fmt.Sprintf("--%s, -%s: Set this flag to force the operation.", forceFlag, forceFlagShort))
	flagset.BoolVarP(&opts.WaitComplete, waitFlag, waitFlagShort, opts.WaitComplete,
		fmt.Sprintf("--%s, -%s: Set this flag to wait for the operation to complete.", waitFlag, waitFlagShort))
	flagset.BoolVarP(&opts.CreateOnly, createOnlyFlag, createOnlyFlagShort, opts.CreateOnly,
		fmt.Sprintf("--%s, -%s: Set this flag for create operation only.", createOnlyFlag, createOnlyFlagShort))
	flagset.DurationVarP(&opts.Timeout, timeoutFlag, timeoutFlagShort, opts.Timeout,
		fmt.Sprintf("--%s, -%s: Set this flag to change the timeout.", timeoutFlag, timeoutFlagShort))
}
