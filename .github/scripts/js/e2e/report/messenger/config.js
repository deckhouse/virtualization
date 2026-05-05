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

/**
 * Normalizes the configured Loop API base URL to the `/api/v4/posts` endpoint.
 *
 * @param {string} value Raw Loop API base URL.
 * @returns {string} Normalized posts endpoint URL or an empty string.
 */
function normalizeLoopApiBaseUrl(value) {
  const trimmedValue = String(value || "")
    .trim()
    .replace(/\/+$/, "");

  if (!trimmedValue) {
    return "";
  }

  if (trimmedValue.endsWith("/api/v4/posts")) {
    return trimmedValue;
  }

  if (trimmedValue.endsWith("/api/v4")) {
    return `${trimmedValue}/posts`;
  }

  return `${trimmedValue}/api/v4/posts`;
}

/**
 * Reads and normalizes the Loop posts API URL from environment variables.
 *
 * @param {NodeJS.ProcessEnv} [env=process.env] Environment variables source.
 * @returns {string} Normalized posts endpoint URL or an empty string.
 */
function getLoopPostsApiUrl(env = process.env) {
  return normalizeLoopApiBaseUrl(env.LOOP_API_BASE_URL);
}

/**
 * Parses the configured cluster list passed via workflow environment variables.
 *
 * @param {string} value JSON-encoded cluster list.
 * @returns {string[]} Ordered cluster names.
 */
function parseConfiguredClusters(value) {
  const parsedValue = JSON.parse(value || "[]");
  return Array.isArray(parsedValue) ? parsedValue : [];
}

const defaultConfiguredClusters = ["replicated", "nfs"];

/**
 * Reads messenger configuration from the environment.
 *
 * @param {NodeJS.ProcessEnv} [env=process.env] Environment variables source.
 * @returns {{
 *   reportsDir: string,
 *   configuredClusters: string[],
 *   loop: {
 *     apiUrl: string,
 *     channelId: string,
 *     token: string
 *   }
 * }} Normalized messenger configuration.
 */
function readMessengerConfigFromEnv(env = process.env) {
  const configuredClusters = env.EXPECTED_STORAGE_TYPES
    ? parseConfiguredClusters(env.EXPECTED_STORAGE_TYPES)
    : defaultConfiguredClusters;

  return {
    reportsDir: env.REPORTS_DIR || "downloaded-artifacts",
    configuredClusters,
    loop: {
      apiUrl: getLoopPostsApiUrl(env),
      channelId: String(env.LOOP_CHANNEL_ID || "").trim(),
      token: String(env.LOOP_TOKEN || "").trim(),
    },
  };
}

module.exports = {
  getLoopPostsApiUrl,
  readMessengerConfigFromEnv,
};
