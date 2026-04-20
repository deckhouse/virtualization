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

const stageLabels = {
  bootstrap: 'BOOTSTRAP CLUSTER',
  'configure-sdn': 'CONFIGURE SDN',
  'storage-setup': 'STORAGE SETUP',
  'virtualization-setup': 'VIRTUALIZATION SETUP',
  'e2e-test': 'E2E TEST',
  success: 'SUCCESS',
  'artifact-missing': 'TEST REPORTS NOT FOUND',
};

const preE2EStages = new Set([
  'bootstrap',
  'configure-sdn',
  'storage-setup',
  'virtualization-setup',
]);

function escapeRegExp(value) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

function readFirstMatchingFile(dirPath, filePattern) {
  if (!fs.existsSync(dirPath)) {
    return null;
  }

  const entries = fs.readdirSync(dirPath, {withFileTypes: true})
    .sort((left, right) => left.name.localeCompare(right.name));
  for (const entry of entries) {
    const fullPath = path.join(dirPath, entry.name);
    if (entry.isDirectory()) {
      const nestedMatch = readFirstMatchingFile(fullPath, filePattern);
      if (nestedMatch) {
        return nestedMatch;
      }
      continue;
    }

    if (filePattern.test(entry.name)) {
      return fullPath;
    }
  }

  return null;
}

function decodeXmlEntities(value) {
  if (!value) {
    return '';
  }

  const namedEntities = {
    amp: '&',
    apos: "'",
    gt: '>',
    lt: '<',
    quot: '"',
  };

  return value.replace(/&(#x?[0-9a-fA-F]+|[a-zA-Z]+);/g, (_, entity) => {
    if (entity[0] === '#') {
      const isHex = entity[1].toLowerCase() === 'x';
      const rawCodePoint = isHex ? entity.slice(2) : entity.slice(1);
      const codePoint = Number.parseInt(rawCodePoint, isHex ? 16 : 10);
      return Number.isNaN(codePoint) ? _ : String.fromCodePoint(codePoint);
    }

    return namedEntities[entity] || _;
  });
}

function parseXmlAttributes(rawAttributes) {
  const attributes = {};
  const pattern = /([A-Za-z_:][A-Za-z0-9_.:-]*)="([^"]*)"/g;
  let match = pattern.exec(rawAttributes);

  while (match) {
    attributes[match[1]] = decodeXmlEntities(match[2]);
    match = pattern.exec(rawAttributes);
  }

  return attributes;
}

function toInteger(value) {
  const parsed = Number.parseInt(value || '0', 10);
  return Number.isNaN(parsed) ? 0 : parsed;
}

function zeroMetrics() {
  return {
    passed: 0,
    failed: 0,
    errors: 0,
    skipped: 0,
    total: 0,
    successRate: 0,
  };
}

