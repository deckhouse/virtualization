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

"""Unit tests for check_changelog_entry.py pure helpers.

The module guards main() behind `if __name__ == "__main__"`, so importing it
runs no network calls and never exits. We exercise the block parsing and
validation logic directly with synthetic MR-description text — no GitLab API
access required.
"""

import os
import sys
import tempfile
import unittest
from pathlib import Path

# Put the python/ dir (parent of tests/) on the path so the scripts under test
# import cleanly regardless of the discover invocation's cwd.
sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

import check_changelog_entry as cc  # noqa: E402


# A representative allowed-sections map used across the validation tests.
# Value = forced impact level ('' when the section forces nothing).
ALLOWED = {"vm": "", "core": "", "ci": "low"}


class FindBlocksTest(unittest.TestCase):
    def test_finds_single_block(self):
        text = "intro\n```changes\nsection: vm\n```\noutro"
        blocks = cc.CHANGES_BLOCK_RE.findall(text)
        self.assertEqual(len(blocks), 1)
        self.assertIn("section: vm", blocks[0])

    def test_finds_multiple_blocks(self):
        text = (
            "```changes\nsection: vm\n```\n"
            "text in between\n"
            "```changes\nsection: core\n```\n"
        )
        blocks = cc.CHANGES_BLOCK_RE.findall(text)
        self.assertEqual(len(blocks), 2)

    def test_no_block_returns_empty(self):
        self.assertEqual(cc.CHANGES_BLOCK_RE.findall("no fenced block here"), [])


class ParseBlockTest(unittest.TestCase):
    def test_parses_keys_lowercased(self):
        fields = cc.parse_block("Section: vm\nType: fix\nSummary: did a thing")
        self.assertEqual(
            fields, {"section": "vm", "type": "fix", "summary": "did a thing"}
        )

    def test_ignores_non_keyvalue_lines(self):
        fields = cc.parse_block("section: vm\nthis is prose\n\ntype: fix")
        self.assertEqual(fields, {"section": "vm", "type": "fix"})

    def test_trims_whitespace_around_value(self):
        fields = cc.parse_block("section:   vm   ")
        self.assertEqual(fields["section"], "vm")


class LoadAllowedSectionsTest(unittest.TestCase):
    def test_strips_comments_and_keeps_forced_level(self):
        with tempfile.TemporaryDirectory() as d:
            p = Path(d) / "sections.txt"
            p.write_text(
                "# a comment\n\nvm\ncore\n   \n# another\nci:low\n",
                encoding="utf-8",
            )
            self.assertEqual(
                cc.load_allowed_sections(p),
                {"vm": "", "core": "", "ci": "low"},
            )


class ParseEntriesTest(unittest.TestCase):
    def test_single_entry(self):
        entries = cc.parse_entries("section: vm\ntype: fix\nsummary: s")
        self.assertEqual(len(entries), 1)
        self.assertEqual(entries[0]["section"], "vm")

    def test_multi_doc_block(self):
        entries = cc.parse_entries(
            "section: vm\ntype: fix\nsummary: a\n---\n"
            "section: core\ntype: feature\nsummary: b"
        )
        self.assertEqual([e["section"] for e in entries], ["vm", "core"])

    def test_comma_separated_sections(self):
        entries = cc.parse_entries("section: vm, core\ntype: fix\nsummary: s")
        self.assertEqual([e["section"] for e in entries], ["vm", "core"])
        self.assertTrue(all(e["summary"] == "s" for e in entries))

    def test_v1_field_aliases(self):
        entries = cc.parse_entries(
            "module: vm\ntype: fix\ndescription: old style\nnote: flap expected"
        )
        self.assertEqual(entries[0]["section"], "vm")
        self.assertEqual(entries[0]["summary"], "old style")
        self.assertEqual(entries[0]["impact"], "flap expected")


