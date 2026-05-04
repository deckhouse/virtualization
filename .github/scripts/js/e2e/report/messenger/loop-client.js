/**
 * Parses a Loop API response body if it is JSON, otherwise returns an empty
 * object and emits a warning for diagnostics.
 *
 * @param {string} responseText Raw response body.
 * @param {{ warning(message: string): void }} core GitHub core API.
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
 * @param {{
 *   apiUrl: string,
 *   channelId: string,
 *   token: string,
 *   message: string,
 *   rootId?: string
 * }} request Loop API request payload.
 * @param {{
 *   info(message: string): void,
 *   warning(message: string): void
 * }} core GitHub core API.
 * @returns {Promise<Record<string, any>>} Parsed Loop API response.
 */
async function postToLoopApi(
  { apiUrl, channelId, token, message, rootId },
  core
) {
  const response = await fetch(apiUrl, {
    method: "POST",
    headers: {
      "Authorization": `Bearer ${token}`,
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
 * @param {{
 *   message: string,
 *   threadMessages: string[],
 *   loop: {
 *     apiUrl: string,
 *     channelId: string,
 *     token: string
 *   }
 * }} params Message payload and Loop credentials.
 * @param {{
 *   setOutput(name: string, value: string): void,
 *   info(message: string): void,
 *   warning(message: string): void
 * }} core GitHub core API.
 * @returns {Promise<void>}
 */
async function publishToLoop({ message, threadMessages, loop }, core) {
  if (!loop.apiUrl && !loop.channelId && !loop.token) {
    return;
  }

  if (!loop.apiUrl || !loop.channelId || !loop.token) {
    throw new Error(
      "LOOP_CHANNEL_ID, LOOP_TOKEN, and LOOP_API_BASE_URL are required"
    );
  }

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
  publishToLoop,
};
