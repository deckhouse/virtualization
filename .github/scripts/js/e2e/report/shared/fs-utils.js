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

function collectMatchingFiles(dirPath, filePattern, acc) {
  if (!fs.existsSync(dirPath)) {
    return;
  }

  let entries;
  try {
    entries = fs
      .readdirSync(dirPath, { withFileTypes: true })
      .sort((left, right) => left.name.localeCompare(right.name));
  } catch (error) {
    throw new Error(`Unable to scan directory ${dirPath}: ${error.message}`);
  }

  for (const entry of entries) {
    const fullPath = path.join(dirPath, entry.name);
    if (entry.isDirectory()) {
      collectMatchingFiles(fullPath, filePattern, acc);
    } else if (filePattern.test(entry.name)) {
      acc.push(fullPath);
    }
  }
}

/**
 * Recursively collects files whose base name matches the provided pattern.
 *
 * @param {string} dirPath Directory to scan.
 * @param {RegExp} filePattern Regular expression applied to file names.
 * @returns {string[]} Matching file paths.
 */
function listMatchingFiles(dirPath, filePattern) {
  const acc = [];
  collectMatchingFiles(dirPath, filePattern, acc);
  return acc;
}

/**
 * Resolves a single file matching the provided pattern.
 *
 * @param {string} dirPath Directory containing candidate files.
 * @param {RegExp} filePattern Pattern matching the expected file name.
 * @param {string} [description="file"] Human-readable file kind for errors.
 * @returns {string|null} Matching file path or null when no match exists.
 * @throws {Error} When more than one matching file is found.
 */
function findSingleMatchingFile(dirPath, filePattern, description = "file") {
  const matchingFiles = listMatchingFiles(dirPath, filePattern);
  if (matchingFiles.length === 0) {
    return null;
  }

  if (matchingFiles.length > 1) {
    throw new Error(
      `Expected a single ${description}, but found ${
        matchingFiles.length
      }: ${matchingFiles.join(", ")}`
    );
  }

  return matchingFiles[0];
}

module.exports = {
  findSingleMatchingFile,
  listMatchingFiles,
};
