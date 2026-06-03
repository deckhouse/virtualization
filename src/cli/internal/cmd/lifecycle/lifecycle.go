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
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"

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
	Migrate(ctx context.Context, name, namespace, targetNodeName string) (msg string, err error)
}

func NewLifecycle(cmd Command) *Lifecycle {
	return &Lifecycle{
		cmd:  cmd,
		opts: DefaultOptions(),
		migrationOpts: MigrationOpts{
			TargetNodeName: "",
		},
	}
}

// TODO: Refactor this structure because `Lifecycle` is a common object
// and should not process custom flags for each subcommand like `Migrate`.
type Lifecycle struct {
	cmd           Command
	opts          Options
	migrationOpts MigrationOpts

	resultErr error
}

func DefaultOptions() Options {
	return Options{
		Confirm:      os.Getenv(confirmEnv) == "yes",
		Force:        false,
		WaitComplete: false,
		CreateOnly:   false,
		Timeout:      5 * time.Minute,
	}
}

// TODO: If none of the flags are set, the operation output appears as if the `CreateOnly` flag is `true`,
// although in reality, it is `false`. This flag should be refactored.
// Consider changing it to a `silence` flag, which could be useful in scripting.
type Options struct {
	Confirm      bool
	Force        bool
	WaitComplete bool
	CreateOnly   bool
	All          bool
	Selector     map[string]string
	Timeout      time.Duration
}

func (o *Options) validate(args []string) error {
	if len(args) > 0 && o.All {
		return fmt.Errorf("cannot use --all flag with specific keys")
	}
	if len(args) > 0 && len(o.Selector) > 0 {
		return fmt.Errorf("cannot use --label-selector flag with specific keys")
	}
	if o.All && len(o.Selector) > 0 {
		return fmt.Errorf("cannot use --all and --label-selector flags together")
	}

	return nil
}

type MigrationOpts struct {
	TargetNodeName string
}

func (l *Lifecycle) Run(cmd *cobra.Command, args []string) error {
	client, defaultNamespace, _, err := clientconfig.ClientAndNamespaceFromContext(cmd.Context())
	if err != nil {
		return err
	}

	if err = l.opts.validate(args); err != nil {
		return err
	}

	var keys []types.NamespacedName

	switch {
	case l.opts.All:
		keys, err = l.getVirtualMachines(cmd.Context(), defaultNamespace, client, metav1.ListOptions{})
		if err != nil {
			return fmt.Errorf("failed to get virtual machines in namespace %q: %w", defaultNamespace, err)
		}
	case len(l.opts.Selector) > 0:
		selector := labels.SelectorFromSet(l.opts.Selector).String()
		opts := metav1.ListOptions{LabelSelector: selector}

		keys, err = l.getVirtualMachines(cmd.Context(), defaultNamespace, client, opts)
		if err != nil {
			return fmt.Errorf("failed to get virtual machines in namespace %q with selector: %q: %w", defaultNamespace, selector, err)
		}
	default:
		keys, err = l.getNamespacedNames(defaultNamespace, args)
		if err != nil {
			return fmt.Errorf("failed to parse keys: %w", err)
		}
	}

	if len(keys) == 0 {
		return fmt.Errorf("no one virtual machine found for execute command")
	}

	forceSet := cmd.Flags().Changed(forceFlag)
	mgr := l.getManager(client, forceSet, len(keys) > 1)

	ctx, cancel := context.WithTimeout(context.Background(), l.opts.Timeout)
	defer cancel()

	switch l.cmd {
	case Stop:
		for _, key := range keys {
			l.withConfirm(cmd, Stop, key, func() {
				cmd.Printf("Stopping virtual machine %q\n", key.String())
				msg, err := mgr.Stop(ctx, key.Name, key.Namespace)
				l.handleMsgError(cmd, msg, err)
			})
		}
	case Start:
		for _, key := range keys {
			l.withConfirm(cmd, Start, key, func() {
				cmd.Printf("Starting virtual machine %q\n", key.String())
				msg, err := mgr.Start(ctx, key.Name, key.Namespace)
				l.handleMsgError(cmd, msg, err)
			})
		}
	case Restart:
		for _, key := range keys {
			l.withConfirm(cmd, Restart, key, func() {
				cmd.Printf("Restarting virtual machine %q\n", key.String())
				msg, err := mgr.Restart(ctx, key.Name, key.Namespace)
				l.handleMsgError(cmd, msg, err)
			})
		}
	case Evict:
		for _, key := range keys {
			l.withConfirm(cmd, Evict, key, func() {
				cmd.Printf("Evicting virtual machine %q\n", key.String())
				msg, err := mgr.Evict(ctx, key.Name, key.Namespace)
				l.handleMsgError(cmd, msg, err)
			})
		}
	case Migrate:
		for _, key := range keys {
			targetNodeName := l.migrationOpts.TargetNodeName

			if err := l.validateNodeName(cmd, key.Name, targetNodeName); err != nil {
				l.handleMsgError(cmd, "", err)
				continue
			}

			l.withConfirm(cmd, Migrate, key, func() {
				cmd.Printf("Migrating virtual machine %q\n", key.String())
				msg, err := mgr.Migrate(ctx, key.Name, key.Namespace, targetNodeName)
				l.handleMsgError(cmd, msg, err)
			})
		}
	default:
		return fmt.Errorf("invalid command %q", l.cmd)
	}

	return l.resultErr
}

