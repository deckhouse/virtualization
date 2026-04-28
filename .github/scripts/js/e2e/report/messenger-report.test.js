const fs = require("fs");
const os = require("os");
const path = require("path");

const renderMessengerReport = require("./messenger-report");
const { readMessengerConfigFromEnv } = require("./messenger-report");

/**
 * Creates a mocked GitHub Actions core object for unit tests.
 *
 * @returns {{
 *   info: jest.Mock,
 *   warning: jest.Mock,
 *   debug: jest.Mock,
 *   setOutput: jest.Mock
 * }} Mocked core object.
 */
function createCore() {
  return {
    info: jest.fn(),
    warning: jest.fn(),
    debug: jest.fn(),
    setOutput: jest.fn(),
  };
}

/**
 * Runs a test body inside a temporary directory and removes it afterwards.
 *
 * @template T
 * @param {(tempDir: string) => Promise<T>|T} testFn Test body.
 * @returns {Promise<T>} Test result.
 */
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

  test("renders test results, stage failures, and per-cluster thread replies", async () =>
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
      expect(result.threadMessages).toEqual([
        "### Failed tests",
        "**replicated**\n\n| Test group |\n|---|\n| fails |",
      ]);
      expect(result.threadMessage).toContain("### Failed tests");
      expect(result.threadMessage).toContain("**replicated**");
      expect(result.threadMessage).toContain("| Test group |");
      expect(result.threadMessage).toContain("| fails |");
      expect(result.threadMessage).not.toContain("**nfs**\n|");
    }));

  test("creates artifact-missing entry for absent cluster report", async () =>
    withTempDir(async (tempDir) => {
      process.env.REPORTS_DIR = tempDir;
      process.env.STORAGE_TYPES = '["replicated"]';

      const result = await renderMessengerReport({ core: createCore() });

      expect(result.message).toContain("### Missing reports");
      expect(result.message).toContain(
        "- replicated: E2E REPORT ARTIFACT NOT FOUND"
      );
      expect(result.threadMessage).toBe("");
      expect(result.threadMessages).toEqual([]);
    }));

  test("skips invalid reports without cluster identity", async () =>
    withTempDir(async (tempDir) => {
      fs.writeFileSync(
        path.join(tempDir, "e2e_report_invalid.json"),
        JSON.stringify({
          reportKind: "stage-failure",
          failedStage: "configure-sdn",
          failedStageLabel: "CONFIGURE SDN",
          status: "failure",
        })
      );

      fs.writeFileSync(
        path.join(tempDir, "e2e_report_nfs.json"),
        JSON.stringify({
          cluster: "nfs",
          storageType: "nfs",
          reportKind: "tests",
          branch: "main",
          workflowRunUrl: "https://example.invalid/nfs",
          startedAt: "2026-04-15T09:30:44",
          metrics: {
            passed: 8,
            skipped: 1,
            failed: 1,
            errors: 0,
            total: 10,
            successRate: 80,
          },
          failedTests: ["[It] nfs fails"],
        })
      );

      process.env.REPORTS_DIR = tempDir;
      process.env.STORAGE_TYPES = '["nfs"]';

      const core = createCore();
      const result = await renderMessengerReport({ core });

      expect(result.message).toContain("### Test results");
      expect(result.message).not.toContain("### Cluster failures");
      expect(result.message).not.toContain("- —:");
      expect(core.warning).toHaveBeenCalledWith(
        "Skipping report without cluster name from parsed JSON payload"
      );
    }));

  test("splits failed tests into separate thread messages per cluster", async () =>
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
            skipped: 0,
            failed: 1,
            errors: 0,
            total: 13,
            successRate: 92.31,
          },
          failedTests: ["[It] replicated fails"],
        })
      );

      fs.writeFileSync(
        path.join(tempDir, "e2e_report_nfs.json"),
        JSON.stringify({
          cluster: "nfs",
          storageType: "nfs",
          reportKind: "tests",
          branch: "main",
          workflowRunUrl: "https://example.invalid/nfs",
          startedAt: "2026-04-15T09:30:44",
          metrics: {
            passed: 8,
            skipped: 1,
            failed: 1,
            errors: 0,
            total: 10,
            successRate: 80,
          },
          failedTests: ["[It] nfs fails"],
        })
      );

      process.env.REPORTS_DIR = tempDir;
      process.env.STORAGE_TYPES = '["replicated","nfs"]';

      const result = await renderMessengerReport({ core: createCore() });

      expect(result.threadMessages).toEqual([
        "### Failed tests",
        "**replicated**\n\n| Test group |\n|---|\n| replicated |",
        "**nfs**\n\n| Test group |\n|---|\n| nfs |",
      ]);
    }));

  test("groups failed tests by top-level describe name", async () =>
    withTempDir(async (tempDir) => {
      fs.writeFileSync(
        path.join(tempDir, "e2e_report_nfs.json"),
        JSON.stringify({
          cluster: "nfs",
          storageType: "nfs",
          reportKind: "tests",
          branch: "main",
          workflowRunUrl: "https://example.invalid/nfs",
          startedAt: "2026-04-15T09:30:44",
          metrics: {
            passed: 90,
            skipped: 34,
            failed: 7,
            errors: 0,
            total: 131,
            successRate: 68.7,
          },
          failedTests: [
            "[It] VirtualMachineOperationRestore restores a virtual machine from a snapshot BestEffort restore mode; manual restart approval mode; always on unless stopped manually run policy [Slow]",
            "[It] VirtualMachineOperationRestore restores a virtual machine from a snapshot Strict restore mode; manual restart approval mode; always on unless stopped manually run policy [Slow]",
            "[It] VirtualMachineOperationRestore restores a virtual machine from a snapshot BestEffort restore mode; manual restart approval mode; always on unless stopped manually run policy; with resource deletion [Slow]",
            "[It] VirtualMachineOperationRestore restores a virtual machine from a snapshot Strict restore mode; manual restart approval mode; always on unless stopped manually run policy; with resource deletion [Slow]",
            "[It] VirtualMachineOperationRestore restores a virtual machine from a snapshot BestEffort restore mode; automatic restart approval mode; always on unless stopped manually run policy [Slow]",
            "[It] VirtualMachineOperationRestore restores a virtual machine from a snapshot BestEffort restore mode; automatic restart approval mode; manual run policy [Slow]",
            "[It] VirtualMachineAdditionalNetworkInterfaces verifies interface name persistence after removing middle ClusterNetwork should preserve interface name after removing middle ClusterNetwork and rebooting",
          ],
        })
      );

      process.env.REPORTS_DIR = tempDir;
      process.env.STORAGE_TYPES = '["nfs"]';

      const result = await renderMessengerReport({ core: createCore() });

      expect(result.threadMessages).toEqual([
        "### Failed tests",
        [
          "**nfs**",
          "",
          "| Test group |",
          "|---|",
          "| VirtualMachineOperationRestore |",
          "| VirtualMachineAdditionalNetworkInterfaces |",
        ].join("\n"),
      ]);
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

      expect(result.message).not.toContain("Branch: `main`");
      expect(result.message).toContain("### Cluster failures");
      expect(result.message).toContain(
        "- [replicated](https://example.invalid/replicated): CONFIGURE SDN"
      );
      expect(result.threadMessage).toBe("");
      expect(result.threadMessages).toEqual([]);
    }));

  test("shows branch line for non-main branches", async () =>
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
      process.env.REPORT_FALLBACK_REPLICATED_BRANCH = "release-1.2";

      const result = await renderMessengerReport({ core: createCore() });

      expect(result.message).toContain("Branch: `release-1.2`");
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

      expect(result.message).toContain("### Missing reports");
      expect(result.message).toContain(
        "- [replicated](https://example.invalid/replicated): TEST REPORTS NOT FOUND"
      );
      expect(result.threadMessage).toBe("");
      expect(result.threadMessages).toEqual([]);
    }));

  test("posts main report and per-cluster failed tests thread via Loop API", async () =>
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
          text: async () => JSON.stringify({ id: "thread-header-post-id" }),
        })
        .mockResolvedValueOnce({
          ok: true,
          status: 201,
          text: async () => JSON.stringify({ id: "thread-cluster-post-id" }),
        });

      const result = await renderMessengerReport({ core: createCore() });

      expect(global.fetch).toHaveBeenCalledTimes(3);
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
        message: "### Failed tests",
        root_id: "root-post-id",
      });
      expect(JSON.parse(global.fetch.mock.calls[2][1].body)).toEqual({
        channel_id: "channel-id",
        message: "**replicated**\n\n| Test group |\n|---|\n| fails |",
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
      expect(core.setOutput).toHaveBeenCalledWith("thread_messages", "[]");
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
      expect(core.setOutput).toHaveBeenCalledWith("thread_messages", "[]");
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
