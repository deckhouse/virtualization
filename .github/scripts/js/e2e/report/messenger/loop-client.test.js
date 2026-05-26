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

const { uploadFileToLoop, makeThreadedReportInLoop } = require("./loop-client");
const { createCore } = require("../shared/test-utils");

function createLoop(overrides = {}) {
  return {
    postsApiUrl: "https://loop.example.invalid/api/v4/posts",
    filesApiUrl: "https://loop.example.invalid/api/v4/files",
    channelId: "channel-id",
    token: "loop-token",
    ...overrides,
  };
}

describe("loop-client", () => {
  afterEach(() => {
    delete global.fetch;
  });

  test("uploads files to Loop multipart endpoint", async () => {
    global.fetch = jest.fn().mockResolvedValue({
      ok: true,
      status: 201,
      text: async () => JSON.stringify({ file_infos: [{ id: "file-id" }] }),
    });

    const fileId = await uploadFileToLoop(
      createLoop(),
      "chart.png",
      Buffer.from("image-bytes"),
      createCore(),
      "image/png"
    );

    expect(fileId).toBe("file-id");
    expect(global.fetch).toHaveBeenCalledWith(
      "https://loop.example.invalid/api/v4/files",
      expect.objectContaining({
        method: "POST",
        headers: {
          Authorization: "Bearer loop-token",
        },
      })
    );

    const body = global.fetch.mock.calls[0][1].body;
    expect(body.get("channel_id")).toBe("channel-id");
    expect(body.get("files").name).toBe("chart.png");
    await expect(body.get("files").text()).resolves.toBe("image-bytes");
  });

  test("posts the reply with uploaded chart file ids", async () => {
    const loop = createLoop();
    const responses = [
      {
        ok: true,
        status: 201,
        text: async () => JSON.stringify({ id: "root-post-id" }),
      },
      {
        ok: true,
        status: 201,
        text: async () => JSON.stringify({ file_infos: [{ id: "file-one" }] }),
      },
      {
        ok: true,
        status: 201,
        text: async () => JSON.stringify({ file_infos: [{ id: "file-two" }] }),
      },
      {
        ok: true,
        status: 201,
        text: async () => JSON.stringify({ id: "reply-post-id" }),
      },
    ];
    global.fetch = jest.fn().mockImplementation(() => Promise.resolve(responses.shift()));

    await makeThreadedReportInLoop(
      {
        message: "main",
        threadMessages: [
          {
            message: "reply",
            files: [
              {
                name: "feature-duration-status.png",
                buffer: Buffer.from("one"),
                mimeType: "image/png",
              },
              {
                name: "feature-duration-status-2.png",
                buffer: Buffer.from("two"),
                mimeType: "image/png",
              },
            ],
          },
        ],
        loop,
      },
      createCore()
    );

    expect(global.fetch).toHaveBeenCalledTimes(4);
    expect(JSON.parse(global.fetch.mock.calls[3][1].body)).toEqual({
      channel_id: "channel-id",
      message: "reply",
      root_id: "root-post-id",
      file_ids: ["file-one", "file-two"],
    });
  });

  test("posts the reply with successful attachments when one upload fails", async () => {
    const loop = createLoop();
    const core = createCore();
    const responses = [
      {
        ok: true,
        status: 201,
        text: async () => JSON.stringify({ id: "root-post-id" }),
      },
      {
        ok: true,
        status: 201,
        text: async () => JSON.stringify({ file_infos: [{ id: "file-one" }] }),
      },
      {
        ok: false,
        status: 403,
        text: async () =>
          JSON.stringify({
            id: "api.context.permissions.app_error",
            message: "permission denied",
          }),
      },
      {
        ok: true,
        status: 201,
        text: async () => JSON.stringify({ id: "reply-post-id" }),
      },
    ];
    global.fetch = jest.fn().mockImplementation(() => Promise.resolve(responses.shift()));

    await makeThreadedReportInLoop(
      {
        message: "main",
        threadMessages: [
          {
            message: "reply",
            files: [
              {
                name: "feature-duration-status.png",
                buffer: Buffer.from("one"),
                mimeType: "image/png",
              },
              {
                name: "feature-duration-status-2.png",
                buffer: Buffer.from("two"),
                mimeType: "image/png",
              },
            ],
          },
        ],
        loop,
      },
      core
    );

    expect(global.fetch).toHaveBeenCalledTimes(4);
    expect(global.fetch.mock.calls[0][0]).toBe(loop.postsApiUrl);
    expect(global.fetch.mock.calls[1][0]).toBe(loop.filesApiUrl);
    expect(global.fetch.mock.calls[2][0]).toBe(loop.filesApiUrl);
    expect(global.fetch.mock.calls[3][0]).toBe(loop.postsApiUrl);

    const replyBody = JSON.parse(global.fetch.mock.calls[3][1].body);
    expect(replyBody.root_id).toBe("root-post-id");
    expect(replyBody.message).toBe("reply");
    expect(replyBody.file_ids).toEqual(["file-one"]);

    expect(core.warning).toHaveBeenCalledWith(
      expect.stringContaining(
        "Loop file upload failed for one attachment: Loop file upload failed with status 403"
      )
    );
    expect(core.warning).toHaveBeenCalledTimes(1);
  });

  test("fails when strict file upload mode is enabled", async () => {
    const loop = createLoop({
      strictFileUploads: true,
    });
    const responses = [
      {
        ok: true,
        status: 201,
        text: async () => JSON.stringify({ id: "root-post-id" }),
      },
      {
        ok: false,
        status: 403,
        text: async () => "permission denied",
      },
    ];
    global.fetch = jest
      .fn()
      .mockImplementation(() => Promise.resolve(responses.shift()));

    await expect(
      makeThreadedReportInLoop(
        {
          message: "main",
          threadMessages: [
            {
              message: "reply",
              files: [
                {
                  name: "chart.png",
                  buffer: Buffer.from("image-bytes"),
                  mimeType: "image/png",
                },
              ],
            },
          ],
          loop,
        },
        createCore()
      )
    ).rejects.toThrow("Strict file uploads enabled; at least one attachment failed");
    expect(global.fetch).toHaveBeenCalledTimes(2);
  });

  test("uses injected fetch without touching the global fetch", async () => {
    const originalFetch = globalThis.fetch;
    const loop = createLoop();
    const responses = [
      {
        ok: true,
        status: 201,
        text: async () => JSON.stringify({ id: "root-post-id" }),
      },
      {
        ok: true,
        status: 201,
        text: async () => JSON.stringify({ id: "reply-post-id" }),
      },
    ];
    const fetch = jest.fn().mockImplementation(() => Promise.resolve(responses.shift()));

    await makeThreadedReportInLoop(
      {
        message: "main",
        threadMessages: [{ message: "reply", files: [] }],
        loop,
      },
      createCore(),
      { fetch }
    );

    expect(fetch).toHaveBeenCalledTimes(2);
    expect(globalThis.fetch).toBe(originalFetch);
  });
});
