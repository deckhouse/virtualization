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
const project = 'DVP';
const defaultLogin = 'Almighty PM';
const octokit = new Octokit({ auth: process.env.RELEASE_PLEASE_TOKEN });
const recentDays = 2;

async function fetchPullRequests() {
  try {
    const { data } = await octokit.request('GET /repos/{owner}/{repo}/pulls', {
      owner,
      repo,
      per_page: 500,
      state: 'open',
    });
    return data.filter(pr => !pr.draft);
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

async function formatPR(pr) {
  let reviewersInfo = `NO REVIEWERS! ${defaultLogin}, your care is required here.`;
  if (pr.requested_reviewers && pr.requested_reviewers.length > 0 && pr.number != 329 ) {
    const reviewers = await Promise.all(
      pr.requested_reviewers.map(async reviewer => {
        const details = await fetchReviewerDetails(reviewer.login);
        return details ? `${details.login} (${details.name})` : reviewer.login;
      })
    );
    reviewersInfo = `Reviewers: ${reviewers.join(', ')}`;
  }

  return `- [${pr.title}](${pr.html_url}) (Created: ${moment(pr.created_at).fromNow()}) - ${reviewersInfo}`;
}

async function generateSummary(prs) {
  const now = moment();
  const reviewRequired = prs.filter(pr => pr.requested_reviewers.length > 0);
  const recent = reviewRequired.filter(pr => moment().diff(moment(pr.created_at), 'days') <= recentDays);
  const lasting = reviewRequired.filter(pr => moment().diff(moment(pr.created_at), 'days') > recentDays);

  let summary = `## ${project} PRs ${now.format('YYYY-MM-DD')}\n\n`;

  if (reviewRequired.length === 0) {
    summary += `:tada: No review required for today\n`;
    return summary;
  }

  if (recent.length > 0) {
    const recentPRsInfo = await Promise.all(recent.map(formatPR));
    summary += `### Recent PRs requiring review\n\n${recentPRsInfo.join('\n')}\n\n`;
  }

  if (lasting.length > 0) {
    const lastingPRsInfo = await Promise.all(lasting.map(formatPR));
    summary += `### PRs requiring review\n\n${lastingPRsInfo.join('\n')}\n`;
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