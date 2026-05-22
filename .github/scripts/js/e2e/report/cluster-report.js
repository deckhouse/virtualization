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

const { findSingleMatchingFile } = require("./shared/fs-utils");
const { parseGinkgoOutput, parseGinkgoReport } = require("./shared/ginkgo-report-utils");
const {
  archivedReportPattern,
  buildClusterStatus,
  buildReportSummary,
  buildTestStatus,
  ginkgoOutputPattern,
  reportFileName,
  zeroMetrics,
} = require("./shared/report-model");

/**
 * @typedef {Record<string, string|undefined>} StageResults
 */

/**
 * @typedef {Record<string, string|undefined>} StageUrls
 */

/**
 * @typedef {Object} ClusterReportCore
 * @property {function(string): void} info
 * @property {function(string): void} warning
 * @property {function(string, string): void} setOutput
 */

/**
 * @typedef {Object} ClusterReportContext
 * @property {string} serverUrl
 * @property {{ owner: string, repo: string }} repo
 * @property {string|number} runId
 * @property {string} [ref]
 */

/**
 * @typedef {Object} ClusterReportConfig
 * @property {string} storageType
 * @property {string} pipelineJobName
 * @property {string} reportsDir
 * @property {string} reportFile
 * @property {StageResults} stageResults
 * @property {StageUrls} [stageJobUrls]
 */

/**
 * @typedef {Object} ClusterReportParams
 * @property {ClusterReportCore} core
 * @property {ClusterReportContext} context
 * @property {any} [github]
 * @property {ClusterReportConfig} [config]
 */

const workflowStages = [
  { name: "bootstrap",            displayName: "Bootstrap cluster",        needsJobId: "bootstrap" },
  { name: "configure-sdn",        displayName: "Configure SDN",            needsJobId: "configure-sdn" },
  { name: "storage-setup",        displayName: "Configure storage",        needsJobId: "configure-storage" },
  { name: "virtualization-setup", displayName: "Configure Virtualization", needsJobId: "configure-virtualization" },
  { name: "e2e-test",             displayName: "E2E test",                 needsJobId: "e2e-test" },
];

function readClusterReportConfigFromEnv(env = process.env) {
  const storageType = String(env.STORAGE_TYPE || "").trim();

  return {
    storageType,
    pipelineJobName: String(env.PIPELINE_JOB_NAME || "").trim(),
    reportsDir: env.REPORTS_DIR || "test/e2e",
    reportFile: env.REPORT_FILE || reportFileName(storageType),
  };
}

const requiredClusterReportConfigKeys = ["storageType", "reportsDir", "reportFile"];

function requireClusterReportConfig(config) {
  for (const key of requiredClusterReportConfigKeys) {
    if (!config[key]) {
      throw new Error(`buildClusterReport requires ${key}`);
    }
  }

  return { ...config };
}

function getWorkflowRunUrl(context) {
  return `${context.serverUrl}/${context.repo.owner}/${context.repo.repo}/actions/runs/${context.runId}`;
}

