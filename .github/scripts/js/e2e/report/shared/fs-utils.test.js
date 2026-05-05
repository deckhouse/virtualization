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
const os = require("os");
const path = require("path");

const { listMatchingFiles } = require("./fs-utils");

/**
 * Runs a test body inside a temporary directory and removes it afterwards.
 *
 * @template T
 * @param {function(string): (Promise<T>|T)} testFn Test body.
 * @returns {Promise<T>} Test result.
 */
async function withTempDir(testFn) {
  const tempDir = fs.mkdtempSync(path.join(os.tmpdir(), "fs-utils-test-"));
  try {
    return await testFn(tempDir);
  } finally {
    fs.rmSync(tempDir, { recursive: true, force: true });
  }
}

describe("fs-utils", () => {
  test("returns sorted matching files recursively", async () =>
    withTempDir((tempDir) => {
      const nestedDir = path.join(tempDir, "nested");
      fs.mkdirSync(nestedDir, { recursive: true });
      fs.writeFileSync(path.join(tempDir, "b.json"), "{}\n");
      fs.writeFileSync(path.join(tempDir, "a.txt"), "nope\n");
      fs.writeFileSync(path.join(nestedDir, "a.json"), "{}\n");

      expect(listMatchingFiles(tempDir, /\.json$/)).toEqual([
        path.join(tempDir, "b.json"),
        path.join(nestedDir, "a.json"),
      ]);
    }));

  test("throws a descriptive error when a directory cannot be scanned", async () =>
    withTempDir((tempDir) => {
      const readdirSpy = jest
        .spyOn(fs, "readdirSync")
        .mockImplementation(() => {
          throw new Error("permission denied");
        });

      try {
        expect(() => listMatchingFiles(tempDir, /\.json$/)).toThrow(
          `Unable to scan directory ${tempDir}: permission denied`
        );
      } finally {
        readdirSpy.mockRestore();
      }
    }));
});
