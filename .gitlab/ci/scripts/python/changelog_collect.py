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

Strategy chosen per migration plan §11.5.3 (Variant B - rewrite in python).

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
ALLOWED_TYPES = {"feature", "fix", "breaking", "chore", "docs", "refactor", "test"}


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


def parse_changes_block(block_text: str) -> dict[str, str] | None:
    fields: dict[str, str] = {}
    for raw_line in block_text.splitlines():
        match = KEY_VALUE_RE.match(raw_line.rstrip())
        if not match:
            continue
        key = match.group(1).strip().lower()
        value = match.group(2).strip()
        fields[key] = value
    required = {"section", "type", "summary"}
    if not required.issubset(fields):
        return None
    return fields


def collect_entries_for_milestone(
    api_base: str, project_id: str, milestone_title: str, token: str,
    allowed_sections: set[str],
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
        description = (mr.get("description") or "").strip()
        if not description:
            continue
        for raw_block in CHANGES_BLOCK_RE.findall(description):
            parsed = parse_changes_block(raw_block)
            if parsed is None:
                continue
            section = parsed["section"]
            if section not in allowed_sections:
                log(f"WARN: MR !{mr['iid']} uses unknown section '{section}', skipping.")
                continue
            # impact_level: if section has :low suffix, pin to low unless explicit.
            impact_level = parsed.get("impact_level", "")
            if ":" in section:
                impact_level = section.split(":", 1)[1]
            entries.append(
                {
                    "section": section,
                    "type": parsed["type"],
                    "summary": parsed["summary"],
                    "impact_level": impact_level or "high",
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


def render_yaml(entries: list[dict], milestone_title: str) -> str:
    grouped = group_entries(entries)
    lines = [f"# Changelog for {milestone_title}", ""]
    for section in sorted(grouped.keys()):
        section_entries = grouped[section]
        lines.append(f"## {section}")
        lines.append("")
        for entry in section_entries:
            lines.append(
                f"- **{entry['type']}** ({entry['impact_level']}): {entry['summary']} "
                f"(MR !{entry['mr_iid']})"
            )
        lines.append("")
    return "\n".join(lines).rstrip() + "\n"


def render_markdown(entries: list[dict], milestone_title: str, minor_version: str) -> str:
    grouped = group_entries(entries)
    lines = [
        f"# Changelog {minor_version}",
        "",
        f"Auto-generated summary for milestone `{milestone_title}`.",
        "",
    ]
    for section in sorted(grouped.keys()):
        lines.append(f"## {section}")
        lines.append("")
        for entry in grouped[section]:
            lines.append(
                f"- **{entry['type']}** ({entry['impact_level']}): "
                f"{entry['summary']} ([!{entry['mr_iid']}]({entry['mr_url']}))"
            )
        lines.append("")
    return "\n".join(lines).rstrip() + "\n"


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
    md_path.write_text(
        render_markdown(entries, milestone_title, minor), encoding="utf-8"
    )
    log(f"Wrote {yml_path.relative_to(project_dir)} and {md_path.relative_to(project_dir)}.")
    return yml_path, md_path


def push_changelog_mr(
    project_dir: Path,
    project_path: str,
    server_host: str,
    token: str,
    milestone_title: str,
    milestone_number: str,
    base_branch: str,
    pr_body_path: Path,
) -> None:
    """Commit, push, and open a changelog MR."""
    branch = f"changelog/{milestone_title}"
    subprocess.check_call(["git", "config", "user.email", "ci-changelog@flant.com"], cwd=project_dir)
    subprocess.check_call(["git", "config", "user.name", "GitLab CI Changelog Bot"], cwd=project_dir)

    subprocess.check_call(["git", "checkout", "-B", branch], cwd=project_dir)
    subprocess.check_call(["git", "add", "CHANGELOG/"], cwd=project_dir)
    if subprocess.call(["git", "diff", "--cached", "--quiet"], cwd=project_dir) == 0:
        log("No staged changes; skipping commit and MR creation.")
        return

    subprocess.check_call(
        ["git", "commit", "-m", f"Re-generate changelog {milestone_title}"],
        cwd=project_dir,
    )

    repo_url = f"https://oauth2:{token}@{server_host}/{project_path}.git"
    subprocess.check_call(["git", "remote", "set-url", "origin", repo_url], cwd=project_dir)

    push_cmd = [
        "git", "push", "--force",
        "-o", "merge_request.create",
        "-o", f"merge_request.target={base_branch}",
        "-o", f"merge_request.source={branch}",
        "-o", f"merge_request.title=Changelog {milestone_title}",
        "-o", f"merge_request.label=changelog",
        "-o", f"merge_request.label=auto",
        "-o", f"merge_request.label=status/backport",
        "-o", f"merge_request.milestone={milestone_number}",
        "-o", f"merge_request.description={pr_body_path.read_text(encoding='utf-8')}",
        "-o", "merge_request.remove_source_branch",
        "origin", branch,
    ]
    subprocess.check_call(push_cmd, cwd=project_dir)
    log(f"Pushed branch '{branch}' and opened MR via push options.")


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
    allowed_sections = {
        line.strip()
        for line in sections_path.read_text(encoding="utf-8").splitlines()
        if line.strip() and not line.startswith("#")
    }

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

        if open_mr:
            pr_body = (
                f"## Changelog {title}\n\n"
                f"Auto-generated changelog covering milestone `{title}` "
                f"({len(entries)} change entries).\n\n"
                f"See:\n"
                f"- `{yml_path.relative_to(project_dir)}`\n"
                f"- `{md_path.relative_to(project_dir)}`\n"
            )
            body_path = project_dir / "CHANGELOG" / f".mr-body-{title}.md"
            body_path.write_text(pr_body, encoding="utf-8")
            try:
                push_changelog_mr(
                    project_dir=project_dir,
                    project_path=project_path,
                    server_host=server_host,
                    token=token,
                    milestone_title=title,
                    milestone_number=str(iid),
                    base_branch=base_branch,
                    pr_body_path=body_path,
                )
            except subprocess.CalledProcessError as exc:
                log(f"ERROR pushing changelog MR for {title}: {exc}")
                overall_errors += 1
                continue
            finally:
                if body_path.exists():
                    body_path.unlink()

    return 1 if overall_errors else 0


if __name__ == "__main__":
    sys.exit(main())
