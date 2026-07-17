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

"""Re-generate CHANGELOG/<milestone>.yml and CHANGELOG/<minor>.md from merged MRs.

Migration of .github/actions/milestone-changelog/action.yml (composite action)
which used deckhouse/changelog-action@v2.6.0.

Strategy: rewrite the parser in Python (Variant B).

Behaviour:
  1. Resolve target milestone from MILESTONE_TITLE or list open milestones.
  2. Fetch all merged MRs with that milestone (paginated) via GitLab API.
  3. Parse ```changes fenced blocks from each MR description.
  4. Group entries by `section` and `impact_level`.
  5. Emit:
       CHANGELOG/CHANGELOG-<milestone_title>.yml
       CHANGELOG/CHANGELOG-<minor_version>.md
  6. Open a changelog MR to the base branch (CHANGELOG_FROM_MR=true) via push
     options and CI_JOB_TOKEN (no separate API call).

Required environment:
  GITLAB_API_TOKEN, CI_API_V4_URL, CI_PROJECT_ID, CI_SERVER_HOST,
  CI_PROJECT_PATH, CI_PROJECT_DIR

Optional:
  MILESTONE_TITLE        - generate for a specific milestone
  OPEN_CHANGELOG_MR      - "true" to push branch + open MR (default false)
  CHANGELOG_BASE_BRANCH  - default "main"
  CHANGELOG_SECTIONS_FILE - default .gitlab/ci/changelog-sections.txt
"""

from __future__ import annotations

import json
import os
import re
import subprocess
import sys
import urllib.error
import urllib.parse
import urllib.request
from collections import defaultdict
from pathlib import Path


CHANGES_BLOCK_RE = re.compile(
    r"```changes\s*\n(.*?)\n```",
    re.DOTALL,
)
KEY_VALUE_RE = re.compile(r"^([A-Za-z_]+)\s*:\s*(.*)$")
# Only these keys start a new field in a ```changes block. Any other line is
# treated as a continuation of the current field, so multi-line values (most
# importantly `impact:`, the high-impact migration note) are preserved instead
# of being dropped. Mirrors the deckhouse/changelog-action block schema.
KNOWN_BLOCK_KEYS = {"section", "type", "summary", "impact", "impact_level"}
# deckhouse/changelog-action@v2.6.0 only renders 'feature' (-> features) and 'fix'
# (-> fixes) sections in CHANGELOG-*.yml. Keep in sync with
# check_changelog_entry.py.
ALLOWED_TYPES = {"feature", "fix"}
TYPE_TO_SECTION = {
    "feature": "features",
    "fix": "fixes",
}


def log(message: str) -> None:
    print(message, file=sys.stderr)


def require_env(name: str) -> str:
    value = os.environ.get(name, "").strip()
    if not value:
        log(f"ERROR: required environment variable {name} is not set")
        sys.exit(1)
    return value


def api_get_paginated(
    api_base: str, path: str, token: str, params: dict[str, str] | None = None
) -> list[dict]:
    """GET all pages of a list endpoint, return combined JSON array."""
    results: list[dict] = []
    url = f"{api_base}{path}"
    if params:
        url = f"{url}?{urllib.parse.urlencode(params)}"
    while url:
        req = urllib.request.Request(
            url,
            headers={"PRIVATE-TOKEN": token, "Accept": "application/json"},
            method="GET",
        )
        with urllib.request.urlopen(req) as response:
            # GitLab returns Link header for next page (RFC 5988).
            link_header = response.headers.get("Link", "")
            payload = json.loads(response.read().decode("utf-8"))
            if isinstance(payload, list):
                results.extend(payload)
            else:
                # Non-list (single object): treat as one-item result and stop.
                results.append(payload)
                break
        url = next_link(link_header)
    return results


def next_link(link_header: str) -> str:
    """Parse GitLab's Link header and return the next rel='next' URL, or ''."""
    if not link_header:
        return ""
    for part in link_header.split(","):
        section = part.strip()
        match = re.match(r'<([^>]+)>;\s*rel="([^"]+)"', section)
        if match and match.group(2) == "next":
            return match.group(1)
    return ""


def api_request(
    api_base: str, path: str, token: str, method: str, data: dict
) -> dict:
    """Send a JSON request (POST/PUT) and return the decoded response."""
    req = urllib.request.Request(
        f"{api_base}{path}",
        data=json.dumps(data).encode("utf-8"),
        headers={
            "PRIVATE-TOKEN": token,
            "Accept": "application/json",
            "Content-Type": "application/json",
        },
        method=method,
    )
    with urllib.request.urlopen(req) as response:
        return json.loads(response.read().decode("utf-8"))


