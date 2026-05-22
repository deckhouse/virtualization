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

const { zeroMetrics } = require("./report-model");

/**
 * @typedef {Object} GinkgoMetrics
 * @property {number} passed
 * @property {number} failed
 * @property {number} errors
 * @property {number} skipped
 * @property {number} total
 * @property {number} successRate
 */

/**
 * Normalizes a value into an array.
 *
 * @param {any} value Input value.
 * @returns {any[]} Array view of the input.
 */
function toArray(value) {
  if (!value) {
    return [];
  }

  return Array.isArray(value) ? value : [value];
}

/**
 * Flattens nested Ginkgo label arrays into a stable, unique list.
 *
 * @param {Array<string|string[]|null>|string[]|null} labelGroups Raw label data.
 * @returns {string[]} Flattened unique labels.
 */
function flattenLabels(labelGroups) {
  const seen = new Set();
  const labels = [];

  for (const group of toArray(labelGroups)) {
    for (const label of toArray(group)) {
      const normalizedLabel = String(label || "").trim();
      if (normalizedLabel && !seen.has(normalizedLabel)) {
        seen.add(normalizedLabel);
        labels.push(normalizedLabel);
      }
    }
  }

  return labels;
}

/**
 * Builds a human-readable test name close to the JUnit testcase naming that
 * existing reports already expose to messenger output.
 *
 * @param {Record<string, any>} specReport Raw Ginkgo spec report entry.
 * @returns {string} Formatted test name.
 */
function formatSpecName(specReport) {
  const nodeType = String(specReport.LeafNodeType || "Spec").trim();
  const hierarchyParts = toArray(specReport.ContainerHierarchyTexts)
    .map((part) => String(part || "").trim())
    .filter(Boolean);
  const leafText = String(specReport.LeafNodeText || "").trim();
  const labels = [...new Set([
    ...flattenLabels(specReport.ContainerHierarchyLabels),
    ...flattenLabels(specReport.LeafNodeLabels),
  ])];
  const labelSuffix = labels.map((label) => `[${label}]`).join(" ");
  const body = [...hierarchyParts, leafText].filter(Boolean).join(" ");

  return [`[${nodeType}]`, body, labelSuffix]
    .filter(Boolean)
    .join(" ")
    .replace(/\s+/g, " ")
    .trim();
}

function runtimeMs(value) {
  const runtime = Number(value || 0);
  return Number.isFinite(runtime) ? Math.round(runtime / 1_000_000) : 0;
}

/**
 * Maps a raw Ginkgo spec state into the metrics bucket used by the final
 * messenger report.
 *
 * @param {string} state Raw `SpecReport.State` value.
 * @returns {"passed"|"failed"|"errors"|"skipped"} Metrics key.
 */
function getMetricKeyForState(state) {
  const normalizedState = String(state || "")
    .trim()
    .toLowerCase();

  if (normalizedState === "passed") {
    return "passed";
  }

  if (normalizedState === "failed") {
    return "failed";
  }

  if (normalizedState === "skipped" || normalizedState === "pending") {
    return "skipped";
  }

  return "errors";
}

function formatFailureReason(specReport) {
  const failure = (specReport && specReport.Failure) || {};
  return (
    String(failure.Message || failure.ForwardedPanic || "").trim() ||
    String(specReport.State || "failed").trim()
  );
}

const failureStates = new Set(["failed", "errors"]);

function isSuiteNodeFailure(specReport) {
  const leafNodeType = String((specReport && specReport.LeafNodeType) || "").trim();
  if (!leafNodeType || leafNodeType === "It") {
    return false;
  }

  return failureStates.has(getMetricKeyForState(specReport && specReport.State));
}

function buildFailureDetail(specReport) {
  const specName = formatSpecName(specReport);
  if (!specName) {
    return null;
  }

  return {
    name: specName,
    reason: formatFailureReason(specReport),
  };
}

