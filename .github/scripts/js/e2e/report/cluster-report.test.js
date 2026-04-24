const fs = require("fs");
const os = require("os");
const path = require("path");

const buildClusterReport = require("./cluster-report");
const { parseJUnitReport } = require("./cluster-report");
const { determineStage } = require("./cluster-report");
const { readClusterConfigFromEnv } = require("./cluster-report");

function createCore() {
  return {
    info: jest.fn(),
    warning: jest.fn(),
    debug: jest.fn(),
    setOutput: jest.fn(),
  };
}

function createContext() {
  return {
    serverUrl: "https://github.com",
    repo: { owner: "test", repo: "repo" },
    runId: "12345",
    ref: "refs/heads/main",
  };
}

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

function setStageEnv(overrides = {}) {
  process.env.STORAGE_TYPE = "replicated";
  process.env.BOOTSTRAP_RESULT = "success";
  process.env.CONFIGURE_SDN_RESULT = "success";
  process.env.CONFIGURE_STORAGE_RESULT = "success";
  process.env.CONFIGURE_VIRTUALIZATION_RESULT = "success";
  process.env.E2E_TEST_RESULT = "success";
  Object.assign(process.env, overrides);
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

  test("renders test report from JUnit when E2E succeeded", async () =>
    withTempDir(async (tempDir) => {
      const xmlPath = path.join(
        tempDir,
        "e2e_summary_replicated_2026-04-15.xml"
      );
      fs.writeFileSync(
        xmlPath,
        `<?xml version="1.0" encoding="UTF-8"?>
<testsuites tests="4" failures="1" errors="1" disabled="1">
  <testsuite name="Tests" tests="4" failures="1" errors="1" skipped="1" timestamp="2026-04-15T09:30:44">
    <testcase name="[It] passes" status="passed"></testcase>
    <testcase name="[It] fails &amp; burns" status="failed"><failure message="boom">boom</failure></testcase>
    <testcase name="[It] errors &lt;loudly&gt;" status="error"><error message="broken">broken</error></testcase>
    <testcase name="[It] skipped" status="skipped"></testcase>
  </testsuite>
</testsuites>
`
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
        "[It] fails & burns",
        "[It] errors <loudly>",
      ]);
      expect(report.reportSource).toBe("junit");
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

  test("fails when multiple matching JUnit reports exist", async () =>
    withTempDir(async (tempDir) => {
      const firstXmlPath = path.join(
        tempDir,
        "nested",
        "e2e_summary_replicated_2026-04-15.xml"
      );
      const secondXmlPath = path.join(
        tempDir,
        "e2e_summary_replicated_2026-04-16.xml"
      );
      fs.mkdirSync(path.dirname(firstXmlPath), { recursive: true });

      fs.writeFileSync(
        firstXmlPath,
        `<?xml version="1.0" encoding="UTF-8"?>
<testsuites tests="2" failures="1" errors="0" skipped="0">
  <testsuite name="Tests" tests="2" failures="1" errors="0" skipped="0" timestamp="2026-04-15T09:30:44">
    <testcase name="[It] old pass" status="passed"></testcase>
    <testcase name="[It] old fail" status="failed"><failure message="boom">boom</failure></testcase>
  </testsuite>
</testsuites>
`
      );
      fs.writeFileSync(
        secondXmlPath,
        `<?xml version="1.0" encoding="UTF-8"?>
<testsuite name="Tests" tests="1" failures="0" errors="0" skipped="0" timestamp="2026-04-16T09:30:44">
  <testcase name="[It] latest pass" status="passed"></testcase>
</testsuite>
`
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
        "Expected a single JUnit report, but found 2"
      );
      expect(fs.existsSync(reportFile)).toBe(false);
    }));

  test("parses current replicated fixture report", () => {
    const fixturePath = path.resolve(
      __dirname,
      "__fixtures__/e2e_summary_replicated_2026-04-20.xml"
    );
    const parsed = parseJUnitReport(fs.readFileSync(fixturePath, "utf8"));

    expect(parsed.metrics).toEqual({
      passed: 117,
      failed: 11,
      errors: 0,
      skipped: 4,
      total: 132,
      successRate: 88.64,
    });
    expect(parsed.startedAt).toBe("2026-04-20T12:48:10");
    expect(parsed.failedTests).toHaveLength(11);
    expect(parsed.failedTests).toContain(
      "[It] VirtualMachineAdditionalNetworkInterfaces verifies additional network interfaces and connectivity before and after migration Main + additional network"
    );
    expect(parsed.failedTests).toContain(
      "[It] VirtualMachineOperationRestore restores a virtual machine from a snapshot BestEffort restore mode; automatic restart approval mode; always on unless stopped manually run policy [Slow]"
    );
  });

  test("parses current nfs fixture report", () => {
    const fixturePath = path.resolve(
      __dirname,
      "__fixtures__/e2e_summary_nfs_2026-04-20.xml"
    );
    const parsed = parseJUnitReport(fs.readFileSync(fixturePath, "utf8"));

    expect(parsed.metrics).toEqual({
      passed: 93,
      failed: 8,
      errors: 0,
      skipped: 31,
      total: 132,
      successRate: 70.45,
    });
    expect(parsed.startedAt).toBe("2026-04-20T12:38:34");
    expect(parsed.failedTests).toHaveLength(8);
    expect(parsed.failedTests).toContain(
      "[It] RWOVirtualDiskMigration should be successful two migrations in a row"
    );
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
