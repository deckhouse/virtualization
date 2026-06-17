# setup-e2e-toolchain

Installs the common E2E workflow toolchain: checkout, Task, deckhouse-cli (`d8`), and kubectl.

Use `checkout: "false"` for jobs that already checked out the repository before calling this action.
Set `install-htpasswd: "true"` for jobs that need the `htpasswd` utility from `apache2-utils`.
