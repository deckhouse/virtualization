const fs = require("fs");
const os = require("os");
const path = require("path");

const renderMessengerReport = require("./messenger-report");
const { readMessengerConfigFromEnv } = require("./messenger-report");

function createCore() {
  return {
    info: jest.fn(),
    warning: jest.fn(),
    debug: jest.fn(),
    setOutput: jest.fn(),
  };
}

async function withTempDir(testFn) {
  const tempDir = fs.mkdtempSync(
    path.join(os.tmpdir(), "messenger-report-test-")
  );
  try {
    return await testFn(tempDir);
  } finally {
    fs.rmSync(tempDir, { recursive: true, force: true });
  }
}

describe("messenger-report", () => {
  afterEach(() => {
    delete process.env.REPORTS_DIR;
    delete process.env.STORAGE_TYPES;
    delete process.env.REPORT_FALLBACK_REPLICATED_REPORT_KIND;
    delete process.env.REPORT_FALLBACK_REPLICATED_STATUS;
    delete process.env.REPORT_FALLBACK_REPLICATED_FAILED_STAGE;
    delete process.env.REPORT_FALLBACK_REPLICATED_FAILED_STAGE_LABEL;
    delete process.env.REPORT_FALLBACK_REPLICATED_WORKFLOW_RUN_URL;
    delete process.env.REPORT_FALLBACK_REPLICATED_BRANCH;
    delete process.env.REPORT_FALLBACK_NFS_REPORT_KIND;
    delete process.env.REPORT_FALLBACK_NFS_STATUS;
    delete process.env.REPORT_FALLBACK_NFS_FAILED_STAGE;
    delete process.env.REPORT_FALLBACK_NFS_FAILED_STAGE_LABEL;
    delete process.env.REPORT_FALLBACK_NFS_WORKFLOW_RUN_URL;
    delete process.env.REPORT_FALLBACK_NFS_BRANCH;
    delete process.env.LOOP_API_BASE_URL;
    delete process.env.LOOP_CHANNEL_ID;
    delete process.env.LOOP_TOKEN;
    delete global.fetch;
  });

  test("reads normalized messenger config from env", () => {
    const config = readMessengerConfigFromEnv({
      REPORTS_DIR: "custom-reports",
      STORAGE_TYPES: '["replicated","nfs"]',
      LOOP_API_BASE_URL: "https://loop.example.invalid/api/v4/",
      LOOP_CHANNEL_ID: " channel-id ",
      LOOP_TOKEN: " token ",
    });

    expect(config).toEqual({
      reportsDir: "custom-reports",
      configuredClusters: ["replicated", "nfs"],
      reportFallbacks: {},
      loop: {
        apiUrl: "https://loop.example.invalid/api/v4/posts",
        channelId: "channel-id",
        token: "token",
      },
    });
  });

  test("renders test results and stage failures in separate sections", async () =>
    withTempDir(async (tempDir) => {
      fs.writeFileSync(
        path.join(tempDir, "e2e_report_replicated.json"),
        JSON.stringify({
          cluster: "replicated",
          storageType: "replicated",
          reportKind: "tests",
          branch: "main",
          workflowRunUrl: "https://example.invalid/replicated",
          startedAt: "2026-04-15T09:30:44",
          metrics: {
            passed: 12,
            skipped: 2,
            failed: 1,
            errors: 0,
            total: 15,
            successRate: 80,
          },
          failedTests: ["[It] fails"],
        })
      );

      fs.writeFileSync(
        path.join(tempDir, "e2e_report_nfs.json"),
        JSON.stringify({
          cluster: "nfs",
          storageType: "nfs",
          reportKind: "stage-failure",
          branch: "main",
          workflowRunUrl: "https://example.invalid/nfs",
          failedStage: "configure-sdn",
          failedStageLabel: "CONFIGURE SDN",
          metrics: {
            passed: 0,
            failed: 0,
            errors: 0,
            total: 0,
            successRate: 0,
          },
          failedTests: [],
        })
      );

      process.env.REPORTS_DIR = tempDir;
      process.env.STORAGE_TYPES = '["replicated","nfs"]';

      const result = await renderMessengerReport({ core: createCore() });

      expect(result.message).toContain("### Test results");
      expect(result.message).toContain(
        "| [replicated](https://example.invalid/replicated) | 12 | 2 | 1 | 0 | 15 | 80.00% |"
      );
      expect(result.message).toContain("### Cluster failures");
      expect(result.message).toContain(
        "- [nfs](https://example.invalid/nfs): CONFIGURE SDN"
      );
      expect(result.message).not.toContain("### Failed tests");
      expect(result.threadMessage).toContain("### Failed tests");
      expect(result.threadMessage).toContain("**replicated**");
      expect(result.threadMessage).toContain("- [It] fails");
      expect(result.threadMessage).not.toContain("**nfs**");
    }));

  test("creates artifact-missing entry for absent cluster report", async () =>
    withTempDir(async (tempDir) => {
      process.env.REPORTS_DIR = tempDir;
      process.env.STORAGE_TYPES = '["replicated"]';

      const result = await renderMessengerReport({ core: createCore() });

      expect(result.message).toContain("### Cluster failures");
      expect(result.message).toContain(
        "- replicated: E2E REPORT ARTIFACT NOT FOUND"
      );
      expect(result.threadMessage).toBe("");
    }));

  test("uses workflow fallback metadata for missing cluster report", async () =>
    withTempDir(async (tempDir) => {
      process.env.REPORTS_DIR = tempDir;
      process.env.STORAGE_TYPES = '["replicated"]';
      process.env.REPORT_FALLBACK_REPLICATED_REPORT_KIND = "stage-failure";
      process.env.REPORT_FALLBACK_REPLICATED_STATUS = "failure";
      process.env.REPORT_FALLBACK_REPLICATED_FAILED_STAGE = "configure-sdn";
      process.env.REPORT_FALLBACK_REPLICATED_FAILED_STAGE_LABEL =
        "CONFIGURE SDN";
      process.env.REPORT_FALLBACK_REPLICATED_WORKFLOW_RUN_URL =
        "https://example.invalid/replicated";
      process.env.REPORT_FALLBACK_REPLICATED_BRANCH = "main";

      const result = await renderMessengerReport({ core: createCore() });

      expect(result.message).toContain("Branch: `main`");
      expect(result.message).toContain(
        "- [replicated](https://example.invalid/replicated): CONFIGURE SDN"
      );
      expect(result.threadMessage).toBe("");
    }));

  test("preserves test-reports-missing fallback from workflow metadata", async () =>
    withTempDir(async (tempDir) => {
      process.env.REPORTS_DIR = tempDir;
      process.env.STORAGE_TYPES = '["replicated"]';
      process.env.REPORT_FALLBACK_REPLICATED_REPORT_KIND = "artifact-missing";
      process.env.REPORT_FALLBACK_REPLICATED_STATUS = "missing";
      process.env.REPORT_FALLBACK_REPLICATED_FAILED_STAGE = "artifact-missing";
      process.env.REPORT_FALLBACK_REPLICATED_FAILED_STAGE_LABEL =
        "TEST REPORTS NOT FOUND";
      process.env.REPORT_FALLBACK_REPLICATED_WORKFLOW_RUN_URL =
        "https://example.invalid/replicated";

      const result = await renderMessengerReport({ core: createCore() });

      expect(result.message).toContain(
        "- [replicated](https://example.invalid/replicated): TEST REPORTS NOT FOUND"
      );
      expect(result.threadMessage).toBe("");
    }));

  test("posts main report and failed tests thread via Loop API", async () =>
    withTempDir(async (tempDir) => {
      fs.writeFileSync(
        path.join(tempDir, "e2e_report_replicated.json"),
        JSON.stringify({
          cluster: "replicated",
          storageType: "replicated",
          reportKind: "tests",
          branch: "main",
          workflowRunUrl: "https://example.invalid/replicated",
          startedAt: "2026-04-15T09:30:44",
          metrics: {
            passed: 10,
            skipped: 1,
            failed: 1,
            errors: 0,
            total: 12,
            successRate: 83.33,
          },
          failedTests: ["[It] fails"],
        })
      );

      process.env.REPORTS_DIR = tempDir;
      process.env.STORAGE_TYPES = '["replicated"]';
      process.env.LOOP_API_BASE_URL = "https://loop.example.invalid";
      process.env.LOOP_CHANNEL_ID = "channel-id";
      process.env.LOOP_TOKEN = "loop-token";

      global.fetch = jest
        .fn()
        .mockResolvedValueOnce({
          ok: true,
          status: 201,
          text: async () => JSON.stringify({ id: "root-post-id" }),
        })
        .mockResolvedValueOnce({
          ok: true,
          status: 201,
          text: async () => JSON.stringify({ id: "thread-post-id" }),
        });

      const result = await renderMessengerReport({ core: createCore() });

      expect(global.fetch).toHaveBeenCalledTimes(2);
      expect(global.fetch).toHaveBeenNthCalledWith(
        1,
        "https://loop.example.invalid/api/v4/posts",
        expect.objectContaining({
          method: "POST",
          headers: expect.objectContaining({
            Authorization: "Bearer loop-token",
            "Content-Type": "application/json",
          }),
        })
      );
      expect(JSON.parse(global.fetch.mock.calls[0][1].body)).toEqual({
        channel_id: "channel-id",
        message: result.message,
      });
      expect(JSON.parse(global.fetch.mock.calls[1][1].body)).toEqual({
        channel_id: "channel-id",
        message: result.threadMessage,
        root_id: "root-post-id",
      });
    }));

  test("tolerates an empty Loop API response body", async () =>
    withTempDir(async (tempDir) => {
      fs.writeFileSync(
        path.join(tempDir, "e2e_report_replicated.json"),
        JSON.stringify({
          cluster: "replicated",
          storageType: "replicated",
          reportKind: "tests",
          branch: "main",
          workflowRunUrl: "https://example.invalid/replicated",
          startedAt: "2026-04-15T09:30:44",
          metrics: {
            passed: 11,
            skipped: 0,
            failed: 0,
            errors: 0,
            total: 11,
            successRate: 100,
          },
          failedTests: [],
        })
      );

      process.env.REPORTS_DIR = tempDir;
      process.env.STORAGE_TYPES = '["replicated"]';
      process.env.LOOP_API_BASE_URL = "https://loop.example.invalid";
      process.env.LOOP_CHANNEL_ID = "channel-id";
      process.env.LOOP_TOKEN = "loop-token";

      const core = createCore();
      global.fetch = jest.fn().mockResolvedValue({
        ok: true,
        status: 201,
        text: async () => "",
      });

      await renderMessengerReport({ core });

      expect(global.fetch).toHaveBeenCalledTimes(1);
      expect(core.warning).not.toHaveBeenCalledWith(
        expect.stringContaining("Unable to deliver report to Loop API")
      );
      expect(core.setOutput).toHaveBeenCalledWith("root_post_id", "");
      expect(core.setOutput).toHaveBeenCalledWith("thread_post_id", "");
    }));

  test("tolerates an invalid JSON Loop API response body", async () =>
    withTempDir(async (tempDir) => {
      fs.writeFileSync(
        path.join(tempDir, "e2e_report_replicated.json"),
        JSON.stringify({
          cluster: "replicated",
          storageType: "replicated",
          reportKind: "tests",
          branch: "main",
          workflowRunUrl: "https://example.invalid/replicated",
          startedAt: "2026-04-15T09:30:44",
          metrics: {
            passed: 11,
            skipped: 0,
            failed: 0,
            errors: 0,
            total: 11,
            successRate: 100,
          },
          failedTests: [],
        })
      );

      process.env.REPORTS_DIR = tempDir;
      process.env.STORAGE_TYPES = '["replicated"]';
      process.env.LOOP_API_BASE_URL = "https://loop.example.invalid";
      process.env.LOOP_CHANNEL_ID = "channel-id";
      process.env.LOOP_TOKEN = "loop-token";

      const core = createCore();
      global.fetch = jest.fn().mockResolvedValue({
        ok: true,
        status: 201,
        text: async () => "not-json",
      });

      await renderMessengerReport({ core });

      expect(global.fetch).toHaveBeenCalledTimes(1);
      expect(core.warning).toHaveBeenCalledWith(
        expect.stringContaining("Loop API returned a non-JSON response body")
      );
      expect(core.setOutput).toHaveBeenCalledWith("root_post_id", "");
      expect(core.setOutput).toHaveBeenCalledWith("thread_post_id", "");
    }));

  test("logs readable Loop API errors for failed responses", async () =>
    withTempDir(async (tempDir) => {
      fs.writeFileSync(
        path.join(tempDir, "e2e_report_replicated.json"),
        JSON.stringify({
          cluster: "replicated",
          storageType: "replicated",
          reportKind: "tests",
          branch: "main",
          workflowRunUrl: "https://example.invalid/replicated",
          startedAt: "2026-04-15T09:30:44",
          metrics: {
            passed: 11,
            skipped: 0,
            failed: 0,
            errors: 0,
            total: 11,
            successRate: 100,
          },
          failedTests: [],
        })
      );

      process.env.REPORTS_DIR = tempDir;
      process.env.STORAGE_TYPES = '["replicated"]';
      process.env.LOOP_API_BASE_URL = "https://loop.example.invalid";
      process.env.LOOP_CHANNEL_ID = "channel-id";
      process.env.LOOP_TOKEN = "loop-token";

      const core = createCore();
      global.fetch = jest.fn().mockResolvedValue({
        ok: false,
        status: 500,
        text: async () => "server exploded",
      });

      await renderMessengerReport({ core });

      expect(global.fetch).toHaveBeenCalledTimes(1);
      expect(core.warning).toHaveBeenCalledWith(
        "Unable to deliver report to Loop API: Loop API request failed with status 500: server exploded"
      );
    }));
});
