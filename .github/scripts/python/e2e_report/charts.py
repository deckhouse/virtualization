# Copyright 2026 Flant JSC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#     http://www.apache.org/licenses/LICENSE-2.0
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

from __future__ import annotations

import argparse
import json
import math
import re
from dataclasses import dataclass
from pathlib import Path
from typing import Any

import matplotlib

matplotlib.use("Agg")

import matplotlib.pyplot as plt  # noqa: E402
from matplotlib.ticker import FuncFormatter, MultipleLocator  # noqa: E402


STATUSES = ("passed", "failed", "errors", "skipped")
STATUS_COLORS = {
    "passed": "#00b83f",
    "failed": "#ff3333",
    "errors": "#d9a300",
    "skipped": "#8f9aa3",
}
DURATION_COLORS = {
    "fast": "#7ee787",
    "medium": "#3fb950",
    "slow": "#238636",
}
DEFAULT_TOP_N = 15
REPORT_FILE_PATTERN = re.compile(r"^e2e_report_.*\.json$")


@dataclass(frozen=True)
class Timing:
    name: str
    group: str
    state: str
    runtime_ms: float

    @property
    def full_name(self) -> str:
        return self.name if self.group == self.name else f"{self.group} / {self.name}"


def sanitize_filename_part(value: Any) -> str:
    safe = re.sub(r"[^a-zA-Z0-9_-]+", "_", str(value or "cluster"))
    return safe or "cluster"


def to_seconds(ms: float) -> float:
    return round(float(ms) / 1000, 2)


def normalize_timing(raw: dict[str, Any] | None) -> Timing:
    raw = raw or {}
    state = str(raw.get("state") or "errors")
    if state not in STATUSES:
        state = "errors"

    runtime = raw.get("runtimeMs", 0)
    try:
        runtime_ms = float(runtime)
    except (TypeError, ValueError):
        runtime_ms = 0
    if not math.isfinite(runtime_ms) or runtime_ms < 0:
        runtime_ms = 0

    return Timing(
        name=str(raw.get("name") or "Unnamed spec"),
        group=str(raw.get("group") or "Top-level Its"),
        state=state,
        runtime_ms=runtime_ms,
    )


def aggregate(spec_timings: list[dict[str, Any]] | None) -> tuple[list[Timing], dict[str, dict[str, Any]]]:
    timings: list[Timing] = []
    by_group: dict[str, dict[str, Any]] = {}

    for raw_timing in spec_timings or []:
        timing = normalize_timing(raw_timing)
        timings.append(timing)
        group = by_group.setdefault(
            timing.group,
            {
                "status_count": {status: 0 for status in STATUSES},
                "status_durations": {status: 0.0 for status in STATUSES},
                "total": 0.0,
            },
        )
        group["status_count"][timing.state] += 1
        group["status_durations"][timing.state] += timing.runtime_ms
        group["total"] += timing.runtime_ms

    return timings, by_group


def duration_bucket(timing: Timing) -> str:
    if timing.runtime_ms > 300_000:
        return "slow"
    if timing.runtime_ms >= 60_000:
        return "medium"
    return "fast"


def format_seconds(seconds: float) -> str:
    return f"{seconds:.0f}s" if seconds >= 10 else f"{seconds:.1f}s"


def format_axis_seconds(value: float, _position: int) -> str:
    return f"{int(value):,}"


def next_tick(value: float, step: int) -> int:
    if value <= 0:
        return step
    return int(math.ceil(value / step) * step)


