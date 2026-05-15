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

/**
 * Parses a Ginkgo JSON report into metrics and failed test names used by the
 * markdown report.
 *
 * @param {string} jsonContent Raw JSON content.
 * @returns {{
 *   metrics: GinkgoMetrics,
 *   failedTests: string[],
 *   failedTestDetails: Array<{name: string, reason: string}>,
 *   startedAt: string|null
 * }} Parsed report payload.
 */
function parseGinkgoReport(jsonContent) {
  const suites = toArray(JSON.parse(jsonContent));
  const metrics = zeroMetrics();
  const failedTests = [];
  const failedTestDetails = [];
  const startedAt =
    suites.find((suite) => suite && suite.StartTime)?.StartTime || null;

  for (const suite of suites) {
    for (const specReport of toArray(suite && suite.SpecReports)) {
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

      if (metricKey === "failed" || metricKey === "errors") {
        const specName = formatSpecName(specReport);
        if (specName) {
          failedTests.push(specName);
          failedTestDetails.push({
            name: specName,
            reason: formatFailureReason(specReport),
          });
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
    startedAt,
  };
}

module.exports = {
  parseGinkgoReport,
};
