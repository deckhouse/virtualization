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
 * @property {string} postsApiUrl
 * @property {string} filesApiUrl
 * @property {string} channelId
 * @property {string} token
 */

/**
 * @typedef {Object} LoopPublishParams
 * @property {string} message
 * @property {Array<{message: string, files: Array<{name: string, buffer: Buffer, mimeType: string}>}>} threadMessages
 * @property {LoopCredentials} loop
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

function createLoopClient({ loop, core, fetch: fetchFn = globalThis.fetch }) {
  async function loopRequest(url, init, errorPrefix) {
    const response = await fetchFn(url, init);
    const responseText = await response.text();
    if (!response.ok) {
      throw new Error(`${errorPrefix} failed with status ${response.status}: ${responseText}`);
    }
    const payload = parseLoopApiPayload(responseText, core);
    core.info(`Loop API accepted ${errorPrefix.toLowerCase()} with status ${response.status}`);
    return payload;
  }

  async function postMessage(message, rootId, fileIds = []) {
    const body = {
      channel_id: loop.channelId,
      message,
      ...(rootId ? { root_id: rootId } : {}),
      ...(fileIds.length > 0 ? { file_ids: fileIds } : {}),
    };
    return loopRequest(
      loop.postsApiUrl,
      {
        method: "POST",
        headers: {
          Authorization: `Bearer ${loop.token}`,
          "Content-Type": "application/json",
        },
        body: JSON.stringify(body),
      },
      "Loop API request"
    );
  }

  async function uploadFile({ name, buffer, mimeType }) {
    const formData = new FormData();
    formData.append("channel_id", loop.channelId);
    formData.append("files", new Blob([buffer], { type: mimeType }), name);
    const payload = await loopRequest(
      loop.filesApiUrl,
      {
        method: "POST",
        headers: {
          Authorization: `Bearer ${loop.token}`,
        },
        body: formData,
      },
      "Loop file upload"
    );
    const fileId = payload.file_infos && payload.file_infos[0] && payload.file_infos[0].id;
    if (!fileId) {
      throw new Error("Loop API did not return uploaded file id");
    }
    return fileId;
  }

  return { postMessage, uploadFile };
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
  const client = createLoopClient({ loop, core, fetch: fetchFn });
  return client.uploadFile({ name: fileName, buffer, mimeType });
}

/**
 * Publishes the main report and optional failed-tests thread to Loop.
 *
 * @param {LoopPublishParams} params Message payload and Loop credentials.
 * @param {LoopClientCore} core GitHub core API.
 * @param {{fetch?: typeof fetch}} [options] Optional HTTP client dependencies.
 * @returns {Promise<void>}
 */
async function makeThreadedReportInLoop(
  { message, threadMessages, loop },
  core,
  { fetch: fetchFn = globalThis.fetch } = {}
) {
  const client = createLoopClient({ loop, core, fetch: fetchFn });
  const rootPost = await client.postMessage(message);

  if (!rootPost.id) {
    throw new Error("Loop API did not return a post id; thread replies cannot be attached");
  }

  for (const reply of threadMessages) {
    const files = Array.isArray(reply.files) ? reply.files : [];
    let fileIds = [];
    if (files.length > 0) {
      const results = await Promise.allSettled(files.map((file) => client.uploadFile(file)));
      fileIds = results.filter((result) => result.status === "fulfilled").map((result) => result.value);

      const failures = results.filter((result) => result.status === "rejected");
      const failureDetails = failures.map((failure) => {
        const reason = failure.reason;
        return reason && reason.message ? reason.message : String(reason);
      });
      for (const details of failureDetails) {
        core.warning(`Loop file upload failed for one attachment: ${details}`);
      }
    }
    await client.postMessage(reply.message, rootPost.id, fileIds);
  }
}

module.exports = {
  makeThreadedReportInLoop,
  uploadFileToLoop,
};