def parse_changes_block(block_text: str) -> dict[str, str] | None:
    fields: dict[str, str] = {}
    current_key: str | None = None
    for raw_line in block_text.splitlines():
        match = KEY_VALUE_RE.match(raw_line.rstrip())
        if match and match.group(1).strip().lower() in KNOWN_BLOCK_KEYS:
            key = match.group(1).strip().lower()
            fields[key] = match.group(2).strip()
            current_key = key
        elif current_key is not None:
            # Continuation line of the current field (e.g. a multi-line impact).
            cont = raw_line.strip()
            if cont:
                fields[current_key] = (
                    f"{fields[current_key]}\n{cont}" if fields[current_key] else cont
                )
    required = {"section", "type", "summary"}
    if not required.issubset(fields):
        return None
    return fields


def has_label(mr: dict, label: str) -> bool:
    labels = mr.get("labels") or []
    for item in labels:
        if isinstance(item, str) and item == label:
            return True
        if isinstance(item, dict) and item.get("name") == label:
            return True
    return False


def collect_entries_for_milestone(
    api_base: str, project_id: str, milestone_title: str, token: str,
    allowed_sections: dict[str, bool],
) -> list[dict]:
    log(f"Fetching merged MRs for milestone '{milestone_title}'...")
    mrs = api_get_paginated(
        api_base,
        f"/projects/{project_id}/merge_requests",
        token,
        params={
            "state": "merged",
            "milestone": milestone_title,
            "per_page": "100",
            "order_by": "created_at",
            "sort": "asc",
        },
    )
    log(f"Found {len(mrs)} merged MR(s) for milestone '{milestone_title}'.")

    entries: list[dict] = []
    for mr in mrs:
        if has_label(mr, "changelog"):
            log(f"Skipping changelog MR !{mr['iid']}.")
            continue
        description = (mr.get("description") or "").strip()
        if not description:
            continue
        for raw_block in CHANGES_BLOCK_RE.findall(description):
            parsed = parse_changes_block(raw_block)
            if parsed is None:
                continue
            # A legacy ':low' suffix in the block is accepted; the bare name is authoritative.
            section = parsed["section"].split(":", 1)[0]
            if section not in allowed_sections:
                log(f"WARN: MR !{mr['iid']} uses unknown section '{section}', skipping.")
                continue
            # Sections flagged in allowed_sections (':low') force impact_level
            # to low; others use the block value, defaulting to high.
            if allowed_sections[section]:
                impact_level = "low"
            else:
                impact_level = parsed.get("impact_level", "") or "high"
            entries.append(
                {
                    "section": section,
                    "type": parsed["type"],
                    "summary": parsed["summary"],
                    "impact": parsed.get("impact", ""),
                    "impact_level": impact_level,
                    "mr_iid": mr["iid"],
                    "mr_title": mr.get("title", ""),
                    "mr_url": mr.get("web_url", ""),
                    "author": (mr.get("author") or {}).get("username", ""),
                }
            )
    return entries


def group_entries(entries: list[dict]) -> dict[str, list[dict]]:
    grouped: dict[str, list[dict]] = defaultdict(list)
    for entry in entries:
        grouped[entry["section"]].append(entry)
    return grouped


def yaml_summary_scalar(value: str) -> str:
    """Emit a YAML scalar for a changelog summary line.

    Plain style when safe (matches deckhouse/changelog-action output for the
    common case); double-quoted otherwise to avoid YAML injection.
    """
    if value == "":
        return '""'
    if (
        re.search(r"[:#]", value)
        or value[0] in "-?,[]{}'\"&*!|>%@`"
        or value.endswith(" ")
        or ": " in value
        or " #" in value
    ):
        return json.dumps(value, ensure_ascii=False)
    return value


