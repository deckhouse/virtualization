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

const fs = require("fs");
const { XMLParser } = require("fast-xml-parser");

const { listMatchingFiles } = require("./fs-utils");

const stageLabels = {
  bootstrap: "BOOTSTRAP CLUSTER",
  "configure-sdn": "CONFIGURE SDN",
  "storage-setup": "STORAGE SETUP",
  "virtualization-setup": "VIRTUALIZATION SETUP",
  "e2e-test": "E2E TEST",
  success: "SUCCESS",
  "artifact-missing": "TEST REPORTS NOT FOUND",
};

const preE2EStages = new Set([
  "bootstrap",
  "configure-sdn",
  "storage-setup",
  "virtualization-setup",
]);

const junitXmlParser = new XMLParser({
  ignoreAttributes: false,
  attributeNamePrefix: "",
  parseTagValue: false,
  parseAttributeValue: false,
  trimValues: false,
  processEntities: true,
});

function escapeRegExp(value) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function readClusterConfigFromEnv(env = process.env) {
  const storageType = env.STORAGE_TYPE;

  return {
    storageType,
    reportsDir: env.E2E_REPORT_DIR || "test/e2e",
    reportFile: env.REPORT_FILE || `e2e_report_${storageType}.json`,
    workflowRunUrlOverride: env.WORKFLOW_RUN_URL || "",
    branchNameOverride: env.BRANCH_NAME || "",
    stageResults: {
      bootstrap: env.BOOTSTRAP_RESULT,
      "configure-sdn": env.CONFIGURE_SDN_RESULT,
      "storage-setup": env.CONFIGURE_STORAGE_RESULT,
      "virtualization-setup": env.CONFIGURE_VIRTUALIZATION_RESULT,
      "e2e-test": env.E2E_TEST_RESULT,
    },
  };
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
  const parsed = Number.parseInt(value || "0", 10);
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
  return ["tests", "failures", "errors", "skipped", "disabled"].some(
    (attributeName) => hasOwnProperty(node, attributeName)
  );
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
    ({ total, failed, errors, skipped } = readMetricsFromNode(aggregateSource));
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
  const successRate =
    total > 0 ? Number(((passed / total) * 100).toFixed(2)) : 0;
  const failedTests = [];

  for (const suite of allSuites) {
    for (const testcase of toArray(suite.testcase)) {
      const testcaseStatus = String(testcase.status || "").toLowerCase();
      const hasFailure = testcase.failure !== undefined;
      const hasError = testcase.error !== undefined;

      if (
        hasFailure ||
        hasError ||
        testcaseStatus === "failed" ||
        testcaseStatus === "error"
      ) {
        const testcaseName = String(testcase.name || "").trim();
        if (testcaseName) {
          failedTests.push(testcaseName);
        }
      }
    }
  }

  const startedAt =
    allSuites.find((suite) => suite.timestamp)?.timestamp || null;

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
  const result = (resultValue || "").trim();
  const stageLabel = stageLabels[stageName] || stageName;
  const reportKind = preE2EStages.has(stageName) ? "stage-failure" : "tests";

  if (result === "cancelled") {
    return {
      failedStage: stageName,
      failedStageLabel: stageLabel,
      failedJobName: `${stageLabel} (${storageType})`,
      reportKind,
      status: "cancelled",
      statusMessage: `⚠️ ${stageLabel} CANCELLED`,
    };
  }

  return {
    failedStage: stageName,
    failedStageLabel: stageLabel,
    failedJobName: `${stageLabel} (${storageType})`,
    reportKind,
    status: "failure",
    statusMessage: `❌ ${stageLabel} FAILED`,
  };
}

function determineStage(storageType, stageResults) {
  const orderedStages = [
    ["bootstrap", stageResults.bootstrap],
    ["configure-sdn", stageResults["configure-sdn"]],
    ["storage-setup", stageResults["storage-setup"]],
    ["virtualization-setup", stageResults["virtualization-setup"]],
    ["e2e-test", stageResults["e2e-test"]],
  ];

  for (const [stageName, resultValue] of orderedStages) {
    if ((resultValue || "success") !== "success") {
      return getStageDescriptor(storageType, stageName, resultValue);
    }
  }

  return {
    failedStage: "success",
    failedStageLabel: stageLabels.success,
    failedJobName: `E2E test (${storageType})`,
    reportKind: "tests",
    status: "success",
    statusMessage: "✅ SUCCESS",
  };
}

function buildArtifactMissingDescriptor(storageType) {
  const stageLabel = stageLabels["artifact-missing"];
  return {
    failedStage: "artifact-missing",
    failedStageLabel: stageLabel,
    failedJobName: `E2E test (${storageType})`,
    reportKind: "artifact-missing",
    status: "missing",
    statusMessage: `⚠️ ${stageLabel}`,
  };
}

function setReportOutputs(report, reportFile, core) {
  core.setOutput("report_file", reportFile);
  core.setOutput("report_kind", report.reportKind || "");
  core.setOutput("status", report.status || "");
  core.setOutput("failed_stage", report.failedStage || "");
  core.setOutput("failed_stage_label", report.failedStageLabel || "");
  core.setOutput("workflow_run_url", report.workflowRunUrl || "");
  core.setOutput("branch", report.branch || "");
}

async function buildClusterReport({ core, context }) {
  const config = readClusterConfigFromEnv();
  const workflowRunUrl =
    config.workflowRunUrlOverride ||
    `${context.serverUrl}/${context.repo.owner}/${context.repo.repo}/actions/runs/${context.runId}`;
  const branchName =
    config.branchNameOverride ||
    String(context.ref || "").replace(/^refs\/heads\//, "");
  const junitPattern = new RegExp(
    `^e2e_summary_${escapeRegExp(config.storageType)}_.*\\.xml$`
  );
  const junitReportPath = pickLatestMatchingFile(
    config.reportsDir,
    junitPattern,
    core
  );
  const stageInfo = determineStage(config.storageType, config.stageResults);

  let parsedReport = {
    metrics: zeroMetrics(),
    failedTests: [],
    startedAt: null,
    source: "empty",
  };

  if (junitReportPath) {
    core.info(`Found JUnit report: ${junitReportPath}`);
    parsedReport = {
      ...parseJUnitReport(fs.readFileSync(junitReportPath, "utf8")),
      source: "junit",
    };
  } else {
    core.warning(
      `JUnit report was not found for ${config.storageType} under ${config.reportsDir}`
    );
  }

  const effectiveStageInfo =
    stageInfo.reportKind === "tests" && parsedReport.source === "empty"
      ? buildArtifactMissingDescriptor(config.storageType)
      : stageInfo;

  const report = {
    cluster: config.storageType,
    storageType: config.storageType,
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

  fs.writeFileSync(config.reportFile, `${JSON.stringify(report, null, 2)}\n`);

  setReportOutputs(report, config.reportFile, core);
  core.info(`Created report file: ${config.reportFile}`);
  core.info(JSON.stringify(report, null, 2));

  return report;
}

module.exports = buildClusterReport;
module.exports.determineStage = determineStage;
module.exports.parseJUnitReport = parseJUnitReport;
module.exports.buildArtifactMissingDescriptor = buildArtifactMissingDescriptor;
module.exports.readClusterConfigFromEnv = readClusterConfigFromEnv;
