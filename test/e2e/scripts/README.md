# E2E suite selector

`e2e-fzf` collects top-level Ginkgo `Describe` suites from `test/e2e`, lets you select one or more suites with `fzf`, and builds a runnable `task run` command.

## Requirements

- `python3`
- `fzf`
- `task`
- e2e runtime requirements used by `task run`: `kubectl`, `d8`, cluster access

## Usage

From the repository root:

```bash
task e2e:fzf
```

Or directly from any directory inside the repository:

```bash
test/e2e/scripts/e2e-fzf
```

The script resolves the repository root with:

```bash
git rev-parse --show-toplevel
```

## Actions

After selecting suites in `fzf`, the script prints the command and asks for an action:

```text
Action: [Enter] done, [r]un once, [n] repeat, [c]opy:
```

Actions:

- `Enter` — only print the command.
- `r` — run each selected suite once.
- `n` — ask for a repeat count and run each selected suite that many times.
- `c` — copy the printed command to the clipboard.

## Options

```bash
test/e2e/scripts/e2e-fzf --list
test/e2e/scripts/e2e-fzf --run
test/e2e/scripts/e2e-fzf --repeat 5
test/e2e/scripts/e2e-fzf --copy
```

With `task`:

```bash
task e2e:fzf -- --list
task e2e:fzf -- --run
task e2e:fzf -- --repeat 5
task e2e:fzf -- --copy
```

## Repeated runs

When multiple suites are selected, repeated mode runs every suite separately on every iteration. The final table shows status per suite per run:

```text
Results
+-----+-------------------------+--------+-----------+----------+
| Run | Suite                   | Status | Exit code | Duration |
+=====+=========================+========+===========+==========+
| 1   | RWOVirtualDiskMigration | PASS   | 0         | 10.2s    |
| 1   | StorageClassMigration   | FAIL   | 1         | 12.8s    |
| 2   | RWOVirtualDiskMigration | PASS   | 0         | 9.7s     |
| 2   | StorageClassMigration   | PASS   | 0         | 11.4s    |
+-----+-------------------------+--------+-----------+----------+
Summary: total=4 passed=3 failed=1
```

`FAIL` means the underlying `task run` process returned a non-zero exit code for that suite.

## Environment

- `E2E_DIR` — override the e2e directory. Defaults to `<repo>/test/e2e`.
- `FZF_OPTS` — extra options passed to `fzf`.
