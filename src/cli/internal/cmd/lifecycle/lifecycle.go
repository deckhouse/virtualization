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
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"

	"github.com/deckhouse/virtualization/api/client/kubeclient"
	"github.com/deckhouse/virtualization/src/cli/internal/clientconfig"
	"github.com/deckhouse/virtualization/src/cli/internal/cmd/lifecycle/vmop"
	"github.com/deckhouse/virtualization/src/cli/internal/templates"
)

type Command string

const (
	Stop    Command = "stop"
	Start   Command = "start"
	Restart Command = "restart"
	Evict   Command = "evict"
	Migrate Command = "migrate"
)

type Manager interface {
	Stop(ctx context.Context, name, namespace string) (msg string, err error)
	Start(ctx context.Context, name, namespace string) (msg string, err error)
	Restart(ctx context.Context, name, namespace string) (msg string, err error)
	Evict(ctx context.Context, name, namespace string) (msg string, err error)
	Migrate(ctx context.Context, name, namespace string, nodeSelector map[string]string) (msg string, err error)
}

func NewLifecycle(cmd Command) *Lifecycle {
	return &Lifecycle{
		cmd:  cmd,
		opts: DefaultOptions(),
		migrationOpts: MigrationOpts{
			NodeSelector: "",
		},
	}
}

type Lifecycle struct {
	cmd           Command
	opts          Options
	migrationOpts MigrationOpts
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

type MigrationOpts struct {
	NodeSelector string
}

func (l *Lifecycle) Run(cmd *cobra.Command, args []string) error {
	client, defaultNamespace, _, err := clientconfig.ClientAndNamespaceFromContext(cmd.Context())
	if err != nil {
		return err
	}
	name, namespace, err := l.getNameNamespace(defaultNamespace, args)
	key := types.NamespacedName{Namespace: namespace, Name: name}
	if err != nil {
		return err
	}
	mgr := l.getManager(client)

	ctx, cancel := context.WithTimeout(context.Background(), l.opts.Timeout)
	defer cancel()
	var msg string
	switch l.cmd {
	case Stop:
		cmd.Printf("Stopping virtual machine %q\n", key.String())
		msg, err = mgr.Stop(ctx, name, namespace)
	case Start:
		cmd.Printf("Starting virtual machine %q\n", key.String())
		msg, err = mgr.Start(ctx, name, namespace)
	case Restart:
		cmd.Printf("Restarting virtual machine %q\n", key.String())
		msg, err = mgr.Restart(ctx, name, namespace)
	case Evict:
		cmd.Printf("Evicting virtual machine %q\n", key.String())
		msg, err = mgr.Evict(ctx, name, namespace)
	case Migrate:
		cmd.Printf("Migrating virtual machine %q\n", key.String())
		msg, err = mgr.Migrate(ctx, name, namespace, l.NodeSelector())
	default:
		return fmt.Errorf("invalid command %q", l.cmd)
	}
	if msg != "" {
		cmd.Printf("%s", msg)
	}
	return err
}

func (l *Lifecycle) Usage() string {
	opts := DefaultOptions()
	usage := fmt.Sprintf(` # %s VirtualMachine 'myvm':`, cases.Title(language.English).String(string(l.cmd)))
	usage += strings.ReplaceAll(fmt.Sprintf(`
  {{ProgramName}} {{operation}} myvm
  {{ProgramName}} {{operation}} myvm.mynamespace
  {{ProgramName}} {{operation}} myvm -n mynamespace
  # Configure one minute timeout (default: timeout=%v)
  {{ProgramName}} {{operation}} --%s=1m myvm
  # Configure wait vm phase (default: wait=%v)
  {{ProgramName}} {{operation}} --%s myvm`, opts.Timeout, timeoutFlag, opts.WaitComplete, waitFlag), "{{operation}}", string(l.cmd))
	if l.cmd != Start && l.cmd != Evict {
		usage += fmt.Sprintf(`
  # Configure shutdown policy (default: force=%v)
  {{ProgramName}} %s --%s myvm`, opts.Force, l.cmd, forceFlag)
	}
	return usage
}

func (l *Lifecycle) getNameNamespace(defaultNamespace string, args []string) (string, string, error) {
	namespace, name, err := templates.ParseTarget(args[0])
	if err != nil {
		return "", "", err
	}
	if namespace == "" {
		namespace = defaultNamespace
	}
	return name, namespace, nil
}

func (l *Lifecycle) getManager(client kubeclient.Client) Manager {
	return vmop.New(
		client,
		vmop.WithCreateOnly(l.opts.CreateOnly),
		vmop.WithWaitComplete(l.opts.WaitComplete),
		vmop.WithForce(l.opts.Force),
	)
}

func (l *Lifecycle) NodeSelector() map[string]string {
	var nodeSelector map[string]string

	if l.migrationOpts.NodeSelector != "" {
		nodeSelector = make(map[string]string)
		selectors := strings.SplitSeq(l.migrationOpts.NodeSelector, ",")
		for selector := range selectors {
			parts := strings.SplitN(selector, "=", 2)
			nodeSelector[parts[0]] = parts[1]
		}
	}

	return nodeSelector
}

func (l *Lifecycle) ValidateNodeSelector(nodeSelector string) error {
	if nodeSelector != "" {
		selectors := strings.SplitSeq(nodeSelector, ",")
		for selector := range selectors {
			parts := strings.SplitN(selector, "=", 2)
			if len(parts) != 2 {
				return fmt.Errorf("invalid node-selector format, expected key=value")
			}
			key, value := parts[0], parts[1]

			if errs := validation.IsQualifiedName(key); len(errs) != 0 {
				return fmt.Errorf("invalid label key: %v", errs)
			}

			if errs := validation.IsValidLabelValue(value); len(errs) != 0 {
				return fmt.Errorf("invalid label value: %v", errs)
			}
		}
	}

	return nil
}

const (
	forceFlag, forceFlagShort           = "force", "f"
	waitFlag, waitFlagShort             = "wait", "w"
	createOnlyFlag, createOnlyFlagShort = "create-only", "c"
	timeoutFlag, timeoutFlagShort       = "timeout", "t"
)

func AddCommandLineArgs(flagset *pflag.FlagSet, opts *Options) {
	flagset.BoolVarP(&opts.Force, forceFlag, forceFlagShort, opts.Force,
		fmt.Sprintf("--%s, -%s: Set this flag to force the operation.", forceFlag, forceFlagShort))
	flagset.BoolVarP(&opts.WaitComplete, waitFlag, waitFlagShort, opts.WaitComplete,
		fmt.Sprintf("--%s, -%s: Set this flag to wait for the operation to complete.", waitFlag, waitFlagShort))
	flagset.BoolVarP(&opts.CreateOnly, createOnlyFlag, createOnlyFlagShort, opts.CreateOnly,
		fmt.Sprintf("--%s, -%s: Set this flag for create operation only.", createOnlyFlag, createOnlyFlagShort))
	flagset.DurationVarP(&opts.Timeout, timeoutFlag, timeoutFlagShort, opts.Timeout,
		fmt.Sprintf("--%s, -%s: Set this flag to change the timeout.", timeoutFlag, timeoutFlagShort))
}

func AddCommandLineMigrationArgs(flagset *pflag.FlagSet, migrationOpts *MigrationOpts) {
	flagset.StringVar(
		&migrationOpts.NodeSelector,
		"node-selector",
		"",
		"Node selector in key=value format, multiple selectors can be separated by commas.",
	)
}
