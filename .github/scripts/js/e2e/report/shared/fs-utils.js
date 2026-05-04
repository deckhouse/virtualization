const fs = require("fs");
const path = require("path");

/**
 * Recursively collects files whose base name matches the provided pattern.
 *
 * @param {string} dirPath Directory to scan.
 * @param {RegExp} filePattern Regular expression applied to file names.
 * @param {string[]} [files=[]] Accumulator used during recursion.
 * @returns {string[]} Matching file paths.
 */
function listMatchingFiles(dirPath, filePattern, files = []) {
  if (!fs.existsSync(dirPath)) {
    return files;
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
      listMatchingFiles(fullPath, filePattern, files);
      continue;
    }

    if (filePattern.test(entry.name)) {
      files.push(fullPath);
    }
  }

  return files;
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
      `Expected a single ${description}, but found ${matchingFiles.length}: ${matchingFiles.join(
        ", "
      )}`
    );
  }

  return matchingFiles[0];
}

module.exports = {
  findSingleMatchingFile,
  listMatchingFiles,
};