/**
 * Parses a Ginkgo JSON report into metrics and failed test names used by the
 * markdown report.
 *
 * @param {string} jsonContent Raw JSON content.
 * @returns {{
 *   metrics: GinkgoMetrics,
 *   failedTests: string[],
 *   failedTestDetails: Array<{name: string, reason: string}>,
 *   specTimings: Array<{name: string, group: string, state: string, runtimeMs: number, labels: string[]}>,
 *   suiteTotalMs: number,
 *   startedAt: string|null
 * }} Parsed report payload.
 */
function parseGinkgoReport(jsonContent) {
  const suites = toArray(JSON.parse(jsonContent));
  const metrics = zeroMetrics();
  const failedTests = [];
  const failedTestDetails = [];
  const specTimings = [];
  const startedAt =
    suites.find((suite) => suite && suite.StartTime)?.StartTime || null;
  let suiteTotalMs = 0;

  for (const suite of suites) {
    suiteTotalMs += runtimeMs(suite && suite.RunTime);

    for (const specReport of toArray(suite && suite.SpecReports)) {
      if (isSuiteNodeFailure(specReport)) {
        const failureDetail = buildFailureDetail(specReport);
        if (failureDetail) {
          failedTests.push(failureDetail.name);
          failedTestDetails.push(failureDetail);
        }
        continue;
      }

      // SpecReports can contain suite-level setup/teardown entries
      // (BeforeSuite, AfterSuite, etc.) in addition to regular specs.
      // `Specify` is a pure alias for `It` and serializes to the same
      // "It" value. We only count actual spec nodes in the metrics.
      if (String(specReport && specReport.LeafNodeType) !== "It") {
        continue;
      }

      metrics.total += 1;
      const metricKey = getMetricKeyForState(specReport.State);
      metrics[metricKey] += 1;
      const hierarchyParts = toArray(specReport.ContainerHierarchyTexts)
        .map((part) => String(part || "").trim())
        .filter(Boolean);
      const leafText = String(specReport.LeafNodeText || "").trim();
      specTimings.push({
        name: leafText,
        group: hierarchyParts[0] || "Top-level Its",
        state: metricKey,
        runtimeMs: runtimeMs(specReport.RunTime),
        labels: flattenLabels([
          ...toArray(specReport.ContainerHierarchyLabels),
          ...toArray(specReport.LeafNodeLabels),
        ]),
      });

      if (failureStates.has(metricKey)) {
        const failureDetail = buildFailureDetail(specReport);
        if (failureDetail) {
          failedTests.push(failureDetail.name);
          failedTestDetails.push(failureDetail);
        }
      }
    }
  }

  const completedSpecs = metrics.passed + metrics.failed + metrics.errors;
  metrics.successRate =
    completedSpecs > 0
      ? Number(((metrics.passed / completedSpecs) * 100).toFixed(2))
      : 0;

  return {
    metrics,
    failedTests: Array.from(new Set(failedTests)),
    failedTestDetails: Array.from(
      new Map(
        failedTestDetails.map((test) => [
          `${test.name}\u0000${test.reason}`,
          test,
        ])
      ).values()
    ),
    specTimings,
    suiteTotalMs,
    startedAt,
  };
}

const suiteNodeTypes = [
  "SynchronizedBeforeSuite",
  "BeforeSuite",
  "SynchronizedAfterSuite",
  "AfterSuite",
];

// Match Ginkgo failure markers for suite-level nodes in two forms:
//   1. "[<SuiteNode>] [FAILED]"   — main failure line in the stdout body.
//   2. "[FAIL] [<SuiteNode>]"     — line from the "Summarizing N Failure:" footer.
// Both forms guarantee that the matched suite node actually failed, so we
// never confuse them with the plain "[<SuiteNode>]" section header.
const suiteNodeAlternatives = suiteNodeTypes.join("|");
const suiteNodePattern = new RegExp(
  `(?:\\[(${suiteNodeAlternatives})\\]\\s+\\[FAILED\\])|(?:\\[FAIL\\]\\s+\\[(${suiteNodeAlternatives})\\])`
);

// Lines that mark the end of the failure block in Ginkgo stdout. Anything
// after these belongs to the next suite section or the summary footer.
const reasonStopPrefixes = [
  "------------------------------",
  "[SynchronizedAfterSuite]",
  "[ReportAfterSuite]",
  "Summarizing ",
];

