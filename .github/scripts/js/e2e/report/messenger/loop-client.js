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
 */

/**
 * @typedef {Object} LoopCredentials
 * @property {string} apiUrl
 * @property {string} channelId
 * @property {string} token
 */

/**
 * @typedef {Object} LoopPublishParams
 * @property {string} message
 * @property {Array<{message: string, files: Array<{name: string, buffer: Buffer, mimeType: string}>}>} threadMessages
 * @property {LoopCredentials & {strictFileUploads?: boolean}} loop
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
    core.warning(`Loop API returned a non-JSON response body: ${error.message}`);
    return {};
  }
}

/**
 * Sends a single post to Loop and returns the parsed API payload.
 *
 * @param {LoopCredentials} loop Loop API credentials.
 * @param {string} message Post body.
 * @param {string} [rootId] Optional thread root id for reply posts.
 * @param {LoopClientCore} core GitHub core API.
 * @param {string[]} [fileIds] Uploaded Loop file ids to attach.
 * @param {{fetch?: typeof fetch}} [options] Optional HTTP client dependencies.
 * @returns {Promise<Record<string, any>>} Parsed Loop API response.
 */
async function postToLoopApi(loop, message, rootId, core, fileIds = [], { fetch: fetchFn = globalThis.fetch } = {}) {
  const body = {
    channel_id: loop.channelId,
    message,
    ...(rootId ? { root_id: rootId } : {}),
    ...(fileIds.length > 0 ? { file_ids: fileIds } : {}),
  };

  const response = await fetchFn(loop.apiUrl, {
    method: "POST",
    headers: {
      Authorization: `Bearer ${loop.token}`,
      "Content-Type": "application/json",
    },
    body: JSON.stringify(body),
  });
  const responseText = await response.text();

  if (!response.ok) {
    throw new Error(`Loop API request failed with status ${response.status}: ${responseText}`);
  }

  const payload = parseLoopApiPayload(responseText, core);
  core.info(`Loop API accepted report with status ${response.status}`);
  return payload;
}

function getFilesApiUrl(apiUrl) {
  return String(apiUrl || "").replace(/\/posts$/, "/files");
}

/**
 * Uploads a single file to Loop and returns the created file id.
 *
 * @param {LoopCredentials} loop Loop API credentials.
 * @param {string} fileName File name shown in Loop.
 * @param {Buffer} buffer File content.
 * @param {LoopClientCore} core GitHub core API.
 * @param {string} mimeType File MIME type.
 * @param {{fetch?: typeof fetch}} [options] Optional HTTP client dependencies.
 * @returns {Promise<string>} Uploaded Loop file id.
 */
async function uploadFileToLoop(loop, fileName, buffer, core, mimeType, { fetch: fetchFn = globalThis.fetch } = {}) {
  const formData = new FormData();
  formData.append("channel_id", loop.channelId);
  formData.append("files", new Blob([buffer], { type: mimeType }), fileName);

  const response = await fetchFn(getFilesApiUrl(loop.apiUrl), {
    method: "POST",
    headers: {
      Authorization: `Bearer ${loop.token}`,
    },
    body: formData,
  });
  const responseText = await response.text();

  if (!response.ok) {
    throw new Error(`Loop file upload failed with status ${response.status}: ${responseText}`);
  }

  const payload = parseLoopApiPayload(responseText, core);
  const fileId = payload.file_infos && payload.file_infos[0] && payload.file_infos[0].id;
  if (!fileId) {
    throw new Error("Loop API did not return uploaded file id");
  }

  core.info(`Loop API accepted file ${fileName} with status ${response.status}`);
  return fileId;
}

/**
 * Publishes the main report and optional failed-tests thread to Loop.
 *
 * @param {LoopPublishParams} params Message payload and Loop credentials.
 * @param {LoopClientCore} core GitHub core API.
 * @param {{fetch?: typeof fetch}} [options] Optional HTTP client dependencies.
 * @returns {Promise<void>}
 */
async function makeThreadedReportInLoop({ message, threadMessages, loop }, core, { 
  fetch: fetchFn = globalThis.fetch } = {}) {
  const rootPost = await postToLoopApi(loop, message, undefined, core, [], { fetch: fetchFn });

  if (!rootPost.id) {
    throw new Error("Loop API did not return a post id; thread replies cannot be attached");
  }

  for (const reply of threadMessages) {
    const files = Array.isArray(reply.files) ? reply.files : [];
    let fileIds = [];
    if (files.length > 0) {
      const results = await Promise.allSettled(
        files.map((file) =>
          uploadFileToLoop(loop, file.name, file.buffer, core, file.mimeType, {
            fetch: fetchFn,
          })
        )
      );
      fileIds = results.filter((result) => result.status === "fulfilled").map((result) => result.value);

      const failures = results.filter((result) => result.status === "rejected");
      for (const failure of failures) {
        const reason = failure.reason;
        const details = reason && reason.message ? reason.message : reason;
        core.warning(`Loop file upload failed for one attachment: ${details}`);
      }
      if (loop.strictFileUploads && failures.length > 0) {
        throw new Error("Strict file uploads enabled; at least one attachment failed");
      }
    }
    await postToLoopApi(loop, reply.message, rootPost.id, core, fileIds, {
      fetch: fetchFn,
    });
  }
}

module.exports = {
  makeThreadedReportInLoop,
  uploadFileToLoop,
};
