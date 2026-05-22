#!/usr/bin/env node

const fs = require("fs");
const path = require("path");
const { spawnSync } = require("child_process");

function printUsage() {
  console.log(`Usage:
  node tmp/test-ci/report/run-local-report.js --json /absolute/path/e2e_report_replicated_2026-05-15.json [options]
  node tmp/test-ci/report/run-local-report.js --json /path/replicated.json --json /path/nfs.json [options]
  node tmp/test-ci/report/run-local-report.js --json-dir /absolute/path/with/reports [options]
  node tmp/test-ci/report/run-local-report.js --cluster replicated --stage configure-sdn [options]
  node tmp/test-ci/report/run-local-report.js --spoiler-samples [options]

Options:
  --storage <name>         Storage/cluster name for a single JSON without a CI-style file name
  --cluster <name>         Build a local report entry without JSON input
  --branch <name>          Branch label in the report. Default: local-test
  --run-url <url>          URL used in report links. Default: https://example.invalid/local-run
  --loop-api-base-url <url>
                           Optional Loop base URL. Examples:
                           https://loop.flant.ru
                           https://loop.flant.ru/api/v4
  --channel-id <id>        Optional Loop channel id for API delivery
  --token <token>          Optional Loop bot token for API delivery
  --strict-loop-delivery   Fail the local run when Loop delivery fails
  --strict-loop-file-upload
                           Fail the local run when chart upload fails instead of posting without files
  --json-dir <path>        Load all *.json files from a directory
  --xml <path>             Backward-compatible alias for --json
  --xml-dir <path>         Backward-compatible alias for --json-dir
  --stage <name>           Stage result to emulate. Default: success
                           Allowed: success, bootstrap, configure-sdn, storage-setup, virtualization-setup, e2e-test
  --result <value>         Result for the selected stage. Default: failure
                           Allowed for non-success stages: failure, cancelled, skipped
  --out-dir <path>         Output directory. Default: tmp/test-ci/report/out
  --charts-dir <path>      Chart PNG output directory. Default: tmp/charts
  --charts-manifest <path> Messenger chart manifest. Default: <work-dir>/messenger-charts/manifest.json
  --spoiler-samples        Render/send several spoiler/collapsible markdown samples instead of an E2E report

Examples:
  node tmp/test-ci/report/run-local-report.js --json tmp/test-ci/report/e2e_report_replicated_2026-05-15.json
  node tmp/test-ci/report/run-local-report.js --json tmp/test-ci/report/e2e_report_replicated_2026-05-15.json --cluster nfs --stage configure-sdn
  node tmp/test-ci/report/run-local-report.js --json-dir /tmp/e2e-reports
  node tmp/test-ci/report/run-local-report.js --cluster replicated --stage e2e-test --result cancelled
  node tmp/test-ci/report/run-local-report.js --json-dir /tmp/e2e-reports --loop-api-base-url "https://loop.flant.ru" --channel-id "$LOOP_CHANNEL_ID" --token "$LOOP_TOKEN"
  node tmp/test-ci/report/run-local-report.js --json-dir /tmp/e2e-reports --loop-api-base-url "https://loop.flant.ru" --channel-id "$LOOP_CHANNEL_ID" --token "$LOOP_TOKEN" --strict-loop-delivery --strict-loop-file-upload
  node tmp/test-ci/report/run-local-report.js --spoiler-samples --loop-api-base-url "https://loop.flant.ru" --channel-id "$LOOP_CHANNEL_ID" --token "$LOOP_TOKEN"
`);
}

function parseArgs(argv) {
  const args = {};

  for (let i = 0; i < argv.length; i += 1) {
    const token = argv[i];
    if (!token.startsWith("--")) {
      continue;
    }

    const key = token.slice(2);
    const value = argv[i + 1];
    if (!value || value.startsWith("--")) {
      args[key] = true;
      continue;
    }

    if (Object.prototype.hasOwnProperty.call(args, key)) {
      if (Array.isArray(args[key])) {
        args[key].push(value);
      } else {
        args[key] = [args[key], value];
      }
    } else {
      args[key] = value;
    }
    i += 1;
  }

  return args;
}

function toArray(value) {
  if (typeof value === "undefined") {
    return [];
  }

  return Array.isArray(value) ? value : [value];
}

