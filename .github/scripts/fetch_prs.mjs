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

const owner = 'deckhouse';
const repo = 'virtualization';
const project = ':dvp: DVP';
const defaultLogin = '@anton.nikonov';
const octokit = new Octokit({ auth: process.env.RELEASE_PLEASE_TOKEN });
const recentDays = 2;
const approvalsRequired = 1

async function fetchPullRequests() {
  try {
    const { data } = await octokit.request('GET /repos/{owner}/{repo}/pulls', {
      owner,
      repo,
      per_page: 500,
      state: 'open',
    });
    return data.filter(pr => {
      if (pr.draft) return false;
      const hasAutoreleaseLabel = pr.labels.some(label => label.name.includes('autorelease'));
      return !hasAutoreleaseLabel;
    });
  } catch (error) {
    console.error('Error fetching pull requests:', error);
    throw error;
  }
}

async function fetchReviewerDetails(login) {
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

async function fetchReviewsForPR(prNumber) {
  try {
    const { data } = await octokit.request('GET /repos/{owner}/{repo}/pulls/{pull_number}/reviews', {
      owner,
      repo,
      pull_number: prNumber,
    });
    return data;
  } catch (error) {
    console.error(`Error fetching reviews for PR ${prNumber}:`, error);
    return [];
  }
}

function formatReviewer(reviewer, details) {
  if (!details) {
    return reviewer.login;  
  }
  const loopName = details.name ? details.name.replace(/ /g, '.').toLowerCase() : 'Set name in profile!';
  return `${details.login} (@${loopName})`;
}

async function formatPR(pr) {
  let reviewersInfo = `NO REVIEWERS! ${defaultLogin} (opezdulit')`;

  const uniqueLogins = new Set();
  const fetchedReviewers = [];

  // Add requested reviewers
  if (pr.requested_reviewers && pr.requested_reviewers.length > 0) {
    for (const reviewer of pr.requested_reviewers) {
      const details = await fetchReviewerDetails(reviewer.login);
      uniqueLogins.add(details.login);
      fetchedReviewers.push(formatReviewer(reviewer, details));
    }
  }

  // Add more reviewers from reviews.
  const reviews = await fetchReviewsForPR(pr.number);
  for (const review of reviews) {
    if (uniqueLogins.has(review.user.login)) {
      continue;
    }
    uniqueLogins.add(review.user.login);
    const details = await fetchReviewerDetails(review.user.login); 
    fetchedReviewers.push(formatReviewer(review.user, details));
  }

  if (fetchedReviewers.length > 0) {
    reviewersInfo = `Reviewers: ${fetchedReviewers.join(', ')}`;
  }

  return `- pr${pr.number}: [${pr.title}](${pr.html_url}) (Created: ${moment(pr.created_at).fromNow()}) - ${reviewersInfo}`;
}

async function generateSummary(prs) {
  const now = moment();
  const recent = [];
  const lasting = [];
  const readyForMerge = [];

  for (const pr of prs) {
    const reviews = await fetchReviewsForPR(pr.number);
    const approvals = reviews.filter(review => review.state.toLowerCase() === 'approved');
    const isReadyForMerge = approvals.length >= approvalsRequired;

    if (isReadyForMerge) {
      readyForMerge.push(pr);
    } else if (moment().diff(moment(pr.created_at), 'days') <= recentDays) {
      recent.push(pr);
    } else {
      lasting.push(pr);
    }
  }

  let summary = `## ${project} PRs ${now.format('YYYY-MM-DD')}\n\n`;

  if (prs.length === 0) {
    summary += `:tada: No review required for today\n`;
    return summary;
  }

  if (readyForMerge.length > 0) {
    const readyForMergePRsInfo = await Promise.all(readyForMerge.map(pr => formatPR(pr)));
    summary += `### Ready for merge PRs\n\n${readyForMergePRsInfo.join('\n')}\n\n`;
  }

  if (recent.length > 0) {
    const recentPRsInfo = await Promise.all(recent.map(pr => formatPR(pr)));
    summary += `### Recent PRs requiring review\n\n${recentPRsInfo.join('\n')}\n\n`;
  }

  if (lasting.length > 0) {
    const lastingPRsInfo = await Promise.all(lasting.map(pr => formatPR(pr)));
    summary += `### Requiring review PRs\n\n${lastingPRsInfo.join('\n')}\n`;
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
