// Copyright 2026 Flant JSC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//     http://www.apache.org/licenses/LICENSE-2.0
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

const fs = require('fs');
const path = require('path');

function readMatchingFiles(dirPath, filePattern, files = []) {
  if (!fs.existsSync(dirPath)) {
    return files;
  }

  const entries = fs.readdirSync(dirPath, {withFileTypes: true});
  for (const entry of entries) {
    const fullPath = path.join(dirPath, entry.name);
    if (entry.isDirectory()) {
      readMatchingFiles(fullPath, filePattern, files);
      continue;
    }

    if (filePattern.test(entry.name)) {
      files.push(fullPath);
    }
  }

  return files;
}

function createMissingReport(clusterName) {
  return {
    cluster: clusterName,
    storageType: clusterName,
    reportKind: 'artifact-missing',
    status: 'missing',
    statusMessage: '⚠️ TEST REPORTS NOT FOUND',
    failedStage: 'artifact-missing',
    failedStageLabel: 'TEST REPORTS NOT FOUND',
    branch: '',
    workflowRunUrl: '',
    metrics: {
      passed: 0,
      failed: 0,
      errors: 0,
      total: 0,
      successRate: 0,
    },
    failedTests: [],
  };
}

function sanitizeCell(value) {
  return String(value || '—')
    .replace(/\|/g, '\\|')
    .replace(/\r?\n/g, ' ')
    .trim();
}

function sanitizeListItem(value) {
  return String(value || '')
    .replace(/\r?\n/g, ' ')
    .trim();
}

function formatRate(value) {
  const rate = Number(value || 0);
  return `${Number.isFinite(rate) ? rate.toFixed(2) : '0.00'}%`;
}

function getReportDate(reports) {
  const datedReport = reports.find((report) => report.startedAt);
  if (!datedReport) {
    return new Date().toISOString().slice(0, 10);
  }

  return String(datedReport.startedAt).slice(0, 10);
}

function sortReports(reports, preferredOrder) {
  const orderMap = new Map(preferredOrder.map((name, index) => [name, index]));

  return [...reports].sort((left, right) => {
    const leftKey = left.storageType || left.cluster;
    const rightKey = right.storageType || right.cluster;
    const leftOrder = orderMap.has(leftKey) ? orderMap.get(leftKey) : Number.MAX_SAFE_INTEGER;
    const rightOrder = orderMap.has(rightKey) ? orderMap.get(rightKey) : Number.MAX_SAFE_INTEGER;

    if (leftOrder !== rightOrder) {
      return leftOrder - rightOrder;
    }

    return String(left.cluster || left.storageType).localeCompare(String(right.cluster || right.storageType));
  });
}

function formatClusterLink(report) {
  const clusterName = sanitizeCell(report.cluster || report.storageType);
  return report.workflowRunUrl ? `[${clusterName}](${report.workflowRunUrl})` : clusterName;
}

function normalizeLoopApiBaseUrl(value) {
  const trimmedValue = String(value || '').trim().replace(/\/+$/, '');

  if (!trimmedValue) {
    return '';
  }

  if (trimmedValue.endsWith('/api/v4/posts')) {
    return trimmedValue;
  }

  if (trimmedValue.endsWith('/api/v4')) {
    return `${trimmedValue}/posts`;
  }

  return `${trimmedValue}/api/v4/posts`;
}

function getLoopPostsApiUrl() {
  return normalizeLoopApiBaseUrl(process.env.LOOP_API_BASE_URL);
}

