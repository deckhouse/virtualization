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

Behaviour (mirrors deckhouse/changelog-action@v2.6.0, validate_only mode):
  - Fetch MR description via GitLab API (CI_API_V4_URL).
  - Locate fenced code blocks with language ``changes``. A block may contain
    several entries separated by ``---`` lines, and ``section`` may list
    several comma-separated sections (one entry per section).
  - Required per entry: section (from the allowed list), type
    (feature/fix/chore/docs), summary.
  - ``impact_level`` is optional; when present it must be one of
    default/low/high. ``impact_level: high`` requires an ``impact``
    description ("missing high impact detail").
  - The allowed list (.gitlab/ci/changelog-sections.txt) uses the upstream
    ``section:forced_impact_level`` format; a forced level (``ci:low``)
    silently overrides the entry's impact_level — it is not an error.
    Blocks use the bare section name (``section: ci``).
  - Legacy v1 field names (module, description, note) are accepted as
    aliases for section, summary and impact.
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


# Anchor both fences to the start of a line (MULTILINE): upstream
# deckhouse/changelog-action extracts blocks with a real Markdown lexer
# (marked, GFM), which only treats a fence at column 0 as a code block and
# never mis-joins an indented example fence with a real one. Without the
# anchors a naive DOTALL match starting at an indented ```changes fence would
# swallow everything up to the next line-start backticks, capturing garbage.
CHANGES_BLOCK_RE = re.compile(
    r"^```changes[ \t]*\n(.*?)\n```",
    re.DOTALL | re.MULTILINE,
)
# HTML comments (e.g. the ```changes example shipped in the MR template) are
# opaque html tokens to the upstream Markdown lexer and are never scanned for
# fenced blocks. Mirror that by stripping them before searching.
HTML_COMMENT_RE = re.compile(r"<!--.*?-->", re.DOTALL)
KEY_VALUE_RE = re.compile(r"^([A-Za-z_]+)\s*:\s*(.*)$")
# Types accepted in a ```changes block, matching deckhouse/changelog-action@v2.6.0
# (the GitHub pipeline this was migrated from): feature, fix, chore, docs.
# 'chore' and 'docs' are accepted but NOT rendered into the public CHANGELOG-*.yml
# release notes: changelog_collect.render_yaml maps only feature/fix (upstream
# no-ops chore/docs for YAML), so they land only in the internal per-minor
# CHANGELOG-<minor>.md.
ALLOWED_TYPES = {"feature", "fix", "chore", "docs"}
# Impact levels known to deckhouse/changelog-action@v2.6.0; an absent
# impact_level means "default".
KNOWN_LEVELS = {"default", "low", "high"}


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


def load_allowed_sections(path: Path) -> dict[str, str]:
    """Map each allowed section name to its forced impact level ('' if none).

    The list uses the upstream ``section:forced_impact_level`` format, so a
    ``:low`` entry (e.g. ``ci:low``) forces low impact for that section: the
    forced level silently overrides the entry's ``impact_level``. Changelog
    blocks use the bare section name (``section: ci``).
    """
    sections: dict[str, str] = {}
    for raw in path.read_text(encoding="utf-8").splitlines():
        line = raw.strip()
        if not line or line.startswith("#"):
            continue
        name, _, suffix = line.partition(":")
        sections[name] = suffix if suffix in KNOWN_LEVELS else ""
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


def split_docs(block_text: str) -> list[str]:
    """Split a block into YAML-style documents on '---' separator lines."""
    docs: list[list[str]] = [[]]
    for line in block_text.splitlines():
        if line.strip() == "---":
            docs.append([])
        else:
            docs[-1].append(line)
    return ["\n".join(doc) for doc in docs]


def parse_entries(block_text: str) -> list[dict[str, str]]:
    """Parse a ```changes block into change entries.

    Mirrors deckhouse/changelog-action@v2.6.0 parse.ts: a block may hold
    several documents separated by ``---``; v1 field names (module,
    description, note) are accepted as aliases; a comma-separated ``section``
    yields one entry per section.
    """
    entries: list[dict[str, str]] = []
    for doc in split_docs(block_text):
        fields = parse_block(doc)
        base = {
            "type": fields.get("type", ""),
            "summary": fields.get("description") or fields.get("summary", ""),
            "impact": fields.get("note") or fields.get("impact", ""),
            "impact_level": fields.get("impact_level", ""),
        }
        section = fields.get("module") or fields.get("section", "")
        for name in section.split(","):
            entries.append({**base, "section": name.strip()})
    return entries


def validate_entry(
    entry: dict[str, str],
    allowed_sections: dict[str, str],
) -> list[str]:
    """Validate one entry, mirroring ChangeEntry.validate() plus the
    allowed-sections check of deckhouse/changelog-action@v2.6.0."""
    section = entry["section"]

    level = entry["impact_level"] or "default"
    forced = allowed_sections.get(section, "")
    if forced:
        # A forced level silently overrides the entry value (not an error).
        level = forced

    errors: list[str] = []
    if not entry["summary"]:
        errors.append("missing summary")
    if level not in KNOWN_LEVELS:
        errors.append(f"invalid impact level '{level}'")
    if level == "high" and not entry["impact"]:
        errors.append("missing high impact detail (add an 'impact' key)")
    if not section:
        errors.append("missing section")
    change_type = entry["type"]
    if change_type not in ALLOWED_TYPES:
        errors.append(
            f"invalid type '{change_type}' (allowed: {sorted(ALLOWED_TYPES)})"
            if change_type
            else "missing type"
        )
    errors.sort()
    if section not in allowed_sections:
        errors.append(
            f"unknown section '{section}' "
            f"(see .gitlab/ci/changelog-sections.txt)"
        )
    return errors


def validate_block(
    block_index: int,
    block_text: str,
    allowed_sections: dict[str, str],
) -> list[str]:
    errors: list[str] = []
    entries = parse_entries(block_text)
    for entry_index, entry in enumerate(entries, start=1):
        prefix = f"block #{block_index}"
        if len(entries) > 1:
            prefix += f" entry #{entry_index}"
        errors.extend(
            f"{prefix}: {error}"
            for error in validate_entry(entry, allowed_sections)
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
    description = HTML_COMMENT_RE.sub("", description)
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
