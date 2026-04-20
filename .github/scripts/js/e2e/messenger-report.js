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
    status: 'missing',
    statusMessage: '⚠️ ARTIFACT NOT FOUND',
    failedStage: 'artifact-missing',
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

async function postToWebhook(url, message, core) {
  const response = await fetch(url, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({text: message}),
  });

  if (!response.ok) {
    const body = await response.text();
    throw new Error(`Webhook request failed with status ${response.status}: ${body}`);
  }

  core.info(`Webhook accepted report with status ${response.status}`);
}

module.exports = async function renderMessengerReport({core}) {
  const reportsDir = process.env.REPORTS_DIR || 'downloaded-artifacts';
  const configuredClusters = JSON.parse(process.env.STORAGE_TYPES || '[]');
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
  const reportDate = getReportDate(orderedReports);
  const branches = Array.from(new Set(orderedReports.map((report) => report.branch).filter(Boolean)));
  const lines = [`## DVP | E2E on nested clusters | ${reportDate}`, ''];

  if (branches.length === 1) {
    lines.push(`Branch: \`${branches[0]}\``);
    lines.push('');
  }

  lines.push('| Cluster | Status | ✅ Passed | ❌ Failed | ⚠️ Errors | Total | Success Rate |');
  lines.push('|---|---|---:|---:|---:|---:|---:|');

  for (const report of orderedReports) {
    const clusterName = sanitizeCell(report.cluster || report.storageType);
    const clusterCell = report.workflowRunUrl
      ? `[${clusterName}](${report.workflowRunUrl})`
      : clusterName;
    const metrics = report.metrics || {};

    lines.push(
      `| ${clusterCell} | ${sanitizeCell(report.statusMessage)} | ${metrics.passed || 0} | ${metrics.failed || 0} | ${metrics.errors || 0} | ${metrics.total || 0} | ${formatRate(metrics.successRate)} |`
    );
  }

  lines.push('');
  lines.push('### Failed tests');
  lines.push('');

  for (const report of orderedReports) {
    const clusterName = sanitizeCell(report.cluster || report.storageType);
    lines.push(`**${clusterName}**`);

    if (Array.isArray(report.failedTests) && report.failedTests.length > 0) {
      for (const testName of report.failedTests) {
        lines.push(`- ${sanitizeListItem(testName)}`);
      }
    } else if (report.failedStage === 'e2e-test') {
      lines.push('- No testcase-level failures were collected, but the E2E stage failed.');
    } else if (report.failedStage && report.failedStage !== 'success' && report.failedStage !== 'e2e-test') {
      lines.push(`- No failed tests collected; cluster failed before E2E start (${sanitizeListItem(report.failedStage)}).`);
    } else {
      lines.push('- No failed tests');
    }

    lines.push('');
  }

  const message = lines.join('\n').trim();
  core.info(message);
  core.setOutput('message', message);

  if (process.env.LOOP_WEBHOOK_URL) {
    try {
      await postToWebhook(process.env.LOOP_WEBHOOK_URL, message, core);
    } catch (error) {
      core.warning(`Unable to deliver report to webhook: ${error.message}`);
    }
  }

  return message;
};
