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

import json
import tempfile
import unittest
from pathlib import Path

from e2e_report import charts


class ChartsTest(unittest.TestCase):
    def test_aggregate_normalizes_timings(self) -> None:
        timings, by_group = charts.aggregate(
            [
                {"name": "fast", "group": "VM", "state": "passed", "runtimeMs": 10_000},
                {"name": "bad", "group": "VM", "state": "unknown", "runtimeMs": "bad"},
            ]
        )

        self.assertEqual([timing.state for timing in timings], ["passed", "errors"])
        self.assertEqual(timings[1].runtime_ms, 0)
        self.assertEqual(by_group["VM"]["status_count"]["errors"], 1)

    def test_top_describes_uses_duration_desc_then_name(self) -> None:
        self.assertEqual(
            charts.top_describes(
                [
                    {"group": "VM", "runtimeMs": 30_000},
                    {"group": "Disk", "runtimeMs": 20_000},
                    {"group": "Network", "runtimeMs": 20_000},
                    {"group": "VM", "runtimeMs": 5_000},
                ],
                2,
            ),
            ["VM", "Disk"],
        )

    def test_render_slowest_for_describe_writes_expected_artifact(self) -> None:
        with tempfile.TemporaryDirectory() as temp_dir:
            report_path = Path(temp_dir) / "e2e_report_nfs_2026-05-15.json"
            report_path.write_text(
                json.dumps(
                    {
                        "storageType": "nfs",
                        "specTimings": [
                            {"name": "fast", "group": "VM", "state": "passed", "runtimeMs": 10_000},
                            {"name": "slow", "group": "VM", "state": "passed", "runtimeMs": 90_000},
                            {"name": "disk", "group": "Disk", "state": "passed", "runtimeMs": 30_000},
                        ],
                    }
                )
            )

            rendered = charts.render_slowest_for_describe(report_path, "VM", out_dir=temp_dir)

            self.assertEqual(rendered["name"], "nfs-VM-slowest-specs.png")
            self.assertEqual(Path(rendered["path"]), Path(temp_dir) / "charts" / rendered["name"])
            self.assertTrue(Path(rendered["path"]).is_file())

    def test_render_slowest_for_describe_lists_available_describes(self) -> None:
        with tempfile.TemporaryDirectory() as temp_dir:
            report_path = Path(temp_dir) / "e2e_report_nfs_2026-05-15.json"
            report_path.write_text(
                json.dumps(
                    {
                        "specTimings": [
                            {"name": "disk", "group": "Disk", "state": "passed", "runtimeMs": 30_000},
                            {"name": "vm", "group": "VM", "state": "passed", "runtimeMs": 10_000},
                        ],
                    }
                )
            )

            with self.assertRaisesRegex(ValueError, "Available Describes:\\n- Disk\\n- VM"):
                charts.render_slowest_for_describe(report_path, "Network", out_dir=temp_dir)

    def test_render_cluster_charts_writes_messenger_chart_only(self) -> None:
        with tempfile.TemporaryDirectory() as temp_dir:
            files = charts.render_cluster_charts(
                {
                    "cluster": "replicated",
                    "specTimings": [
                        {"name": "slow", "group": "VM", "state": "passed", "runtimeMs": 90_000},
                    ],
                },
                Path(temp_dir),
            )

            self.assertEqual([file["name"] for file in files], ["replicated-feature-duration-status.png"])
            self.assertTrue(Path(files[0]["path"]).is_file())

    def test_render_messenger_charts_writes_manifest(self) -> None:
        with tempfile.TemporaryDirectory() as temp_dir:
            reports_dir = Path(temp_dir) / "reports"
            out_dir = Path(temp_dir) / "charts"
            manifest_path = out_dir / "manifest.json"
            reports_dir.mkdir()
            (reports_dir / "e2e_report_replicated.json").write_text(
                json.dumps(
                    {
                        "storageType": "replicated",
                        "specTimings": [
                            {"name": "slow", "group": "VM", "state": "passed", "runtimeMs": 90_000},
                        ],
                    }
                )
            )

            manifest = charts.render_messenger_charts(reports_dir, out_dir, manifest_path)

            self.assertEqual(
                [file["name"] for file in manifest["clusters"]["replicated"]],
                ["replicated-feature-duration-status.png"],
            )
            self.assertEqual(json.loads(manifest_path.read_text()), manifest)


if __name__ == "__main__":
    unittest.main()
