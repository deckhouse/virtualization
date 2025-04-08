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
const userClusterLables = {
  'e2e/user/nevermarine' : { type: 'e2e-user', id: '72984806'},
  'e2e/user/Isteb4k' : { type: 'e2e-user', id: '93128416'},
  'e2e/user/fl64' : { type: 'e2e-user', id: '2950658'},
  'e2e/user/hardcoretime' : { type: 'e2e-user', id: '36233932'},
  'e2e/user/yaroslavborbat' : { type: 'e2e-user', id: '86148689'},
  'e2e/user/danilrwx' : { type: 'e2e-user', id: '22664775'},
  'e2e/user/eofff' : { type: 'e2e-user', id: '7936159'},
  'e2e/user/LopatinDmitr' : { type: 'e2e-user', id: '93423466'},
  'e2e/user/universal-itengineer' : { type: 'e2e-user', id: '141920865'},
  'e2e/user/hayer969' : { type: 'e2e-user', id: '74240854'},

};
module.exports.userClusterLabels = userClusterLables;

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

// Providers for e2e tests.
const providers = Object.entries(labels)
  .filter(([name, info]) => info.type === 'e2e-run')
  .map(([name, info]) => info.provider)
  .sort();
module.exports.knownProviders = providers;

// Channels available for deploy.
const channels = [
  //
  'alpha',
  'beta',
  'early-access',
  'stable',
  'rock-solid'
];

module.exports.knownChannels = channels;

const criNames = Object.entries(labels)
  .filter(([name, info]) => info.type === 'e2e-use' && !!info.cri)
  .map(([name, info]) => info.cri);
module.exports.knownCRINames = criNames;

const kubernetesVersions = Object.entries(labels)
  .filter(([name, info]) => info.type === 'e2e-use' && !!info.ver)
  .map(([name, info]) => info.ver)
  .sort();
module.exports.knownKubernetesVersions = kubernetesVersions;

module.exports.e2eDefaults = {
  criName: 'Containerd',
  edition: 'FE',
  multimaster: false,
  cis: false
};

const editions = ['CE', 'EE', 'FE', 'BE', 'SE', 'SE-plus'];
module.exports.knownEditions = editions;
