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
  const labels = [];

  for (const group of toArray(labelGroups)) {
    for (const label of toArray(group)) {
      const normalizedLabel = String(label || "").trim();
      if (normalizedLabel && !labels.includes(normalizedLabel)) {
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
  const labels = [
    ...flattenLabels(specReport.ContainerHierarchyLabels),
    ...flattenLabels(specReport.LeafNodeLabels),
  ].filter((label, index, array) => array.indexOf(label) === index);
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
function metricKeyForState(state) {
  const normalizedState = String(state || "").trim().toLowerCase();

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

/**
 * Parses a Ginkgo JSON report into metrics and failed test names used by the
 * markdown report.
 *
 * @param {string} jsonContent Raw JSON content.
 * @param {() => {
 *   passed: number,
 *   failed: number,
 *   errors: number,
 *   skipped: number,
 *   total: number,
 *   successRate: number
 * }} createZeroMetrics Factory creating a zeroed metrics object.
 * @returns {{
 *   metrics: {
 *     passed: number,
 *     failed: number,
 *     errors: number,
 *     skipped: number,
 *     total: number,
 *     successRate: number
 *   },
 *   failedTests: string[],
 *   startedAt: string|null
 * }} Parsed report payload.
 */
function parseGinkgoReport(jsonContent, createZeroMetrics) {
  const suites = toArray(JSON.parse(jsonContent));
  const metrics = createZeroMetrics();
  const failedTests = [];
  const startedAt =
    suites.find((suite) => suite && suite.StartTime)?.StartTime || null;

  for (const suite of suites) {
    for (const specReport of toArray(suite && suite.SpecReports)) {
      if (String(specReport && specReport.LeafNodeType) !== "It") {
        continue;
      }

      metrics.total += 1;
      const metricKey = metricKeyForState(specReport.State);
      metrics[metricKey] += 1;

      if (metricKey === "failed" || metricKey === "errors") {
        const specName = formatSpecName(specReport);
        if (specName) {
          failedTests.push(specName);
        }
      }
    }
  }

  metrics.successRate =
    metrics.total > 0
      ? Number(((metrics.passed / metrics.total) * 100).toFixed(2))
      : 0;

  return {
    metrics,
    failedTests: Array.from(new Set(failedTests)),
    startedAt,
  };
}

module.exports = {
  flattenLabels,
  formatSpecName,
  metricKeyForState,
  parseGinkgoReport,
  toArray,
};
