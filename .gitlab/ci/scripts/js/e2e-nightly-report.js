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

// GitLab-side driver for the nightly e2e Loop report. It reuses the GitHub
// Actions report scripts in place (.github/scripts/js/e2e/report/*), which are
// written against the Actions `core` / `context` / `github` objects, by
// supplying minimal GitLab-flavoured shims instead.
//
// Two modes (argv[2]):
//   cluster-reports  for each OS in E2E_OS_LIST, turn its raw ginkgo report
//                    (test/e2e/e2e_report_<os>_*.json, pulled from the per-OS
//                    nightly jobs) into a processed PROCESSED_DIR/e2e_report_<os>.json
//                    via cluster-report.js. github is null, so the GitHub-API
//                    stage-URL lookup is skipped (cluster-report.js runs it only
//                    `if (github)`); stage results fall back to "skipped".
//   messenger        render the per-cluster charts' manifest (written by the
//                    python step) into a threaded Loop post via messenger-report.js.

const fs = require("fs");
const path = require("path");

const buildClusterReport = require("../../../../.github/scripts/js/e2e/report/cluster-report.js");
const renderMessengerReport = require("../../../../.github/scripts/js/e2e/report/messenger-report.js");

// The report scripts only ever call core.warning / core.info / core.setOutput
// on these paths (no getInput/setFailed/error), so a three-method shim is enough.
const core = {
  warning: (message) => console.error(`[warning] ${message}`),
  info: (message) => console.log(message),
  setOutput: () => {},
};

// cluster-report.js reads context.{serverUrl,repo.{owner,repo},runId,ref} to
// build the clickable run URL and branch. CI_PROJECT_PATH is "group/sub/repo";
// everything but the last segment is the owner.
function buildContext() {
  const projectPath = String(process.env.CI_PROJECT_PATH || "");
  const segments = projectPath.split("/").filter(Boolean);
  const repo = segments.pop() || "";
  const owner = segments.join("/");
  return {
    serverUrl: process.env.CI_SERVER_URL || "",
    repo: { owner, repo },
    runId: process.env.CI_PIPELINE_ID || "",
    ref: process.env.CI_COMMIT_REF_NAME || "",
  };
}

// cluster-report.js was written for the GitHub pipeline, where a cluster is
// bootstrapped in several stages and each stage's result comes from the job
// graph. Here the cluster is leased from the pool and always ready, so there is
// no bootstrap/SDN/storage/virtualization stage to report: mark them all
// "success" so clusterStatus is "ready". The e2e-test stage is "success" too, so
// the test status is driven by the ginkgo metrics (reported failures flip it to
// "failure") rather than defaulting to "skipped".
const LEASED_CLUSTER_STAGE_RESULTS = {
  bootstrap: "success",
  "configure-sdn": "success",
  "storage-setup": "success",
  "virtualization-setup": "success",
  "e2e-test": "success",
};

function rawReportExists(rawReportsDir, os) {
  const pattern = new RegExp(`^e2e_report_${os}_.*\\.json$`);
  let entries = [];
  try {
    entries = fs.readdirSync(rawReportsDir);
  } catch (error) {
    core.warning(`Cannot read raw reports dir ${rawReportsDir}: ${error.message}`);
    return false;
  }
  return entries.some((name) => pattern.test(name));
}

// The raw ginkgo report is named e2e_report_<os>_<csi>_<date>.json, where <csi>
// is the storage label the run used (replicated / local / nfs; see
// nightly-storage-class.sh). Pull it back out so the report can show which
// storage each OS actually exercised on this run.
function rawStorageClass(rawReportsDir, os) {
  const pattern = new RegExp(`^e2e_report_${os}_(.+)_\\d{4}-\\d{2}-\\d{2}\\.json$`);
  let entries = [];
  try {
    entries = fs.readdirSync(rawReportsDir);
  } catch {
    return null;
  }
  for (const name of entries) {
    const match = name.match(pattern);
    if (match) {
      return match[1];
    }
  }
  return null;
}

// Post-process a processed report for the GitLab nightly:
//   1. record the storage label on the display name, so the Loop table row reads
//      "<os> (<storage>)";
//   2. rewrite the run URL: the shared GitHub script builds it in the GitHub
//      "/actions/runs/<id>" shape, which 404s on GitLab, so swap it for the
//      "/-/pipelines/<id>" shape (serverUrl/owner/repo/id are already correct).
// Only display fields change; storageType stays the bare OS, so cluster
// keying/ordering in the messenger are untouched and the shared report scripts
// need no change.
function finalizeProcessedReport(processedDir, os, storageClass, core) {
  const file = path.join(processedDir, `e2e_report_${os}.json`);
  try {
    const report = JSON.parse(fs.readFileSync(file, "utf8"));
    if (storageClass) {
      report.cluster = `${os} (${storageClass})`;
    }
    if (typeof report.workflowRunUrl === "string") {
      report.workflowRunUrl = report.workflowRunUrl.replace("/actions/runs/", "/-/pipelines/");
    }
    fs.writeFileSync(file, `${JSON.stringify(report, null, 2)}\n`);
  } catch (error) {
    core.warning(`Could not finalize ${os} report: ${error.message}`);
  }
}

async function runClusterReports() {
  const context = buildContext();
  const rawReportsDir = process.env.RAW_REPORTS_DIR || "test/e2e";
  const processedDir = process.env.PROCESSED_DIR || "processed";
  const osList = String(process.env.E2E_OS_LIST || "")
    .split(/\s+/)
    .filter(Boolean);

  if (osList.length === 0) {
    throw new Error("E2E_OS_LIST is empty");
  }

  fs.mkdirSync(processedDir, { recursive: true });

  for (const os of osList) {
    if (!rawReportExists(rawReportsDir, os)) {
      // The OS job failed or produced no report; the messenger step renders a
      // synthetic "missing" row for it from EXPECTED_STORAGE_TYPES.
      core.warning(`No raw ginkgo report for ${os} in ${rawReportsDir}, skipping`);
      continue;
    }
    try {
      await buildClusterReport({
        core,
        context,
        github: null,
        config: {
          storageType: os,
          reportsDir: rawReportsDir,
          reportFile: path.join(processedDir, `e2e_report_${os}.json`),
          stageResults: LEASED_CLUSTER_STAGE_RESULTS,
        },
      });
      finalizeProcessedReport(processedDir, os, rawStorageClass(rawReportsDir, os), core);
    } catch (error) {
      // One OS must not sink the whole report: e.g. findSingleMatchingFile
      // throws when a stand left two raw reports (a retried run). Degrade to the
      // synthetic "missing" row the messenger renders from EXPECTED_STORAGE_TYPES.
      core.warning(`Failed to build cluster report for ${os}: ${error.message}`);
    }
  }
}

async function runMessenger() {
  // messenger-report.js reads REPORTS_DIR, EXPECTED_STORAGE_TYPES, the charts
  // manifest (tmp/messenger-charts/manifest.json) and LOOP_* from the env, and
  // swallows Loop delivery failures into core.warning.
  await renderMessengerReport({ core });
}

async function main() {
  const mode = process.argv[2];
  switch (mode) {
    case "cluster-reports":
      await runClusterReports();
      break;
    case "messenger":
      await runMessenger();
      break;
    default:
      throw new Error(`unknown mode "${mode}", expected cluster-reports or messenger`);
  }
}

main().catch((error) => {
  console.error(error && error.stack ? error.stack : String(error));
  process.exit(1);
});