const maxReasonLines = 6;

/**
 * Detects the first suite-level Ginkgo node that failed in the given stdout.
 * Returns the suite node name (for example, "SynchronizedBeforeSuite") or
 * an empty string when there is no suite failure marker in the output.
 *
 * @param {string} output Raw Ginkgo stdout/stderr content.
 * @returns {string} Suite node name or "" when no failure was detected.
 */
function findFailedSuiteNode(output) {
  const match = output.match(suiteNodePattern);
  if (!match) {
    return "";
  }

  // The pattern has two alternatives, so the captured node name is in
  // either group 1 ("[X] [FAILED]") or group 2 ("[FAIL] [X]").
  return match[1] || match[2] || "";
}

function isReasonStopLine(line) {
  return reasonStopPrefixes.some((prefix) => line.startsWith(prefix));
}

function isReasonNoiseLine(line, suiteHeader, failedMarker) {
  return (
    line === suiteHeader ||
    line.startsWith(failedMarker) ||
    line.startsWith("/")
  );
}

/**
 * Extracts a short human-readable failure reason for a failed suite-level
 * node from Ginkgo stdout. Walks the failure block starting at the
 * "[<SuiteNode>] [FAILED]" marker, skipping section headers and source
 * file locations, and stops at the next suite/report section or summary
 * footer. The result is at most `maxReasonLines` non-empty lines joined
 * with a newline, or a generic fallback string when nothing meaningful
 * was found.
 *
 * @param {string} output Raw Ginkgo stdout/stderr content.
 * @param {string} suiteNodeType Suite node name returned by `findFailedSuiteNode`.
 * @returns {string} Multi-line reason string for the failed suite node.
 */
function extractFailureReasonFromOutput(output, suiteNodeType) {
  const suiteHeader = `[${suiteNodeType}]`;
  const failedMarker = `${suiteHeader} [FAILED]`;
  const failedIndex = output.indexOf(failedMarker);
  const failureBlock = failedIndex >= 0 ? output.slice(failedIndex) : output;
  const reasonLines = [];

  for (const rawLine of failureBlock.split(/\r?\n/)) {
    const line = rawLine.trim();
    if (!line) {
      continue;
    }
    if (isReasonStopLine(line)) {
      break;
    }
    if (isReasonNoiseLine(line, suiteHeader, failedMarker)) {
      continue;
    }

    reasonLines.push(line.replace(/^\[FAILED\]\s*/, ""));
    if (reasonLines.length >= maxReasonLines) {
      break;
    }
  }

  return reasonLines.join("\n") || "Ginkgo suite setup failed";
}

/**
 * Parses raw Ginkgo stdout/stderr output as a fallback report source for
 * cases when the JSON report is missing. Currently surfaces only
 * suite-level failures (BeforeSuite/AfterSuite); regular spec metrics are
 * left at zero because per-spec accounting cannot be reliably recovered
 * from plain text.
 *
 * @param {string} outputContent Raw Ginkgo output content.
 * @returns {{
 *   metrics: GinkgoMetrics,
 *   failedTests: string[],
 *   failedTestDetails: Array<{name: string, reason: string}>,
 *   specTimings: [],
 *   suiteTotalMs: 0,
 *   startedAt: null,
 * }} Parsed fallback payload.
 */
function parseGinkgoOutput(outputContent) {
  const output = String(outputContent || "");
  const suiteNodeType = findFailedSuiteNode(output);
  const result = {
    metrics: zeroMetrics(),
    failedTests: [],
    failedTestDetails: [],
    specTimings: [],
    suiteTotalMs: 0,
    startedAt: null,
  };

  if (!suiteNodeType) {
    return result;
  }

  const name = `[${suiteNodeType}]`;
  const reason = extractFailureReasonFromOutput(output, suiteNodeType);
  result.failedTests.push(name);
  result.failedTestDetails.push({ name, reason });
  return result;
}

module.exports = {
  parseGinkgoOutput,
  parseGinkgoReport,
};