function getBranchName(context) {
  return String(context.ref || "").replace(/^refs\/heads\//, "");
}

async function listWorkflowRunJobs(github, context) {
  if (!github || !github.rest || !github.rest.actions) {
    throw new Error("buildClusterReport requires github client");
  }

  const params = {
    owner: context.repo.owner,
    repo: context.repo.repo,
    run_id: context.runId,
    per_page: 100,
  };

  if (github.paginate) {
    return github.paginate(github.rest.actions.listJobsForWorkflowRun, params);
  }

  const response = await github.rest.actions.listJobsForWorkflowRun(params);
  return response.data.jobs || [];
}

function findWorkflowJob(jobs, pipelineJobName, jobName) {
  const nestedJobName = pipelineJobName ? `${pipelineJobName} / ${jobName}` : "";

  return (
    jobs.find((job) => job.name === nestedJobName) ||
    jobs.find((job) => job.name === jobName) ||
    jobs.find((job) => String(job.name || "").endsWith(` / ${jobName}`))
  );
}

function readStageResultsFromEnv(env = process.env) {
  let needs = {};
  try {
    needs = JSON.parse(env.NEEDS_CONTEXT || "{}");
  } catch {
    // malformed JSON — treat all stages as skipped
  }

  const stageResults = {};
  for (const { name, needsJobId } of workflowStages) {
    stageResults[name] = String((needs[needsJobId] || {}).result || "").trim() || "skipped";
  }
  return stageResults;
}

async function readStageJobUrlsFromApi(github, context, config, core) {
  const jobs = await listWorkflowRunJobs(github, context);
  const stageJobUrls = {};

  for (const { name, displayName } of workflowStages) {
    const job = findWorkflowJob(jobs, config.pipelineJobName, displayName);
    if (job) {
      stageJobUrls[name] = job.html_url || "";
    } else {
      core.warning(`Unable to find workflow job "${displayName}" for E2E report`);
    }
  }

  return stageJobUrls;
}

/**
 * Builds a parsed-report payload used as a placeholder when no source data
 * is available, so the downstream report builder can keep working with a
 * uniform shape.
 *
 * @param {string} source Source label to record on the placeholder.
 * @returns {{
 *   metrics: ReturnType<typeof zeroMetrics>,
 *   failedTests: string[],
 *   failedTestDetails: Array<{name: string, reason: string}>,
 *   specTimings: Array<Record<string, any>>,
 *   suiteTotalMs: number,
 *   startedAt: null,
 *   source: string,
 * }} Empty parsed-report payload.
 */
function emptyParsedReport(source) {
  return {
    metrics: zeroMetrics(),
    failedTests: [],
    failedTestDetails: [],
    specTimings: [],
    suiteTotalMs: 0,
    startedAt: null,
    source,
  };
}

const ginkgoJsonSource = {
  label: "Ginkgo JSON report",
  okSource: "ginkgo-json",
  invalidSource: "ginkgo-json-invalid",
  parse: parseGinkgoReport,
  pattern: archivedReportPattern,
};

const ginkgoOutputSource = {
  label: "Ginkgo output log",
  okSource: "ginkgo-output",
  invalidSource: "ginkgo-output-invalid",
  parse: parseGinkgoOutput,
  pattern: ginkgoOutputPattern,
};

/**
 * @typedef {Object} GinkgoSourceDescriptor
 * @property {string} label Human-readable source name for log lines and warnings.
 * @property {string} okSource Source tag stored on a successful parse result.
 * @property {string} invalidSource Source tag stored when parsing fails.
 * @property {function(string): {
 *   metrics: ReturnType<typeof zeroMetrics>,
 *   failedTests: string[],
 *   failedTestDetails: Array<{name: string, reason: string}>,
 *   specTimings: Array<Record<string, any>>,
 *   suiteTotalMs: number,
 *   startedAt: string|null,
 * }} parse Parser function for the source content.
 * @property {function(string): RegExp} pattern Builds the file-name regex for the source.
 */

/**
 * Locates a single Ginkgo source file (JSON report or stdout fallback log)
 * for the configured storage type. Throws when more than one matching file
 * exists; returns null when none is found.
 *
 * @param {ClusterReportConfig} config Resolved cluster report config.
 * @param {GinkgoSourceDescriptor} source Source descriptor.
 * @returns {string|null} Path to the source file, or null when none exists.
 */
function findGinkgoSource(config, source) {
  return findSingleMatchingFile(
    config.reportsDir,
    source.pattern(config.storageType),
    source.label
  );
}

/**
 * Reads and parses a Ginkgo source file (JSON report or stdout log) using
 * the provided source descriptor. Returns an empty placeholder when the
 * file path is missing or the parser throws, so the caller always receives
 * a consistent parsed-report shape.
 *
 * @param {string|null} filePath Path to the source file, or null/empty.
 * @param {ClusterReportCore} core GitHub Actions core API.
 * @param {GinkgoSourceDescriptor} source Source descriptor.
 * @returns {{
 *   metrics: ReturnType<typeof zeroMetrics>,
 *   failedTests: string[],
 *   failedTestDetails: Array<{name: string, reason: string}>,
 *   specTimings: Array<Record<string, any>>,
 *   suiteTotalMs: number,
 *   startedAt: string|null,
 *   source: string,
 * }} Parsed report payload with a source tag.
 */
function parseGinkgoFile(filePath, core, source) {
  if (!filePath) {
    return emptyParsedReport("empty");
  }

  core.info(`Found ${source.label}: ${filePath}`);
  try {
    return {
      ...source.parse(fs.readFileSync(filePath, "utf8")),
      source: source.okSource,
    };
  } catch (error) {
    core.warning(
      `Unable to parse ${source.label} ${filePath}: ${error.message}`
    );
    return emptyParsedReport(source.invalidSource);
  }
}

function buildReportPayload({
  config,
  context,
  fallbackWorkflowRunUrl,
  branchName,
  parsedReport,
  sourcePath,
}) {
  const clusterStatus = buildClusterStatus(config.stageResults);
  const testStatus = buildTestStatus(
    config.stageResults["e2e-test"],
    parsedReport.source,
    clusterStatus,
    parsedReport.metrics
  );
  const reportSummary = buildReportSummary(
    config.storageType,
    clusterStatus,
    testStatus
  );
  const workflowRunUrl = getReportJobUrl(
    reportSummary,
    config.stageJobUrls,
    fallbackWorkflowRunUrl
  );

  return {
    schemaVersion: 1,
    cluster: config.storageType,
    storageType: config.storageType,
    reportKind: reportSummary.reportKind,
    status: reportSummary.status,
    statusMessage: reportSummary.statusMessage,
    failedStage: reportSummary.failedStage,
    failedStageLabel: reportSummary.failedStageLabel,
    failedJobName: reportSummary.failedJobName,
    workflowRunId: String(context.runId),
    workflowRunUrl,
    branch: branchName,
    clusterStatus,
    testStatus,
    startedAt: parsedReport.startedAt,
    metrics: parsedReport.metrics,
    failedTests: parsedReport.failedTests,
    failedTestDetails: parsedReport.failedTestDetails,
    specTimings: parsedReport.specTimings || [],
    suiteTotalMs: parsedReport.suiteTotalMs || 0,
    sourceReport: sourcePath,
    reportSource: parsedReport.source,
  };
}

function getReportJobUrl(
  reportSummary,
  stageJobUrls = {},
  fallbackWorkflowRunUrl
) {
  if (reportSummary.failedStage && stageJobUrls[reportSummary.failedStage]) {
    return stageJobUrls[reportSummary.failedStage];
  }

  if (stageJobUrls["e2e-test"]) {
    return stageJobUrls["e2e-test"];
  }

  return fallbackWorkflowRunUrl;
}

/**
 * Exposes the generated report fields as GitHub Actions step outputs.
 *
 * @param {Record<string, any>} report Final cluster report payload.
 * @param {string} reportFile Path to the written JSON report file.
 * @param {ClusterReportCore} core GitHub core API.
 */
function setReportOutputs(report, reportFile, core) {
  core.setOutput("report_file", reportFile);
  core.setOutput("report_kind", report.reportKind || "");
  core.setOutput("status", report.status || "");
  core.setOutput("failed_stage", report.failedStage || "");
  core.setOutput("failed_stage_label", report.failedStageLabel || "");
  core.setOutput("workflow_run_url", report.workflowRunUrl || "");
  core.setOutput("branch", report.branch || "");
}

/**
 * Builds a per-cluster JSON report from workflow stage results and an optional
 * raw Ginkgo JSON report, writes it to disk, and publishes step outputs.
 *
 * @param {ClusterReportParams} params GitHub script dependencies.
 * @returns {Promise<Record<string, any>>} Generated cluster report.
 * @throws {Error} If config is incomplete or the report file cannot be written.
 */
async function buildClusterReport({ core, context, github, config } = {}) {
  const resolvedConfig = requireClusterReportConfig(config || readClusterReportConfigFromEnv());

  if (!resolvedConfig.stageResults) {
    resolvedConfig.stageResults = readStageResultsFromEnv();
  }

  if (!resolvedConfig.stageJobUrls && github) {
    resolvedConfig.stageJobUrls = await readStageJobUrlsFromApi(github, context, resolvedConfig, core);
  }

  const fallbackWorkflowRunUrl = getWorkflowRunUrl(context);
  const branchName = getBranchName(context);
  const rawReportPath = findGinkgoSource(resolvedConfig, ginkgoJsonSource);
  const outputPath = rawReportPath ? null : findGinkgoSource(resolvedConfig, ginkgoOutputSource);
  const sourcePath = rawReportPath || outputPath;
  const sourceDescriptor = rawReportPath ? ginkgoJsonSource : ginkgoOutputSource;

  if (!rawReportPath) {
    core.warning(
      `Ginkgo JSON report was not found for ${resolvedConfig.storageType} under ${resolvedConfig.reportsDir}`
    );
  }

  const parsedReport = parseGinkgoFile(sourcePath, core, sourceDescriptor);
  const report = buildReportPayload({
    config: resolvedConfig,
    context,
    fallbackWorkflowRunUrl,
    branchName,
    parsedReport,
    sourcePath,
  });

  try {
    fs.writeFileSync(resolvedConfig.reportFile, `${JSON.stringify(report, null, 2)}\n`);
  } catch (error) {
    throw new Error(`Unable to write cluster report file ${resolvedConfig.reportFile}: ${error.message}`);
  }

  setReportOutputs(report, resolvedConfig.reportFile, core);
  core.info(`Created report file: ${resolvedConfig.reportFile}`);
  core.info(JSON.stringify(report, null, 2));

  return report;
}

module.exports = buildClusterReport;
module.exports.readClusterReportConfigFromEnv = readClusterReportConfigFromEnv;
