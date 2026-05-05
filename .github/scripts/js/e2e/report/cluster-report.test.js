const fs = require("fs");
const os = require("os");
const path = require("path");

const buildClusterReport = require("./cluster-report");
const { buildClusterStatus } = require("./cluster-report");
const { parseGinkgoReport } = require("./shared/ginkgo-report-utils");

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
 * Creates a minimal GitHub Actions context object for unit tests.
 *
 * @returns {{
 *   serverUrl: string,
 *   repo: { owner: string, repo: string },
 *   runId: string,
 *   ref: string
 * }} Mocked context object.
 */
function createContext() {
  return {
    serverUrl: "https://github.com",
    repo: { owner: "test", repo: "repo" },
    runId: "12345",
    ref: "refs/heads/main",
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
    path.join(os.tmpdir(), "cluster-report-test-")
  );
  try {
    return await testFn(tempDir);
  } finally {
    fs.rmSync(tempDir, { recursive: true, force: true });
  }
}

/**
 * Creates explicit cluster report config for unit tests.
 *
 * @param {Partial<Record<string, any>>} [overrides={}] Config overrides.
 * @returns {Record<string, any>} Cluster report config.
 */
function createClusterConfig(overrides = {}) {
  return {
    storageType: "replicated",
    reportsDir: "test/e2e",
    reportFile: "e2e_report_replicated.json",
    ...overrides,
    stageResults: {
      "bootstrap": "success",
      "configure-sdn": "success",
      "storage-setup": "success",
      "virtualization-setup": "success",
      "e2e-test": "success",
      ...(overrides.stageResults || {}),
    },
  };
}

/**
 * @typedef {Object} SpecReportOptions
 * @property {string[]} [containerHierarchyTexts]
 * @property {Array<string[]>} [containerHierarchyLabels]
 * @property {string} [leafNodeText]
 * @property {string} [leafNodeType]
 * @property {string[]} [leafNodeLabels]
 * @property {string} [state]
 * @property {string} [startTime]
 * @property {string} [endTime]
 * @property {Record<string, any>|undefined} [failure]
 */

/**
 * Creates a synthetic Ginkgo spec report for parser tests.
 *
 * @param {SpecReportOptions} [options={}] Spec overrides.
 * @returns {Record<string, any>} Synthetic spec report.
 */
function createSpecReport({
  containerHierarchyTexts = [],
  containerHierarchyLabels = [],
  leafNodeText = "",
  leafNodeType = "It",
  leafNodeLabels = [],
  state = "passed",
  startTime = "2026-04-15T09:30:44Z",
  endTime = "2026-04-15T09:31:44Z",
  failure = undefined,
} = {}) {
  return {
    ContainerHierarchyTexts: containerHierarchyTexts,
    ContainerHierarchyLocations: [],
    ContainerHierarchyLabels: containerHierarchyLabels,
    LeafNodeType: leafNodeType,
    LeafNodeLocation: {},
    LeafNodeLabels: leafNodeLabels,
    LeafNodeText: leafNodeText,
    State: state,
    StartTime: startTime,
    EndTime: endTime,
    RunTime: 60000000000,
    ParallelProcess: 1,
    ...(failure ? { Failure: failure } : {}),
  };
}

/**
 * Creates a serialized single-suite Ginkgo report for unit tests.
 *
 * @param {{ startedAt: string, specs: Array<Record<string, any>> }} params Report contents.
 * @returns {string} JSON-serialized report.
 */
function createGinkgoReport({ startedAt, specs }) {
  return JSON.stringify(
    [
      {
        SuitePath: "/tmp/test/e2e",
        SuiteDescription: "Tests",
        SuiteSucceeded: false,
        StartTime: startedAt,
        EndTime: "2026-04-15T10:00:00Z",
        RunTime: 1800000000000,
        SpecReports: specs,
      },
    ],
    null,
    2
  );
}

/**
 * Creates a zero-filled metrics object for parser tests.
 *
 * @returns {{
 *   passed: number,
 *   failed: number,
 *   errors: number,
 *   skipped: number,
 *   total: number,
 *   successRate: number
 * }} Zeroed metrics payload.
 */
function createZeroMetrics() {
  return {
    passed: 0,
    failed: 0,
    errors: 0,
    skipped: 0,
    total: 0,
    successRate: 0,
  };
}