function ensureFileExists(filePath, label) {
  if (!filePath || !fs.existsSync(filePath)) {
    throw new Error(`${label} does not exist: ${filePath || "<empty>"}`);
  }
}

function mkdirp(dirPath) {
  fs.mkdirSync(dirPath, { recursive: true });
}

function copyFile(sourcePath, targetPath) {
  mkdirp(path.dirname(targetPath));
  fs.copyFileSync(sourcePath, targetPath);
}

function resetDirectory(dirPath) {
  fs.rmSync(dirPath, { recursive: true, force: true });
  mkdirp(dirPath);
}

function createCore(outputs) {
  return {
    info(message) {
      console.log(`[INFO] ${message}`);
    },
    warning(message) {
      console.warn(`[WARN] ${message}`);
    },
    debug(message) {
      console.debug(`[DEBUG] ${message}`);
    },
    setOutput(name, value) {
      outputs[name] = value;
      console.log(`[OUTPUT] ${name}=${value}`);
    },
  };
}

function buildStageResults(stage, stageResult) {
  const stageResults = {
    bootstrap: "success",
    "configure-sdn": "success",
    "storage-setup": "success",
    "virtualization-setup": "success",
    "e2e-test": "success",
  };

  if (!stage || stage === "success") {
    return stageResults;
  }

  if (!Object.prototype.hasOwnProperty.call(stageResults, stage)) {
    throw new Error(`Unsupported --stage value: ${stage}`);
  }
  if (!["failure", "cancelled", "skipped"].includes(stageResult)) {
    throw new Error(
      `Unsupported --result value: ${stageResult}. Allowed: failure, cancelled, skipped`
    );
  }

  stageResults[stage] = stageResult;
  return stageResults;
}

function collectJsonFilesFromDir(dirPath) {
  if (!fs.existsSync(dirPath)) {
    throw new Error(`JSON directory does not exist: ${dirPath}`);
  }

  const entries = fs
    .readdirSync(dirPath, { withFileTypes: true })
    .filter((entry) => entry.isFile() && entry.name.endsWith(".json"))
    .map((entry) => path.join(dirPath, entry.name))
    .sort((left, right) => left.localeCompare(right));

  if (entries.length === 0) {
    throw new Error(`No JSON files found in directory: ${dirPath}`);
  }

  return entries;
}

function deriveStorageType(reportPath, fallbackStorage) {
  const baseName = path.basename(reportPath);
  const datedMatch = baseName.match(
    /^e2e_report_(.+)_(\d{4}-\d{2}-\d{2}.*)\.json$/
  );
  if (datedMatch) {
    return datedMatch[1];
  }

  const genericMatch = baseName.match(/^e2e_report_(.+?)_.*\.json$/);
  if (genericMatch) {
    return genericMatch[1];
  }

  if (fallbackStorage) {
    return fallbackStorage;
  }

  throw new Error(
    `Unable to derive storage type from file name "${baseName}". Use a CI-style name or pass --storage for a single JSON.`
  );
}

function normalizeLoopApiBaseUrl(value) {
  const trimmedValue = String(value || "")
    .trim()
    .replace(/\/+$/, "");

  if (!trimmedValue) {
    return "";
  }

  if (trimmedValue.endsWith("/api/v4/posts")) {
    return trimmedValue;
  }

  if (trimmedValue.endsWith("/api/v4")) {
    return `${trimmedValue}/posts`;
  }

  return `${trimmedValue}/api/v4/posts`;
}

function buildLoopConfig(
  loopApiBaseUrl,
  loopChannelId,
  loopToken,
  options = {}
) {
  const apiUrl = normalizeLoopApiBaseUrl(loopApiBaseUrl);
  const channelId = String(loopChannelId || "").trim();
  const token = String(loopToken || "").trim();

  if (!apiUrl && !channelId && !token) {
    return null;
  }
  if (!apiUrl || !channelId || !token) {
    throw new Error(
      "LOOP_CHANNEL_ID, LOOP_TOKEN, and LOOP_API_BASE_URL are required"
    );
  }

  return {
    apiUrl,
    channelId,
    token,
    strictDelivery: Boolean(options.strictDelivery),
    strictFileUploads: Boolean(options.strictFileUploads),
  };
}

