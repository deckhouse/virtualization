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

// GitLab counterpart of .github/scripts/prs_notifier.mjs.
//
// Reads open MRs from GitLab via REST API, classifies them into
// {ready_to_merge, stuck, changes_requested, review_required},
// and POSTs a markdown summary to LOOP_WEBHOOK_URL.
//
// Environment:
//   GITLAB_API_TOKEN  (required)  Project Access Token, scope api.
//   CI_API_V4_URL     (required)  e.g. https://fox.flant.com/api/v4
//   CI_PROJECT_ID     (required)  numeric project id (use the variable,
//                                 not the slug, to survive renames).
//   LOOP_WEBHOOK_URL  (required)  Loop incoming webhook URL.
//   DOC_REVIEWER      (optional)  GitLab username of doc reviewer.
//                                 Default "z9r5" (TODO: confirm GitLab username).
//   MANAGER_LOOP_NAME (optional)  @firstname.lastname of the manager.
//                                 Default "@yuriy.milyutin".
//
// Mapping cheat-sheet (per migration plan §11.12.2):
//   octokit            -> axios with PRIVATE-TOKEN
//   pr.draft           -> mr.draft (or mr.work_in_progress for older GitLab)
//   pr.head.ref        -> mr.source_branch
//   pr.labels[].name   -> mr.labels[]
//   pr.assignees[]     -> mr.assignees[]
//   pr.requested_reviewers[] -> mr.reviewers[]
//   pr.html_url        -> mr.web_url
//   review state CHANGES_REQUESTED -> unresolved discussion threads.

import axios from 'axios';
import moment from 'moment';
import { fileURLToPath } from 'node:url';

const PROJECT_ID = process.env.CI_PROJECT_ID;
const API_BASE = (process.env.CI_API_V4_URL || '').replace(/\/+$/, '');
const TOKEN = process.env.GITLAB_API_TOKEN;
const LOOP_URL = process.env.LOOP_WEBHOOK_URL;
const DOC_REVIEWER = process.env.DOC_REVIEWER || 'z9r5';
const MANAGER_LOOP_NAME = process.env.MANAGER_LOOP_NAME || '@yuriy.milyutin';
const PROJECT = ':dvp: DVP';

// STUCK_DAYS mirrors .github/scripts/prs_notifier.mjs: a changes-requested
// discussion is considered "stuck" once it stays unresolved for longer than
// this many days (and today is not Monday, see Monday exception below).
const STUCK_DAYS = 1.5;

const api = axios.create({
  baseURL: API_BASE,
  headers: {
    'PRIVATE-TOKEN': TOKEN,
    'Accept': 'application/json',
  },
});

const CHANGES_REQUESTED = 'changes_requested';
const REVIEW_REQUIRED = 'review_required';
const READY_TO_MERGE = 'ready_to_merge';
const STUCK = 'stuck';

function validateEnv() {
  if (!API_BASE || !TOKEN || !PROJECT_ID || !LOOP_URL) {
    console.error('ERROR: one of CI_API_V4_URL, GITLAB_API_TOKEN, CI_PROJECT_ID, LOOP_WEBHOOK_URL is not set.');
    process.exit(1);
  }
}

// Pure predicate: decide whether an open MR belongs in the review summary.
// Excludes drafts/WIP, release-* source branches, and autorelease/changelog
// bot MRs (these are not human review work). Exported for unit testing.
export function shouldNotifyMR(mr) {
  if (mr.draft || mr.work_in_progress) return false;
  const head = (mr.source_branch || '').toLowerCase();
  if (head.startsWith('release-')) return false;
  const labels = (mr.labels || []).map((l) => l.toLowerCase());
  if (labels.some((l) => l.startsWith('autorelease'))) return false;
  if (labels.includes('changelog')) return false;
  return true;
}

async function fetchOpenMRs() {
  const { data } = await api.get(`/projects/${PROJECT_ID}/merge_requests`, {
    params: {
      state: 'opened',
      per_page: 100,
      order_by: 'created_at',
      sort: 'asc',
    },
  });
  return data.filter(shouldNotifyMR);
}

async function fetchUser(id) {
  if (!id) return null;
  try {
    const { data } = await api.get(`/users/${id}`);
    return data;
  } catch (err) {
    console.error(`Error fetching user ${id}: ${err.message}`);
    return null;
  }
}

// Pure helper: render a Loop mention for a user. Prefers the profile name
// rendered as @first.last; falls back to the username with a nudge to set a
// real name. Exported for unit testing.
export function formatUser(user, details) {
  if (!user) return 'unknown';
  if (details && details.name) {
    const loopName = details.name.replace(/ /g, '.').toLowerCase();
    if (loopName.length > 0) return `@${loopName}`;
  }
  return `${user.username || user.login} (Set name in profile!)`;
}