async function postToLoopApi({apiUrl, channelId, token, message, rootId}, core) {
  const response = await fetch(apiUrl, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${token}`,
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({
      channel_id: channelId,
      message,
      ...(rootId ? {root_id: rootId} : {}),
    }),
  });
  const responseText = await response.text();

  if (!response.ok) {
    throw new Error(`Loop API request failed with status ${response.status}: ${responseText}`);
  }

  const payload = responseText ? JSON.parse(responseText) : {};
  core.info(`Loop API accepted report with status ${response.status}`);
  return payload;
}

function readReports(reportsDir, configuredClusters, core) {
  const reportFiles = readMatchingFiles(reportsDir, /^e2e_report_.*\.json$/);
  const reports = [];

  for (const reportFile of reportFiles) {
    try {
      reports.push(JSON.parse(fs.readFileSync(reportFile, 'utf8')));
    } catch (error) {
      core.warning(`Unable to parse ${reportFile}: ${error.message}`);
    }
  }

  const reportsByCluster = new Map();
  for (const report of reports) {
    const clusterName = report.storageType || report.cluster;
    reportsByCluster.set(clusterName, report);
  }

  for (const clusterName of configuredClusters) {
    if (!reportsByCluster.has(clusterName)) {
      reportsByCluster.set(clusterName, createMissingReport(clusterName));
    }
  }

  const orderedReports = sortReports(Array.from(reportsByCluster.values()), configuredClusters);
  return orderedReports;
}

function buildMainMessage(orderedReports) {
  const reportDate = getReportDate(orderedReports);
  const branches = Array.from(new Set(orderedReports.map((report) => report.branch).filter(Boolean)));
  const lines = [`## DVP | E2E on nested clusters | ${reportDate}`, ''];

  if (branches.length === 1) {
    lines.push(`Branch: \`${branches[0]}\``);
    lines.push('');
  }

  const testsReports = orderedReports.filter((report) => report.reportKind === 'tests');
  const nonTestReports = orderedReports.filter((report) => report.reportKind !== 'tests');

  if (testsReports.length > 0) {
    lines.push('### Test results');
    lines.push('');
    lines.push('| Cluster | ✅ Passed | ❌ Failed | ⚠️ Errors | Total | Success Rate |');
    lines.push('|---|---:|---:|---:|---:|---:|');

    for (const report of testsReports) {
      const metrics = report.metrics || {};
      lines.push(
        `| ${formatClusterLink(report)} | ${metrics.passed || 0} | ${metrics.failed || 0} | ${metrics.errors || 0} | ${metrics.total || 0} | ${formatRate(metrics.successRate)} |`
      );
    }

    lines.push('');
  }

  if (nonTestReports.length > 0) {
    lines.push('### Cluster failures');
    lines.push('');

    for (const report of nonTestReports) {
      lines.push(`- ${formatClusterLink(report)}: ${sanitizeListItem(report.failedStageLabel || report.statusMessage || report.failedStage)}`);
    }

    lines.push('');
  }

  return lines.join('\n').trim();
}

function buildThreadMessage(orderedReports) {
  const testsReports = orderedReports.filter((report) => report.reportKind === 'tests');
  const failedTestReports = testsReports.filter((report) => {
    if (Array.isArray(report.failedTests) && report.failedTests.length > 0) {
      return true;
    }

    return Boolean((report.metrics && report.metrics.failed) || (report.metrics && report.metrics.errors));
  });

  if (failedTestReports.length === 0) {
    return '';
  }

  const lines = ['### Failed tests', ''];

  for (const report of failedTestReports) {
    const clusterName = sanitizeListItem(report.cluster || report.storageType);
    lines.push(`**${clusterName}**`);

    if (Array.isArray(report.failedTests) && report.failedTests.length > 0) {
      for (const testName of report.failedTests) {
        lines.push(`- ${sanitizeListItem(testName)}`);
      }
    } else {
      lines.push('- No testcase-level failures were collected, but the E2E stage reported failures.');
    }

    lines.push('');
  }

  return lines.join('\n').trim();
}

function buildMessengerMessages({reportsDir, configuredClusters, core}) {
  const orderedReports = readReports(reportsDir, configuredClusters, core);
  return {
    message: buildMainMessage(orderedReports),
    threadMessage: buildThreadMessage(orderedReports),
  };
}

async function renderMessengerReport({core}) {
  const reportsDir = process.env.REPORTS_DIR || 'downloaded-artifacts';
  const configuredClusters = JSON.parse(process.env.STORAGE_TYPES || '[]');
  const {message, threadMessage} = buildMessengerMessages({reportsDir, configuredClusters, core});

  core.info(message);
  core.setOutput('message', message);
  core.setOutput('thread_message', threadMessage);

  const loopPostsApiUrl = getLoopPostsApiUrl();
  const loopChannelId = String(process.env.LOOP_CHANNEL_ID || '').trim();
  const loopToken = String(process.env.LOOP_TOKEN || '').trim();

  if (loopPostsApiUrl || loopChannelId || loopToken) {
    try {
      if (!loopPostsApiUrl || !loopChannelId || !loopToken) {
        throw new Error('LOOP_CHANNEL_ID, LOOP_TOKEN, and LOOP_API_BASE_URL are required');
      }

      const rootPost = await postToLoopApi({
        apiUrl: loopPostsApiUrl,
        channelId: loopChannelId,
        token: loopToken,
        message,
      }, core);

      if (threadMessage) {
        const replyPost = await postToLoopApi({
          apiUrl: loopPostsApiUrl,
          channelId: loopChannelId,
          token: loopToken,
          message: threadMessage,
          rootId: rootPost.id,
        }, core);

        core.setOutput('root_post_id', rootPost.id || '');
        core.setOutput('thread_post_id', replyPost.id || '');
      } else {
        core.setOutput('root_post_id', rootPost.id || '');
        core.setOutput('thread_post_id', '');
      }
    } catch (error) {
      core.warning(`Unable to deliver report to Loop API: ${error.message}`);
    }
  }

  return {message, threadMessage};
}

module.exports = renderMessengerReport;
module.exports.createMissingReport = createMissingReport;
module.exports.buildMessengerMessages = buildMessengerMessages;
module.exports.getLoopPostsApiUrl = getLoopPostsApiUrl;