def render_feature_duration_status(report: dict[str, Any], output_dir: Path) -> dict[str, str]:
    _, by_group = aggregate(report.get("specTimings") or [])
    entries = sorted(
        by_group.items(),
        key=lambda item: (
            -(item[1]["status_count"]["failed"] + item[1]["status_count"]["errors"]),
            -item[1]["total"],
            item[0],
        ),
    )
    labels = [name for name, _ in entries]
    height = max(6.4, 0.75 + len(labels) * 0.285)
    fig, ax = plt.subplots(figsize=(10.24, height), dpi=100)
    left = [0.0] * len(entries)

    for status in STATUSES:
        values = [to_seconds(group["status_durations"][status]) for _, group in entries]
        ax.barh(labels, values, left=left, label=status, color=STATUS_COLORS[status], height=0.72)
        for row, (offset, value) in enumerate(zip(left, values)):
            if value <= 0:
                continue
            ax.text(
                offset + value / 2,
                row,
                format_seconds(value),
                ha="center",
                va="center",
                fontsize=6,
                color="#333333",
            )
        left = [current + value for current, value in zip(left, values)]

    x_limit = next_tick(max(left, default=0), 60)
    ax.set_xlim(0, x_limit)
    ax.set_title("Overall durations for Describes", fontsize=8, pad=12)
    ax.set_xlabel("Duration, seconds", fontsize=7)
    ax.invert_yaxis()
    if labels:
        ax.set_ylim(len(labels) - 0.6, -0.6)
    ax.legend(
        loc="upper center",
        bbox_to_anchor=(0.5, 1.015),
        ncol=len(STATUSES),
        frameon=False,
        fontsize=6,
        handlelength=2.4,
        columnspacing=0.8,
    )
    ax.margins(y=0)
    ax.xaxis.set_major_locator(MultipleLocator(60))
    ax.xaxis.set_major_formatter(FuncFormatter(format_axis_seconds))
    ax.grid(axis="x", color="#c8c8c8", alpha=0.45, linewidth=0.5)
    ax.grid(axis="y", color="#d9d9d9", alpha=0.55, linewidth=0.5)
    ax.set_axisbelow(True)
    ax.tick_params(axis="both", labelsize=6, colors="#555555", length=0)
    for spine in ax.spines.values():
        spine.set_color("#dddddd")
        spine.set_linewidth(0.5)

    cluster_name = sanitize_filename_part(report.get("cluster") or report.get("storageType") or "cluster")
    return save_figure(fig, output_dir / f"{cluster_name}-feature-duration-status.png")


def render_slowest_specs(
    spec_timings: list[dict[str, Any]],
    storage_name: str,
    describe: str,
    output_dir: Path,
    top_n: int = DEFAULT_TOP_N,
) -> dict[str, str]:
    timings, _ = aggregate(spec_timings)
    top = sorted(timings, key=lambda timing: (-timing.runtime_ms, timing.full_name))[:top_n]
    labels = [timing.full_name for timing in top]
    values = [to_seconds(timing.runtime_ms) for timing in top]
    colors = [DURATION_COLORS[duration_bucket(timing)] for timing in top]
    edge_colors = [
        STATUS_COLORS[timing.state] if timing.state in {"failed", "errors"} else "none"
        for timing in top
    ]
    line_widths = [2 if timing.state in {"failed", "errors"} else 0 for timing in top]

    fig, ax = plt.subplots(figsize=(20.48, 7.2), dpi=100)
    ax.barh(labels, values, color=colors, edgecolor=edge_colors, linewidth=line_widths)
    ax.set_title("Top slowest successful specs and failed specs (It/Entry)")
    ax.set_xlabel("Duration, seconds")
    ax.invert_yaxis()
    ax.grid(axis="x", alpha=0.2)

    for row, (seconds, timing) in enumerate(zip(values, top)):
        suffix = f" [{timing.state}]" if timing.state in {"failed", "errors"} else ""
        ax.text(seconds, row, f" {format_seconds(seconds)}{suffix}", va="center", fontsize=8)

    file_name = (
        f"{sanitize_filename_part(storage_name)}-"
        f"{sanitize_filename_part(describe)}-slowest-specs.png"
    )
    return save_figure(fig, output_dir / file_name)


def save_figure(fig: plt.Figure, target_path: Path) -> dict[str, str]:
    target_path.parent.mkdir(parents=True, exist_ok=True)
    fig.tight_layout()
    fig.savefig(target_path, format="png")
    plt.close(fig)
    return {
        "name": target_path.name,
        "path": str(target_path),
        "mimeType": "image/png",
    }


