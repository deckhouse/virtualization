# registry-login

Parses a base64-encoded dockerconfigjson secret, logs in to its registry, and exposes the parsed registry host as the `registry` output.

The action keeps the existing E2E workflow parsing approach based on `base64 -d`, `jq`, a second `base64 -d`, and `cut`.
