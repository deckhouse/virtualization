const fs = require('fs');
const os = require('os');
const path = require('path');

const renderMessengerReport = require('./messenger-report');

function createCore() {
  return {
    info: jest.fn(),
    warning: jest.fn(),
    debug: jest.fn(),
    setOutput: jest.fn(),
  };
}

async function withTempDir(testFn) {
  const tempDir = fs.mkdtempSync(path.join(os.tmpdir(), 'messenger-report-test-'));
  try {
    return await testFn(tempDir);
  } finally {
    fs.rmSync(tempDir, {recursive: true, force: true});
  }
}

describe('messenger-report', () => {
  afterEach(() => {
    delete process.env.REPORTS_DIR;
    delete process.env.STORAGE_TYPES;
    delete process.env.LOOP_API_BASE_URL;
    delete process.env.LOOP_CHANNEL_ID;
    delete process.env.LOOP_TOKEN;
    delete global.fetch;
  });

  test('renders test results and stage failures in separate sections', async () => withTempDir(async (tempDir) => {
    fs.writeFileSync(path.join(tempDir, 'e2e_report_replicated.json'), JSON.stringify({
      cluster: 'replicated',
      storageType: 'replicated',
      reportKind: 'tests',
      branch: 'main',
      workflowRunUrl: 'https://example.invalid/replicated',
      startedAt: '2026-04-15T09:30:44',
      metrics: {
        passed: 12,
        failed: 1,
        errors: 0,
        total: 13,
        successRate: 92.31,
      },
      failedTests: ['[It] fails'],
    }));

    fs.writeFileSync(path.join(tempDir, 'e2e_report_nfs.json'), JSON.stringify({
      cluster: 'nfs',
      storageType: 'nfs',
      reportKind: 'stage-failure',
      branch: 'main',
      workflowRunUrl: 'https://example.invalid/nfs',
      failedStage: 'configure-sdn',
      failedStageLabel: 'CONFIGURE SDN',
      metrics: {
        passed: 0,
        failed: 0,
        errors: 0,
        total: 0,
        successRate: 0,
      },
      failedTests: [],
    }));

    process.env.REPORTS_DIR = tempDir;
    process.env.STORAGE_TYPES = '["replicated","nfs"]';

    const result = await renderMessengerReport({core: createCore()});

    expect(result.message).toContain('### Test results');
    expect(result.message).toContain('| [replicated](https://example.invalid/replicated) | 12 | 1 | 0 | 13 | 92.31% |');
    expect(result.message).toContain('### Cluster failures');
    expect(result.message).toContain('- [nfs](https://example.invalid/nfs): CONFIGURE SDN');
    expect(result.message).not.toContain('### Failed tests');
    expect(result.threadMessage).toContain('### Failed tests');
    expect(result.threadMessage).toContain('**replicated**');
    expect(result.threadMessage).toContain('- [It] fails');
    expect(result.threadMessage).not.toContain('**nfs**');
  }));

  test('creates artifact-missing entry for absent cluster report', async () => withTempDir(async (tempDir) => {
    process.env.REPORTS_DIR = tempDir;
    process.env.STORAGE_TYPES = '["replicated"]';

    const result = await renderMessengerReport({core: createCore()});

    expect(result.message).toContain('### Cluster failures');
    expect(result.message).toContain('- replicated: TEST REPORTS NOT FOUND');
    expect(result.threadMessage).toBe('');
  }));

  test('posts main report and failed tests thread via Loop API', async () => withTempDir(async (tempDir) => {
    fs.writeFileSync(path.join(tempDir, 'e2e_report_replicated.json'), JSON.stringify({
      cluster: 'replicated',
      storageType: 'replicated',
      reportKind: 'tests',
      branch: 'main',
      workflowRunUrl: 'https://example.invalid/replicated',
      startedAt: '2026-04-15T09:30:44',
      metrics: {
        passed: 10,
        failed: 1,
        errors: 0,
        total: 11,
        successRate: 90.91,
      },
      failedTests: ['[It] fails'],
    }));

    process.env.REPORTS_DIR = tempDir;
    process.env.STORAGE_TYPES = '["replicated"]';
    process.env.LOOP_API_BASE_URL = 'https://loop.example.invalid';
    process.env.LOOP_CHANNEL_ID = 'channel-id';
    process.env.LOOP_TOKEN = 'loop-token';

    global.fetch = jest.fn()
      .mockResolvedValueOnce({
        ok: true,
        status: 201,
        text: async () => JSON.stringify({id: 'root-post-id'}),
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 201,
        text: async () => JSON.stringify({id: 'thread-post-id'}),
      });

    const result = await renderMessengerReport({core: createCore()});

    expect(global.fetch).toHaveBeenCalledTimes(2);
    expect(global.fetch).toHaveBeenNthCalledWith(
      1,
      'https://loop.example.invalid/api/v4/posts',
      expect.objectContaining({
        method: 'POST',
        headers: expect.objectContaining({
          Authorization: 'Bearer loop-token',
          'Content-Type': 'application/json',
        }),
      }),
    );
    expect(JSON.parse(global.fetch.mock.calls[0][1].body)).toEqual({
      channel_id: 'channel-id',
      message: result.message,
    });
    expect(JSON.parse(global.fetch.mock.calls[1][1].body)).toEqual({
      channel_id: 'channel-id',
      message: result.threadMessage,
      root_id: 'root-post-id',
    });
  }));
});