function buildSpoilerSampleMessages() {
  const longReason =
    "Timed out after 300.001s. object v12n-e2e-testdata-iso status.phase is Pending, expected Ready. Expected <string>: Pending to equal <string>: Ready.";

  return {
    message: [
      "## :dvp: Spoiler/collapsible markdown test",
      "",
      "Root message. Replies contain different syntax variants for hiding long `Reason` text.",
    ].join("\n"),
    threadMessages: [
      {
        message: [
          "**Variant 1: HTML `<details>` / `<summary>`**",
          "",
          "<details>",
          "<summary>Show Reason</summary>",
          "",
          longReason,
          "",
          "</details>",
        ].join("\n"),
        files: [],
      },
      {
        message: [
          "**Variant 2: inline HTML `<details>` inside table cell**",
          "",
          "| Tests | Reason |",
          "|---|---|",
          `| SynchronizedBeforeSuite | <details><summary>Show</summary>${longReason}</details> |`,
        ].join("\n"),
        files: [],
      },
      {
        message: [
          "**Variant 3: StackExchange-style spoiler blockquote**",
          "",
          ">! " + longReason,
        ].join("\n"),
        files: [],
      },
      {
        message: [
          "**Variant 4: Discord/Telegram-style spoiler delimiters**",
          "",
          `||${longReason}||`,
        ].join("\n"),
        files: [],
      },
      {
        message: [
          "**Variant 5: Mattermost spoiler plugin command text**",
          "",
          "/spoiler " + longReason,
        ].join("\n"),
        files: [],
      },
    ],
  };
}

function normalizeThreadMessage(threadMessage) {
  return typeof threadMessage === "string"
    ? { message: threadMessage, files: [] }
    : {
        message: String(threadMessage.message || ""),
        files: Array.isArray(threadMessage.files) ? threadMessage.files : [],
      };
}

function writeThreadArtifacts(threadMessages, threadMessageFile, chartDir) {
  const normalizedThreadMessages = threadMessages.map(normalizeThreadMessage);
  if (normalizedThreadMessages.length === 0) {
    return [];
  }

  fs.writeFileSync(
    threadMessageFile,
    `${normalizedThreadMessages
      .map((threadMessage) => threadMessage.message)
      .join("\n\n---\n\n")}\n`
  );

  const writtenFiles = [];
  for (const [
    messageIndex,
    threadMessage,
  ] of normalizedThreadMessages.entries()) {
    for (const file of threadMessage.files) {
      if (!file || !file.buffer || !file.name) {
        continue;
      }

      mkdirp(chartDir);
      const targetPath = path.join(
        chartDir,
        `${messageIndex + 1}-${path.basename(file.name)}`
      );
      fs.writeFileSync(targetPath, file.buffer);
      writtenFiles.push(targetPath);
    }
  }

  return writtenFiles;
}

function generateMessengerCharts({
  repoRoot,
  reportsDir,
  chartDir,
  manifestPath,
}) {
  const result = spawnSync(
    process.env.PYTHON || "python3",
    [
      path.join(repoRoot, ".github/scripts/python/e2e_report/charts.py"),
      "messenger-all",
      "--reports-dir",
      reportsDir,
      "--out-dir",
      chartDir,
      "--manifest",
      manifestPath,
    ],
    { encoding: "utf8" }
  );

  if (result.error) {
    throw result.error;
  }
  if (result.status !== 0) {
    const output = (result.stderr || result.stdout || "").trim();
    throw new Error(`Chart rendering failed with status ${result.status}: ${output}`);
  }

  return manifestPath;
}

async function renderSpoilerSamples({ core, outDir, loop }) {
  const { makeThreadedReportInLoop } = require(path.join(
    path.resolve(__dirname, "../../.."),
    ".github/scripts/js/e2e/report/messenger/loop-client"
  ));
  const { message, threadMessages } = buildSpoilerSampleMessages();

  resetDirectory(outDir);
  fs.writeFileSync(
    path.join(outDir, "spoiler-samples-main.md"),
    `${message}\n`
  );
  fs.writeFileSync(
    path.join(outDir, "spoiler-samples-thread.md"),
    `${threadMessages
      .map((threadMessage) => threadMessage.message)
      .join("\n\n---\n\n")}\n`
  );

  core.info(message);
  core.setOutput("message", message);
  core.setOutput("thread_messages", JSON.stringify(threadMessages));

  if (loop) {
    await makeThreadedReportInLoop({ message, threadMessages, loop }, core);
  }

  console.log("");
  console.log("Artifacts written:");
  console.log(
    `- Main markdown: ${path.join(outDir, "spoiler-samples-main.md")}`
  );
  console.log(
    `- Thread markdown: ${path.join(outDir, "spoiler-samples-thread.md")}`
  );
  if (loop) {
    console.log("- Loop delivery: attempted");
  }
}