def render_yaml(entries: list[dict], milestone_title: str) -> str:
    """Render CHANGELOG-<milestone>.yml in the deckhouse schema.

    Schema (matches deckhouse/changelog-action@v2.6.0 release_yaml)::

        <section>:
          features:
            - summary: <text>
              pull_request: <mr_url>
          fixes:
            - summary: <text>
              pull_request: <mr_url>

    Sections are sorted alphabetically and emitted compactly (no blank lines
    between sections). Entries already store the bare section name; any ':low'
    suffix is stripped here as a defensive fallback and is never represented in
    the YAML. Within each section, entries are ordered by MR iid descending,
    matching the historical generator output. An empty milestone yields '{}'
    (same as the historical generator).
    """
    grouped: dict[str, dict[str, list[dict]]] = {}
    for entry in entries:
        section_key = entry["section"].split(":", 1)[0]
        bucket = TYPE_TO_SECTION.get(entry["type"])
        if bucket is None:
            log(
                f"WARN: MR !{entry['mr_iid']} has unsupported type "
                f"'{entry['type']}' (allowed: {sorted(ALLOWED_TYPES)}), skipping."
            )
            continue
        grouped.setdefault(section_key, {"features": [], "fixes": []})[bucket].append(entry)

    if not grouped:
        return "{}\n\n"

    lines: list[str] = []
    for section in sorted(grouped.keys()):
        buckets = grouped[section]
        lines.append(f"{section}:")
        for bucket in ("features", "fixes"):
            items = sorted(buckets[bucket], key=lambda e: e["mr_iid"], reverse=True)
            if not items:
                continue
            lines.append(f"  {bucket}:")
            for entry in items:
                lines.append(f"    - summary: {yaml_summary_scalar(entry['summary'])}")
                lines.append(f"      pull_request: {entry['mr_url']}")
                # High-impact entries carry a free-text `impact` migration note.
                # Preserve it (deckhouse/changelog-action emits it after
                # pull_request); a multi-line note becomes a literal block.
                impact = entry.get("impact", "")
                if impact:
                    if "\n" in impact:
                        lines.append("      impact: |-")
                        for impact_line in impact.split("\n"):
                            lines.append(f"        {impact_line}" if impact_line else "")
                    else:
                        lines.append(f"      impact: {yaml_summary_scalar(impact)}")
    return "\n".join(lines) + "\n\n"


def render_milestone_md_block(entries: list[dict], milestone_title: str) -> str:
    """Render the markdown block for ONE milestone (patch version).

    Heading is `## <milestone_title>` so that
    :func:`merge_minor_markdown` can merge multiple patch versions into the
    cumulative `CHANGELOG-<minor>.md` idempotently.
    """
    grouped = group_entries(entries)
    lines = [f"## {milestone_title}", ""]
    if not grouped:
        lines.append("_No changelog entries._")
        return "\n".join(lines).rstrip() + "\n"
    for section in sorted(grouped.keys()):
        lines.append(f"### {section}")
        lines.append("")
        for entry in grouped[section]:
            lines.append(
                f"- **{entry['type']}** ({entry['impact_level']}): "
                f"{entry['summary']} ([!{entry['mr_iid']}]({entry['mr_url']}))"
            )
        lines.append("")
    return "\n".join(lines).rstrip() + "\n"


def md_version_sort_key(title: str) -> tuple[int, int, int]:
    """Sort key for `## vX.Y.Z` headings; missing parts sort as 0."""
    m = re.match(r"^v?(\d+)\.(\d+)(?:\.(\d+))?", title)
    if not m:
        return (0, 0, 0)
    return tuple(int(part) if part else 0 for part in m.groups())  # type: ignore[return-value]


def parse_minor_md_blocks(text: str) -> dict[str, str]:
    """Split an existing CHANGELOG-<minor>.md into {milestone_title: block}.

    Content before the first `## ` heading (the file header) is dropped — it is
    regenerated. Each block keeps its own `## <title>` heading.
    """
    blocks: dict[str, str] = {}
    current_title: str | None = None
    current_lines: list[str] = []
    for line in text.splitlines():
        if line.startswith("## "):
            if current_title is not None:
                blocks[current_title] = "\n".join(current_lines).rstrip() + "\n"
            current_title = line[3:].strip()
            current_lines = [line]
        elif current_title is not None:
            current_lines.append(line)
    if current_title is not None:
        blocks[current_title] = "\n".join(current_lines).rstrip() + "\n"
    return blocks


def merge_minor_markdown(
    existing_text: str, minor_version: str, milestone_title: str, block: str
) -> str:
    """Merge ``block`` for ``milestone_title`` into the cumulative minor file.

    Replaces this milestone's block if present (idempotent re-generation) or
    inserts it, then re-emits all patch blocks newest-first. This is what keeps
    CHANGELOG-<minor>.md cumulative across patch releases (the GitHub
    changelog-action produced a cumulative ``branch_markdown``); rendering only
    the current milestone would drop the earlier patches.
    """
    blocks = parse_minor_md_blocks(existing_text)
    blocks[milestone_title] = block
    ordered = sorted(blocks.keys(), key=md_version_sort_key, reverse=True)
    out = [f"# Changelog {minor_version}", ""]
    for title in ordered:
        out.append(blocks[title].rstrip())
        out.append("")
    return "\n".join(out).rstrip() + "\n"


