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

"""Unit tests for changelog_collect.py pure helpers.

main() is guarded by `if __name__ == "__main__"`, so the import is side-effect
free. The network/git helpers (api_get_paginated, push_changelog_mr) are not
exercised here; everything below is the pure parse/group/render logic that
produces the deckhouse-schema CHANGELOG-*.yml and the .md summary.
"""

import os
import sys
import unittest

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

import changelog_collect as cl  # noqa: E402


def entry(section, type_, summary, mr_iid, impact_level="high", mr_url=None, impact=""):
    return {
        "section": section,
        "type": type_,
        "summary": summary,
        "impact": impact,
        "impact_level": impact_level,
        "mr_iid": mr_iid,
        "mr_title": f"MR {mr_iid}",
        "mr_url": mr_url or f"https://gl/-/merge_requests/{mr_iid}",
        "author": "alice",
    }


class NextLinkTest(unittest.TestCase):
    def test_extracts_next_url(self):
        header = (
            '<https://gl/api/v4/x?page=1>; rel="prev", '
            '<https://gl/api/v4/x?page=3>; rel="next"'
        )
        self.assertEqual(cl.next_link(header), "https://gl/api/v4/x?page=3")

    def test_no_next_returns_empty(self):
        header = '<https://gl/api/v4/x?page=1>; rel="prev"'
        self.assertEqual(cl.next_link(header), "")

    def test_empty_header(self):
        self.assertEqual(cl.next_link(""), "")


class ParseChangesBlockTest(unittest.TestCase):
    def test_parses_required_keys(self):
        parsed = cl.parse_changes_block("section: vm\ntype: fix\nsummary: did it")
        self.assertEqual(parsed["section"], "vm")
        self.assertEqual(parsed["type"], "fix")
        self.assertEqual(parsed["summary"], "did it")

    def test_returns_none_when_required_key_missing(self):
        # summary missing -> not a valid changes block.
        self.assertIsNone(cl.parse_changes_block("section: vm\ntype: fix"))

    def test_keeps_optional_keys(self):
        parsed = cl.parse_changes_block(
            "section: vm\ntype: fix\nsummary: s\nimpact_level: low"
        )
        self.assertEqual(parsed["impact_level"], "low")

    def test_multiline_impact_is_preserved(self):
        parsed = cl.parse_changes_block(
            "section: core\ntype: feature\nsummary: containerd v2\n"
            "impact: First line.\nSecond line.\nimpact_level: high"
        )
        self.assertEqual(parsed["impact"], "First line.\nSecond line.")
        self.assertEqual(parsed["impact_level"], "high")
        self.assertEqual(parsed["summary"], "containerd v2")


class HasLabelTest(unittest.TestCase):
    def test_string_labels(self):
        self.assertTrue(cl.has_label({"labels": ["changelog", "auto"]}, "changelog"))
        self.assertFalse(cl.has_label({"labels": ["auto"]}, "changelog"))

    def test_dict_labels(self):
        self.assertTrue(
            cl.has_label({"labels": [{"name": "changelog"}]}, "changelog")
        )

    def test_missing_labels_key(self):
        self.assertFalse(cl.has_label({}, "changelog"))


class GroupEntriesTest(unittest.TestCase):
    def test_groups_by_section(self):
        entries = [
            entry("vm", "fix", "a", 1),
            entry("vm", "feature", "b", 2),
            entry("core", "fix", "c", 3),
        ]
        grouped = cl.group_entries(entries)
        self.assertEqual(len(grouped["vm"]), 2)
        self.assertEqual(len(grouped["core"]), 1)


class YamlSummaryScalarTest(unittest.TestCase):
    def test_plain_when_safe(self):
        self.assertEqual(cl.yaml_summary_scalar("simple summary"), "simple summary")

    def test_empty_is_quoted(self):
        self.assertEqual(cl.yaml_summary_scalar(""), '""')

    def test_colon_space_is_quoted(self):
        self.assertEqual(cl.yaml_summary_scalar("fix: thing"), '"fix: thing"')

    def test_leading_special_char_is_quoted(self):
        self.assertEqual(cl.yaml_summary_scalar("- dash start"), '"- dash start"')

    def test_trailing_space_is_quoted(self):
        self.assertEqual(cl.yaml_summary_scalar("trailing "), '"trailing "')

    def test_hash_comment_is_quoted(self):
        self.assertEqual(cl.yaml_summary_scalar("note #5"), '"note #5"')


