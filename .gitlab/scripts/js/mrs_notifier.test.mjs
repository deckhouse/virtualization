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

// Unit tests for mrs_notifier.mjs classification logic.
//
// mrs_notifier.mjs guards its `run()` call behind a direct-invocation check
// and validates env vars only inside `run()`, so it can be safely imported
// here without triggering network calls or process.exit. The pure helpers
// `classifyMR` and `extractUnresolvedThreads` are exercised directly with
// synthetic data — no network access required.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, statSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

import { classifyMR, extractUnresolvedThreads } from './mrs_notifier.mjs';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const MODULE_PATH = path.join(__dirname, 'mrs_notifier.mjs');

const STUCK_DAYS = 1.5;
const DAY_MS = 24 * 60 * 60 * 1000;

function author(id, username) {
  return { id, username, name: username };
}

function note({ author: a, createdAt, resolvable = true, resolved = false }) {
  return {
    author: a,
    created_at: createdAt,
    resolvable,
    resolved,
  };
}

// An ISO timestamp `days` days ago from now.
function daysAgo(days) {
  return new Date(Date.now() - days * DAY_MS).toISOString();
}

// ---- smoke / structural tests (kept from the original test file) ----

test('mrs_notifier.mjs exists and is non-empty', () => {
  const stat = statSync(MODULE_PATH);
  assert.ok(stat.isFile(), 'mrs_notifier.mjs should be a file');
  const src = readFileSync(MODULE_PATH, 'utf8');
  assert.ok(src.trim().length > 0, 'mrs_notifier.mjs should not be empty');
});

test('mrs_notifier.mjs is syntactically valid (node --check)', () => {
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

test('mrs_notifier.mjs does not fabricate a fake old date in classifyMR', () => {
  const src = readFileSync(MODULE_PATH, 'utf8');
  assert.ok(
    !/fakeDate/.test(src),
    'classifyMR must not use a fabricated fake date',
  );
});

// ---- classifyMR behavior ----

test('classifyMR: no unresolved, approved => ready_to_merge', () => {
  const approved = [author(1, 'alice')];
  assert.equal(classifyMR(approved, []), 'ready_to_merge');
});

test('classifyMR: no unresolved, no approved => review_required', () => {
  assert.equal(classifyMR([], []), 'review_required');
});

test('classifyMR: recent unresolved => changes_requested', () => {
  const threads = [{ author: author(2, 'bob'), timestamp: Date.now() }];
  assert.equal(classifyMR([], threads), 'changes_requested');
});

test('classifyMR: recent unresolved with approvals still changes_requested', () => {
  // Unresolved discussions take precedence over approvals, matching GitHub.
  const approved = [author(1, 'alice')];
  const threads = [{ author: author(2, 'bob'), timestamp: Date.now() }];
  assert.equal(classifyMR(approved, threads), 'changes_requested');
});

test('classifyMR: old unresolved => stuck', () => {
  const threads = [
    { author: author(2, 'bob'), timestamp: Date.now() - (STUCK_DAYS + 1) * DAY_MS },
  ];
  // Only assert STUCK when today is not Monday; on Monday the exception
  // kicks in and the result is changes_requested.
  if (new Date().getDay() !== 1) {
    assert.equal(classifyMR([], threads), 'stuck');
  } else {
    assert.equal(classifyMR([], threads), 'changes_requested');
  }
});

test('classifyMR: exactly STUCK_DAYS boundary is not stuck yet', () => {
  // timestamp + STUCK_DAYS == now => submittedAt < now is false => not stuck.
  const threads = [
    { author: author(2, 'bob'), timestamp: Date.now() - STUCK_DAYS * DAY_MS + 1000 },
  ];
  assert.equal(classifyMR([], threads), 'changes_requested');
});

test('classifyMR: no timestamp => changes_requested (never stuck)', () => {
  const threads = [{ author: author(2, 'bob'), timestamp: null }];
  assert.equal(classifyMR([], threads), 'changes_requested');
});

test('classifyMR: one old + one no-timestamp thread => stuck (when not Monday)', () => {
  const threads = [
    { author: author(2, 'bob'), timestamp: null },
    { author: author(3, 'carol'), timestamp: Date.now() - (STUCK_DAYS + 2) * DAY_MS },
  ];
  if (new Date().getDay() !== 1) {
    assert.equal(classifyMR([], threads), 'stuck');
  } else {
    assert.equal(classifyMR([], threads), 'changes_requested');
  }
});

// ---- extractUnresolvedThreads ----

test('extractUnresolvedThreads: skips fully resolved threads', () => {
  const discussions = [
    {
      notes: [
        note({ author: author(2, 'bob'), createdAt: daysAgo(5), resolved: true }),
      ],
    },
  ];
  assert.deepEqual(extractUnresolvedThreads(discussions), []);
});

test('extractUnresolvedThreads: skips non-resolvable threads', () => {
  const discussions = [
    {
      notes: [
        note({ author: author(2, 'bob'), createdAt: daysAgo(5), resolvable: false }),
      ],
    },
  ];
  assert.deepEqual(extractUnresolvedThreads(discussions), []);
});

test('extractUnresolvedThreads: returns earliest unresolved note timestamp', () => {
  const old = daysAgo(5);
  const recent = daysAgo(1);
  const discussions = [
    {
      notes: [
        note({ author: author(2, 'bob'), createdAt: recent, resolved: false }),
        note({ author: author(2, 'bob'), createdAt: old, resolved: false }),
      ],
    },
  ];
  const threads = extractUnresolvedThreads(discussions);
  assert.equal(threads.length, 1);
  assert.equal(threads[0].author.id, 2);
  assert.equal(threads[0].timestamp, new Date(old).getTime());
});

test('extractUnresolvedThreads: prefers unresolved note timestamp over resolved ones', () => {
  const resolvedOld = daysAgo(10);
  const unresolvedRecent = daysAgo(1);
  const discussions = [
    {
      notes: [
        note({ author: author(1, 'alice'), createdAt: resolvedOld, resolved: true }),
        note({ author: author(2, 'bob'), createdAt: unresolvedRecent, resolved: false }),
      ],
    },
  ];
  const threads = extractUnresolvedThreads(discussions);
  assert.equal(threads.length, 1);
  assert.equal(threads[0].author.id, 2);
  assert.equal(threads[0].timestamp, new Date(unresolvedRecent).getTime());
});

test('extractUnresolvedThreads: null timestamp when no created_at present', () => {
  const discussions = [
    {
      notes: [note({ author: author(2, 'bob'), createdAt: undefined, resolved: false })],
    },
  ];
  const threads = extractUnresolvedThreads(discussions);
  assert.equal(threads.length, 1);
  assert.equal(threads[0].timestamp, null);
});

test('extractUnresolvedThreads: empty / null input is safe', () => {
  assert.deepEqual(extractUnresolvedThreads([]), []);
  assert.deepEqual(extractUnresolvedThreads(null), []);
  assert.deepEqual(extractUnresolvedThreads(undefined), []);
});

test('extractUnresolvedThreads + classifyMR integration: old unresolved => stuck', () => {
  const discussions = [
    {
      notes: [
        note({
          author: author(2, 'bob'),
          createdAt: daysAgo(STUCK_DAYS + 1),
          resolved: false,
        }),
      ],
    },
  ];
  const threads = extractUnresolvedThreads(discussions);
  if (new Date().getDay() !== 1) {
    assert.equal(classifyMR([], threads), 'stuck');
  } else {
    assert.equal(classifyMR([], threads), 'changes_requested');
  }
});