describe("cluster-report", () => {
  test("requires explicit config", async () => {
    await expect(
      buildClusterReport({
        core: createCore(),
        context: createContext(),
      })
    ).rejects.toThrow("buildClusterReport requires a config object");
  });

  test("determines cluster setup status from explicit stage results", () => {
    expect(
      buildClusterStatus({
        "bootstrap": "success",
        "configure-sdn": "failure",
        "storage-setup": "skipped",
        "virtualization-setup": "skipped",
      })
    ).toMatchObject({
      status: "failure",
      stage: "configure-sdn",
      stageLabel: "CONFIGURE SDN",
      reason: "cluster-stage-failed",
    });
  });

  test("builds report from explicit config without reading env", async () =>
    withTempDir(async (tempDir) => {
      const reportFile = path.join(tempDir, "explicit-report.json");

      const report = await buildClusterReport({
        core: createCore(),
        context: createContext(),
        config: {
          storageType: "nfs",
          reportsDir: tempDir,
          reportFile,
          workflowRunUrl: "https://example.invalid/run/explicit",
          branchName: "feature/report",
          stageResults: {
            "bootstrap": "success",
            "configure-sdn": "failure",
            "storage-setup": "skipped",
            "virtualization-setup": "skipped",
            "e2e-test": "skipped",
          },
        },
      });

      expect(report.cluster).toBe("nfs");
      expect(report.workflowRunUrl).toBe("https://example.invalid/run/explicit");
      expect(report.branch).toBe("feature/report");
      expect(report.clusterStatus).toMatchObject({
        status: "failure",
        stage: "configure-sdn",
      });
      expect(JSON.parse(fs.readFileSync(reportFile, "utf8")).cluster).toBe("nfs");
    }));

  test("marks Ginkgo JSON with failed specs as failed", async () =>
    withTempDir(async (tempDir) => {
      const rawReportPath = path.join(
        tempDir,
        "e2e_report_replicated_2026-04-15.json"
      );
      fs.writeFileSync(
        rawReportPath,
        createGinkgoReport({
          startedAt: "2026-04-15T09:30:44Z",
          specs: [
            createSpecReport({
              leafNodeType: "SynchronizedBeforeSuite",
              state: "passed",
            }),
            createSpecReport({
              containerHierarchyTexts: ["Suite"],
              leafNodeText: "passes",
              state: "passed",
            }),
            createSpecReport({
              containerHierarchyTexts: ["Suite"],
              leafNodeText: "fails & burns",
              state: "failed",
              leafNodeLabels: ["Slow"],
            }),
            createSpecReport({
              containerHierarchyTexts: ["Other"],
              leafNodeText: "errors <loudly>",
              state: "timedout",
            }),
            createSpecReport({
              leafNodeText: "skipped",
              state: "skipped",
            }),
          ],
        })
      );

      const reportFile = path.join(tempDir, "report.json");
      const config = createClusterConfig({
        reportsDir: tempDir,
        reportFile,
      });

      const core = createCore();
      const report = await buildClusterReport({
        core,
        context: createContext(),
        config,
      });

      expect(report.reportKind).toBe("tests");
      expect(report.failedStage).toBe("e2e-test");
      expect(report.clusterStatus).toMatchObject({
        status: "success",
        stage: "ready",
        stageLabel: "CLUSTER READY",
      });
      expect(report.testStatus).toMatchObject({
        status: "failure",
        reason: "ginkgo-failed",
      });
      expect(report.metrics).toEqual({
        passed: 1,
        failed: 1,
        errors: 1,
        skipped: 1,
        total: 4,
        successRate: 25,
      });
      expect(report.failedTests).toEqual([
        "[It] Suite fails & burns [Slow]",
        "[It] Other errors <loudly>",
      ]);
      expect(report.reportSource).toBe("ginkgo-json");
      expect(report.sourceReport).toBe(rawReportPath);
      expect(JSON.parse(fs.readFileSync(reportFile, "utf8")).reportKind).toBe(
        "tests"
      );
      expect(core.setOutput).toHaveBeenCalledWith("report_file", reportFile);
      expect(core.setOutput).toHaveBeenCalledWith("report_kind", "tests");
      expect(core.setOutput).toHaveBeenCalledWith("status", "failure");
      expect(core.setOutput).toHaveBeenCalledWith("failed_stage", "e2e-test");
      expect(core.setOutput).toHaveBeenCalledWith(
        "failed_stage_label",
        "E2E TEST"
      );
      expect(core.setOutput).toHaveBeenCalledWith(
        "workflow_run_url",
        "https://github.com/test/repo/actions/runs/12345"
      );
      expect(core.setOutput).toHaveBeenCalledWith("branch", "main");
    }));

  test("fails when multiple matching Ginkgo JSON reports exist", async () =>
    withTempDir(async (tempDir) => {
      const firstReportPath = path.join(
        tempDir,
        "nested",
        "e2e_report_replicated_2026-04-15.json"
      );
      const secondReportPath = path.join(
        tempDir,
        "e2e_report_replicated_2026-04-16.json"
      );
      fs.mkdirSync(path.dirname(firstReportPath), { recursive: true });

      fs.writeFileSync(
        firstReportPath,
        createGinkgoReport({
          startedAt: "2026-04-15T09:30:44Z",
          specs: [createSpecReport({ leafNodeText: "old pass", state: "passed" })],
        })
      );
      fs.writeFileSync(
        secondReportPath,
        createGinkgoReport({
          startedAt: "2026-04-16T09:30:44Z",
          specs: [createSpecReport({ leafNodeText: "latest pass", state: "passed" })],
        })
      );

      const reportFile = path.join(tempDir, "report.json");
      const config = createClusterConfig({
        reportsDir: tempDir,
        reportFile,
      });

      await expect(
        buildClusterReport({
          core: createCore(),
          context: createContext(),
          config,
        })
      ).rejects.toThrow(
        "Expected a single Ginkgo JSON report, but found 2"
      );
      expect(fs.existsSync(reportFile)).toBe(false);
    }));

  test("falls back to missing-report status when raw Ginkgo JSON is invalid", async () =>
    withTempDir(async (tempDir) => {
      const rawReportPath = path.join(
        tempDir,
        "e2e_report_replicated_2026-04-15.json"
      );
      fs.writeFileSync(rawReportPath, "{not-valid-json");

      const reportFile = path.join(tempDir, "report.json");
      const config = createClusterConfig({
        reportsDir: tempDir,
        reportFile,
      });

      const core = createCore();
      const report = await buildClusterReport({
        core,
        context: createContext(),
        config,
      });

      expect(report.reportKind).toBe("artifact-missing");
      expect(report.failedStage).toBe("artifact-missing");
      expect(report.status).toBe("missing");
      expect(report.reportSource).toBe("ginkgo-json-invalid");
      expect(report.testStatus).toMatchObject({
        status: "missing",
        reason: "ginkgo-report-invalid",
      });
      expect(core.warning).toHaveBeenCalledWith(
        expect.stringContaining("Unable to parse Ginkgo JSON report")
      );
    }));

  test("throws a descriptive error when writing the cluster report fails", async () =>
    withTempDir(async (tempDir) => {
      const reportFile = path.join(tempDir, "report.json");
      const config = createClusterConfig({
        reportsDir: tempDir,
        reportFile,
      });

      const writeSpy = jest
        .spyOn(fs, "writeFileSync")
        .mockImplementation(() => {
          throw new Error("disk full");
        });

      try {
        await expect(
          buildClusterReport({
            core: createCore(),
            context: createContext(),
            config,
          })
        ).rejects.toThrow(
          `Unable to write cluster report file ${reportFile}: disk full`
        );
      } finally {
        writeSpy.mockRestore();
      }
    }));

  test("parses CI-like nfs counts from Ginkgo JSON and ignores non-It specs", () => {
    const specs = [
      createSpecReport({
        leafNodeType: "SynchronizedBeforeSuite",
        state: "passed",
      }),
    ];

    for (let index = 1; index <= 90; index += 1) {
      specs.push(
        createSpecReport({
          containerHierarchyTexts: ["PassingSuite"],
          leafNodeText: `passed ${index}`,
          state: "passed",
        })
      );
    }

    specs.push(
      createSpecReport({
        containerHierarchyTexts: [
          "VirtualMachineOperationRestore",
          "restores a virtual machine from a snapshot",
        ],
        containerHierarchyLabels: [["Slow"], []],
        leafNodeText:
          "BestEffort restore mode; automatic restart approval mode; manual run policy",
        state: "failed",
      })
    );

    for (let index = 2; index <= 7; index += 1) {
      specs.push(
        createSpecReport({
          containerHierarchyTexts: ["FailingSuite"],
          leafNodeText: `failed ${index}`,
          state: "failed",
        })
      );
    }

    specs.push(
      createSpecReport({
        containerHierarchyTexts: ["SkippedSuite"],
        leafNodeText: "skipped with reason",
        state: "skipped",
        failure: {
          Message: "skip reason must not turn into a failure metric",
        },
      })
    );

    for (let index = 2; index <= 34; index += 1) {
      specs.push(
        createSpecReport({
          containerHierarchyTexts: ["SkippedSuite"],
          leafNodeText: `skipped ${index}`,
          state: "skipped",
        })
      );
    }

    const parsed = parseGinkgoReport(
      createGinkgoReport({
        startedAt: "2026-04-28T03:11:27.708387575Z",
        specs,
      }),
      createZeroMetrics
    );

    expect(parsed.metrics).toEqual({
      passed: 90,
      failed: 7,
      errors: 0,
      skipped: 34,
      total: 131,
      successRate: 68.7,
    });
    expect(parsed.startedAt).toBe("2026-04-28T03:11:27.708387575Z");
    expect(parsed.failedTests).toHaveLength(7);
    expect(parsed.failedTests).toContain(
      "[It] VirtualMachineOperationRestore restores a virtual machine from a snapshot BestEffort restore mode; automatic restart approval mode; manual run policy [Slow]"
    );
  });

  test("reports configure-sdn as the failed pre-E2E phase", async () =>
    withTempDir(async (tempDir) => {
      const reportFile = path.join(tempDir, "report.json");
      const config = createClusterConfig({
        reportsDir: tempDir,
        reportFile,
        stageResults: {
          "configure-sdn": "failure",
          "storage-setup": "skipped",
          "virtualization-setup": "skipped",
          "e2e-test": "skipped",
        },
      });

      const report = await buildClusterReport({
        core: createCore(),
        context: createContext(),
        config,
      });

      expect(report.reportKind).toBe("stage-failure");
      expect(report.failedStage).toBe("configure-sdn");
      expect(report.failedStageLabel).toBe("CONFIGURE SDN");
      expect(report.status).toBe("failure");
      expect(report.clusterStatus).toMatchObject({
        status: "failure",
        stage: "configure-sdn",
        reason: "cluster-stage-failed",
      });
      expect(report.testStatus).toMatchObject({
        status: "not-run",
        reason: "cluster-stage-failed",
      });
    }));

  test("marks missing artifacts when test stage is successful but no reports were found", async () =>
    withTempDir(async (tempDir) => {
      const reportFile = path.join(tempDir, "report.json");
      const config = createClusterConfig({
        reportsDir: tempDir,
        reportFile,
      });

      const report = await buildClusterReport({
        core: createCore(),
        context: createContext(),
        config,
      });

      expect(report.reportKind).toBe("artifact-missing");
      expect(report.failedStage).toBe("artifact-missing");
      expect(report.failedStageLabel).toBe("TEST REPORTS NOT FOUND");
      expect(report.status).toBe("missing");
      expect(report.clusterStatus.status).toBe("success");
      expect(report.testStatus).toMatchObject({
        status: "missing",
        reason: "ginkgo-report-missing",
      });
    }));

  test("keeps cancelled test stage when no reports were found", async () =>
    withTempDir(async (tempDir) => {
      const reportFile = path.join(tempDir, "report.json");
      const config = createClusterConfig({
        reportsDir: tempDir,
        reportFile,
        stageResults: {
          "e2e-test": "cancelled",
        },
      });

      const report = await buildClusterReport({
        core: createCore(),
        context: createContext(),
        config,
      });

      expect(report.reportKind).toBe("tests");
      expect(report.failedStage).toBe("e2e-test");
      expect(report.failedStageLabel).toBe("E2E TEST");
      expect(report.status).toBe("cancelled");
      expect(report.clusterStatus.status).toBe("success");
      expect(report.testStatus).toMatchObject({
        status: "cancelled",
        reason: "e2e-cancelled",
      });
    }));

  test("keeps failed test stage when no reports were found", async () =>
    withTempDir(async (tempDir) => {
      const reportFile = path.join(tempDir, "report.json");
      const config = createClusterConfig({
        reportsDir: tempDir,
        reportFile,
        stageResults: {
          "e2e-test": "failure",
        },
      });

      const report = await buildClusterReport({
        core: createCore(),
        context: createContext(),
        config,
      });

      expect(report.reportKind).toBe("tests");
      expect(report.failedStage).toBe("e2e-test");
      expect(report.failedStageLabel).toBe("E2E TEST");
      expect(report.status).toBe("failure");
      expect(report.clusterStatus.status).toBe("success");
      expect(report.testStatus).toMatchObject({
        status: "failure",
        reason: "ginkgo-report-missing",
      });
    }));
});
