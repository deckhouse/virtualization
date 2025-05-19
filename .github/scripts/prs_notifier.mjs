// Copyright 2024 Flant JSC
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


import { Octokit } from '@octokit/core';
import axios from 'axios';
import moment from 'moment';

const octokit = new Octokit({ auth: process.env.GITHUB_TOKEN });
const REPO = 'virtualization';
const OWNER = 'deckhouse';
const PROJECT = ':dvp: DVP';
const MANAGER_LOGIN = '@yuriy.milyutin';
const DOC_REVIEWER = "z9r5";

async function fetchPullRequests() {
  try {
    const { data } = await octokit.request('GET /repos/{owner}/{repo}/pulls', {
      owner: OWNER,
      repo: REPO,
      per_page: 500,
      state: 'open',
    });
    return data.filter(pr => {
      if (pr.draft) return false;

      const isReleaseBranch = pr.head.ref.startsWith('release-');
      const hasAutoreleaseLabel = pr.labels.some(label => label.name.includes('autorelease'));
      const hasChangelogLabel = pr.labels.some(label => label.name === 'changelog');
      return !(isReleaseBranch || hasChangelogLabel || hasAutoreleaseLabel);
    });
  } catch (error) {
    console.error('Error fetching pull requests:', error);
    throw error;
  }
}

async function fetchUserDetails(login) {
  try {
    const { data } = await octokit.request('GET /users/{username}', {
      username: login,
    });
    return data;
  } catch (error) {
    console.error(`Error fetching details for reviewer ${login}:`, error);
    return null;
  }
}

function formatUser(user, details) {
  if (!details) {
    return user.login;
  }

  const loopName = details.name ? details.name.replace(/ /g, '.').toLowerCase() : '';

  if (loopName.length !== 0) {
    return `@${loopName}`
  }

  return `${user.login} (Set name in profile!)`;
}

async function getAssigneesInfo(pr) {
  let assigneesInfo = `NO ASSIGNEES! ${MANAGER_LOGIN} (opezdulit')`;

  const fetchedAssignees = [];
  if (pr.assignees && pr.assignees.length > 0) {
    for (const assignee of pr.assignees) {
      const details = await fetchUserDetails(assignee.login);
      fetchedAssignees.push(formatUser(assignee, details));
    }
  }

  if (fetchedAssignees.length > 0) {
    assigneesInfo = `Assignees: ${fetchedAssignees.join(', ')}`;
  }

  return assigneesInfo;
}

async function getReviewersInfo(pr) {
  let reviewersInfo = `NO REVIEWERS! ${MANAGER_LOGIN} (opezdulit')`;

  const uniqueLogins = new Set();
  const fetchedReviewers = [];

  // Add requested reviewers
  if (pr.requested_reviewers && pr.requested_reviewers.length > 0) {
    for (const reviewer of pr.requested_reviewers) {
      uniqueLogins.add(reviewer.login);
      const details = await fetchUserDetails(reviewer.login);
      let user = formatUser(reviewer.login, details);
      user = DOC_REVIEWER === reviewer.login ? user : user.replace(/@/g, "");
      fetchedReviewers.push(user);
    }
  }

  const reviews = await fetchReviewsForPR(pr.number);
  const { changesRequestedMap } = getChangesRequestedAndApproves(reviews);
  for (let [, review] of changesRequestedMap) {
    if (uniqueLogins.has(review.user.login)) {
      continue;
    }
    uniqueLogins.add(review.user.login);
    const details = await fetchUserDetails(review.user.login);

    let user = formatUser(review.user.login, details);
    user = DOC_REVIEWER === review.user.login ? user : user.replace(/@/g, "");
    fetchedReviewers.push(user);
  }

  if (fetchedReviewers.length > 0) {
    reviewersInfo = `Reviewers: ${fetchedReviewers.join(', ')}`;
  }

  return reviewersInfo
}

async function formatForAssignees(pr) {
  const assigneesInfo = await getAssigneesInfo(pr)

  return `- pr${pr.number}: [${pr.title}](${pr.html_url}) (created: ${moment(pr.created_at).fromNow()}) - ${assigneesInfo}`;
}

async function formatForReviewers(pr) {
  const assigneesInfo = await getAssigneesInfo(pr)
  const reviewersInfo = await getReviewersInfo(pr)

  return `- pr${pr.number}: [${pr.title}](${pr.html_url}) (created: ${moment(pr.created_at).fromNow()}) - ${assigneesInfo}. ${reviewersInfo}`;
}

async function fetchReviewsForPR(prNumber) {
  try {
    const { data } = await octokit.request('GET /repos/{owner}/{repo}/pulls/{pull_number}/reviews', {
      owner: OWNER,
      repo: REPO,
      pull_number: prNumber,
    });
    return data;
  } catch (error) {
    console.error(`Error fetching reviews for PR ${prNumber}:`, error);
    return [];
  }
}

async function fetchRequestedReviewersForPR(prNumber) {
  try {
    const { data } = await octokit.request('GET /repos/{owner}/{repo}/pulls/{pull_number}/requested_reviewers', {
      owner: OWNER,
      repo: REPO,
      pull_number: prNumber,
    });
    return data;
  } catch (error) {
    console.error(`Error fetching requested reviewers for PR ${prNumber}:`, error);
    return [];
  }
}

