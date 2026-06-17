## Encrypted artifacts

Some uploaded artifacts in this workflow are encrypted with GPG symmetric encryption.

Secret used for decryption passphrase:
- `E2E_ARTIFACTS_GPG_PASSPHRASE`

Encrypted artifact types:
- `*-generated-files-*.zip.gpg`
- `*-generated-files-ssh-*.zip.gpg`
- `*-generated-files-kubeconfig-*.gpg`
- `*-release-generated-files-*.zip.gpg`
- `*-release-generated-files-ssh-*.zip.gpg`
- `*-release-generated-files-kubeconfig-*.gpg`

Decrypt examples:

```bash
# zip.gpg artifact
gpg --decrypt --batch --yes --pinentry-mode loopback \
  --passphrase "$E2E_ARTIFACTS_GPG_PASSPHRASE" \
  --output artifact.zip \
  artifact.zip.gpg

unzip -o artifact.zip

# same, but with simultaneous decryption and extraction of the whole archive
gpg --decrypt --batch --yes --pinentry-mode loopback \
  --passphrase "$E2E_ARTIFACTS_GPG_PASSPHRASE" \
  artifact.zip.gpg > artifact.zip && unzip -o artifact.zip

# single-file .gpg artifact
gpg --decrypt --batch --yes --pinentry-mode loopback \
  --passphrase "$E2E_ARTIFACTS_GPG_PASSPHRASE" \
  --output kube-config \
  artifact.gpg
```
