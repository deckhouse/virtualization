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

import contextlib
import io
import json
import tempfile
import unittest
from pathlib import Path

from e2e_report import charts


def write_json(path: Path, payload: object) -> None:
    path.write_text(json.dumps(payload), encoding="utf-8")


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
            write_json(
                report_path,
                {
                    "storageType": "nfs",
                    "specTimings": [
                        {"name": "fast", "group": "VM", "state": "passed", "runtimeMs": 10_000},
                        {"name": "slow", "group": "VM", "state": "passed", "runtimeMs": 90_000},
                        {"name": "disk", "group": "Disk", "state": "passed", "runtimeMs": 30_000},
                    ],
                },
            )

            rendered = charts.render_slowest_for_describe(report_path, "VM", out_dir=temp_dir)

            self.assertEqual(rendered["name"], "nfs-VM-slowest-specs.png")
            self.assertEqual(Path(rendered["path"]), Path(temp_dir) / rendered["name"])
            self.assertTrue(Path(rendered["path"]).is_file())

    def test_render_slowest_for_describe_lists_available_describes(self) -> None:
        with tempfile.TemporaryDirectory() as temp_dir:
            report_path = Path(temp_dir) / "e2e_report_nfs_2026-05-15.json"
            write_json(
                report_path,
                {
                    "specTimings": [
                        {"name": "disk", "group": "Disk", "state": "passed", "runtimeMs": 30_000},
                        {"name": "vm", "group": "VM", "state": "passed", "runtimeMs": 10_000},
                    ],
                },
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

    def test_render_feature_duration_status_writes_non_empty_png(self) -> None:
        with tempfile.TemporaryDirectory() as temp_dir:
            file = charts.render_feature_duration_status(
                {
                    "storageType": "replicated",
                    "specTimings": [
                        {"name": "slow", "group": "VM", "state": "passed", "runtimeMs": 90_000},
                    ],
                },
                Path(temp_dir),
            )

            self.assertGreater(Path(file["path"]).stat().st_size, 1000)

    def test_render_messenger_charts_writes_manifest(self) -> None:
        with tempfile.TemporaryDirectory() as temp_dir:
            reports_dir = Path(temp_dir) / "reports"
            out_dir = Path(temp_dir) / "charts"
            manifest_path = out_dir / "manifest.json"
            reports_dir.mkdir()
            write_json(
                reports_dir / "e2e_report_replicated.json",
                {
                    "storageType": "replicated",
                    "specTimings": [
                        {"name": "slow", "group": "VM", "state": "passed", "runtimeMs": 90_000},
                    ],
                },
            )

            manifest = charts.render_messenger_charts(reports_dir, out_dir, manifest_path)

            self.assertEqual(
                [file["name"] for file in manifest["clusters"]["replicated"]],
                ["replicated-feature-duration-status.png"],
            )
            self.assertEqual(json.loads(manifest_path.read_text(encoding="utf-8")), manifest)

    def test_report_cluster_key_prefers_storage_type(self) -> None:
        report = {"storageType": "storage-first", "cluster": "cluster-second"}

        self.assertEqual(charts.report_cluster_key(report), "storage-first")

    def test_render_messenger_charts_uses_storage_type_key(self) -> None:
        with tempfile.TemporaryDirectory() as temp_dir:
            reports_dir = Path(temp_dir) / "reports"
            out_dir = Path(temp_dir) / "charts"
            manifest_path = out_dir / "manifest.json"
            reports_dir.mkdir()
            write_json(
                reports_dir / "e2e_report_replicated.json",
                {
                    "storageType": "storage-first",
                    "cluster": "cluster-second",
                    "specTimings": [
                        {"name": "slow", "group": "VM", "state": "passed", "runtimeMs": 90_000},
                    ],
                },
            )

            manifest = charts.render_messenger_charts(reports_dir, out_dir, manifest_path)

            self.assertIn("storage-first", manifest["clusters"])
            self.assertNotIn("cluster-second", manifest["clusters"])


class RuntimeNsToMsTest(unittest.TestCase):
    def test_runtime_ns_to_ms(self) -> None:
        cases = [
            (1_500_000.0, 2),
            (1_000_000, 1),
            (-1, 0),
            (float("nan"), 0),
            (None, 0),
            ("bad", 0),
        ]

        for value, expected in cases:
            with self.subTest(value=value):
                self.assertEqual(charts.runtime_ns_to_ms(value), expected)


class ParseGinkgoReportTest(unittest.TestCase):
    def test_parse_ginkgo_report_filters_non_it_and_uses_default_group(self) -> None:
        payload = {
            "SpecReports": [
                {
                    "LeafNodeType": "It",
                    "LeafNodeText": "creates vm",
                    "ContainerHierarchyTexts": ["VM", "Nested"],
                    "State": "passed",
                    "RunTime": 1_500_000,
                },
                {
                    "LeafNodeType": "BeforeSuite",
                    "LeafNodeText": "setup",
                    "ContainerHierarchyTexts": ["Suite"],
                    "State": "failed",
                    "RunTime": 10_000_000,
                },
                {
                    "LeafNodeType": "It",
                    "LeafNodeText": "top level",
                    "ContainerHierarchyTexts": [],
                    "State": "pending",
                    "RunTime": 2_000_000,
                },
            ],
        }

        self.assertEqual(
            charts.parse_ginkgo_report(payload),
            [
                {"name": "creates vm", "group": "VM", "state": "passed", "runtimeMs": 2},
                {"name": "top level", "group": "Top-level Its", "state": "skipped", "runtimeMs": 2},
            ],
        )


class DeriveStorageTypeTest(unittest.TestCase):
    def test_derive_storage_type(self) -> None:
        self.assertEqual(
            charts.derive_storage_type("e2e_report_replicated_2026-05-15.json"),
            "replicated",
        )
        self.assertEqual(charts.derive_storage_type("e2e_report_nfs_local.json"), "nfs")
        self.assertEqual(charts.derive_storage_type("custom.json", "fallback"), "fallback")

        with self.assertRaisesRegex(ValueError, "Unable to derive storage type"):
            charts.derive_storage_type("custom.json")


class ListReportFilesTest(unittest.TestCase):
    def test_list_report_files_returns_sorted_matches(self) -> None:
        with tempfile.TemporaryDirectory() as temp_dir:
            root = Path(temp_dir)
            nested = root / "nested"
            nested.mkdir()
            write_json(nested / "e2e_report_z.json", {})
            write_json(root / "e2e_report_a.json", {})
            write_json(root / "other.json", {})
            (root / "e2e_report_dir.json").mkdir()

            self.assertEqual(
                [path.name for path in charts.list_report_files(root)],
                ["e2e_report_a.json", "e2e_report_z.json"],
            )


class RenderTopDescribesTest(unittest.TestCase):
    def test_render_top_describes_writes_expected_pngs(self) -> None:
        with tempfile.TemporaryDirectory() as temp_dir:
            root = Path(temp_dir)
            reports_dir = root / "reports"
            out_dir = root / "charts"
            reports_dir.mkdir()
            write_json(
                reports_dir / "e2e_report_nfs_2026-05-15.json",
                {
                    "storageType": "nfs",
                    "specTimings": [
                        {"name": "vm", "group": "VM", "state": "passed", "runtimeMs": 30_000},
                        {"name": "disk", "group": "Disk", "state": "passed", "runtimeMs": 20_000},
                        {"name": "net", "group": "Network", "state": "passed", "runtimeMs": 10_000},
                    ],
                },
            )

            files = charts.render_top_describes(reports_dir, out_dir, top_n=2)

            self.assertEqual(
                [file["name"] for file in files],
                ["nfs-VM-slowest-specs.png", "nfs-Disk-slowest-specs.png"],
            )
            self.assertTrue((out_dir / "nfs-VM-slowest-specs.png").is_file())


class MainDispatchTest(unittest.TestCase):
    def test_main_dispatches_messenger_all_slowest_and_top(self) -> None:
        with tempfile.TemporaryDirectory() as temp_dir:
            root = Path(temp_dir)
            reports_dir = root / "reports"
            messenger_out = root / "messenger"
            chart_out = root / "charts"
            manifest_path = messenger_out / "manifest.json"
            debug_path = root / "debug.json"
            report_path = reports_dir / "e2e_report_nfs_2026-05-15.json"
            reports_dir.mkdir()
            write_json(
                report_path,
                {
                    "storageType": "nfs",
                    "specTimings": [
                        {"name": "vm", "group": "VM", "state": "passed", "runtimeMs": 30_000},
                        {"name": "disk", "group": "Disk", "state": "passed", "runtimeMs": 20_000},
                    ],
                },
            )

            with contextlib.redirect_stdout(io.StringIO()):
                charts.main(
                    [
                        "messenger-all",
                        "--reports-dir",
                        str(reports_dir),
                        "--out-dir",
                        str(messenger_out),
                        "--manifest",
                        str(manifest_path),
                        "--debug-json",
                        str(debug_path),
                    ]
                )
                charts.main(
                    [
                        "slowest",
                        "--json",
                        str(report_path),
                        "--describe",
                        "VM",
                        "--out-dir",
                        str(chart_out),
                    ]
                )
                charts.main(
                    [
                        "top",
                        "--reports-dir",
                        str(reports_dir),
                        "--out-dir",
                        str(chart_out),
                        "--top-n",
                        "1",
                    ]
                )

            self.assertTrue(manifest_path.is_file())
            self.assertTrue(debug_path.is_file())
            self.assertTrue((chart_out / "nfs-VM-slowest-specs.png").is_file())

    def test_top_with_zero_matching_reports_fails_clearly(self) -> None:
        with tempfile.TemporaryDirectory() as temp_dir:
            with self.assertRaisesRegex(SystemExit, "No report files found"):
                charts.main(["top", "--reports-dir", temp_dir])

    def test_messenger_all_with_zero_matching_reports_fails_clearly(self) -> None:
        with tempfile.TemporaryDirectory() as temp_dir:
            with self.assertRaisesRegex(SystemExit, "No report files found"):
                charts.main(["messenger-all", "--reports-dir", temp_dir])


if __name__ == "__main__":
    unittest.main()
