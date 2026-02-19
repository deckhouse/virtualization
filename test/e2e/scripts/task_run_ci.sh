#!/usr/bin/env bash

# Copyright 2025 Flant JSC
# 
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
# 
#      http://www.apache.org/licenses/LICENSE-2.0
# 
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

GINKGO_RESULT=$(mktemp)

DATE=$(date +"%Y-%m-%d")
echo "DATE=$DATE" >> $GITHUB_ENV
START_TIME=$(date +"%H:%M:%S")
echo "START_TIME=$START_TIME" >> $GITHUB_ENV

go tool ginkgo \
    --focus="VirtualMachineAdditionalNetworkInterfaces" \
    -v --race --timeout=$TIMEOUT | tee $GINKGO_RESULT
EXIT_CODE="${PIPESTATUS[0]}"
RESULT=$(sed -e "s/\x1b\[[0-9;]*m//g" $GINKGO_RESULT | grep --color=never -E "FAIL!|SUCCESS!")
if [[ $RESULT == FAIL!* || $EXIT_CODE -ne "0" ]]; then
    RESULT_STATUS=":x: FAIL!"
elif [[ $RESULT == SUCCESS!* ]]; then
    RESULT_STATUS=":white_check_mark: SUCCESS!"
else
    RESULT_STATUS=":question: UNKNOWN"
    EXIT_CODE=1
fi

PASSED=$(echo "$RESULT" | grep -oP "\d+(?= Passed)")
FAILED=$(echo "$RESULT" | grep -oP "\d+(?= Failed)")
PENDING=$(echo "$RESULT" | grep -oP "\d+(?= Pending)")
SKIPPED=$(echo "$RESULT" | grep -oP "\d+(?= Skipped)")

SUMMARY=$(jq -n \
    --arg csi "$CSI" \
    --arg date "$DATE" \
    --arg startTime "$START_TIME" \
    --arg branch "$GITHUB_REF_NAME" \
    --arg status "$RESULT_STATUS" \
    --argjson passed "$PASSED" \
    --argjson failed "$FAILED" \
    --argjson pending "$PENDING" \
    --argjson skipped "$SKIPPED" \
    --arg link "$GITHUB_SERVER_URL/$GITHUB_REPOSITORY/actions/runs/$GITHUB_RUN_ID" \
    '{
    CSI: $csi,
    Date: $date,
    StartTime: $startTime,
    Branch: $branch,
    Status: $status,
    Passed: $passed,
    Failed: $failed,
    Pending: $pending,
    Skipped: $skipped,
    Link: $link
    }'
)

echo "$SUMMARY"
echo "SUMMARY=$(echo "$SUMMARY" | jq -c .)" >> $GITHUB_ENV
exit $EXIT_CODE
