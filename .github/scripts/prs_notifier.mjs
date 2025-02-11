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
const MIN_APPROVALS_NUMBER = 1

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
      console.log(pr.head.ref);
      console.log(pr.base.ref);
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

async function formatForAssignees(pr) {
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

  return `- pr${pr.number}: [${pr.title}](${pr.html_url}) (created: ${moment(pr.created_at).fromNow()}) - ${assigneesInfo}`;
}

async function formatPRForReviewers(pr) {
  let reviewersInfo = `NO REVIEWERS! ${MANAGER_LOGIN} (opezdulit')`;

  const uniqueLogins = new Set();
  const fetchedReviewers = [];

  // Add requested reviewers
  if (pr.requested_reviewers && pr.requested_reviewers.length > 0) {
    for (const reviewer of pr.requested_reviewers) {
      const details = await fetchUserDetails(reviewer.login);
      uniqueLogins.add(reviewer.login);
      fetchedReviewers.push(formatUser(reviewer, details));
    }
  }

  // Add more reviewers from reviews.
  const reviews = await fetchReviewsForPR(pr.number);
  for (const review of reviews) {
    if (uniqueLogins.has(review.user.login)) {
      continue;
    }
    uniqueLogins.add(review.user.login);
    const details = await fetchUserDetails(review.user.login);
    fetchedReviewers.push(formatUser(review.user, details));
  }

  if (fetchedReviewers.length > 0) {
    reviewersInfo = `Reviewers: ${fetchedReviewers.join(', ')}`;
  }

  return `- pr${pr.number}: [${pr.title}](${pr.html_url}) (created: ${moment(pr.created_at).fromNow()}) - ${reviewersInfo}`;
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

async function generateSummary(prs) {
  const now = moment();

  const readyToMerge = [];
  const reviewRequired = [];
  const changesRequested = [];

  for (const pr of prs) {
    const reviews = await fetchReviewsForPR(pr.number);
    const areChangesRequested = reviews.some(review => review.state === 'CHANGES_REQUESTED');
    if (areChangesRequested) {
      changesRequested.push(pr);
      continue;
    }

    const approvals = reviews.filter(review => review.state === 'APPROVED');
    const isReadyToMerge = approvals.length >= MIN_APPROVALS_NUMBER;

    if (isReadyToMerge) {
      readyToMerge.push(pr);
      continue;
    }

    reviewRequired.push(pr);
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

  if (changesRequested.length > 0) {
    const changesRequestedPRsInfo = await Promise.all(changesRequested.map(pr => formatForAssignees(pr)));
    summary += `### Changes requested\nPRs have the highest priority for comments to be resolved :fire:\n\n${changesRequestedPRsInfo.join('\n')}\n\n`;
  }

  if (reviewRequired.length > 0) {
    const reviewRequiredPRsInfo = await Promise.all(reviewRequired.map(pr => formatPRForReviewers(pr)));
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
