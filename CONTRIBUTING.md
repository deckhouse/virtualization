# Contributing

## Feedback

The first thing we recommend is to check the existing [issues](https://github.com/deckhouse/virtualization/issues) — there may already be a discussion or solution on your topic. If not, choose the appropriate way to address the issue on [the new issue form](https://github.com/deckhouse/virtualization/issues/new/choose).

## Code contributions

1. Prepare an environment. To build and run common workflows locally, you'll need to _at least_ have the following installed:

   - [Git](https://git-scm.com/book/en/v2/Getting-Started-Installing-Git)
   - [Go](https://golang.org/doc/install)
   - [Docker](https://docs.docker.com/get-docker/)
   - [go-task](https://taskfile.dev/installation/) (task runner)
   - [ginkgo](https://onsi.github.io/ginkgo/#installing-ginkgo) (testing framework required to run tests)

2. [Fork the project](https://github.com/deckhouse/virtualization/fork).

3. Clone the project:

    ```shell
    git clone https://github.com/[GITHUB_USERNAME]/virtualization
    ```

4. Create branch following the [branch name convention](#branch-name):

    ```shell
    git branch -b feat/core/add-new-feature
    ```

5. Make changes.

6. Commit changes:

   - Follow [the commit message convention](#commit-message).
   - Sign off every commit you contributed as an acknowledgment of the [DCO](https://developercertificate.org/).

7. Push commits.

8. Create a pull request following the [pull request name convention](#pull-request-name).

## Images

The module images are located in the ./images directory.

Images, such as build images or images with binary artifacts, should not be included in the module. To do so, they must be labeled as follows in the `werf.inc.yaml` file: `final: false`.

## Conventions

### Commit message

Each commit message consists of a **header** and a [**body**](#body). The header has a special format that includes a [**type**](#type), a [**scope**](#scope) and a [**subject**](#subject):

```
<type>(<scope>): <subject>
<BLANK LINE>
<body>
```

**Examples:**

  - _feat(api): bump resource version to v1beta1_
  - _feat(images): implement ImageLost phase_
  - _feat(images, vi): add PVC as a storage_
  - _fix(vm, vmclass): fix unsupported type of class_
  - _refactor(core): rename 3rd party resources_
  - _docs(module): describe how to install module_
  - _chore(core): use alt linux as base image_
  - _chore: update go dependencies_

#### Type

Must be one of the following:

* **feat**: new features or capabilities that enhance the user's experience.
* **fix**: bug fixes that enhance the user's experience.
* **refactor**: a code changes that neither fixes a bug nor adds a feature.
* **docs**: updates or improvements to documentation.
* **test**: additions or corrections to tests.
* **chore**: updates that don't fit into other types.

#### Scope

Scope indicates the area of the project affected by the changes. The scope can consist of a top-level scope, which broadly categorizes the changes, and can optionally include nested scopes that provide further detail.

Supported scopes are the following:

  ```
  # The end-user functionalities, aiming to streamline and optimize user experiences.
  # NOTE! The api scope should be omitted if a more specific sub-scope is used.
  - api
    - vm
      - vmop
      - vmbda
      - vmclass
      - vmip
      - vmipl
      - vdsnapshot
      - vmsnapshot
      - vmrestore
    - disks
      - vd
    - images
      - vi
      - cvi

  # Core mechanisms and low-level system functionalities.
  - core
    - api-service
    - vm-route-forge
    - kubevirt
    - kube-api-rewriter
    - cdi
    - dvcr

  # Integration with the Deckhouse.
  - module

  # User metrics, alerts, dashboards, and logs that provide insights into system performance and health.
  - observability

  # Maintaining, improving code quality and development workflow.
  - ci
  
  # Maintaining, improving documentation.
  - docs

  # Network related changes important for end-user.
  - network  
  ```

#### Subject

The subject contains a succinct description of the change:

  - use the imperative, present tense: "change" not "changed" nor "changes"
  - don't capitalize the first letter
  - no dot (.) at the end

#### Body

Just as in the **subject**, use the imperative, present tense: "change" not "changed" nor "changes".
The body should include the motivation for the change and contrast this with previous behavior.

### Branch name

Each branch name consists of a [**type**](#type), [**scope**](#scope), and a [**short-description**](#short-description):

```
<type>/<scope>/<short-description>
```

When naming branches, only the top-level scope should be used. Multiple or nested scopes are not allowed in branch names, ensuring that each branch is clearly associated with a broad area of the project.

**Examples:**

  - _feat/disks/support-new-image-source_
  - _chore/ci/speed-up-builds_

### Changes Block

When submitting a pull request, include a **changes block** to document modifications for the changelog. This block helps automate the release changelog creation, tracks updates, and prepares release notes.

#### Format

The changes block consists of YAML documents, each detailing a specific change. Use the following structure:

````
```changes
section: <affected-section>
type: <feature|fix|chore>
summary: <Concise description of the change.>
impact_level: <low|high>  # Optional
impact: |
  <Detailed impact description if impact_level is high>
```
````

#### Fields Description

  - **section**: (Required) Specifies the affected scope of the project. Should be in kebab-case, choose one of [available scopes](#scope). If PR affects multiple scopes, add change block for each scope.
    - Examples: `api`, `core`, `ci`

  - **type**: (Required) Defines the nature of the change:
    - `feature`: Adds new functionality.
    - `fix`: Resolves user-facing issues.
    - `chore`: Maintenance tasks without direct user impact.

  - **summary**: (Required) A concise explanation of the change, ending with a period.

  - **impact_level**: (Optional) Indicates the significance of the change.
    - `high`: Requires an **impact** description and will be included in "Know before update" sections.
    - `low`: Minor changes, typically omitted from user-facing changelogs.

  - **impact**: (Required if `impact_level` is high) Describes the change's effects, such as expected restarts or downtime.
    - Examples:
      - "Ingress controller will restart."
      - "Expect slow downtime due to kube-apiserver restarts."

#### Example

```changes
section: core
type: feature
summary: "Node restarts can be avoided by pinning a checksum to a node group in config values."
impact: Recommended to use as a last resort.
---
section: core
type: fix
summary: "Nodes with outdated manifests are no longer provisioned on *InstanceClass update."
impact_level: high
impact: |
  Expect nodes of "Cloud" type to restart.

  Node checksum calculation is fixed, as well as a race condition during
  the machines (MCM) rendering which caused outdated nodes to spawn.
---
section: ci
type: fix
summary: "Improved comments tracking workflow progress."
impact_level: low
```

For full guidelines, refer to [here](https://github.com/deckhouse/deckhouse/wiki/Guidelines-for-working-with-PRs).

#### Short description

A concise, hyphen-separated phrase in kebab-case that clearly describes the main focus of the branch.

### Pull request name

Each pull request title should clearly reflect the changes introduced, adhering to [**the header format** of a commit message](#commit-message), typically mirroring the main commit's text in the PR.

**Examples**

  - _feat(vm): add live migration capability_
  - _docs(api): update REST API documentation for clarity_

## Coding

  - [Effective Go](https://golang.org/doc/effective_go.html).
  - [Go's commenting conventions](http://blog.golang.org/godoc-documenting-go-code).