class ValidateBlockTest(unittest.TestCase):
    def test_valid_block_has_no_errors(self):
        block = "section: vm\ntype: fix\nsummary: fixed it"
        self.assertEqual(cc.validate_block(1, block, ALLOWED), [])

    def test_impact_level_is_optional(self):
        # Upstream defaults an absent impact_level to "default".
        block = "section: vm\ntype: fix\nsummary: s"
        self.assertEqual(cc.validate_block(1, block, ALLOWED), [])

    def test_invalid_impact_level(self):
        block = "section: vm\ntype: fix\nsummary: s\nimpact_level: bogus"
        errors = cc.validate_block(1, block, ALLOWED)
        self.assertTrue(any("invalid impact level 'bogus'" in e for e in errors))

    def test_high_impact_requires_impact_detail(self):
        block = "section: vm\ntype: fix\nsummary: s\nimpact_level: high"
        errors = cc.validate_block(1, block, ALLOWED)
        self.assertTrue(any("missing high impact detail" in e for e in errors))

    def test_high_impact_with_detail_is_ok(self):
        block = (
            "section: vm\ntype: fix\nsummary: s\n"
            "impact_level: high\nimpact: nodes restart"
        )
        self.assertEqual(cc.validate_block(1, block, ALLOWED), [])

    def test_missing_section(self):
        block = "type: fix\nsummary: s"
        errors = cc.validate_block(1, block, ALLOWED)
        self.assertTrue(any("missing section" in e for e in errors))

    def test_section_not_allowed(self):
        block = "section: bogus\ntype: fix\nsummary: s"
        errors = cc.validate_block(1, block, ALLOWED)
        self.assertTrue(any("unknown section 'bogus'" in e for e in errors))

    def test_missing_type(self):
        block = "section: vm\nsummary: s"
        errors = cc.validate_block(1, block, ALLOWED)
        self.assertTrue(any("missing type" in e for e in errors))

    def test_chore_and_docs_types_are_accepted(self):
        # chore/docs are accepted (matching deckhouse/changelog-action@v2.6.0);
        # they are dropped from the public CHANGELOG-*.yml downstream, but must
        # not fail MR validation.
        for change_type in ("chore", "docs"):
            block = f"section: vm\ntype: {change_type}\nsummary: s"
            errors = cc.validate_block(1, block, ALLOWED)
            self.assertEqual(errors, [], f"type '{change_type}' should be accepted")

    def test_type_not_allowed(self):
        block = "section: vm\ntype: bogus\nsummary: s"
        errors = cc.validate_block(1, block, ALLOWED)
        self.assertTrue(any("invalid type 'bogus'" in e for e in errors))

    def test_allowed_types(self):
        self.assertEqual(cc.ALLOWED_TYPES, {"feature", "fix", "chore", "docs"})

    def test_missing_summary(self):
        block = "section: vm\ntype: fix"
        errors = cc.validate_block(1, block, ALLOWED)
        self.assertTrue(any("missing summary" in e for e in errors))

    def test_forced_low_section_may_omit_impact_level(self):
        block = "section: ci\ntype: fix\nsummary: s"
        self.assertEqual(cc.validate_block(1, block, ALLOWED), [])

    def test_forced_low_section_with_explicit_low_is_ok(self):
        block = "section: ci\ntype: fix\nsummary: s\nimpact_level: low"
        self.assertEqual(cc.validate_block(1, block, ALLOWED), [])

    def test_forced_low_silently_overrides_conflicting_impact(self):
        # Upstream ValidatorImpl overrides the level without an error, so a
        # forced-low section with impact_level: high passes (and needs no
        # 'impact' detail — the effective level is low).
        block = "section: ci\ntype: fix\nsummary: s\nimpact_level: high"
        self.assertEqual(cc.validate_block(1, block, ALLOWED), [])

    def test_low_suffix_in_block_is_unknown_section(self):
        # Upstream never strips ':low' from the block value; 'ci:low' is
        # simply not an allowed section name.
        block = "section: ci:low\ntype: fix\nsummary: s"
        errors = cc.validate_block(1, block, ALLOWED)
        self.assertTrue(any("unknown section 'ci:low'" in e for e in errors))

    def test_multi_entry_errors_are_prefixed_per_entry(self):
        block = (
            "section: vm\ntype: fix\nsummary: a\n---\n"
            "section: core\ntype: fix\n"
        )
        errors = cc.validate_block(2, block, ALLOWED)
        self.assertEqual(len(errors), 1)
        self.assertIn("block #2 entry #2: missing summary", errors[0])


if __name__ == "__main__":
    unittest.main()
