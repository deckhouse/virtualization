# GitLab CI for the `deckhouse/virtualization` module

This directory contains the GitLab CI migration artifacts for the
`deckhouse/virtualization` module.

The migration source of truth is
[`tmp/ai-summary/gitlab-ci-migration-plan.md`](../tmp/ai-summary/gitlab-ci-migration-plan.md).
Anything below that disagrees with the plan is a bug.

The repository's root [`.gitlab-ci.yml`](../.gitlab-ci.yml) is the entry point
and `include`s the files in this directory.

---

## Table of contents

1. [Quick start](#1-quick-start)
2. [Layout](#2-layout)
3. [Required CI/CD variables](#3-required-cicd-variables)
4. [Migrating from `EXTERNAL_MODULES_*` to `DEV/PROD_MODULES_REGISTRY_*`](#4-migrating-from-external_modules_-to-devprod_modules_registry_)
5. [Token setup (`GITLAB_API_TOKEN`)](#5-token-setup-gitlab_api_token)
6. [Runner tags](#6-runner-tags)
7. [Jobs reference](#7-jobs-reference)
8. [Manual pipelines](#8-manual-pipelines)
9. [Scheduled pipelines](#9-scheduled-pipelines)
10. [Known TODOs / migration risks](#10-known-todos--migration-risks)
11. [Updating upstream templates (`modules-gitlab-ci`)](#11-updating-upstream-templates-modules-gitlab-ci)
12. [Slash commands and webhook listener](#12-slash-commands-and-webhook-listener)

---

## 1. Quick start

For a developer opening a new MR today, no action is required:
- Linting, unit tests, and the build pipeline trigger automatically on MR.
- Some validation jobs run only on changes to relevant paths.

For a release engineer:
1. Verify all CI/CD variables from [§3](#3-required-cicd-variables) exist in
   the project's `Settings -> CI/CD -> Variables`. Add missing ones — they are
   not auto-provisioned.
2. Run `bash .gitlab/ci/scripts/bash/setup-mr-settings.sh --dry-run` to preview
   the project MR settings that the script will apply, then drop `--dry-run`
   to apply them once.
3. For a release: see [§8](#8-manual-pipelines) (`backport`, `changelog:milestone`,
   `translate:changelog`).

## 2. Layout

```
.gitlab/
├── README.md                                  # this file
└── ci/
    ├── changelog-sections.txt                 # shared allowed_sections list
    ├── jobs/                                  # job definitions
    │   ├── auto-assign-author.yml             # auto-assign MR author
    │   ├── backport.yml                       # cherry-pick + open MR
    │   ├── changelog.yml                      # re-generate CHANGELOG from milestone
    │   ├── check-changelog.yml                # validate ```changes blocks
    │   ├── check-milestone.yml                # MR has a milestone
    │   ├── manual-tools.yml                   # mrs:summary (Loop notification)
    │   └── translate-changelog.yml            # ru -> en changelog + MR
    └── scripts/
        ├── bash/
        │   ├── auto-assign-author.sh
        │   ├── backport.sh
        │   ├── changelog-milestone.sh         # wrapper for ../python/changelog_collect.py
        │   ├── check-changelog-entry.sh       # wrapper for ../python/check_changelog_entry.py
        │   ├── check-milestone.sh
        │   ├── check-runner-tools.sh          # shell-executor tool preflight
        │   ├── gitlab-ci-lint.sh
        │   ├── set-vars.sh
        │   ├── setup-mr-settings.sh           # one-off project settings
        │   └── lib/
        │       └── api.sh                     # shared GitLab API helper
        └── python/
            ├── changelog_collect.py
            └── check_changelog_entry.py
.gitlab/scripts/js/
├── package.json
└── mrs_notifier.mjs                           # GitLab counterpart of prs_notifier.mjs
```

Every job `extends` (or `include`s) a script in `.gitlab/ci/scripts/bash/`.
Scripts source `.gitlab/ci/scripts/bash/lib/api.sh` for the `api GET / POST / PUT`
helper.

## 3. Required CI/CD variables

The table below lists every variable referenced from this directory's CI
files. The full list (including build/deploy) is in
`tmp/ai-summary/gitlab-ci-migration-plan.md` §4 / §11.13.

### Secrets (must be marked `Masked`)

| Variable | Scope | Description |
|---|---|---|
| `GITLAB_API_TOKEN` | api, write_repository | Project Access Token. Used by every `api.sh`-backed script (auto-assign, backport, changelog, check-milestone, mrs-summary, project settings). See [§5](#5-token-setup-gitlab_api_token). |
| `RELEASE_TOKEN` | api, write_repository | Alias used by upstream `Translate_Changelog` template. Create a separate token if you prefer to scope it tighter; otherwise use the same PAT as `GITLAB_API_TOKEN`. |
| `DEV_MODULES_REGISTRY_PASSWORD` | dev registry | Write access to DEV modules registry. |
| `PROD_MODULES_REGISTRY_PASSWORD` | prod registry, protected | Write access to PROD modules registry. Only available on protected branches/tags. |
| `PROD_READ_REGISTRY_PASSWORD` | prod read registry | Read-only access for `check:requirements`. |
| `PROD_READ_REGISTRY_USER` | prod read registry | Read-only login. |
| `SOURCE_REPO` | private source repo | URL for `werf import`. |
| `SOURCE_REPO_SSH_KEY` | private source repo, type=file | SSH key for cloning the source repo. Mark `Expand variable reference = false`. |
| `LOOP_WEBHOOK_URL` | Loop chat | Incoming webhook URL. Mark `Expand variable reference = false`. |
| `LOOP_TOKEN` | Loop API (optional) | Only needed if Loop API is used in addition to the webhook. |
| `DMT_METRICS_TOKEN` | DMT linter | Auth token for DMT metrics endpoint. |
| `DMT_METRICS_URL` | DMT linter | Endpoint URL for DMT metrics. |

### Plain variables (`Masked = off`)

| Variable | Description |
|---|---|
| `MODULE_NAME` | `virtualization` (already set in the root `.gitlab-ci.yml`). |
| `DEV_REGISTRY` | DEV modules registry host (e.g. `dev-registry.deckhouse.io`). |
| `DEV_MODULE_SOURCE` | DEV modules path (e.g. `dev-registry.deckhouse.io/sys/deckhouse-oss/modules`). |
| `DEV_MODULES_REGISTRY_LOGIN` | DEV registry login. |
| `PROD_REGISTRY` | PROD modules registry host (e.g. `registry-write.deckhouse.io`). |
| `PROD_READ_REGISTRY` | PROD read-only registry host. |
| `PROD_MODULES_REGISTRY_LOGIN` | PROD registry login. |
| `PROD_MODULE_SOURCE_NAME` | `deckhouse` (used in `${PROD_REGISTRY}/${PROD_MODULE_SOURCE_NAME}/${EDITION}/modules`). |
| `LOOP_CHANNEL_ID` | Loop channel ID (not secret). |
| `LOOP_API_BASE_URL` | Loop API base URL (not secret). |

### Not needed anymore (legacy, remove from project variables)

- `GITHUB_TOKEN` — replaced by `CI_JOB_TOKEN` / `GITLAB_API_TOKEN`.
- `RELEASE_PLEASE_TOKEN` — replaced by `GITLAB_API_TOKEN` / `RELEASE_TOKEN`.
- `K8S_CLUSTER_SECRET`, `VIRT_E2E_NIGHTLY_SA_TOKEN`, all `E2E_*` — these are
  scoped to e2e workflows which are **not** migrated.

## 4. Migrating from `EXTERNAL_MODULES_*` to `DEV/PROD_MODULES_REGISTRY_*`

The legacy root [`.gitlab-ci.yml`](../.gitlab-ci.yml) (pre-migration) referenced:

- `EXTERNAL_MODULES_DEV_REGISTRY_LOGIN`
- `EXTERNAL_MODULES_DEV_REGISTRY_PASSWORD`
- `EXTERNAL_MODULES_PROD_REGISTRY_LOGIN`
- `EXTERNAL_MODULES_PROD_REGISTRY_PASSWORD`

These were renamed (and several new vars were added) to match the upstream
`modules-gitlab-ci@v13.0` template names. Migration steps:

1. Open `Settings -> CI/CD -> Variables`.
2. For each legacy variable in the table below, create the new name with the
   same value, then delete the old one.

   | Old name | New name |
   |---|---|
   | `EXTERNAL_MODULES_DEV_REGISTRY_LOGIN` | `DEV_MODULES_REGISTRY_LOGIN` |
   | `EXTERNAL_MODULES_DEV_REGISTRY_PASSWORD` | `DEV_MODULES_REGISTRY_PASSWORD` |
   | `EXTERNAL_MODULES_PROD_REGISTRY_LOGIN` | `PROD_MODULES_REGISTRY_LOGIN` |
   | `EXTERNAL_MODULES_PROD_REGISTRY_PASSWORD` | `PROD_MODULES_REGISTRY_PASSWORD` |

3. Add the new plain vars from [§3](#3-required-cicd-variables):
   `DEV_REGISTRY`, `DEV_MODULE_SOURCE`, `PROD_REGISTRY`, `PROD_READ_REGISTRY`,
   `PROD_MODULE_SOURCE_NAME`.
4. Trigger a test pipeline on a non-protected branch. The first pipeline will
   fail with a clear error if any variable is missing.
5. Once the test pipeline is green, delete the legacy variables.

## 5. Token setup (`GITLAB_API_TOKEN`)

`GITLAB_API_TOKEN` must be a **Project Access Token** (PAT) scoped to this
project, with:

- Role: **Maintainer** (or higher).
- Scopes: `api`, `write_repository`.

Steps:

1. `Settings -> Access Tokens -> Add new token`.
2. Name: `ci-bot` (or anything).
3. Role: `Maintainer`.
4. Scopes: `api` + `write_repository`.
5. Pick an expiry (90 days is the default; rotate manually when prompted).
6. Save the value into `Settings -> CI/CD -> Variables -> GITLAB_API_TOKEN`
   with `Masked = true`, `Protected = false` (this script set needs it on
   feature branches too).
7. The same value should also be stored as `RELEASE_TOKEN` (the upstream
   Translate_Changelog template prefers that name).

For local debugging (e.g. `setup-mr-settings.sh`) you can export
`GITLAB_API_TOKEN` in your shell, but never commit it.

## 6. Runner tags

All jobs in this directory specify `tags: [deckhouse]`. This is a placeholder
until concrete runner tags are registered at
<https://fox.flant.com/deckhouse/virtualization/-/runners>.

Once registration is complete, update the value:

```bash
# Find every "tags:" line in this directory that says "deckhouse".
grep -rn 'tags:' .gitlab/ci/jobs/ | grep deckhouse
# Update each to the real runner tag, e.g. "deckhouse-large" or "dvp".
```

Look for `TODO_RUNNER_TAG` comments in each job yml; replace the tag and
remove the comment when finalised.

### Shell executor requirements

The project runner is expected to use the GitLab Runner `shell` executor.
For that executor, `image:` and container `entrypoint:` settings are ignored,
so project jobs do not install packages with `apk`, `apt-get`, or other host
package managers. Tools must already be installed on the runner host. Jobs that
need non-trivial tools call `.gitlab/ci/scripts/bash/check-runner-tools.sh` in
`before_script` and fail early with a clear message if a tool is missing.

Expected host tools for project-owned jobs:

| Job family | Required runner tools |
|---|---|
| Common GitLab API helpers | `bash`, `curl`, `jq` |
| Go/task validation jobs | `go`, `task`; `lint:shellcheck` also needs `shellcheck` |
| Generated-file checks | `go`, `task`, `git` |
| Build/deploy/precache/cleanup templates | upstream `Setup.gitlab-ci.yml` needs `bash`, `curl`, `trdl` bootstrap support, `werf`, `jq`, `crane`, registry credentials, and SSH tools when Svace keys are configured |
| Changelog/check-changelog jobs | `bash`, `python3`, `curl`, `jq`; changelog MR creation also needs `git`, `ssh-agent`, `ssh-add` |
| Backport | `bash`, `git`, `curl`, `jq`, `ssh-agent`, `ssh-add` |
| MR summary | `node`, `npm` |
| Upstream scanning templates | use the requirements from `modules-gitlab-ci@v13.0` (for example CVE scan downloads `d8` and uses `curl`, `tar`, `jq`, `git`, SSH tools) |

## 7. Jobs reference

| Job | Stage | Trigger | Required token | What it does |
|---|---|---|---|---|
| `auto-assign-author` | info | MR opened / reopened | `GITLAB_API_TOKEN` | Assigns the MR author via API. Skips silently if the MR already has an assignee (plan §0(4)). |
| `check:milestone` | lint | MR open / synchronize | `GITLAB_API_TOKEN` | Fails if MR has no `milestone` assigned. |
| `check:changelog` | lint | MR open / synchronize | `GITLAB_API_TOKEN` | Validates ` ```changes ` blocks in MR description against `.gitlab/ci/changelog-sections.txt`. |
| `translate:changelog` | (template) | push to any branch except default | `RELEASE_TOKEN` (or `GITLAB_API_TOKEN`) | Extends upstream `.translate_and_create_mr` from `modules-gitlab-ci@v13.0`. Translates `CHANGELOG/v*.ru.yml` to English and opens an MR. |
| `changelog:milestone` | lint | manual / scheduled | `GITLAB_API_TOKEN` | Re-generates `CHANGELOG/CHANGELOG-<milestone>.yml` and `CHANGELOG/CHANGELOG-<minor>.md` from MRs with a milestone. Optionally opens a changelog MR. |
| `changelog:all-active-milestones` | lint | manual / scheduled | `GITLAB_API_TOKEN` | Same as above, but iterates over all active milestones. |
| `backport` | lint | manual with `TARGET_BRANCH` OR MR labelled `backport-release-X.Y` | `GITLAB_API_TOKEN` | Cherry-picks the merged MR into a new `backport/<iid>/<release>` branch, pushes it, and opens an MR to the release branch. |
| `mrs:summary` | notify | manual / scheduled | `GITLAB_API_TOKEN`, `LOOP_WEBHOOK_URL` | Posts a markdown summary of open MRs to Loop (replaces `prs_notifier.mjs`). |

## 8. Manual pipelines

GitLab has no native equivalent of `workflow_dispatch`; instead, jobs are
marked `when: manual` and triggered from `Run pipeline` UI. To run a manual
pipeline:

1. Open `CI/CD -> Pipelines -> Run pipeline`.
2. Pick the branch (default: current default branch).
3. Fill in variables:
   - For `backport`: set `TARGET_BRANCH=release-1.21` (or whichever).
   - For `changelog:milestone`: optionally set `MILESTONE_TITLE=v1.21.3` and
     `OPEN_CHANGELOG_MR=true`.
   - For `mrs:summary`: ensure `LOOP_WEBHOOK_URL` is set.
4. Submit. The pipeline starts; manual jobs appear under the pipeline view
   with a `play` button.

The `dispatch-slash-command.yml` GitHub workflow (slash commands like
`/changelog`, `/backport`, `/e2e` in MR comments) was intentionally **not**
migrated as reactive automation. See [§12](#12-slash-commands-and-webhook-listener)
for the future plan.

## 9. Scheduled pipelines

Some jobs are intended to run on a schedule (e.g. `mrs:summary` once per day
at 10:00 Moscow time). Configure them at
`CI/CD -> Schedules -> New schedule`:

| Schedule name | Cron | Target branch | Variables |
|---|---|---|---|
| `mrs-summary-daily` | `0 7 * * *` (10:00 MSK) | `main` | _(none — uses project vars)_ |
| `changelog-sweep` | `0 3 * * *` (06:00 MSK) | `main` | `OPEN_CHANGELOG_MR=false` |

Schedules trigger pipelines whose `CI_PIPELINE_SOURCE == "schedule"`. Jobs
that should run on a schedule have a corresponding rule with
`when: manual allow_failure: true` so they don't break the schedule if a
maintainer hasn't pre-approved them.

## 10. Known TODOs / migration risks

These are intentional gaps from the first-iteration migration. Track them
under the `virtualization-m9e.3` Beads issue.

- **`TODO_RUNNER_TAG`** — every job uses `tags: [deckhouse]` as a
  placeholder. Replace with the real registered runner tag once
  available. Search for `TODO_RUNNER_TAG` in this directory.
- **Webhook listener for slash-commands** — GitLab does not natively start
  pipelines on MR comment creation or label change. See
  [§12](#12-slash-commands-and-webhook-listener).
- **Reactive `changelog:milestone`** — currently manual + scheduled. When
  the webhook listener lands, add a `merge_request.closed` / `milestoned`
  handler that triggers `changelog:milestone` with the right
  `MILESTONE_TITLE`.
- **Edition `se-plus`** — the legacy `.gitlab-ci.yml` builds only
  `ce`/`ee`/`fe`. GitHub's `release_module_build-and-registration.yml`
  also built `se-plus`. Add `se-plus` to the `parallel.matrix.EDITION`
  list if/when Deckhouse supports it for this module.
- **Inputs of `release_module_release-channels.yml`** — the GH workflow
  exposed `channel`, `ce`, `ee`, `tag`, `enableBuild`,
  `release_to_github`, `check_only`, `skip_requirements_check`,
  `send_results_to_loop`. The current prod deploy jobs only accept a
  tag-based trigger. Variables for `channel`, `tag`, and `check_only` are
  already supported in the deploy job UI. The rest are tracked under the
  build/deploy epic.
- **`prs_notifier.mjs` STUCK detection** — the original GitHub version
  uses per-review `submitted_at` to compute "stuck for 1.5 days". The
  GitLab port currently treats all unresolved discussions as "stuck"
  without checking thread age. TODO: pull `discussions[].notes[].created_at`
  to refine the heuristic.
- **GitLab username for `z9r5`** — the doc reviewer is hard-coded as
  `DOC_REVIEWER=z9r5` in `mrs:summary`. Override via the
  `DOC_REVIEWER` CI/CD variable until the real username is confirmed.
- **Vault integration in `cve_scan_on_pr`** — the GH workflow read secrets
  from HashiCorp Vault via `hashicorp/vault-action@v2`. After migration,
  any secret that CVE scan needs is expected to live in CI/CD variables.
  If a secret remains in Vault only, the CVE-scan job needs a JWT-auth
  sidecar (out of scope for the first iteration).

## 11. Updating upstream templates (`modules-gitlab-ci`)

The `include: project: 'deckhouse/3p/deckhouse/modules-gitlab-ci' ref: 'v13.0'`
in [`.gitlab/ci/jobs/translate-changelog.yml`](ci/jobs/translate-changelog.yml)
currently tracks the `v13.0` **branch**. After the migration stabilises
(~2–4 weeks), pin to a commit SHA:

```bash
git ls-remote git@fox.flant.com:deckhouse/3p/deckhouse/modules-gitlab-ci.git refs/heads/v13.0
# Pick the first column as <SHA>. Then in translate-changelog.yml:
#   ref: '<SHA>'   # was: v13.0 — pinned at YYYY-MM-DD
```

After pinning, run a test pipeline on a feature branch before merging.

## 12. Slash commands and webhook listener

GitHub let us react to MR comments (`/changelog`, `/backport`, `/e2e`) and
label changes (`status/backport`, `analyze/svace`) without a webhook. GitLab
does not — see upstream issues:

- <https://gitlab.com/gitlab-org/gitlab/-/issues/341000> (label change triggers)
- <https://gitlab.com/gitlab-org/gitlab/-/issues/553940> (comment triggers)

For full GitHub parity, deploy a small **webhook-listener** service that:

1. Accepts GitLab webhooks:
   - `Merge Request Hook` (filter on `action=open|update`, `labels.title changed`,
     `action=close`).
   - `Note Hook` (filter on `noteable_type=MergeRequest`, `action=create`).
2. Parses the payload and calls
   `POST /api/v4/projects/:id/trigger/pipeline` with the right variables.

Until that exists, the manual job matrix in [§7](#7-jobs-reference) and the
two scheduled jobs in [§9](#9-scheduled-pipelines) cover the same surface
area, with a human pressing the button.

## See also

- [`tmp/ai-summary/gitlab-ci-migration-plan.md`](../tmp/ai-summary/gitlab-ci-migration-plan.md) — full migration plan.
- [`.gitlab-ci.yml`](../.gitlab-ci.yml) — root pipeline.
- [`/Users/korolevn/repos/Virtualization-tasks/github/3p-deckhouse/modules-gitlab-ci`](../) — local checkout of upstream templates used by `translate:changelog`.