def runtime_ms(value: Any) -> int:
    try:
        runtime = float(value or 0)
    except (TypeError, ValueError):
        return 0
    return round(runtime / 1_000_000) if math.isfinite(runtime) else 0


def metric_key_for_state(state: Any) -> str:
    normalized = str(state or "").strip().lower()
    if normalized in {"passed", "failed"}:
        return normalized
    if normalized in {"skipped", "pending"}:
        return "skipped"
    return "errors"


def parse_ginkgo_report(payload: Any) -> list[dict[str, Any]]:
    suites = payload if isinstance(payload, list) else [payload]
    timings: list[dict[str, Any]] = []

    for suite in suites:
        if not isinstance(suite, dict):
            continue
        for spec_report in suite.get("SpecReports") or []:
            if not isinstance(spec_report, dict) or spec_report.get("LeafNodeType") != "It":
                continue
            hierarchy = [
                str(part).strip()
                for part in spec_report.get("ContainerHierarchyTexts") or []
                if str(part).strip()
            ]
            timings.append(
                {
                    "name": str(spec_report.get("LeafNodeText") or "").strip(),
                    "group": hierarchy[0] if hierarchy else "Top-level Its",
                    "state": metric_key_for_state(spec_report.get("State")),
                    "runtimeMs": runtime_ms(spec_report.get("RunTime")),
                }
            )

    return timings


def read_report(json_path: str | Path) -> dict[str, Any]:
    path = Path(json_path)
    payload = json.loads(path.read_text())
    if isinstance(payload, dict) and isinstance(payload.get("specTimings"), list):
        return payload
    return {"specTimings": parse_ginkgo_report(payload)}


def available_describes(spec_timings: list[dict[str, Any]]) -> list[str]:
    return sorted(
        {
            str(timing.get("group") or "").strip()
            for timing in spec_timings
            if str(timing.get("group") or "").strip()
        }
    )


def derive_storage_type(report_path: str | Path, fallback_storage: str | None = None) -> str:
    base_name = Path(report_path).name
    dated_match = re.match(r"^e2e_report_(.+)_(\d{4}-\d{2}-\d{2}.*)\.json$", base_name)
    if dated_match:
        return dated_match.group(1)
    generic_match = re.match(r"^e2e_report_(.+?)_.*\.json$", base_name)
    if generic_match:
        return generic_match.group(1)
    if fallback_storage:
        return fallback_storage
    raise ValueError(f'Unable to derive storage type from file name "{base_name}". Pass --storage.')


def report_cluster_key(report: dict[str, Any]) -> str:
    return str(report.get("cluster") or report.get("storageType") or "").strip()


def top_describes(spec_timings: list[dict[str, Any]] | None, top_n: int = 5) -> list[str]:
    totals: dict[str, float] = {}
    for raw_timing in spec_timings or []:
        group = str(raw_timing.get("group") or "").strip()
        if not group:
            continue
        try:
            runtime = float(raw_timing.get("runtimeMs") or 0)
        except (TypeError, ValueError):
            runtime = 0
        totals[group] = totals.get(group, 0) + runtime

    return [name for name, _ in sorted(totals.items(), key=lambda item: (-item[1], item[0]))[:top_n]]


def render_cluster_charts(report: dict[str, Any], output_dir: Path) -> list[dict[str, str]]:
    if not report.get("specTimings"):
        return []
    return [render_feature_duration_status(report, output_dir)]


def render_messenger_charts(
    reports_dir: str | Path = "downloaded-artifacts",
    out_dir: str | Path = "tmp/messenger-charts",
    manifest_path: str | Path = "tmp/messenger-charts/manifest.json",
) -> dict[str, dict[str, list[dict[str, str]]]]:
    output_dir = Path(out_dir)
    clusters: dict[str, list[dict[str, str]]] = {}

    for report_file in list_report_files(reports_dir):
        report = read_report(report_file)
        cluster_name = report_cluster_key(report) or derive_storage_type(report_file)
        files = render_cluster_charts(report, output_dir)
        if files:
            clusters[cluster_name] = files

    manifest = {"clusters": clusters}
    target_path = Path(manifest_path)
    target_path.parent.mkdir(parents=True, exist_ok=True)
    target_path.write_text(json.dumps(manifest, indent=2, sort_keys=True))
    return manifest


