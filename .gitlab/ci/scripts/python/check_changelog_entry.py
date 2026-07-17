#!/usr/bin/env python3
# Copyright 2026 Flant JSC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""Validate ```changes fenced blocks in a GitLab MR description.

Migration of the validation logic in
.github/workflows/check-changelog-entry.yml which used
deckhouse/changelog-action@v2.6.0 with validate_only=true.

Behaviour:
  - Fetch MR description via GitLab API (CI_API_V4_URL).
  - Locate fenced code blocks with language ``changes``.
  - For each block validate required keys: section, type, summary.
  - ``section`` must be in the allowed list (.gitlab/ci/changelog-sections.txt);
    write the bare name (``ci``). The list uses the upstream
    ``section:forced_impact_level`` format, so a ``:low`` entry (``ci:low``)
    forces low impact for that section. A legacy ``:low`` suffix in the block
    is still accepted.
  - ``impact_level`` is required unless the section forces a low impact level.
  - If no ```changes blocks at all -> OK (PR may not require changelog).
  - Otherwise collect errors and exit non-zero.

Required environment:
  GITLAB_API_TOKEN, CI_API_V4_URL, CI_PROJECT_ID, CI_MERGE_REQUEST_IID

Optional:
  CHANGELOG_SECTIONS_FILE (default: .gitlab/ci/changelog-sections.txt)
"""

from __future__ import annotations

import json
import os
import re
import sys
import urllib.error
import urllib.request
from pathlib import Path


CHANGES_BLOCK_RE = re.compile(
    r"```changes\s*\n(.*?)\n```",
    re.DOTALL,
)
KEY_VALUE_RE = re.compile(r"^([A-Za-z_]+)\s*:\s*(.*)$")
# deckhouse/changelog-action@v2.6.0 only renders 'feature' (-> features) and
# 'fix' (-> fixes) in CHANGELOG-*.yml. Keep in sync with changelog_collect.py.
ALLOWED_TYPES = {"feature", "fix"}


def log(message: str) -> None:
    print(message, file=sys.stderr)


def require_env(name: str) -> str:
    value = os.environ.get(name, "").strip()
    if not value:
        log(f"ERROR: required environment variable {name} is not set")
        sys.exit(1)
    return value


def fetch_mr_description(
    api_base: str, project_id: str, mr_iid: str, token: str
) -> str:
    """GET the MR via REST API and return its description."""
    url = f"{api_base}/projects/{project_id}/merge_requests/{mr_iid}"
    req = urllib.request.Request(
        url,
        headers={
            "PRIVATE-TOKEN": token,
            "Accept": "application/json",
        },
        method="GET",
    )
    try:
        with urllib.request.urlopen(req) as response:
            payload = json.loads(response.read().decode("utf-8"))
    except urllib.error.HTTPError as exc:
        log(f"ERROR: failed to fetch MR: HTTP {exc.code} {exc.reason}")
        sys.exit(1)
    return (payload.get("description") or "").strip()


def load_allowed_sections(path: Path) -> dict[str, bool]:
    """Map each allowed section name to whether it forces a low impact level.

    The list uses the upstream ``section:forced_impact_level`` format, so a
    ``:low`` entry (e.g. ``ci:low``) forces low impact for that section and
    ``impact_level`` may be omitted. Changelog blocks use the bare section
    name (``section: ci``).
    """
    sections: dict[str, bool] = {}
    for raw in path.read_text(encoding="utf-8").splitlines():
        line = raw.strip()
        if not line or line.startswith("#"):
            continue
        name, _, suffix = line.partition(":")
        sections[name] = suffix == "low"
    return sections


def parse_block(block_text: str) -> dict[str, str]:
    fields: dict[str, str] = {}
    for raw_line in block_text.splitlines():
        match = KEY_VALUE_RE.match(raw_line.rstrip())
        if not match:
            continue
        key, value = match.group(1).strip().lower(), match.group(2).strip()
        fields[key] = value
    return fields


def validate_block(
    block_index: int,
    block_text: str,
    allowed_sections: dict[str, bool],
) -> list[str]:
    errors: list[str] = []
    fields = parse_block(block_text)

    section_raw = fields.get("section", "")
    # A legacy ':low' suffix in the block is accepted; the bare name is authoritative.
    section = section_raw.split(":", 1)[0]
    if not section_raw:
        errors.append(f"block #{block_index}: missing required key 'section'")
    elif section not in allowed_sections:
        errors.append(
            f"block #{block_index}: section '{section}' is not in "
            f"allowed_sections (see .gitlab/ci/changelog-sections.txt)"
        )

    change_type = fields.get("type", "")
    if not change_type:
        errors.append(f"block #{block_index}: missing required key 'type'")
    elif change_type not in ALLOWED_TYPES:
        errors.append(
            f"block #{block_index}: type '{change_type}' is not supported; "
            f"allowed types are {sorted(ALLOWED_TYPES)} "
            f"(deckhouse changelog only supports 'feature' and 'fix', "
            f"rendered as the 'features'/'fixes' sections)"
        )

    summary = fields.get("summary", "")
    if not summary:
        errors.append(f"block #{block_index}: missing required key 'summary'")

    # A section that forces low impact makes impact_level optional (and pinned
    # to low); every other section requires an explicit impact_level.
    forces_low = allowed_sections.get(section, False)
    impact_level = fields.get("impact_level", "")
    if not forces_low and not impact_level:
        errors.append(
            f"block #{block_index}: missing required key 'impact_level' "
            f"(section '{section}' does not force a low impact level)"
        )
    elif forces_low and impact_level and impact_level != "low":
        errors.append(
            f"block #{block_index}: section '{section}' forces impact_level=low "
            f"but impact_level='{impact_level}' was provided"
        )

    return errors


def main() -> int:
    api_base = require_env("CI_API_V4_URL").rstrip("/")
    project_id = require_env("CI_PROJECT_ID")
    mr_iid = require_env("CI_MERGE_REQUEST_IID")
    token = require_env("GITLAB_API_TOKEN")

    pipeline_source = os.environ.get("CI_PIPELINE_SOURCE", "")
    if pipeline_source != "merge_request_event":
        log(f"Not a merge request pipeline (CI_PIPELINE_SOURCE={pipeline_source}). Skipping.")
        return 0

    sections_path = Path(
        os.environ.get(
            "CHANGELOG_SECTIONS_FILE",
            ".gitlab/ci/changelog-sections.txt",
        )
    )
    if not sections_path.is_file():
        log(f"ERROR: allowed sections file not found: {sections_path}")
        return 1
    allowed_sections = load_allowed_sections(sections_path)

    description = fetch_mr_description(api_base, project_id, mr_iid, token)
    blocks = CHANGES_BLOCK_RE.findall(description)

    if not blocks:
        log("No ```changes blocks in MR description — OK (changelog not required).")
        return 0

    log(f"Found {len(blocks)} ```changes block(s) in MR description — validating.")

    all_errors: list[str] = []
    for index, raw_block in enumerate(blocks, start=1):
        all_errors.extend(
            validate_block(index, raw_block, allowed_sections)
        )

    if all_errors:
        for err in all_errors:
            log(f"ERROR: {err}")
        log(f"{len(all_errors)} validation error(s) found.")
        return 1

    log(f"All {len(blocks)} ```changes block(s) are valid.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
