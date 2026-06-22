#!/bin/sh
# TencentBlueKing is pleased to support the open source community by making
# 蓝鲸智云 - bk-cli (BlueKing - Cli) available.
# Copyright (C) Tencent. All rights reserved.
# Licensed under the MIT License (the "License"); you may not use this file except
# in compliance with the License. You may obtain a copy of the License at
#
#     http://opensource.org/licenses/MIT
#
# Unless required by applicable law or agreed to in writing, software distributed under
# the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND,
# either express or implied. See the License for the specific language governing permissions and
# limitations under the License.
#
# We undertake not to change the open source license (MIT license) applicable
# to the current version of the project delivered to anyone in the future.

set -eu

require_var() {
  name="$1"
  eval "value=\${$name:-}"
  if [ -z "$value" ]; then
    echo "missing required environment variable: $name" >&2
    exit 2
  fi
}

require_var BK_CLI_BIN
require_var RUNNER_BIN
require_var CASES_DIR
require_var FIXTURES_DIR
require_var REPORT_DIR

if [ ! -x "$BK_CLI_BIN" ]; then
  echo "bk-cli binary not found or not executable at $BK_CLI_BIN" >&2
  exit 2
fi

if [ ! -x "$RUNNER_BIN" ]; then
  echo "integration runner not found or not executable at $RUNNER_BIN" >&2
  exit 2
fi

if [ ! -d "$CASES_DIR" ]; then
  echo "case directory not found at $CASES_DIR" >&2
  exit 2
fi

if [ ! -d "$FIXTURES_DIR" ]; then
  echo "fixtures directory not found at $FIXTURES_DIR" >&2
  exit 2
fi

mkdir -p "$REPORT_DIR"
rm -f "$REPORT_DIR/results.json" "$REPORT_DIR/results.xml"

set -- "$RUNNER_BIN" \
  --bk-cli-bin "$BK_CLI_BIN" \
  --cases-root "$CASES_DIR" \
  --report-dir "$REPORT_DIR" \
  --fixtures-dir "$FIXTURES_DIR"

if [ -n "${SCENARIO:-}" ]; then
  set -- "$@" --scenario "$SCENARIO"
fi

if [ -n "${CASE:-}" ]; then
  set -- "$@" --case "$CASE"
fi

exec "$@"
