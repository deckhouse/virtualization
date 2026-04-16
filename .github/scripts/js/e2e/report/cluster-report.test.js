const fs = require("fs");
const os = require("os");
const path = require("path");

const buildClusterReport = require("./cluster-report");
const { determineStage } = require("./cluster-report");
const { parseGinkgoReport } = require("./ginkgo-report-utils");
const { readClusterConfigFromEnv } = require("./cluster-report");

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
 * @param {(tempDir: string) => Promise<T>|T} testFn Test body.
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
 * Seeds environment variables representing workflow stage results.
 *
 * @param {Record<string, string>} [overrides={}] Environment overrides.
 */
function setStageEnv(overrides = {}) {
  process.env.STORAGE_TYPE = "replicated";
  process.env.BOOTSTRAP_RESULT = "success";
  process.env.CONFIGURE_SDN_RESULT = "success";
  process.env.CONFIGURE_STORAGE_RESULT = "success";
  process.env.CONFIGURE_VIRTUALIZATION_RESULT = "success";
  process.env.E2E_TEST_RESULT = "success";
  Object.assign(process.env, overrides);
}

/**
 * Creates a synthetic Ginkgo spec report for parser tests.
 *
 * @param {{
 *   containerHierarchyTexts?: string[],
 *   containerHierarchyLabels?: Array<string[]>,
 *   leafNodeText?: string,
 *   leafNodeType?: string,
 *   leafNodeLabels?: string[],
 *   state?: string,
 *   startTime?: string,
 *   endTime?: string,
 *   failure?: Record<string, any>|undefined
 * }} [options={}] Spec overrides.
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
 * @param {{ startedAt: string, specs: Record<string, any>[] }} params Report contents.
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
  afterEach(() => {
    delete process.env.STORAGE_TYPE;
    delete process.env.E2E_REPORT_DIR;
    delete process.env.REPORT_FILE;
    delete process.env.BRANCH_NAME;
    delete process.env.WORKFLOW_RUN_URL;
    delete process.env.BOOTSTRAP_RESULT;
    delete process.env.CONFIGURE_SDN_RESULT;
    delete process.env.CONFIGURE_STORAGE_RESULT;
    delete process.env.CONFIGURE_VIRTUALIZATION_RESULT;
    delete process.env.E2E_TEST_RESULT;
  });

  test("reads cluster config from env", () => {
    const config = readClusterConfigFromEnv({
      STORAGE_TYPE: "replicated",
      E2E_REPORT_DIR: "custom-reports",
      REPORT_FILE: "custom.json",
      WORKFLOW_RUN_URL: "https://example.invalid/run/1",
      BRANCH_NAME: "release",
      BOOTSTRAP_RESULT: "success",
      CONFIGURE_SDN_RESULT: "failure",
      CONFIGURE_STORAGE_RESULT: "skipped",
      CONFIGURE_VIRTUALIZATION_RESULT: "skipped",
      E2E_TEST_RESULT: "skipped",
    });

    expect(config).toEqual({
      storageType: "replicated",
      reportsDir: "custom-reports",
      reportFile: "custom.json",
      workflowRunUrlOverride: "https://example.invalid/run/1",
      branchNameOverride: "release",
      stageResults: {
        bootstrap: "success",
        "configure-sdn": "failure",
        "storage-setup": "skipped",
        "virtualization-setup": "skipped",
        "e2e-test": "skipped",
      },
    });
  });

  test("determines stage from explicit stage results", () => {
    expect(
      determineStage("replicated", {
        bootstrap: "success",
        "configure-sdn": "failure",
        "storage-setup": "skipped",
        "virtualization-setup": "skipped",
        "e2e-test": "skipped",
      })
    ).toMatchObject({
      failedStage: "configure-sdn",
      failedStageLabel: "CONFIGURE SDN",
      reportKind: "stage-failure",
      status: "failure",
    });
  });

  test("renders test report from Ginkgo JSON when E2E succeeded", async () =>
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
      setStageEnv({
        E2E_REPORT_DIR: tempDir,
        REPORT_FILE: reportFile,
      });

      const core = createCore();
      const report = await buildClusterReport({
        core,
        context: createContext(),
      });

      expect(report.reportKind).toBe("tests");
      expect(report.failedStage).toBe("success");
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
      expect(core.setOutput).toHaveBeenCalledWith("status", "success");
      expect(core.setOutput).toHaveBeenCalledWith("failed_stage", "success");
      expect(core.setOutput).toHaveBeenCalledWith(
        "failed_stage_label",
        "SUCCESS"
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
      setStageEnv({
        E2E_REPORT_DIR: tempDir,
        REPORT_FILE: reportFile,
      });

      await expect(
        buildClusterReport({
          core: createCore(),
          context: createContext(),
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
      setStageEnv({
        E2E_REPORT_DIR: tempDir,
        REPORT_FILE: reportFile,
      });

      const core = createCore();
      const report = await buildClusterReport({
        core,
        context: createContext(),
      });

      expect(report.reportKind).toBe("artifact-missing");
      expect(report.failedStage).toBe("artifact-missing");
      expect(report.status).toBe("missing");
      expect(report.reportSource).toBe("empty");
      expect(core.warning).toHaveBeenCalledWith(
        expect.stringContaining("Unable to parse Ginkgo JSON report")
      );
    }));

  test("throws a descriptive error when writing the cluster report fails", async () =>
    withTempDir(async (tempDir) => {
      const reportFile = path.join(tempDir, "report.json");
      setStageEnv({
        E2E_REPORT_DIR: tempDir,
        REPORT_FILE: reportFile,
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
      setStageEnv({
        E2E_REPORT_DIR: tempDir,
        REPORT_FILE: reportFile,
        CONFIGURE_SDN_RESULT: "failure",
        CONFIGURE_STORAGE_RESULT: "skipped",
        CONFIGURE_VIRTUALIZATION_RESULT: "skipped",
        E2E_TEST_RESULT: "skipped",
      });

      const report = await buildClusterReport({
        core: createCore(),
        context: createContext(),
      });

      expect(report.reportKind).toBe("stage-failure");
      expect(report.failedStage).toBe("configure-sdn");
      expect(report.failedStageLabel).toBe("CONFIGURE SDN");
      expect(report.status).toBe("failure");
    }));

  test("marks missing artifacts when test stage is successful but no reports were found", async () =>
    withTempDir(async (tempDir) => {
      const reportFile = path.join(tempDir, "report.json");
      setStageEnv({
        E2E_REPORT_DIR: tempDir,
        REPORT_FILE: reportFile,
      });

      const report = await buildClusterReport({
        core: createCore(),
        context: createContext(),
      });

      expect(report.reportKind).toBe("artifact-missing");
      expect(report.failedStage).toBe("artifact-missing");
      expect(report.failedStageLabel).toBe("TEST REPORTS NOT FOUND");
      expect(report.status).toBe("missing");
    }));

  test("keeps cancelled test stage when no reports were found", async () =>
    withTempDir(async (tempDir) => {
      const reportFile = path.join(tempDir, "report.json");
      setStageEnv({
        E2E_REPORT_DIR: tempDir,
        REPORT_FILE: reportFile,
        E2E_TEST_RESULT: "cancelled",
      });

      const report = await buildClusterReport({
        core: createCore(),
        context: createContext(),
      });

      expect(report.reportKind).toBe("tests");
      expect(report.failedStage).toBe("e2e-test");
      expect(report.failedStageLabel).toBe("E2E TEST");
      expect(report.status).toBe("cancelled");
    }));

  test("keeps failed test stage when no reports were found", async () =>
    withTempDir(async (tempDir) => {
      const reportFile = path.join(tempDir, "report.json");
      setStageEnv({
        E2E_REPORT_DIR: tempDir,
        REPORT_FILE: reportFile,
        E2E_TEST_RESULT: "failure",
      });

      const report = await buildClusterReport({
        core: createCore(),
        context: createContext(),
      });

      expect(report.reportKind).toBe("tests");
      expect(report.failedStage).toBe("e2e-test");
      expect(report.failedStageLabel).toBe("E2E TEST");
      expect(report.status).toBe("failure");
    }));
});
