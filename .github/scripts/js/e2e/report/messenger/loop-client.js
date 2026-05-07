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
 * @typedef {Object} LoopClientCore
 * @property {function(string): void} warning
 * @property {function(string): void} [info]
 * @property {function(string, string): void} [setOutput]
 */

/**
 * @typedef {Object} LoopPostRequest
 * @property {string} apiUrl
 * @property {string} channelId
 * @property {string} token
 * @property {string} message
 * @property {string} [rootId]
 */

/**
 * @typedef {Object} LoopPublishParams
 * @property {string} message
 * @property {string[]} threadMessages
 * @property {{ apiUrl: string, channelId: string, token: string }} loop
 */

/**
 * Parses a Loop API response body if it is JSON, otherwise returns an empty
 * object and emits a warning for diagnostics.
 *
 * @param {string} responseText Raw response body.
 * @param {LoopClientCore} core GitHub core API.
 * @returns {Record<string, any>} Parsed response payload or an empty object.
 */
function parseLoopApiPayload(responseText, core) {
  if (!responseText) {
    return {};
  }

  try {
    return JSON.parse(responseText);
  } catch (error) {
    core.warning(
      `Loop API returned a non-JSON response body: ${error.message}`
    );
    return {};
  }
}

/**
 * Sends a single post to Loop and returns the parsed API payload.
 *
 * @param {LoopPostRequest} request Loop API request payload.
 * @param {LoopClientCore} core GitHub core API.
 * @returns {Promise<Record<string, any>>} Parsed Loop API response.
 */
async function postToLoopApi(
  { apiUrl, channelId, token, message, rootId },
  core
) {
  const response = await fetch(apiUrl, {
    method: "POST",
    headers: {
      Authorization: `Bearer ${token}`,
      "Content-Type": "application/json",
    },
    body: JSON.stringify({
      channel_id: channelId,
      message,
      ...(rootId ? { root_id: rootId } : {}),
    }),
  });
  const responseText = await response.text();

  if (!response.ok) {
    throw new Error(
      `Loop API request failed with status ${response.status}: ${responseText}`
    );
  }

  const payload = parseLoopApiPayload(responseText, core);
  core.info(`Loop API accepted report with status ${response.status}`);
  return payload;
}

/**
 * Publishes the main report and optional failed-tests thread to Loop.
 *
 * @param {LoopPublishParams} params Message payload and Loop credentials.
 * @param {LoopClientCore} core GitHub core API.
 * @returns {Promise<void>}
 */
async function makeThreadedReportInLoop({ message, threadMessages, loop }, core) {
  const rootPost = await postToLoopApi(
    {
      apiUrl: loop.apiUrl,
      channelId: loop.channelId,
      token: loop.token,
      message,
    },
    core
  );

  let lastReplyPost = null;
  for (const replyMessage of threadMessages) {
    lastReplyPost = await postToLoopApi(
      {
        apiUrl: loop.apiUrl,
        channelId: loop.channelId,
        token: loop.token,
        message: replyMessage,
        rootId: rootPost.id,
      },
      core
    );
  }

  core.setOutput("root_post_id", rootPost.id || "");
  core.setOutput(
    "thread_post_id",
    lastReplyPost && lastReplyPost.id ? lastReplyPost.id : ""
  );
}

module.exports = {
  makeThreadedReportInLoop,
};
