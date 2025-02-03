## Description
<!---
  Describe your changes with technical details.
-->


## Why do we need it, and what problem does it solve?
<!---
  Tell a story about the problem we've faced, why we've decided to fix it
  and what effect users will get after merging. Add links if applicable.
-->


## What is the expected result?
<!---
  Describe steps to reproduce the expected result.
  What ACTION(s) to take to ensure the problem is gone.
-->


## Checklist
- [ ] The code is covered by unit tests.
- [ ] e2e tests passed.
- [ ] Documentation updated according to the changes.
- [ ] Changes were tested in the Kubernetes cluster manually.


## Changelog entries
<!---
  Add one or more changelog entries to present your changes to end users.

  Changelog entry fields description:
  - `section` - a project scope in the kebab-case (See CONTRIBUTING.md#scope for the list).
  - `type` - one of the following: fix, feature, chore
  - `summary` - a ONE-LINE description on how change affects users.
  - `impact_level` - Optional field: set to 'low' to exclude entry from changelog.
  - `impact` - Optional field: multiline message on what user should know in the first place, i.e. restarts, breaking changes, deprecated fields, etc. Requires `impact_level: high`.

  /!\ See CONTRIBUTING.md for more details. /!\

  Example 1. Significant message at the beginning of the changelog:

section: disks
type: feat
summary: "Disks serials are based on uid now."
impact_level: high
impact: |
  Disk serial is now uid-based.

  Using /dev/disk/by-serial/ is not supported anymore.


  Example 2. Chore change (not in the Changelog):

section: ci
type: chore
summary: "Update checkout and github-script action versions."
impact_level: low


  Example 3. Regular entry in the Changelog:

section: vm
type: feature
summary: "virtualMachineClassName field is now required."

-->

```changes
section: 
type: 
summary: 
```
