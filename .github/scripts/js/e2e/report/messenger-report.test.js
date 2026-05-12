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

const fs = require("fs");
const os = require("os");
const path = require("path");

const renderMessengerReport = require("./messenger-report");
const { readMessengerConfigFromEnv } = require("./messenger/config");

/**
 * Creates a mocked GitHub Actions core object for unit tests.
 *
 * @returns {{
 *   info: jest.Mock,
 *   warning: jest.Mock,
 *   setOutput: jest.Mock
 * }} Mocked core object.
 */
function createCore() {
  return {
    info: jest.fn(),
    warning: jest.fn(),
    setOutput: jest.fn(),
  };
}

/**
 * Runs a test body inside a temporary directory and removes it afterwards.
 *
 * @template T
 * @param {function(string): (Promise<T>|T)} testFn Test body.
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
    delete process.env.EXPECTED_STORAGE_TYPES;
    delete process.env.LOOP_API_BASE_URL;
    delete process.env.LOOP_CHANNEL_ID;
    delete process.env.LOOP_TOKEN;
    delete global.fetch;
  });

  test("reads normalized messenger config from env", () => {
    const config = readMessengerConfigFromEnv({
      REPORTS_DIR: "custom-reports",
      LOOP_API_BASE_URL: "https://loop.example.invalid/api/v4/",
      LOOP_CHANNEL_ID: " channel-id ",
      LOOP_TOKEN: " token ",
    });

    expect(config).toEqual({
      reportsDir: "custom-reports",
      configuredClusters: ["replicated", "nfs"],
      loop: {
        apiUrl: "https://loop.example.invalid/api/v4/posts",
        channelId: "channel-id",
        token: "token",
      },
    });
  });

  test("returns null loop config when no Loop credentials are set", () => {
    const config = readMessengerConfigFromEnv({});

    expect(config.loop).toBeNull();
  });

  test("throws when Loop credentials are only partially configured", () => {
    expect(() =>
      readMessengerConfigFromEnv({
        LOOP_API_BASE_URL: "https://loop.example.invalid",
        // LOOP_CHANNEL_ID and LOOP_TOKEN intentionally absent
      })
    ).toThrow("LOOP_CHANNEL_ID, LOOP_TOKEN, and LOOP_API_BASE_URL are required");
  });

  test("uses default configured clusters when env override is absent", () => {
    const config = readMessengerConfigFromEnv({});

    expect(config.configuredClusters).toEqual(["replicated", "nfs"]);
    expect(config.reportsDir).toBe("downloaded-artifacts");
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
      process.env.EXPECTED_STORAGE_TYPES = '["replicated","nfs"]';

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
        "### Failed tests\n\n**replicated**\n\n| Test group |\n|---|\n| fails |",
      ]);
    }));

  test("creates artifact-missing entry for absent cluster report", async () =>
    withTempDir(async (tempDir) => {
      process.env.REPORTS_DIR = tempDir;
      process.env.EXPECTED_STORAGE_TYPES = '["replicated"]';

      const result = await renderMessengerReport({ core: createCore() });

      expect(result.message).toContain("### Missing reports");
      expect(result.message).toContain(
        "- replicated: ⚠️ E2E REPORT ARTIFACT NOT FOUND"
      );
      expect(result.threadMessages).toEqual([]);
    }));

  test("warns and skips report files that are missing storageType/cluster fields", async () =>
    withTempDir(async (tempDir) => {
      fs.writeFileSync(
        path.join(tempDir, "e2e_report_corrupt.json"),
        JSON.stringify({
          reportKind: "stage-failure",
          failedStage: "configure-sdn",
          failedStageLabel: "CONFIGURE SDN",
          status: "failure",
          // no storageType / cluster fields
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
      process.env.EXPECTED_STORAGE_TYPES = '["nfs"]';

      const core = createCore();
      const result = await renderMessengerReport({ core });

      // The valid "nfs" report is still rendered normally.
      expect(result.message).toContain("### Test results");
      // The corrupt file is dropped; no phantom entry appears in the output.
      expect(result.message).not.toContain("corrupt");
      // A warning is emitted so the problem is visible in CI logs.
      expect(core.warning).toHaveBeenCalledWith(
        expect.stringContaining("report is missing storageType/cluster fields")
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
      process.env.EXPECTED_STORAGE_TYPES = '["replicated","nfs"]';

      const result = await renderMessengerReport({ core: createCore() });

      expect(result.threadMessages).toEqual([
        "### Failed tests\n\n**replicated**\n\n| Test group |\n|---|\n| replicated |",
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
      process.env.EXPECTED_STORAGE_TYPES = '["nfs"]';

      const result = await renderMessengerReport({ core: createCore() });

      expect(result.threadMessages).toEqual([
        [
          "### Failed tests",
          "",
          "**nfs**",
          "",
          "| Test group |",
          "|---|",
          "| VirtualMachineOperationRestore |",
          "| VirtualMachineAdditionalNetworkInterfaces |",
        ].join("\n"),
      ]);
    }));

  test("renders cluster status from downloaded report artifact", async () =>
    withTempDir(async (tempDir) => {
      fs.writeFileSync(
        path.join(tempDir, "e2e_report_replicated.json"),
        JSON.stringify({
          cluster: "replicated",
          storageType: "replicated",
          branch: "main",
          workflowRunUrl: "https://example.invalid/replicated",
          clusterStatus: {
            status: "failure",
            stage: "configure-sdn",
            stageLabel: "CONFIGURE SDN",
            message: "❌ CONFIGURE SDN FAILED",
            reason: "cluster-stage-failure",
          },
          testStatus: {
            status: "not-run",
            reason: "cluster-stage-failure",
            message:
              "E2E tests were not run because cluster setup did not finish",
          },
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

      const result = await renderMessengerReport({ core: createCore() });

      expect(result.message).not.toContain("Branch: `main`");
      expect(result.message).toContain("### Cluster failures");
      expect(result.message).toContain(
        "- [replicated](https://example.invalid/replicated): ❌ CONFIGURE SDN FAILED"
      );
      expect(result.threadMessages).toEqual([]);
    }));

  test("shows branch line for non-main branches", async () =>
    withTempDir(async (tempDir) => {
      fs.writeFileSync(
        path.join(tempDir, "e2e_report_replicated.json"),
        JSON.stringify({
          cluster: "replicated",
          storageType: "replicated",
          branch: "release-1.2",
          clusterStatus: {
            status: "failure",
            stage: "configure-sdn",
            stageLabel: "CONFIGURE SDN",
            message: "❌ CONFIGURE SDN FAILED",
            reason: "cluster-stage-failure",
          },
          testStatus: {
            status: "not-run",
            reason: "cluster-stage-failure",
            message:
              "E2E tests were not run because cluster setup did not finish",
          },
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

      const result = await renderMessengerReport({ core: createCore() });

      expect(result.message).toContain("Branch: `release-1.2`");
    }));

  test("renders missing test report status from downloaded report artifact", async () =>
    withTempDir(async (tempDir) => {
      fs.writeFileSync(
        path.join(tempDir, "e2e_report_replicated.json"),
        JSON.stringify({
          cluster: "replicated",
          storageType: "replicated",
          branch: "main",
          workflowRunUrl: "https://example.invalid/replicated",
          clusterStatus: {
            status: "success",
            stage: "ready",
            stageLabel: "CLUSTER READY",
            message: "✅ CLUSTER READY",
            reason: "",
          },
          testStatus: {
            status: "missing",
            reason: "ginkgo-report-missing",
            message: "⚠️ E2E TEST REPORT NOT FOUND",
          },
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

      const result = await renderMessengerReport({ core: createCore() });

      expect(result.message).toContain("### Missing reports");
      expect(result.message).toContain(
        "- [replicated](https://example.invalid/replicated): ⚠️ E2E TEST REPORT NOT FOUND"
      );
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
      process.env.EXPECTED_STORAGE_TYPES = '["replicated"]';
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
        message:
          "### Failed tests\n\n**replicated**\n\n| Test group |\n|---|\n| fails |",
        root_id: "root-post-id",
      });
    }));

  test("warns when Loop API returns an empty response body (no post id)", async () =>
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
      process.env.EXPECTED_STORAGE_TYPES = '["replicated"]';
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

      // Empty body → no post id → thread replies cannot be sent → warning emitted.
      expect(global.fetch).toHaveBeenCalledTimes(1);
      expect(core.warning).toHaveBeenCalledWith(
        expect.stringContaining("Loop API did not return a post id")
      );
      // Report outputs are still set because the message was built before sending.
      expect(core.setOutput).toHaveBeenCalledWith("thread_messages", "[]");
    }));

  test("warns when Loop API returns a non-JSON response body (no post id)", async () =>
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
      process.env.EXPECTED_STORAGE_TYPES = '["replicated"]';
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

      // Non-JSON body → parse warning → no post id → delivery warning.
      expect(global.fetch).toHaveBeenCalledTimes(1);
      expect(core.warning).toHaveBeenCalledWith(
        expect.stringContaining("Loop API returned a non-JSON response body")
      );
      expect(core.warning).toHaveBeenCalledWith(
        expect.stringContaining("Loop API did not return a post id")
      );
      // Report outputs are still set because the message was built before sending.
      expect(core.setOutput).toHaveBeenCalledWith("thread_messages", "[]");
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
      process.env.EXPECTED_STORAGE_TYPES = '["replicated"]';
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