func (l *Lifecycle) withConfirm(cmd *cobra.Command, command Command, key types.NamespacedName, fn func()) {
	if l.opts.Confirm {
		fn()
		return
	}

	cmd.Printf("Are you sure you want to execute command %q for virtual machine %q? [y/N] ", command, key.String())
	reader := bufio.NewReader(cmd.InOrStdin())
	answer, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		cmd.PrintErrf("Error: failed to read confirmation: %s\n", err)
		return
	}

	answer = strings.TrimSpace(strings.ToLower(answer))
	if answer != "y" && answer != "yes" {
		cmd.Printf("Skipping virtual machine %q\n", key.String())
		return
	}

	fn()
}

func (l *Lifecycle) handleMsgError(cmd *cobra.Command, msg string, err error) {
	if msg != "" {
		cmd.Printf("%s\n", msg)
	}
	if err != nil {
		cmd.Printf("Error: %s\n", err.Error())

		if l.resultErr == nil {
			l.resultErr = fmt.Errorf("something went wrong: %w", err)
		}
	}
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
	if l.cmd != Start && l.cmd != Evict && l.cmd != Migrate {
		usage += fmt.Sprintf(`
  # Configure shutdown policy (default: force=%v)
  {{ProgramName}} %s --%s myvm`, opts.Force, l.cmd, forceFlag)
	}
	return usage
}

func (l *Lifecycle) getNamespacedName(defaultNamespace, arg string) (types.NamespacedName, error) {
	namespace, name, err := templates.ParseTarget(arg)
	if err != nil {
		return types.NamespacedName{}, err
	}
	if namespace == "" {
		namespace = defaultNamespace
	}
	return types.NamespacedName{Namespace: namespace, Name: name}, nil
}

func (l *Lifecycle) getNamespacedNames(defaultNamespace string, args []string) ([]types.NamespacedName, error) {
	var keys []types.NamespacedName
	for _, arg := range args {
		key, err := l.getNamespacedName(defaultNamespace, arg)
		if err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, nil
}

func (l *Lifecycle) getVirtualMachines(ctx context.Context, namespace string, client kubeclient.Client, opts metav1.ListOptions) ([]types.NamespacedName, error) {
	vmList, err := client.VirtualMachines(namespace).List(ctx, opts)
	if err != nil {
		return nil, err
	}

	var keys []types.NamespacedName
	for _, vm := range vmList.Items {
		keys = append(keys, types.NamespacedName{Namespace: vm.Namespace, Name: vm.Name})
	}

	return keys, nil
}

func (l *Lifecycle) getManager(client kubeclient.Client, forceSet, severalVms bool) Manager {
	var forcePtr *bool
	if forceSet {
		forcePtr = ptr.To(l.opts.Force)
	}

	return vmop.New(
		client,
		vmop.WithCreateOnly(l.opts.CreateOnly || severalVms),
		vmop.WithWaitComplete(l.opts.WaitComplete),
		vmop.WithForce(forcePtr),
	)
}

func (l *Lifecycle) validateNodeName(cmd *cobra.Command, vmName, targetNodeName string) error {
	if !cmd.Flags().Changed("target-node-name") {
		return nil
	}

	if targetNodeName == "" {
		return errors.New("flag --target-node-name cannot be empty")
	}

	client, namespace, _, err := clientconfig.ClientAndNamespaceFromContext(cmd.Context())
	if err != nil {
		return err
	}

	vm, err := client.VirtualMachines(namespace).Get(context.Background(), vmName, metav1.GetOptions{})
	if err != nil {
		return err
	}
	if targetNodeName == vm.Status.Node {
		return fmt.Errorf("the virtual machine cannot be migrated to the same node: %s", vm.Status.Node)
	}

	_, err = client.CoreV1().Nodes().Get(context.Background(), targetNodeName, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return fmt.Errorf("there is no node with the name %s in the cluster", targetNodeName)
		}
		return fmt.Errorf("failed to validate target node name: %w", err)
	}

	return nil
}

const (
	confirmFlag, confirmFlagShort       = "yes", "y"
	forceFlag, forceFlagShort           = "force", "f"
	waitFlag, waitFlagShort             = "wait", "w"
	createOnlyFlag, createOnlyFlagShort = "create-only", "c"
	allFlag                             = "all"
	selectorFlag, selectorFlagShort     = "label-selector", "l"
	timeoutFlag, timeoutFlagShort       = "timeout", "t"

	confirmEnv = "D8_VIRTUALIZATION_LIFECYCLE_CONFIRM"
)

func AddCommandLineArgs(flagset *pflag.FlagSet, opts *Options) {
	flagset.BoolVarP(&opts.Confirm, confirmFlag, confirmFlagShort, opts.Confirm,
		"Set this flag to confirm the action without prompting for confirmation.")
	flagset.BoolVarP(&opts.Force, forceFlag, forceFlagShort, opts.Force,
		"Set this flag to force the operation.")
	flagset.BoolVarP(&opts.WaitComplete, waitFlag, waitFlagShort, opts.WaitComplete,
		"Set this flag to wait for the operation to complete.")
	flagset.BoolVarP(&opts.CreateOnly, createOnlyFlag, createOnlyFlagShort, opts.CreateOnly,
		"Set this flag to only create the action without status warnings or notifications.")
	flagset.BoolVar(&opts.All, allFlag, opts.All,
		"Set this flag to apply the action to all VMs.")
	flagset.StringToStringVarP(&opts.Selector, selectorFlag, selectorFlagShort, opts.Selector,
		"Set this flag to apply the action to VMs with the specified labels.")
	flagset.DurationVarP(&opts.Timeout, timeoutFlag, timeoutFlagShort, opts.Timeout,
		"Set this flag to change the timeout.")
}

func AddCommandLineMigrationArgs(flagset *pflag.FlagSet, migrationOpts *MigrationOpts) {
	flagset.StringVar(&migrationOpts.TargetNodeName, "target-node-name", "",
		"Set the target node name for virtual machine migration.",
	)
}
