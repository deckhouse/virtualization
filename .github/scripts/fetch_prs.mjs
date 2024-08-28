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
const defaultEmail = 'alert@fl.com'; //<- Need to change or delete
const octokit = new Octokit({ auth: process.env.RELEASE_PLEASE_TOKEN });
const recentDays = 2;

async function fetchPullRequests() {
  try {
    const { data } = await octokit.request('GET /repos/{owner}/{repo}/pulls', {
      owner,
      repo,
      per_page: 100,
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
    console.log(data.email, data.login, data.created_at)
    return data;
  } catch (error) {
    console.error(`Error fetching details for reviewer ${login}:`, error);
    return null;
  }
}

async function formatPR(pr) {
  const reviewers = await Promise.all(
    pr.requested_reviewers.map(async reviewer => {
      const details = await fetchReviewerDetails(reviewer.login);
      const email = (details && details.email) ? details.email : defaultEmail;
      return `${reviewer.login} (${email})`;
    })
  );
  return `- [${pr.title}](${pr.html_url}) (Created: ${moment(pr.created_at).fromNow()}) - Reviewers: ${reviewers.join(', ')}`;
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
    summary += `### Recent PRs requiring review\n\n${await Promise.all(recent.map(formatPR)).then(results => results.join('\n'))}\n`;
  }

  if (lasting.length > 0) {
    summary += `### PRs requiring review\n\n${await Promise.all(lasting.map(formatPR)).then(results => results.join('\n'))}\n`;
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