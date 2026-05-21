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

const { aggregate } = require("./data");
const featureDurationStatus = require("./builders/feature-duration-status");
const slowestSpecs = require("./builders/slowest-specs");

// Order of charts matches the order of attachments in the messenger thread.
const CHART_BUILDERS = [featureDurationStatus];

function buildClusterChartConfigs(specTimings) {
  const data = aggregate(specTimings);
  return CHART_BUILDERS.map((build) => build(data));
}

module.exports = {
  CHART_BUILDERS,
  buildClusterChartConfigs,
  slowestSpecs,
};
