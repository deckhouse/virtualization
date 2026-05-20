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
const path = require("path");

jest.mock("./messenger/charts/chart-renderer", () => ({
  renderClusterCharts: jest.fn().mockResolvedValue([]),
}));

const renderMessengerReport = require("./messenger-report");
const { renderClusterCharts } = require("./messenger/charts/chart-renderer");
const { readMessengerConfigFromEnv } = require("./messenger/config");
const { createCore, withTempDir } = require("./shared/test-utils");

const inTempDir = (testFn) => withTempDir("messenger-report-test", testFn);

describe("messenger-report", () => {
  afterEach(() => {
    delete process.env.REPORTS_DIR;
    delete process.env.EXPECTED_STORAGE_TYPES;
    delete process.env.LOOP_API_BASE_URL;
    delete process.env.LOOP_CHANNEL_ID;
    delete process.env.LOOP_TOKEN;
    delete process.env.LOOP_STRICT_DELIVERY;
    delete process.env.LOOP_STRICT_FILE_UPLOAD;
    delete global.fetch;
    renderClusterCharts.mockReset();
    renderClusterCharts.mockResolvedValue([]);
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
        strictDelivery: false,
        strictFileUploads: false,
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
    ).toThrow(
      "LOOP_CHANNEL_ID, LOOP_TOKEN, and LOOP_API_BASE_URL are required"
    );
  });

  test("uses default configured clusters when env override is absent", () => {
    const config = readMessengerConfigFromEnv({});

    expect(config.configuredClusters).toEqual(["replicated", "nfs"]);
    expect(config.reportsDir).toBe("downloaded-artifacts");
  });

  test("renders test results, stage failures, and per-cluster thread replies", async () =>
    inTempDir(async (tempDir) => {
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
          failedTestDetails: [
            {
              name: "[It] fails",
              reason: "Unexpected error:\ncommand timed out\noccurred",
            },
          ],
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
        "| [replicated](https://example.invalid/replicated) | 12 | 2 | 1 | 15 | 80.00% |"
      );
      expect(result.message).not.toContain("⚠️ Errors");
      expect(result.message).toContain("### Cluster failures");
      expect(result.message).toContain(
        "- [nfs](https://example.invalid/nfs): CONFIGURE SDN"
      );
      expect(result.message).not.toContain("### Top slowest tests");
      expect(result.message).not.toContain("### Failed tests");
      expect(result.threadMessages).toEqual([
        {
          message: [
            "### Failed tests",
            "",
            "**[replicated](https://example.invalid/replicated)**",
            "",
            "| Tests | Reason |",
            "|---|---|",
            "| fails | Unexpected error: command timed out occurred |",
          ].join("\n"),
          files: [],
        },
      ]);
    }));

  test("creates artifact-missing entry for absent cluster report", async () =>
    inTempDir(async (tempDir) => {
      process.env.REPORTS_DIR = tempDir;
      process.env.EXPECTED_STORAGE_TYPES = '["replicated"]';

      const result = await renderMessengerReport({ core: createCore() });

      expect(result.message).toContain("### Missing reports");
      expect(result.message).toContain(
        "- replicated: ⚠️ E2E REPORT ARTIFACT NOT FOUND"
      );
      expect(result.threadMessages).toEqual([]);
    }));

  test("attaches duration chart files to thread reply without a text caption", async () =>
    inTempDir(async (tempDir) => {
      const chartFile = {
        name: "replicated-slowest-specs.png",
        buffer: Buffer.from("png"),
        mimeType: "image/png",
      };
      renderClusterCharts.mockResolvedValue([chartFile]);
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
            passed: 3,
            skipped: 0,
            failed: 0,
            errors: 0,
            total: 3,
            successRate: 100,
          },
          failedTests: [],
          specTimings: [
            { name: "fast", group: "VM", state: "passed", runtimeMs: 1000 },
            {
              name: "slow | pipe",
              group: "Disk",
              state: "passed",
              runtimeMs: 90000,
            },
            { name: "medium", group: "VM", state: "passed", runtimeMs: 30000 },
          ],
        })
      );

      process.env.REPORTS_DIR = tempDir;
      process.env.EXPECTED_STORAGE_TYPES = '["replicated"]';

      const core = createCore();
      const result = await renderMessengerReport({ core });

      expect(result.message).not.toContain("### Top slowest tests");
      expect(result.threadMessages).toEqual([
        {
          message: "**[replicated](https://example.invalid/replicated)**",
          files: [chartFile],
        },
      ]);
      expect(result.threadMessages[0].message).not.toContain(
        "### Test durations"
      );
      expect(result.threadMessages[0].message).not.toContain(
        "Attached charts:"
      );
      expect(core.setOutput).toHaveBeenCalledWith(
        "thread_messages",
        JSON.stringify([result.threadMessages[0].message])
      );
    }));

  test("warns and surfaces a placeholder when chart rendering fails", async () =>
    inTempDir(async (tempDir) => {
      renderClusterCharts.mockRejectedValue(new Error("canvas unavailable"));
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
            passed: 1,
            skipped: 0,
            failed: 0,
            errors: 0,
            total: 1,
            successRate: 100,
          },
          failedTests: [],
          specTimings: [
            { name: "slow", group: "VM", state: "passed", runtimeMs: 90000 },
          ],
        })
      );

      process.env.REPORTS_DIR = tempDir;
      process.env.EXPECTED_STORAGE_TYPES = '["replicated"]';

      const core = createCore();
      const result = await renderMessengerReport({ core });

      expect(core.warning).toHaveBeenCalledWith(
        expect.stringContaining(
          "Unable to render duration charts for cluster replicated"
        )
      );
      expect(result.threadMessages).toEqual([
        {
          message: expect.stringContaining("Charts unavailable."),
          files: [],
        },
      ]);
    }));

  test("warns and skips report files that are missing storageType/cluster fields", async () =>
    inTempDir(async (tempDir) => {
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
    inTempDir(async (tempDir) => {
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
        {
          message:
            "### Failed tests\n\n**[replicated](https://example.invalid/replicated)**\n\n| Tests | Reason |\n|---|---|\n| replicated | — |",
          files: [],
        },
        {
          message:
            "**[nfs](https://example.invalid/nfs)**\n\n| Tests | Reason |\n|---|---|\n| nfs | — |",
          files: [],
        },
      ]);
    }));

  test("groups failed tests by top-level describe name", async () =>
    inTempDir(async (tempDir) => {
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
        {
          message: [
            "### Failed tests",
            "",
            "**[nfs](https://example.invalid/nfs)**",
            "",
            "| Tests | Reason |",
            "|---|---|",
            "| VirtualMachineOperationRestore | — |",
            "| VirtualMachineAdditionalNetworkInterfaces | — |",
          ].join("\n"),
          files: [],
        },
      ]);
    }));

  test("renders cluster status from downloaded report artifact", async () =>
    inTempDir(async (tempDir) => {
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
    inTempDir(async (tempDir) => {
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
    inTempDir(async (tempDir) => {
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
    inTempDir(async (tempDir) => {
      const chartFile = {
        name: "replicated-slowest-specs.png",
        buffer: Buffer.from("png"),
        mimeType: "image/png",
      };
      renderClusterCharts.mockResolvedValue([chartFile]);
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
          specTimings: [
            { name: "slow", group: "VM", state: "failed", runtimeMs: 90000 },
          ],
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
          text: async () => JSON.stringify({ file_infos: [{ id: "file-id" }] }),
        })
        .mockResolvedValueOnce({
          ok: true,
          status: 201,
          text: async () => JSON.stringify({ id: "thread-post-id" }),
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
      expect(global.fetch).toHaveBeenNthCalledWith(
        2,
        "https://loop.example.invalid/api/v4/files",
        expect.objectContaining({
          method: "POST",
          headers: {
            Authorization: "Bearer loop-token",
          },
        })
      );
      expect(JSON.parse(global.fetch.mock.calls[2][1].body)).toEqual({
        channel_id: "channel-id",
        message: [
          "### Failed tests",
          "",
          "**[replicated](https://example.invalid/replicated)**",
          "",
          "| Tests | Reason |",
          "|---|---|",
          "| fails | — |",
        ].join("\n"),
        root_id: "root-post-id",
        file_ids: ["file-id"],
      });
    }));

  test("warns when Loop API returns an empty response body (no post id)", async () =>
    inTempDir(async (tempDir) => {
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
    inTempDir(async (tempDir) => {
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
    inTempDir(async (tempDir) => {
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

  test("fails local delivery when strict Loop delivery mode is enabled", async () =>
    inTempDir(async (tempDir) => {
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
      process.env.LOOP_STRICT_DELIVERY = "1";

      global.fetch = jest.fn().mockResolvedValue({
        ok: false,
        status: 500,
        text: async () => "server exploded",
      });

      await expect(
        renderMessengerReport({ core: createCore() })
      ).rejects.toThrow(
        "Loop API request failed with status 500: server exploded"
      );
    }));
});