const CHANGES_REQUESTED = "changes_requested"
const REVIEW_REQUIRED = "review_required"
const READY_TO_MERGE = "ready_to_merge"
const STUCK = "stuck"

function getChangesRequestedAndApproves(reviews) {
  // Sort reviews by submitted_at date
  reviews = reviews.sort((a, b) => {
    const dateA = new Date(a.submitted_at);
    const dateB = new Date(b.submitted_at);
    return dateA - dateB;
  });

  const changesRequestedMap = new Map();
  const approvedMap = new Map();

  reviews.forEach(review => {
    const { user, state } = review;

    if (state === 'APPROVED') {
      if (changesRequestedMap.has(user.id)) {
        changesRequestedMap.delete(user.id);
      }

      approvedMap.set(user.id, review);
    }

    if (state === 'CHANGES_REQUESTED') {
      if (approvedMap.has(user.id)) {
        approvedMap.delete(user.id);
      }

      changesRequestedMap.set(user.id, review);
    }
  });

  return { changesRequestedMap, approvedMap };
}

function getPullRequestGroup(reviews, requestedReviewers) {
  const requestedReviewersMap = new Map();

  requestedReviewers.users.forEach(requestedReviewer => {
    const { id } = requestedReviewer;
    requestedReviewersMap.set(id, requestedReviewer);
  });

  const { changesRequestedMap, approvedMap } = getChangesRequestedAndApproves(reviews);

  const now = new Date();
  let areChangesRequested = false;

  for (let [userId, review] of changesRequestedMap.entries()) {
    // Skip if review is re-requested.
    if (requestedReviewersMap.has(userId)) {
      continue;
    }

    const submittedAt = new Date(review.submitted_at)
    submittedAt.setTime(submittedAt.getTime() + 1.5 * 24 * 60 * 60 * 1000); // submittedAt + 1.5 days.
    if (now.getDay() !== 1 && submittedAt < now) {
      return STUCK;
    }

    areChangesRequested = true;
  }

  if (areChangesRequested) {
    return CHANGES_REQUESTED;
  }

  if (approvedMap.size > 0) {
    return READY_TO_MERGE;
  }

  return REVIEW_REQUIRED;
}

async function generateSummary(prs) {
  const now = moment();

  const readyToMerge = [];
  const reviewRequired = [];
  const changesRequested = [];
  const stuck = [];

  for (const pr of prs) {
    const reviews = await fetchReviewsForPR(pr.number);
    const requestedReviewers = await fetchRequestedReviewersForPR(pr.number);

    const group = getPullRequestGroup(reviews, requestedReviewers);

    switch (group) {
      case CHANGES_REQUESTED:
        changesRequested.push(pr);
        break;
      case READY_TO_MERGE:
        readyToMerge.push(pr);
        break;
      case STUCK:
        stuck.push(pr);
        break;
      default:
        reviewRequired.push(pr);
        break;
    }
  }

  let summary = `## ${PROJECT} PRs ${now.format('YYYY-MM-DD')}\n\n`;

  if (prs.length === 0) {
    summary += `:tada: No review required for today\n`;
    return summary;
  }

  if (readyToMerge.length > 0) {
    const readyToMergePRsInfo = await Promise.all(readyToMerge.map(pr => formatForAssignees(pr)));
    summary += `### Ready to be merged\nWhy haven't they been merged yet? :thinking_face:\n\n${readyToMergePRsInfo.join('\n')}\n\n`;
  }

  if (stuck.length > 0) {
    const stuckPRsInfo = await Promise.all(stuck.map(pr => formatForAssignees(pr)));
    summary += `### Stuck in resolution\nWhy is there no resolution for the requested changes? :large_red_square:\n\n${stuckPRsInfo.join('\n')}\n\n`;
  }

  if (changesRequested.length > 0) {
    const changesRequestedPRsInfo = await Promise.all(changesRequested.map(pr => formatForAssignees(pr)));
    summary += `### Changes requested\nPRs have the highest priority for comments to be resolved :fire:\n\n${changesRequestedPRsInfo.join('\n')}\n\n`;
  }

  if (reviewRequired.length > 0) {
    const reviewRequiredPRsInfo = await Promise.all(reviewRequired.map(pr => formatForReviewers(pr)));
    summary += `### PRs requiring review\n\n${reviewRequiredPRsInfo.join('\n')}\n`;
  }

  return summary;
}

async function sendSummaryToLoop(summary) {
  const url = process.env.LOOP_WEBHOOK_URL;
  try {
    await axios.post(url, { text: summary });
    console.log('Summary sent successfully.');
  } catch (error) {
    console.error('Error sending summary to Loop:', error);
    throw error;
  }
}

async function run() {
  try {
    const prs = await fetchPullRequests();
    const summary = await generateSummary(prs);

    await sendSummaryToLoop(summary);
  } catch (error) {
    console.error('An error occurred:', error);
    process.exit(1);
  }
}

run();