async function getAssigneesInfo(mr) {
  let info = `NO ASSIGNEES! ${MANAGER_LOOP_NAME} (opezdulit')`;
  const assignees = mr.assignees || [];
  if (assignees.length > 0) {
    const names = [];
    for (const a of assignees) {
      const details = await fetchUser(a.id);
      names.push(formatUser(a, details));
    }
    info = `Assignees: ${names.join(', ')}`;
  }
  return info;
}

async function getReviewersInfo(mr) {
  let info = `NO REVIEWERS! ${MANAGER_LOOP_NAME} (opezdulit')`;
  const requestedReviewers = mr.reviewers || [];
  const unique = new Set();
  const fetched = [];

  for (const reviewer of requestedReviewers) {
    unique.add(reviewer.id);
    const details = await fetchUser(reviewer.id);
    let user = formatUser(reviewer, details);
    // Match GitHub behaviour: keep @ only for DOC_REVIEWER (legacy quirk).
    if (DOC_REVIEWER !== reviewer.username) {
      user = user.replace(/@/g, '');
    }
    fetched.push(user);
  }

  const threads = await fetchUnresolvedThreads(mr);
  for (const thread of threads) {
    const reviewer = thread.author;
    if (unique.has(reviewer.id)) continue;
    unique.add(reviewer.id);
    const details = await fetchUser(reviewer.id);
    let user = formatUser(reviewer, details);
    if (DOC_REVIEWER !== reviewer.username) {
      user = user.replace(/@/g, '');
    }
    fetched.push(user);
  }

  if (fetched.length > 0) {
    info = `Reviewers: ${fetched.join(', ')}`;
  }
  return info;
}

// Pure helper: extract unresolved discussion threads from raw GitLab
// discussions payload. Returns an array of { author, timestamp } where
// timestamp (ms epoch) is the earliest note timestamp belonging to the
// unresolved discussion, or null when no real timestamp is available.
//
// A thread is considered unresolved when at least one of its notes is
// `resolvable` and not `resolved` (GitLab marks resolution at note level).
// Exported so unit tests can exercise it without network access.
export function extractUnresolvedThreads(discussions) {
  const threads = [];
  for (const discussion of discussions || []) {
    const notes = discussion.notes || [];
    if (!notes.length) continue;
    const unresolvedNotes = notes.filter((n) => n.resolvable && !n.resolved);
    if (!unresolvedNotes.length) continue;
    const author = unresolvedNotes[0].author || notes[0].author;
    if (!author) continue;

    // Prefer the earliest timestamp among the unresolved resolvable notes;
    // fall back to the earliest note in the thread if GitLab only exposes
    // note-level resolution metadata inconsistently.
    const tsNotes = unresolvedNotes.some((n) => n.created_at)
      ? unresolvedNotes.filter((n) => n.created_at)
      : notes.filter((n) => n.created_at);

    let timestamp = null;
    if (tsNotes.length > 0) {
      timestamp = tsNotes
        .map((n) => new Date(n.created_at).getTime())
        .reduce((a, b) => (a < b ? a : b), Infinity);
      if (!Number.isFinite(timestamp)) timestamp = null;
    }

    threads.push({ author, timestamp });
  }
  return threads;
}

// Fetch unresolved resolvable discussion threads for an MR, including the
// real note timestamp used for STUCK classification.
async function fetchUnresolvedThreads(mr) {
  try {
    const { data } = await api.get(
      `/projects/${PROJECT_ID}/merge_requests/${mr.iid}/discussions`,
      { params: { per_page: 100 } },
    );
    return extractUnresolvedThreads(data);
  } catch (err) {
    console.error(`Error fetching discussions for MR !${mr.iid}: ${err.message}`);
    return [];
  }
}

// Back-compat wrapper: deduplicated unresolved authors, oldest thread per
// author preserved. Kept for any external caller and for symmetry with the
// GitHub changesRequestedMap semantics.
async function fetchUnresolvedReviewers(mr) {
  const threads = await fetchUnresolvedThreads(mr);
  const byAuthor = new Map();
  for (const thread of threads) {
    const id = thread.author.id;
    if (!byAuthor.has(id)) {
      byAuthor.set(id, thread);
      continue;
    }
    const existing = byAuthor.get(id);
    if (
      existing.timestamp == null
      || (thread.timestamp != null && thread.timestamp < existing.timestamp)
    ) {
      byAuthor.set(id, thread);
    }
  }
  return [...byAuthor.values()].map((t) => t.author);
}

async function fetchApprovals(mr) {
  try {
    const { data } = await api.get(
      `/projects/${PROJECT_ID}/merge_requests/${mr.iid}/approvals`,
    );
    return (data.approved_by || []).map((entry) => entry.user);
  } catch (err) {
    console.error(`Error fetching approvals for MR !${mr.iid}: ${err.message}`);
    return [];
  }
}

