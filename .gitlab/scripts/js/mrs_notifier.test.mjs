// Copyright 2026 Flant JSC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Minimal smoke test for mrs_notifier.mjs.
//
// mrs_notifier.mjs is a script that auto-runs `run()` at import time and
// exits when required env vars are missing, so it cannot be safely imported
// by a unit test without a refactor. This smoke test therefore:
//   - asserts the file exists and is non-empty;
//   - syntax-checks it with `node --check` (no execution, no side effects);
//   - asserts the expected entry points and env-var contract are present.
//
// TODO: refactor mrs_notifier.mjs to export pure helpers (e.g. classifyMR)
// and guard the `run()` call behind a direct-invocation check, then add real
// unit tests over the classification logic here.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, statSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const MODULE_PATH = path.join(__dirname, 'mrs_notifier.mjs');

test('mrs_notifier.mjs exists and is non-empty', () => {
  const stat = statSync(MODULE_PATH);
  assert.ok(stat.isFile(), 'mrs_notifier.mjs should be a file');
  const src = readFileSync(MODULE_PATH, 'utf8');
  assert.ok(src.trim().length > 0, 'mrs_notifier.mjs should not be empty');
});

test('mrs_notifier.mjs is syntactically valid (node --check)', () => {
  // Syntax-check without executing: avoids env-var / network side effects.
  execFileSync('node', ['--check', MODULE_PATH], { stdio: 'pipe' });
});

test('mrs_notifier.mjs declares the expected entry points', () => {
  const src = readFileSync(MODULE_PATH, 'utf8');
  for (const name of ['fetchOpenMRs', 'classifyMR', 'buildSummary', 'run']) {
    assert.ok(
      src.includes(`function ${name}`) || src.includes(`${name}(`),
      `expected function ${name} to be defined`,
    );
  }
});

test('mrs_notifier.mjs references the documented env-var contract', () => {
  const src = readFileSync(MODULE_PATH, 'utf8');
  for (const envVar of ['GITLAB_API_TOKEN', 'CI_API_V4_URL', 'CI_PROJECT_ID', 'LOOP_WEBHOOK_URL']) {
    assert.ok(src.includes(envVar), `expected reference to ${envVar}`);
  }
});
