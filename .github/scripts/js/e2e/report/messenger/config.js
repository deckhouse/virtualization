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

const loopApiRootSegments = ["api", "v4"];

function getUrlPathSegments(url) {
  return url.pathname.split("/").filter(Boolean);
}

function buildUrlPath(segments) {
  return `/${segments.join("/")}`;
}

function findLoopApiRootIndex(segments) {
  return segments.findIndex(
    (segment, index) =>
      segment === loopApiRootSegments[0] &&
      segments[index + 1] === loopApiRootSegments[1]
  );
}

function buildLoopEndpointUrl(apiBaseUrl, endpoint) {
  const url = new URL(apiBaseUrl);
  url.pathname = buildUrlPath([...getUrlPathSegments(url), endpoint]);
  return url.toString();
}

/**
 * Normalizes the configured Loop API base URL to the `/api/v4` root.
 *
 * @param {string} value Raw Loop API base URL.
 * @returns {string} Normalized API root URL or an empty string.
 */
function normalizeLoopApiBaseUrl(value) {
  const rawValue = String(value || "").trim();

  if (!rawValue) {
    return "";
  }

  const url = new URL(rawValue);
  const pathSegments = getUrlPathSegments(url);
  const apiRootIndex = findLoopApiRootIndex(pathSegments);
  const apiRootSegments =
    apiRootIndex === -1
      ? [...pathSegments, ...loopApiRootSegments]
      : pathSegments.slice(0, apiRootIndex + loopApiRootSegments.length);

  url.pathname = buildUrlPath(apiRootSegments);
  url.search = "";
  url.hash = "";
  return url.toString();
}

// Fallback used only when EXPECTED_STORAGE_TYPES is not set (e.g. local runs or tests).
// In CI the list is passed explicitly via the EXPECTED_STORAGE_TYPES env variable.
const defaultConfiguredClusters = ["replicated", "nfs"];

/**
 * Parses the configured cluster list passed via workflow environment variables.
 * Returns the default cluster list when the value is absent, is not valid JSON,
 * or parses to a non-array value (e.g. an object `{}`).
 *
 * @param {string} value JSON-encoded array of cluster names, e.g. '["replicated","nfs"]'.
 * @returns {string[]} Ordered cluster names.
 */
function parseConfiguredClusters(value) {
  try {
    const parsed = JSON.parse(value || "{}");
    return Array.isArray(parsed) ? parsed : defaultConfiguredClusters;
  } catch {
    return defaultConfiguredClusters;
  }
}

function parseBooleanEnv(value) {
  return ["1", "true", "yes"].includes(String(value || "").toLowerCase());
}

/**
 * Reads Loop credentials from the environment.
 *
 * Returns `null` when none of the Loop variables are set, indicating that the
 * messenger integration is intentionally disabled (e.g. local runs or forks).
 * Throws when only some variables are present — that is always a configuration
 * mistake and should surface as an error rather than a silent no-op.
 *
 * @param {NodeJS.ProcessEnv} [env=process.env] Environment variables source.
 * @returns {{ postsApiUrl: string, filesApiUrl: string, channelId: string, token: string, strictDelivery: boolean, strictFileUploads: boolean } | null}
 */
function readLoopConfig(env = process.env) {
  const apiBaseUrl = normalizeLoopApiBaseUrl(env.LOOP_API_BASE_URL);
  const channelId = String(env.LOOP_CHANNEL_ID || "").trim();
  const token = String(env.LOOP_TOKEN || "").trim();

  if (!apiBaseUrl && !channelId && !token) {
    return null;
  }
  if (!apiBaseUrl || !channelId || !token) {
    throw new Error("LOOP_CHANNEL_ID, LOOP_TOKEN, and LOOP_API_BASE_URL are required");
  }
  return {
    postsApiUrl: buildLoopEndpointUrl(apiBaseUrl, "posts"),
    filesApiUrl: buildLoopEndpointUrl(apiBaseUrl, "files"),
    channelId,
    token,
    strictDelivery: parseBooleanEnv(env.LOOP_STRICT_DELIVERY),
    strictFileUploads: parseBooleanEnv(env.LOOP_STRICT_FILE_UPLOAD),
  };
}

/**
 * Reads messenger configuration from the environment.
 *
 * @param {NodeJS.ProcessEnv} [env=process.env] Environment variables source.
 * @returns {{
 *   reportsDir: string,
 *   configuredClusters: string[],
 *   loop: { postsApiUrl: string, filesApiUrl: string, channelId: string, token: string, strictDelivery: boolean, strictFileUploads: boolean } | null
 * }} Normalized messenger configuration.
 */
function readMessengerConfigFromEnv(env = process.env) {
  const configuredClusters = env.EXPECTED_STORAGE_TYPES
    ? parseConfiguredClusters(env.EXPECTED_STORAGE_TYPES)
    : defaultConfiguredClusters;

  return {
    reportsDir: env.REPORTS_DIR || "downloaded-artifacts",
    configuredClusters,
    loop: readLoopConfig(env),
  };
}

module.exports = {
  readLoopConfig,
  readMessengerConfigFromEnv,
};