// Classify an MR given its approvers and unresolved discussion threads.
//
// Mirrors .github/scripts/prs_notifier.mjs getPullRequestGroup:
//   - STUCK when at least one unresolved discussion is older than STUCK_DAYS
//     and today is not Monday (Monday exception: give reviewers a fresh
//     start-of-week window).
//   - CHANGES_REQUESTED when there are unresolved discussions but none is
//     old enough (or a thread has no real timestamp — be conservative and
//     avoid false STUCK).
//   - READY_TO_MERGE when approved and no unresolved discussions.
//   - REVIEW_REQUIRED otherwise.
//
// Exported for unit testing.
export function classifyMR(approvedBy, unresolvedThreads) {
  const approved = approvedBy.length > 0;
  const unresolved = unresolvedThreads.length > 0;

  if (unresolved) {
    const now = new Date();
    let areChangesRequested = false;
    for (const thread of unresolvedThreads) {
      // No real timestamp -> cannot prove it is old enough. Be conservative:
      // treat as changes_requested, never STUCK.
      if (thread.timestamp == null) {
        areChangesRequested = true;
        continue;
      }
      const submittedAt = new Date(thread.timestamp);
      submittedAt.setTime(submittedAt.getTime() + STUCK_DAYS * 24 * 60 * 60 * 1000);
      if (now.getDay() !== 1 && submittedAt < now) {
        return STUCK;
      }
      areChangesRequested = true;
    }
    if (areChangesRequested) return CHANGES_REQUESTED;
  }

  if (approved) return READY_TO_MERGE;
  return REVIEW_REQUIRED;
}

async function buildSummary(mrs) {
  const groups = {
    [READY_TO_MERGE]: [],
    [STUCK]: [],
    [CHANGES_REQUESTED]: [],
    [REVIEW_REQUIRED]: [],
  };

  for (const mr of mrs) {
    const [approvals, unresolvedThreads] = await Promise.all([
      fetchApprovals(mr),
      fetchUnresolvedThreads(mr),
    ]);
    const group = classifyMR(approvals, unresolvedThreads);
    groups[group].push(mr);
  }

  const today = moment().format('YYYY-MM-DD');
  let summary = `## ${PROJECT} MRs ${today}\n\n`;
  if (mrs.length === 0) {
    summary += `:tada: No review required for today\n`;
    return summary;
  }

  if (groups[READY_TO_MERGE].length) {
    const lines = await Promise.all(groups[READY_TO_MERGE].map(formatAssigneeLine));
    summary += `### Ready to be merged\nWhy haven't they been merged yet? :thinking_face:\n\n${lines.join('\n')}\n\n`;
  }
  if (groups[STUCK].length) {
    const lines = await Promise.all(groups[STUCK].map(formatAssigneeLine));
    summary += `### Stuck in resolution\nWhy is there no resolution for the requested changes? :large_red_square:\n\n${lines.join('\n')}\n\n`;
  }
  if (groups[CHANGES_REQUESTED].length) {
    const lines = await Promise.all(groups[CHANGES_REQUESTED].map(formatAssigneeLine));
    summary += `### Changes requested\nMRs have the highest priority for comments to be resolved :fire:\n\n${lines.join('\n')}\n\n`;
  }
  if (groups[REVIEW_REQUIRED].length) {
    const lines = await Promise.all(groups[REVIEW_REQUIRED].map(formatReviewerLine));
    summary += `### MRs requiring review\n\n${lines.join('\n')}\n`;
  }
  return summary;
}

async function formatAssigneeLine(mr) {
  const assignees = await getAssigneesInfo(mr);
  return `- !${mr.iid}: [${mr.title}](${mr.web_url}) (created: ${moment(mr.created_at).fromNow()}) - ${assignees}`;
}

async function formatReviewerLine(mr) {
  const assignees = await getAssigneesInfo(mr);
  const reviewers = await getReviewersInfo(mr);
  return `- !${mr.iid}: [${mr.title}](${mr.web_url}) (created: ${moment(mr.created_at).fromNow()}) - ${assignees}. ${reviewers}`;
}

async function sendSummaryToLoop(summary) {
  try {
    await axios.post(LOOP_URL, { text: summary });
    console.log('Summary sent successfully.');
  } catch (err) {
    console.error(`Error sending summary to Loop: ${err.message}`);
    throw err;
  }
}

async function run() {
  validateEnv();
  try {
    const mrs = await fetchOpenMRs();
    const summary = await buildSummary(mrs);
    await sendSummaryToLoop(summary);
  } catch (err) {
    console.error(`An error occurred: ${err.message}`);
    process.exit(1);
  }
}

// Auto-run only when executed directly (CI), not when imported by tests.
if (process.argv[1] === fileURLToPath(import.meta.url)) {
  run();
}