function collectInputEntries(args) {
  const jsonArgs = [
    ...toArray(args.json),
    ...toArray(args.report),
    ...toArray(args.xml),
  ].map((value) => path.resolve(String(value)));
  const jsonDirArgs = [
    ...toArray(args["json-dir"]),
    ...toArray(args["report-dir"]),
    ...toArray(args["xml-dir"]),
  ].map((value) => path.resolve(String(value)));
  const clusterArgs = toArray(args.cluster)
    .map((value) => String(value).trim())
    .filter(Boolean);
  const dirReports = jsonDirArgs.flatMap((dirPath) =>
    collectJsonFilesFromDir(dirPath)
  );
  const allReports = Array.from(new Set([...jsonArgs, ...dirReports]));

  if (allReports.length === 0 && clusterArgs.length === 0) {
    throw new Error(
      "Pass at least one --json, one --json-dir, or one --cluster."
    );
  }

  allReports.forEach((reportPath) =>
    ensureFileExists(reportPath, "Ginkgo JSON report")
  );

  const fallbackStorage = args.storage ? String(args.storage) : "";
  const reportEntries = allReports.map((reportPath) => {
    const storageType = deriveStorageType(
      reportPath,
      allReports.length === 1 ? fallbackStorage : ""
    );
    return { reportPath, storageType };
  });
  const clusterEntries = clusterArgs.map((storageType) => ({
    reportPath: null,
    storageType,
  }));
  const entries = [...reportEntries, ...clusterEntries];

  const uniqueStorageTypes = new Set(entries.map((entry) => entry.storageType));
  if (uniqueStorageTypes.size !== entries.length) {
    throw new Error(
      "Detected duplicate storage types across JSON or cluster inputs. Multiple entries for the same storage are not merged in the local runner."
    );
  }

  return entries;
}

