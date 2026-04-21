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
const {XMLParser} = require('fast-xml-parser');

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

const junitXmlParser = new XMLParser({
  ignoreAttributes: false,
  attributeNamePrefix: '',
  parseTagValue: false,
  parseAttributeValue: false,
  trimValues: false,
  processEntities: true,
});

function escapeRegExp(value) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

function listMatchingFiles(dirPath, filePattern, files = []) {
  if (!fs.existsSync(dirPath)) {
    return files;
  }

  const entries = fs.readdirSync(dirPath, {withFileTypes: true})
    .sort((left, right) => left.name.localeCompare(right.name));
  for (const entry of entries) {
    const fullPath = path.join(dirPath, entry.name);
    if (entry.isDirectory()) {
      listMatchingFiles(fullPath, filePattern, files);
      continue;
    }

    if (filePattern.test(entry.name)) {
      files.push(fullPath);
    }
  }

  return files;
}

function pickLatestMatchingFile(dirPath, filePattern, core) {
  const matchingFiles = listMatchingFiles(dirPath, filePattern);
  if (matchingFiles.length === 0) {
    return null;
  }

  const rankedFiles = matchingFiles
    .map((filePath) => ({
      filePath,
      mtimeMs: fs.statSync(filePath).mtimeMs,
    }))
    .sort((left, right) => {
      if (right.mtimeMs !== left.mtimeMs) {
        return right.mtimeMs - left.mtimeMs;
      }

      return right.filePath.localeCompare(left.filePath);
    });

  if (rankedFiles.length > 1) {
    core.warning(
      `Found multiple JUnit reports for the cluster; using the newest file: ${rankedFiles[0].filePath}`
    );
  }

  return rankedFiles[0].filePath;
}

function toArray(value) {
  if (!value) {
    return [];
  }

  return Array.isArray(value) ? value : [value];
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

function hasOwnProperty(object, key) {
  return Boolean(object) && Object.prototype.hasOwnProperty.call(object, key);
}

function hasMetricAttributes(node) {
  return ['tests', 'failures', 'errors', 'skipped', 'disabled']
    .some((attributeName) => hasOwnProperty(node, attributeName));
}

function readMetricsFromNode(node) {
  return {
    total: toInteger(node && node.tests),
    failed: toInteger(node && node.failures),
    errors: toInteger(node && node.errors),
    skipped: toInteger((node && (node.skipped || node.disabled)) || 0),
  };
}

function collectSuites(suites, collectedSuites = []) {
  for (const suite of suites) {
    collectedSuites.push(suite);
    collectSuites(toArray(suite.testsuite), collectedSuites);
  }

  return collectedSuites;
}

function collectMetricSuites(suites, collectedSuites = []) {
  for (const suite of suites) {
    const nestedSuites = toArray(suite.testsuite);
    const hasNestedSuites = nestedSuites.length > 0;
    const hasTestcases = toArray(suite.testcase).length > 0;

    if (hasTestcases || !hasNestedSuites) {
      collectedSuites.push(suite);
    }

    if (hasNestedSuites) {
      collectMetricSuites(nestedSuites, collectedSuites);
    }
  }

  return collectedSuites;
}

function parseJUnitReport(xmlContent) {
  const parsedXml = junitXmlParser.parse(xmlContent);
  const testsuitesNode = parsedXml.testsuites || null;
  const topLevelSuites = testsuitesNode
    ? toArray(testsuitesNode.testsuite)
    : toArray(parsedXml.testsuite);
  const allSuites = collectSuites(topLevelSuites);
  const metricSuites = collectMetricSuites(topLevelSuites);
  const aggregateSource = hasMetricAttributes(testsuitesNode)
    ? testsuitesNode
    : topLevelSuites.length === 1 && hasMetricAttributes(topLevelSuites[0])
      ? topLevelSuites[0]
      : null;

  let total = 0;
  let failed = 0;
  let errors = 0;
  let skipped = 0;

  if (aggregateSource) {
    ({total, failed, errors, skipped} = readMetricsFromNode(aggregateSource));
  } else {
    for (const suite of metricSuites) {
      const suiteMetrics = readMetricsFromNode(suite);
      total += suiteMetrics.total;
      failed += suiteMetrics.failed;
      errors += suiteMetrics.errors;
      skipped += suiteMetrics.skipped;
    }
  }

  const passed = Math.max(total - failed - errors - skipped, 0);
  const successRate = total > 0 ? Number(((passed / total) * 100).toFixed(2)) : 0;
  const failedTests = [];

  for (const suite of allSuites) {
    for (const testcase of toArray(suite.testcase)) {
      const testcaseStatus = String(testcase.status || '').toLowerCase();
      const hasFailure = testcase.failure !== undefined;
      const hasError = testcase.error !== undefined;

      if (hasFailure || hasError || testcaseStatus === 'failed' || testcaseStatus === 'error') {
        const testcaseName = String(testcase.name || '').trim();
        if (testcaseName) {
          failedTests.push(testcaseName);
        }
      }
    }
  }

  const startedAt = allSuites.find((suite) => suite.timestamp)?.timestamp || null;

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
  const junitReportPath = pickLatestMatchingFile(reportsDir, junitPattern, core);
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
module.exports.buildArtifactMissingDescriptor = buildArtifactMissingDescriptor;
