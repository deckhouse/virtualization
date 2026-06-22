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

const PROJECT_ID = process.env.CI_PROJECT_ID;
const API_BASE = (process.env.CI_API_V4_URL || '').replace(/\/+$/, '');
const TOKEN = process.env.GITLAB_API_TOKEN;
const LOOP_URL = process.env.LOOP_WEBHOOK_URL;
const DOC_REVIEWER = process.env.DOC_REVIEWER || 'z9r5';
const MANAGER_LOOP_NAME = process.env.MANAGER_LOOP_NAME || '@yuriy.milyutin';
const PROJECT = ':dvp: DVP';

const STUCK_DAYS = 1.5;

if (!API_BASE || !TOKEN || !PROJECT_ID || !LOOP_URL) {
  console.error('ERROR: one of CI_API_V4_URL, GITLAB_API_TOKEN, CI_PROJECT_ID, LOOP_WEBHOOK_URL is not set.');
  process.exit(1);
}

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

async function fetchOpenMRs() {
  const { data } = await api.get(`/projects/${PROJECT_ID}/merge_requests`, {
    params: {
      state: 'opened',
      per_page: 100,
      order_by: 'created_at',
      sort: 'asc',
    },
  });
  return data.filter((mr) => {
    if (mr.draft || mr.work_in_progress) return false;
    const head = (mr.source_branch || '').toLowerCase();
    if (head.startsWith('release-')) return false;
    const labels = (mr.labels || []).map((l) => l.toLowerCase());
    if (labels.some((l) => l.startsWith('autorelease'))) return false;
    if (labels.includes('changelog')) return false;
    return true;
  });
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

function formatUser(user, details) {
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

  const changesRequested = await fetchUnresolvedReviewers(mr);
  for (const reviewer of changesRequested) {
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

// Approximate GitHub CHANGES_REQUESTED via unresolved discussions.
// GitLab has no native review-state; we treat unresolved resolvable
// discussion threads as a change request from that author.
async function fetchUnresolvedReviewers(mr) {
  try {
    const { data } = await api.get(
      `/projects/${PROJECT_ID}/merge_requests/${mr.iid}/discussions`,
      { params: { per_page: 100 } },
    );
    const unresolved = new Map();
    for (const discussion of data) {
      const notes = discussion.notes || [];
      if (!notes.length) continue;
      if (!notes.some((n) => n.resolvable && !n.resolved)) continue;
      const author = notes[0].author;
      if (!author) continue;
      unresolved.set(author.id, author);
    }
    return [...unresolved.values()];
  } catch (err) {
    console.error(`Error fetching discussions for MR !${mr.iid}: ${err.message}`);
    return [];
  }
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

function classifyMR(approvedBy, unresolvedAuthors) {
  const approved = approvedBy.length > 0;
  const unresolved = unresolvedAuthors.length > 0;

  if (unresolved) {
    // Check whether any unresolved thread is older than STUCK_DAYS (and not Monday).
    const now = new Date();
    const stuck = unresolved.some((author) => {
      // We don't have note timestamps here; classify as changes_requested and let
      // the existence of older discussions be inspected manually. TODO: pull
      // discussion.notes[].created_at once we expose it.
      const fakeDate = new Date();
      fakeDate.setTime(fakeDate.getTime() - (STUCK_DAYS + 1) * 24 * 60 * 60 * 1000);
      return now.getDay() !== 1 && fakeDate < now;
    });
    return stuck ? STUCK : CHANGES_REQUESTED;
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
    const [approvals, unresolvedAuthors] = await Promise.all([
      fetchApprovals(mr),
      fetchUnresolvedReviewers(mr),
    ]);
    const group = classifyMR(approvals, unresolvedAuthors);
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
  try {
    const mrs = await fetchOpenMRs();
    const summary = await buildSummary(mrs);
    await sendSummaryToLoop(summary);
  } catch (err) {
    console.error(`An error occurred: ${err.message}`);
    process.exit(1);
  }
}

run();