function stripAnsi(value) {
  return String(value || '').replace(/\x1b\[[0-9;]*m/g, '');
}

function parseJUnitReport(xmlContent) {
  const testsuitePattern = /<testsuite\b([^>]*)>/gi;
  let testsuiteMatch = testsuitePattern.exec(xmlContent);
  let total = 0;
  let failed = 0;
  let errors = 0;
  let skipped = 0;
  let startedAt = null;

  while (testsuiteMatch) {
    const suiteAttributes = parseXmlAttributes(testsuiteMatch[1] || '');
    total += toInteger(suiteAttributes.tests);
    failed += toInteger(suiteAttributes.failures);
    errors += toInteger(suiteAttributes.errors);
    skipped += toInteger(suiteAttributes.skipped || suiteAttributes.disabled);
    startedAt = startedAt || suiteAttributes.timestamp || null;
    testsuiteMatch = testsuitePattern.exec(xmlContent);
  }

  if (total === 0 && failed === 0 && errors === 0 && skipped === 0) {
    const testsuitesMatch = xmlContent.match(/<testsuites\b([^>]*)>/i);
    const rootAttributes = parseXmlAttributes((testsuitesMatch && testsuitesMatch[1]) || '');
    total = toInteger(rootAttributes.tests);
    failed = toInteger(rootAttributes.failures);
    errors = toInteger(rootAttributes.errors);
    skipped = toInteger(rootAttributes.skipped || rootAttributes.disabled);
  }

  const passed = Math.max(total - failed - errors - skipped, 0);
  const successRate = total > 0 ? Number(((passed / total) * 100).toFixed(2)) : 0;

  const failedTests = [];
  const testcasePattern = /<testcase\b([^>]*?)(?:\/>|>([\s\S]*?)<\/testcase>)/gi;
  let testcaseMatch = testcasePattern.exec(xmlContent);

  while (testcaseMatch) {
    const testcaseAttributes = parseXmlAttributes(testcaseMatch[1] || '');
    const testcaseBody = testcaseMatch[2] || '';
    const testcaseStatus = (testcaseAttributes.status || '').toLowerCase();
    const hasFailure = /<failure\b/i.test(testcaseBody);
    const hasError = /<error\b/i.test(testcaseBody);

    if (hasFailure || hasError || testcaseStatus === 'failed' || testcaseStatus === 'error') {
      const testcaseName = decodeXmlEntities(testcaseAttributes.name || '').trim();
      if (testcaseName) {
        failedTests.push(testcaseName);
      }
    }

    testcaseMatch = testcasePattern.exec(xmlContent);
  }

  return {
    metrics: {
      passed,
      failed,
      errors,
      skipped,
      total,
      successRate,
    },
    failedTests: Array.from(new Set(failedTests)),
    startedAt,
  };
}

function parseGinkgoSummaryLog(logContent) {
  const cleanOutput = stripAnsi(logContent);
  const summaryLine = cleanOutput
    .split(/\r?\n/)
    .find((line) => line.includes('FAIL!') || line.includes('SUCCESS!'));

  if (!summaryLine) {
    return null;
  }

  const passed = toInteger((summaryLine.match(/(\d+)(?=\s+Passed)/) || [])[1]);
  const failed = toInteger((summaryLine.match(/(\d+)(?=\s+Failed)/) || [])[1]);
  const skipped = toInteger((summaryLine.match(/(\d+)(?=\s+Skipped)/) || [])[1]);
  const pending = toInteger((summaryLine.match(/(\d+)(?=\s+Pending)/) || [])[1]);
  const total = passed + failed + skipped + pending;
  const successRate = total > 0 ? Number(((passed / total) * 100).toFixed(2)) : 0;

  return {
    metrics: {
      passed,
      failed,
      errors: 0,
      skipped: skipped + pending,
      total,
      successRate,
    },
    failedTests: [],
    startedAt: null,
  };
}

function getStageDescriptor(storageType, stageName, resultValue) {
  const result = (resultValue || '').trim();
  const stageLabel = stageLabels[stageName] || stageName;
  const reportKind = preE2EStages.has(stageName) ? 'stage-failure' : 'tests';

  if (result === 'cancelled') {
    return {
      failedStage: stageName,
      failedStageLabel: stageLabel,
      failedJobName: `${stageLabel} (${storageType})`,
      reportKind,
      status: 'cancelled',
      statusMessage: `⚠️ ${stageLabel} CANCELLED`,
    };
  }

  return {
    failedStage: stageName,
    failedStageLabel: stageLabel,
    failedJobName: `${stageLabel} (${storageType})`,
    reportKind,
    status: 'failure',
    statusMessage: `❌ ${stageLabel} FAILED`,
  };
}

function determineStage(storageType) {
  const orderedStages = [
    ['bootstrap', process.env.BOOTSTRAP_RESULT],
    ['configure-sdn', process.env.CONFIGURE_SDN_RESULT],
    ['storage-setup', process.env.CONFIGURE_STORAGE_RESULT],
    ['virtualization-setup', process.env.CONFIGURE_VIRTUALIZATION_RESULT],
    ['e2e-test', process.env.E2E_TEST_RESULT],
  ];

  for (const [stageName, resultValue] of orderedStages) {
    if ((resultValue || 'success') !== 'success') {
      return getStageDescriptor(storageType, stageName, resultValue);
    }
  }

  return {
    failedStage: 'success',
    failedStageLabel: stageLabels.success,
    failedJobName: `E2E test (${storageType})`,
    reportKind: 'tests',
    status: 'success',
    statusMessage: '✅ SUCCESS',
  };
}

function buildArtifactMissingDescriptor(storageType) {
  const stageLabel = stageLabels['artifact-missing'];
  return {
    failedStage: 'artifact-missing',
    failedStageLabel: stageLabel,
    failedJobName: `E2E test (${storageType})`,
    reportKind: 'artifact-missing',
    status: 'missing',
    statusMessage: `⚠️ ${stageLabel}`,
  };
}

async function buildClusterReport({core, context}) {
  const storageType = process.env.STORAGE_TYPE;
  const reportsDir = process.env.E2E_REPORT_DIR || 'test/e2e';
  const reportFile = process.env.REPORT_FILE || `e2e_report_${storageType}.json`;
  const workflowRunUrl = process.env.WORKFLOW_RUN_URL
    || `${context.serverUrl}/${context.repo.owner}/${context.repo.repo}/actions/runs/${context.runId}`;
  const branchName = process.env.BRANCH_NAME
    || String(context.ref || '').replace(/^refs\/heads\//, '');
  const junitPattern = new RegExp(`^e2e_summary_${escapeRegExp(storageType)}_.*\\.xml$`);
  const logPattern = new RegExp(`^e2e_summary_${escapeRegExp(storageType)}_.*\\.log$`);
  const junitReportPath = readFirstMatchingFile(reportsDir, junitPattern);
  const summaryLogPath = readFirstMatchingFile(reportsDir, logPattern);
  const stageInfo = determineStage(storageType);

  let parsedReport = {
    metrics: zeroMetrics(),
    failedTests: [],
    startedAt: null,
    source: 'empty',
  };

  if (junitReportPath) {
    core.info(`Found JUnit report: ${junitReportPath}`);
    parsedReport = {
      ...parseJUnitReport(fs.readFileSync(junitReportPath, 'utf8')),
      source: 'junit',
    };
  } else if (summaryLogPath) {
    core.warning(`JUnit report was not found for ${storageType}; falling back to ${summaryLogPath}`);
    const fallbackReport = parseGinkgoSummaryLog(fs.readFileSync(summaryLogPath, 'utf8'));
    if (fallbackReport) {
      parsedReport = {
        ...fallbackReport,
        source: 'ginkgo-log',
      };
    }
  } else {
    core.warning(`JUnit report was not found for ${storageType} under ${reportsDir}`);
  }

  const effectiveStageInfo = (
    stageInfo.reportKind === 'tests' && parsedReport.source === 'empty'
      ? buildArtifactMissingDescriptor(storageType)
      : stageInfo
  );

  const report = {
    cluster: storageType,
    storageType,
    reportKind: effectiveStageInfo.reportKind,
    status: effectiveStageInfo.status,
    statusMessage: effectiveStageInfo.statusMessage,
    failedStage: effectiveStageInfo.failedStage,
    failedStageLabel: effectiveStageInfo.failedStageLabel,
    failedJobName: effectiveStageInfo.failedJobName,
    workflowRunId: String(context.runId),
    workflowRunUrl,
    branch: branchName,
    startedAt: parsedReport.startedAt,
    metrics: parsedReport.metrics,
    failedTests: parsedReport.failedTests,
    sourceJUnitReport: junitReportPath,
    sourceGinkgoLog: summaryLogPath,
    reportSource: parsedReport.source,
  };

  fs.writeFileSync(reportFile, `${JSON.stringify(report, null, 2)}\n`);

  core.setOutput('report_file', reportFile);
  core.info(`Created report file: ${reportFile}`);
  core.info(JSON.stringify(report, null, 2));

  return report;
}

module.exports = buildClusterReport;
module.exports.determineStage = determineStage;
module.exports.parseJUnitReport = parseJUnitReport;
module.exports.parseGinkgoSummaryLog = parseGinkgoSummaryLog;
module.exports.buildArtifactMissingDescriptor = buildArtifactMissingDescriptor;
