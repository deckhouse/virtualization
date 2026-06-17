# gpg-encrypt-and-upload

Encrypts artifacts with GPG symmetric AES256 encryption and uploads the resulting `.gpg` file.

Set `archive: "true"` for directory or multi-path inputs that should be zipped before encryption. Set `archive: "false"` for a single file such as a kubeconfig.
