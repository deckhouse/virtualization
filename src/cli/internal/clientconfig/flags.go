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

package clientconfig

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// forwardedFlags are the client-config flags propagated to a nested invocation of the CLI so it
// connects to the same cluster as the outer command. The list is deliberately limited to
// cluster-selection flags: sensitive flags (--token, --password, --as*) are excluded so they do not
// leak into a local process argv or into a generated inventory file.
//
// --namespace is not forwarded either: callers carry the namespace in the name.namespace target.
var forwardedFlags = []string{"context", "server", "kubeconfig"}

// ForwardedFlags returns the `--name=value` tokens for the cluster-selection flags the user set on
// cmd. Each value goes through quote, so the caller picks the escaping its embedding needs.
func ForwardedFlags(cmd *cobra.Command, quote func(string) string) []string {
	var out []string
	for _, name := range forwardedFlags {
		f := cmd.Flag(name)
		if f == nil || !f.Changed {
			continue
		}
		out = append(out, fmt.Sprintf("--%s=%s", name, quote(f.Value.String())))
	}
	return out
}

// ShellQuote single-quotes s so it survives being re-parsed by /bin/sh, which runs the ProxyCommand.
func ShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// shellSafeChars need no escaping in a /bin/sh word.
const shellSafeChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-./:=@,+"

// ShellEscape backslash-escapes every character /bin/sh would otherwise interpret. Use it instead of
// ShellQuote where single quotes are already taken by an enclosing layer, as in the
// ansible_ssh_common_args value.
//
// That value is re-parsed twice: ansible splits it with shlex, then ssh hands the ProxyCommand to
// /bin/sh. shlex keeps a backslash literal inside single quotes, which is what makes the plain
// escaping work for every other character. A single quote is the exception: it closes the quoted
// run no matter what precedes it, so it has to leave the run, arrive at /bin/sh as \' and reopen
// the run afterwards.
func ShellEscape(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r == '\'' {
			b.WriteString(`\'\''`)
			continue
		}
		if r < 0x80 && !strings.ContainsRune(shellSafeChars, r) {
			b.WriteRune('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}