def minor_version_from_tag(tag: str) -> str:
    """v1.21.3 -> v1.21, v1.21 -> v1.21."""
    m = re.match(r"^v(\d+\.\d+)(?:\.\d+)?$", tag)
    if not m:
        return tag
    return f"v{m.group(1)}"


def write_files(
    project_dir: Path,
    milestone_title: str,
    entries: list[dict],
) -> tuple[Path, Path]:
    changelog_dir = project_dir / "CHANGELOG"
    changelog_dir.mkdir(parents=True, exist_ok=True)
    yml_path = changelog_dir / f"CHANGELOG-{milestone_title}.yml"
    minor = minor_version_from_tag(milestone_title)
    md_path = changelog_dir / f"CHANGELOG-{minor}.md"
    yml_path.write_text(render_yaml(entries, milestone_title), encoding="utf-8")
    # Merge this milestone's block into the cumulative minor markdown so earlier
    # patch releases of the same minor are preserved.
    existing_md = md_path.read_text(encoding="utf-8") if md_path.is_file() else ""
    block = render_milestone_md_block(entries, milestone_title)
    md_path.write_text(
        merge_minor_markdown(existing_md, minor, milestone_title, block),
        encoding="utf-8",
    )
    log(f"Wrote {yml_path.relative_to(project_dir)} and {md_path.relative_to(project_dir)}.")
    return yml_path, md_path


def push_changelog_mr(
    project_dir: Path,
    project_path: str,
    server_host: str,
    token: str,
    api_base: str,
    project_id: str,
    milestone_title: str,
    milestone_id: int,
    base_branch: str,
    base_sha: str,
    files: list[Path],
    description: str,
) -> None:
    """Commit, push the branch, and open the changelog MR via the API.

    The MR is created through the REST API rather than `git push -o
    merge_request.*` push options because push options must be single-line
    (`fatal: push options must not have new line characters`) and the MR
    description is multi-line. A JSON API body has no such restriction.
    """
    branch = f"changelog/{milestone_title}"
    subprocess.check_call(["git", "config", "user.email", "ci-changelog@flant.com"], cwd=project_dir)
    subprocess.check_call(["git", "config", "user.name", "GitLab CI Changelog Bot"], cwd=project_dir)

    # Branch from the pipeline's original commit, not from wherever HEAD landed
    # after a previous milestone's commit, so each changelog/<ver> branch
    # carries only its own changes and never cascades onto another milestone.
    subprocess.check_call(["git", "checkout", "-B", branch, base_sha], cwd=project_dir)
    # Stage only this milestone's files (not `git add CHANGELOG/`, which would
    # sweep in skeleton files written for the other milestones in the loop).
    subprocess.check_call(["git", "add", *[str(f) for f in files]], cwd=project_dir)
    if subprocess.call(["git", "diff", "--cached", "--quiet"], cwd=project_dir) == 0:
        log("No staged changes; skipping commit and MR creation.")
        return

    # --signoff: the project push rule (DCO) rejects commits without a
    # Signed-off-by trailer; the committer identity set above supplies it.
    subprocess.check_call(
        ["git", "commit", "--signoff", "-m", f"Re-generate changelog {milestone_title}"],
        cwd=project_dir,
    )

    repo_url = f"https://oauth2:{token}@{server_host}/{project_path}.git"
    subprocess.check_call(["git", "remote", "set-url", "origin", repo_url], cwd=project_dir)
    subprocess.check_call(["git", "push", "--force", "origin", branch], cwd=project_dir)

    # An MR for this branch may already be open from a previous run; the
    # force-push above already refreshed it, so do not create a duplicate.
    existing = api_get_paginated(
        api_base,
        f"/projects/{project_id}/merge_requests",
        token,
        params={
            "source_branch": branch,
            "target_branch": base_branch,
            "state": "opened",
        },
    )
    if existing:
        log(f"MR !{existing[0]['iid']} already open for '{branch}'; branch updated.")
        return

    created = api_request(
        api_base,
        f"/projects/{project_id}/merge_requests",
        token,
        method="POST",
        data={
            "source_branch": branch,
            "target_branch": base_branch,
            "title": f"Changelog {milestone_title}",
            "description": description,
            "labels": "changelog,auto,status/backport",
            "milestone_id": milestone_id,
            "remove_source_branch": True,
        },
    )
    log(f"Pushed branch '{branch}' and opened MR !{created['iid']}.")