class RenderYamlTest(unittest.TestCase):
    def test_empty_entries_render_empty_mapping(self):
        self.assertEqual(cl.render_yaml([], "v1.21.0"), "{}\n\n")

    def test_groups_into_features_and_fixes(self):
        entries = [
            entry("vm", "feature", "added X", 10),
            entry("vm", "fix", "fixed Y", 11),
        ]
        out = cl.render_yaml(entries, "v1.21.0")
        self.assertIn("vm:", out)
        self.assertIn("  features:", out)
        self.assertIn("  fixes:", out)
        self.assertIn("    - summary: added X", out)
        self.assertIn("      pull_request: https://gl/-/merge_requests/10", out)

    def test_entries_ordered_by_mr_iid_descending(self):
        entries = [
            entry("vm", "fix", "older", 5),
            entry("vm", "fix", "newer", 9),
        ]
        out = cl.render_yaml(entries, "v1.21.0")
        self.assertLess(out.index("newer"), out.index("older"))

    def test_sections_sorted_alphabetically(self):
        entries = [
            entry("vm", "fix", "v", 1),
            entry("core", "fix", "c", 2),
        ]
        out = cl.render_yaml(entries, "v1.21.0")
        self.assertLess(out.index("core:"), out.index("vm:"))

    def test_low_suffix_stripped_from_section_key(self):
        entries = [entry("ci:low", "fix", "tweak", 1, impact_level="low")]
        out = cl.render_yaml(entries, "v1.21.0")
        self.assertIn("ci:", out)
        self.assertNotIn("ci:low:", out)

    def test_unsupported_type_is_skipped(self):
        # 'chore' has no features/fixes bucket -> dropped from yaml output.
        entries = [entry("vm", "chore", "noise", 1)]
        self.assertEqual(cl.render_yaml(entries, "v1.21.0"), "{}\n\n")

    def test_single_line_impact_emitted_after_pull_request(self):
        entries = [entry("core", "feature", "containerd v2", 9, impact="Recreate images.")]
        out = cl.render_yaml(entries, "v1.21.0")
        self.assertIn("      pull_request: https://gl/-/merge_requests/9", out)
        self.assertIn("      impact: Recreate images.", out)

    def test_multiline_impact_emitted_as_literal_block(self):
        entries = [entry("core", "feature", "containerd v2", 9, impact="L1\nL2")]
        out = cl.render_yaml(entries, "v1.21.0")
        self.assertIn("      impact: |-", out)
        self.assertIn("        L1", out)
        self.assertIn("        L2", out)

    def test_no_impact_means_no_impact_line(self):
        entries = [entry("vm", "fix", "fixed Y", 11)]
        out = cl.render_yaml(entries, "v1.21.0")
        self.assertNotIn("impact:", out)


class RenderMilestoneMdBlockTest(unittest.TestCase):
    def test_basic_structure(self):
        entries = [entry("vm", "fix", "fixed Y", 11, impact_level="high")]
        out = cl.render_milestone_md_block(entries, "v1.21.0")
        self.assertIn("## v1.21.0", out)
        self.assertIn("### vm", out)
        self.assertIn("**fix** (high): fixed Y ([!11]", out)

    def test_empty_entries_render_placeholder(self):
        out = cl.render_milestone_md_block([], "v1.21.0")
        self.assertIn("## v1.21.0", out)
        self.assertIn("_No changelog entries._", out)


class MergeMinorMarkdownTest(unittest.TestCase):
    def test_new_file_creates_header_and_block(self):
        block = cl.render_milestone_md_block(
            [entry("vm", "fix", "a", 1)], "v1.21.0"
        )
        out = cl.merge_minor_markdown("", "v1.21", "v1.21.0", block)
        self.assertIn("# Changelog v1.21", out)
        self.assertIn("## v1.21.0", out)

    def test_existing_patch_preserved_and_sorted_desc(self):
        first = cl.merge_minor_markdown(
            "", "v1.21", "v1.21.0",
            cl.render_milestone_md_block([entry("vm", "fix", "older", 1)], "v1.21.0"),
        )
        second = cl.merge_minor_markdown(
            first, "v1.21", "v1.21.1",
            cl.render_milestone_md_block([entry("vm", "fix", "newer", 2)], "v1.21.1"),
        )
        # Both patch blocks are present (cumulative)...
        self.assertIn("## v1.21.0", second)
        self.assertIn("## v1.21.1", second)
        self.assertIn("older", second)
        self.assertIn("newer", second)
        # ...and the newer patch is listed first.
        self.assertLess(second.index("## v1.21.1"), second.index("## v1.21.0"))

    def test_regenerating_same_milestone_is_idempotent(self):
        block_v0 = cl.render_milestone_md_block(
            [entry("vm", "fix", "a", 1)], "v1.21.0"
        )
        once = cl.merge_minor_markdown("", "v1.21", "v1.21.0", block_v0)
        twice = cl.merge_minor_markdown(once, "v1.21", "v1.21.0", block_v0)
        self.assertEqual(once, twice)


class MinorVersionFromTagTest(unittest.TestCase):
    def test_patch_version_truncated_to_minor(self):
        self.assertEqual(cl.minor_version_from_tag("v1.21.3"), "v1.21")

    def test_minor_version_unchanged(self):
        self.assertEqual(cl.minor_version_from_tag("v1.21"), "v1.21")

    def test_non_matching_returned_as_is(self):
        self.assertEqual(cl.minor_version_from_tag("nightly"), "nightly")


if __name__ == "__main__":
    unittest.main()