async function main() {
  const args = parseArgs(process.argv.slice(2));
  if (args.help) {
    printUsage();
    process.exit(0);
  }

  const hasInputs =
    Boolean(args["spoiler-samples"]) ||
    toArray(args.json).length > 0 ||
    toArray(args.report).length > 0 ||
    toArray(args.xml).length > 0 ||
    toArray(args["json-dir"]).length > 0 ||
    toArray(args["report-dir"]).length > 0 ||
    toArray(args["xml-dir"]).length > 0 ||
    toArray(args.cluster).length > 0;
  if (!hasInputs) {
    console.log("No report inputs were provided.");
    console.log("Pass at least one --json, one --json-dir, or one --cluster.");
    console.log("");
    printUsage();
    return;
  }

  const repoRoot = path.resolve(__dirname, "../../..");
  const clusterReport = require(path.join(
    repoRoot,
    ".github/scripts/js/e2e/report/cluster-report"
  ));
  const messengerReport = require(path.join(
    repoRoot,
    ".github/scripts/js/e2e/report/messenger-report"
  ));
  const branch = String(args.branch || "local-test");
  const runUrl = String(args["run-url"] || "https://example.invalid/local-run");
  const outDir = path.resolve(
    String(args["out-dir"] || path.join(repoRoot, "tmp/test-ci/report/out"))
  );
  const chartDir = path.resolve(
    String(args["charts-dir"] || path.join(repoRoot, "tmp/charts"))
  );
  const workDir = path.resolve(
    String(args["work-dir"] || path.join(path.dirname(outDir), "work"))
  );
  const messengerChartDir = path.join(workDir, "messenger-charts");
  const chartsManifest = path.resolve(
    String(
      args["charts-manifest"] || path.join(messengerChartDir, "manifest.json")
    )
  );
  const messageFile = path.join(outDir, "messenger-report.md");
  const threadMessageFile = path.join(outDir, "messenger-thread.md");
  const loopApiBaseUrl = args["loop-api-base-url"]
    ? String(args["loop-api-base-url"])
    : String(process.env.LOOP_API_BASE_URL || "");
  const loopChannelId = args["channel-id"]
    ? String(args["channel-id"])
    : String(process.env.LOOP_CHANNEL_ID || "");
  const loopToken = args.token
    ? String(args.token)
    : String(process.env.LOOP_TOKEN || "");
  const stage = String(args.stage || "success");
  const stageResult = String(args.result || "failure");
  const strictLoopDelivery = Boolean(args["strict-loop-delivery"]);
  const strictLoopFileUpload = Boolean(args["strict-loop-file-upload"]);
  const loop = buildLoopConfig(loopApiBaseUrl, loopChannelId, loopToken, {
    strictDelivery: strictLoopDelivery,
    strictFileUploads: strictLoopFileUpload,
  });

  if (args["spoiler-samples"]) {
    const outputs = {};
    await renderSpoilerSamples({ core: createCore(outputs), outDir, loop });
    return;
  }

  const inputEntries = collectInputEntries(args);
  const stageResults = buildStageResults(stage, stageResult);

  resetDirectory(outDir);
  resetDirectory(chartDir);
  resetDirectory(workDir);

  process.env.REPORTS_DIR = outDir;
  process.env.EXPECTED_STORAGE_TYPES = JSON.stringify(
    inputEntries.map((entry) => entry.storageType)
  );
  process.env.LOOP_API_BASE_URL = loopApiBaseUrl;
  process.env.LOOP_CHANNEL_ID = loopChannelId;
  process.env.LOOP_TOKEN = loopToken;
  process.env.LOOP_STRICT_DELIVERY = strictLoopDelivery ? "1" : "";
  process.env.LOOP_STRICT_FILE_UPLOAD = strictLoopFileUpload ? "1" : "";
  process.env.CHARTS_MANIFEST = chartsManifest;

  const outputs = {};
  const core = createCore(outputs);
  const context = {
    serverUrl: "https://github.com",
    repo: { owner: "local", repo: "virtualization2" },
    runId: "local-test",
    ref: `refs/heads/${branch}`,
  };

  const generatedReports = [];
  for (const entry of inputEntries) {
    const reportFile = path.join(
      outDir,
      `e2e_report_${entry.storageType}.json`
    );
    const rawReportPath = entry.reportPath
      ? path.join(workDir, `e2e_report_${entry.storageType}_local.json`)
      : null;

    if (entry.reportPath) {
      copyFile(entry.reportPath, rawReportPath);
    }

    await clusterReport({
      core,
      context,
      config: {
        storageType: entry.storageType,
        pipelineJobName: `Local E2E (${entry.storageType})`,
        reportsDir: workDir,
        reportFile,
        stageResults,
        stageJobUrls: {
          bootstrap: runUrl,
          "configure-sdn": runUrl,
          "storage-setup": runUrl,
          "virtualization-setup": runUrl,
          "e2e-test": runUrl,
        },
      },
    });
    generatedReports.push(reportFile);
  }

  generateMessengerCharts({
    repoRoot,
    reportsDir: outDir,
    chartDir: messengerChartDir,
    manifestPath: chartsManifest,
  });

  const renderedMessages = await messengerReport({ core });
  const message = renderedMessages.message;
  const threadMessages = renderedMessages.threadMessages || [];

  fs.writeFileSync(messageFile, `${message}\n`);
  const chartFiles = writeThreadArtifacts(
    threadMessages,
    threadMessageFile,
    chartDir
  );

  console.log("");
  console.log("Artifacts written:");
  generatedReports.forEach((reportFile) => {
    console.log(`- JSON: ${reportFile}`);
  });
  console.log(`- Main markdown: ${messageFile}`);
  if (threadMessages.length > 0) {
    console.log(`- Thread markdown: ${threadMessageFile}`);
  }
  console.log(`- Chart manifest: ${chartsManifest}`);
  chartFiles.forEach((chartFile) => {
    console.log(`- Chart: ${chartFile}`);
  });

  if (loopApiBaseUrl || loopChannelId || loopToken) {
    console.log("- Loop delivery: attempted");
  }
}

main().catch((error) => {
  console.error(`[ERROR] ${error.message}`);
  if (process.env.DEBUG_LOCAL_REPORT === "1" && error.stack) {
    console.error(error.stack);
  }
  process.exit(1);
});