def main() -> int:
    api_base = require_env("CI_API_V4_URL").rstrip("/")
    project_id = require_env("CI_PROJECT_ID")
    token = require_env("GITLAB_API_TOKEN")
    project_path = require_env("CI_PROJECT_PATH")
    server_host = require_env("CI_SERVER_HOST")
    project_dir = Path(require_env("CI_PROJECT_DIR"))

    sections_path = Path(
        os.environ.get(
            "CHANGELOG_SECTIONS_FILE", ".gitlab/ci/changelog-sections.txt"
        )
    )
    if not sections_path.is_file():
        log(f"ERROR: sections file not found: {sections_path}")
        return 1
    # Map each section name to whether it forces low impact (':low' suffix, the
    # upstream 'section:forced_impact_level' format). Blocks use the bare name.
    allowed_sections: dict[str, bool] = {}
    for raw in sections_path.read_text(encoding="utf-8").splitlines():
        line = raw.strip()
        if not line or line.startswith("#"):
            continue
        name, _, suffix = line.partition(":")
        allowed_sections[name] = suffix == "low"

    base_branch = os.environ.get("CHANGELOG_BASE_BRANCH", "main")
    open_mr = os.environ.get("OPEN_CHANGELOG_MR", "false").lower() == "true"

    target_milestones: list[dict] = []
    explicit = os.environ.get("MILESTONE_TITLE", "").strip()
    if explicit:
        # Resolve to {title, iid}.
        all_ms = api_get_paginated(
            api_base,
            f"/projects/{project_id}/milestones",
            token,
            params={"state": "active", "per_page": "100"},
        )
        match = next((m for m in all_ms if m["title"] == explicit), None)
        if match is None:
            log(f"ERROR: milestone '{explicit}' not found among active milestones.")
            return 1
        target_milestones = [match]
    else:
        log("No MILESTONE_TITLE set — iterating over all active milestones.")
        target_milestones = api_get_paginated(
            api_base,
            f"/projects/{project_id}/milestones",
            token,
            params={"state": "active", "per_page": "100"},
        )

    if not target_milestones:
        log("No milestones to process. Exiting 0.")
        return 0

    # Commit that the pipeline checked out; every changelog branch is cut from
    # here so milestones stay isolated from each other.
    base_sha = subprocess.check_output(
        ["git", "rev-parse", "HEAD"], cwd=project_dir, text=True
    ).strip()

    overall_errors = 0
    for milestone in target_milestones:
        title = milestone["title"]
        iid = milestone["iid"]
        log(f"Processing milestone '{title}' (iid={iid})...")
        try:
            entries = collect_entries_for_milestone(
                api_base, project_id, title, token, allowed_sections
            )
        except urllib.error.HTTPError as exc:
            log(f"ERROR fetching MRs for {title}: HTTP {exc.code} {exc.reason}")
            overall_errors += 1
            continue

        yml_path, md_path = write_files(project_dir, title, entries)

        # Skip milestones with no changelog entries: opening an empty MR per
        # open milestone (e.g. one just created) is pure noise. When real
        # entries land, the next run creates the MR.
        if open_mr and entries:
            description = (
                f"## Changelog {title}\n\n"
                f"Auto-generated changelog covering milestone `{title}` "
                f"({len(entries)} change entries).\n\n"
                f"See:\n"
                f"- `{yml_path.relative_to(project_dir)}`\n"
                f"- `{md_path.relative_to(project_dir)}`\n"
            )
            try:
                push_changelog_mr(
                    project_dir=project_dir,
                    project_path=project_path,
                    server_host=server_host,
                    token=token,
                    api_base=api_base,
                    project_id=project_id,
                    milestone_title=title,
                    milestone_id=milestone["id"],
                    base_branch=base_branch,
                    base_sha=base_sha,
                    files=[yml_path, md_path],
                    description=description,
                )
            except (subprocess.CalledProcessError, urllib.error.HTTPError, urllib.error.URLError) as exc:
                log(f"ERROR pushing changelog MR for {title}: {exc}")
                overall_errors += 1
                continue

    return 1 if overall_errors else 0


if __name__ == "__main__":
    sys.exit(main())
