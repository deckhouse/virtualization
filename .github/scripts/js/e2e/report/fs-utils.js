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

  const entries = fs
    .readdirSync(dirPath, { withFileTypes: true })
    .sort((left, right) => left.name.localeCompare(right.name));

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

module.exports = {
  listMatchingFiles,
};
