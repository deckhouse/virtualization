const fs = require('fs');
const os = require('os');
const path = require('path');

const buildClusterReport = require('./cluster-report');

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
    serverUrl: 'https://github.com',
    repo: {owner: 'test', repo: 'repo'},
    runId: '12345',
    ref: 'refs/heads/main',
  };
}

async function withTempDir(testFn) {
  const tempDir = fs.mkdtempSync(path.join(os.tmpdir(), 'cluster-report-test-'));
  try {
    return await testFn(tempDir);
  } finally {
    fs.rmSync(tempDir, {recursive: true, force: true});
  }
}

function setStageEnv(overrides = {}) {
  process.env.STORAGE_TYPE = 'replicated';
  process.env.BOOTSTRAP_RESULT = 'success';
  process.env.CONFIGURE_SDN_RESULT = 'success';
  process.env.CONFIGURE_STORAGE_RESULT = 'success';
  process.env.CONFIGURE_VIRTUALIZATION_RESULT = 'success';
  process.env.E2E_TEST_RESULT = 'success';
  Object.assign(process.env, overrides);
}

describe('cluster-report', () => {
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

  test('renders test report from JUnit when E2E succeeded', async () => withTempDir(async (tempDir) => {
    const xmlPath = path.join(tempDir, 'e2e_summary_replicated_2026-04-15.xml');
    fs.writeFileSync(xmlPath, `<?xml version="1.0" encoding="UTF-8"?>
<testsuites tests="3" failures="1" errors="0" skipped="1">
  <testsuite name="Tests" tests="3" failures="1" errors="0" skipped="1" timestamp="2026-04-15T09:30:44">
    <testcase name="[It] passes" status="passed"></testcase>
    <testcase name="[It] fails" status="failed"><failure message="boom">boom</failure></testcase>
    <testcase name="[It] skipped" status="skipped"></testcase>
  </testsuite>
</testsuites>
`);

    const reportFile = path.join(tempDir, 'report.json');
    setStageEnv({
      E2E_REPORT_DIR: tempDir,
      REPORT_FILE: reportFile,
    });

    const report = await buildClusterReport({core: createCore(), context: createContext()});

    expect(report.reportKind).toBe('tests');
    expect(report.failedStage).toBe('success');
    expect(report.metrics).toEqual({
      passed: 1,
      failed: 1,
      errors: 0,
      skipped: 1,
      total: 3,
      successRate: 33.33,
    });
    expect(report.failedTests).toEqual(['[It] fails']);
    expect(report.reportSource).toBe('junit');
    expect(JSON.parse(fs.readFileSync(reportFile, 'utf8')).reportKind).toBe('tests');
  }));

  test('reports configure-sdn as the failed pre-E2E phase', async () => withTempDir(async (tempDir) => {
    const reportFile = path.join(tempDir, 'report.json');
    setStageEnv({
      E2E_REPORT_DIR: tempDir,
      REPORT_FILE: reportFile,
      CONFIGURE_SDN_RESULT: 'failure',
      CONFIGURE_STORAGE_RESULT: 'skipped',
      CONFIGURE_VIRTUALIZATION_RESULT: 'skipped',
      E2E_TEST_RESULT: 'skipped',
    });

    const report = await buildClusterReport({core: createCore(), context: createContext()});

    expect(report.reportKind).toBe('stage-failure');
    expect(report.failedStage).toBe('configure-sdn');
    expect(report.failedStageLabel).toBe('CONFIGURE SDN');
    expect(report.status).toBe('failure');
  }));

  test('marks missing artifacts when test stage is successful but no reports were found', async () => withTempDir(async (tempDir) => {
    const reportFile = path.join(tempDir, 'report.json');
    setStageEnv({
      E2E_REPORT_DIR: tempDir,
      REPORT_FILE: reportFile,
    });

    const report = await buildClusterReport({core: createCore(), context: createContext()});

    expect(report.reportKind).toBe('artifact-missing');
    expect(report.failedStage).toBe('artifact-missing');
    expect(report.failedStageLabel).toBe('TEST REPORTS NOT FOUND');
    expect(report.status).toBe('missing');
  }));
});