def render_slowest_for_describe(
    json_path: str | Path,
    describe: str,
    out_dir: str | Path = "tmp",
    storage: str | None = None,
) -> dict[str, str]:
    if not describe:
        raise ValueError("--describe is required")

    report = read_report(json_path)
    spec_timings = report.get("specTimings") or []
    filtered_timings = [
        timing for timing in spec_timings if str(timing.get("group") or "") == describe
    ]
    if not filtered_timings:
        describes = available_describes(spec_timings)
        lines = [
            f'No specs found for Describe "{describe}".',
            "Available Describes:",
            *(f"- {name}" for name in describes or ["<none>"]),
        ]
        raise ValueError("\n".join(lines))

    storage_name = (
        storage
        or str(report.get("storageType") or report.get("cluster") or "").strip()
        or derive_storage_type(json_path)
    )
    return render_slowest_specs(filtered_timings, storage_name, describe, Path(out_dir) / "charts")


def list_report_files(reports_dir: str | Path) -> list[Path]:
    root = Path(reports_dir)
    if not root.exists():
        return []
    return sorted(path for path in root.rglob("*") if path.is_file() and REPORT_FILE_PATTERN.match(path.name))


def render_top_describes(
    reports_dir: str | Path = "downloaded-artifacts",
    out_dir: str | Path = "tmp",
    top_n: int = 5,
) -> list[dict[str, str]]:
    rendered_files: list[dict[str, str]] = []
    for report_file in list_report_files(reports_dir):
        report = read_report(report_file)
        storage_name = report_cluster_key(report) or derive_storage_type(report_file)
        for describe in top_describes(report.get("specTimings") or [], top_n):
            rendered_files.append(
                render_slowest_for_describe(
                    report_file,
                    describe,
                    out_dir=out_dir,
                    storage=storage_name,
                )
            )
    return rendered_files


def print_json(files: list[dict[str, str]]) -> None:
    print(json.dumps(files, separators=(",", ":")))


def main() -> None:
    parser = argparse.ArgumentParser(description="Render E2E report charts")
    subparsers = parser.add_subparsers(dest="command", required=True)

    messenger = subparsers.add_parser("messenger", help="Render charts for one messenger report")
    messenger.add_argument("--json", required=True)
    messenger.add_argument("--out-dir", default="tmp")

    messenger_all = subparsers.add_parser("messenger-all", help="Render messenger charts and write a manifest")
    messenger_all.add_argument("--reports-dir", default="downloaded-artifacts")
    messenger_all.add_argument("--out-dir", default="tmp/messenger-charts")
    messenger_all.add_argument("--manifest", default="tmp/messenger-charts/manifest.json")

    slowest = subparsers.add_parser("slowest", help="Render slowest specs for one Describe")
    slowest.add_argument("--json", required=True)
    slowest.add_argument("--describe", required=True)
    slowest.add_argument("--out-dir", default="tmp")
    slowest.add_argument("--storage")

    top = subparsers.add_parser("top", help="Render slowest specs for top-N Describes")
    top.add_argument("--reports-dir", default="downloaded-artifacts")
    top.add_argument("--out-dir", default="tmp")
    top.add_argument("--top-n", type=int, default=5)

    args = parser.parse_args()

    if args.command == "messenger":
        print_json(render_cluster_charts(read_report(args.json), Path(args.out_dir)))
        return
    if args.command == "messenger-all":
        render_messenger_charts(args.reports_dir, args.out_dir, args.manifest)
        print(args.manifest)
        return
    if args.command == "slowest":
        file = render_slowest_for_describe(args.json, args.describe, args.out_dir, args.storage)
        print(file["path"])
        return
    if args.command == "top":
        files = render_top_describes(args.reports_dir, args.out_dir, args.top_n)
        for file in files:
            print(file["path"])


if __name__ == "__main__":
    main()
