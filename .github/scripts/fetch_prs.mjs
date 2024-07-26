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

const octokit = new Octokit({ auth: process.env.RELEASE_PLEASE_TOKEN });


async function fetchPullRequests() {
  const pulls = await octokit.request('GET /repos/{owner}/{repo}/pulls', {
    owner,
    repo,
    state: 'all'
  });
  return pulls.data.filter(pr => !pr.draft);
}

function generateSummary(prs) {
  const now = moment();
  const reviewRequired = prs.filter(pr => pr.state === 'open' && pr.requested_reviewers.length > 0);
  const pending = reviewRequired.filter(pr => now.diff(moment(pr.created_at), 'days') <= 2);

  return `
## Daily PR Summary

### PRs Requiring Review
${reviewRequired.map(pr => `- [${pr.title}](${pr.html_url}) (Created: ${moment(pr.created_at).fromNow()})`).join('\n') || 'No PRs requiring review.'}

### PRs Pending (<=2 days)
${pending.map(pr => `- [${pr.title}](${pr.html_url}) (Created: ${moment(pr.created_at).fromNow()})`).join('\n') || 'No PRs pending for review (<=2 days).'}
  `;
}

async function sendSummaryToLoop(summary) {
  const url = process.env.LOOP_WEBHOOK_URL;
  await axios.post(url, {
    text: summary
  });
}

async function run() {
  try {
    const prs = await fetchPullRequests();
    const summary = generateSummary(prs);
    await sendSummaryToLoop(summary);
  } catch (error) {
    console.error(error);
    process.exit(1);
  }
}

run();