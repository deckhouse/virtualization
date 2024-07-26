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
  const reviewRequired = prs.filter(pr => pr.state === 'open' && pr.requested_reviewers.length > 0);
  const pending = reviewRequired.filter(pr => moment().diff(moment(pr.created_at), 'days') <= 2);

  let summary = `## Daily PR Summary\n\n### PRs Requiring Review\n\n`;

  if (reviewRequired.length > 0) {
    reviewRequired.forEach(pr => {
      summary += `- [${pr.title}](${pr.html_url}) (Created: ${moment(pr.created_at).fromNow()})\n`;
    });
  } else {
    summary += `No PRs requiring review.\n`;
  }

  summary += `\n### PRs Pending (<=2 days)\n\n`;

  if (pending.length > 0) {
    pending.forEach(pr => {
      summary += `- [${pr.title}](${pr.html_url}) (Created: ${moment(pr.created_at).fromNow()})\n`;
    });
  } else {
    summary += `No PRs pending for review (<=2 days).\n`;
  }

  return summary;
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