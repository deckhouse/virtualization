// Copyright 2022 Flant JSC
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//     http://www.apache.org/licenses/LICENSE-2.0
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//@ts-check

const skipE2eLabel = 'skip/e2e';
const abortFailedE2eCommand = '/e2e/abort';
module.exports.skipE2eLabel = skipE2eLabel;
module.exports.abortFailedE2eCommand = abortFailedE2eCommand;

// Labels available for pull requests.
const labels = {
  // E2E
  'e2e/run': { type: 'e2e-run', provider: 'static' },
  // Allow running workflows for external PRs.
  'status/ok-to-test': { type: 'ok-to-test' },

};
module.exports.knownLabels = labels;

// Label to detect if issue is a release issue.
const releaseIssueLabel = 'issue/release';
module.exports.releaseIssueLabel = releaseIssueLabel;

const slashCommands = {
  deploy: ['deploy/alpha', 'deploy/beta', 'deploy/early-access', 'deploy/stable', 'deploy/rock-solid'],
  suspend: ['suspend/alpha', 'suspend/beta', 'suspend/early-access', 'suspend/stable', 'suspend/rock-solid']
};
module.exports.knownSlashCommands = slashCommands;

module.exports.labelsSrv = {
  /**
   * Search for known label name using its type and property:
   * - search by provider property for e2e-run labels
   * - search by env property for deploy-web labels
   *
   * @param {object} inputs
   * @param {string} inputs.labelType
   * @param {string} inputs.labelSubject
   * @returns {string}
   */
  findLabel: ({ labelType, labelSubject }) => {
    return (Object.entries(labels).find(([name, info]) => {
      if (info.type === labelType) {
        if (labelType === 'e2e-run') {
          return info.provider === labelSubject;
        }
        if (labelType === 'deploy-web') {
          return info.env === labelSubject;
        }

        return true;
      }
      return false;
    }) || [''])[0];
  }
};
