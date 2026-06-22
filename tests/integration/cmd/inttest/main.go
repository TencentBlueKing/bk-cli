/*
 * TencentBlueKing is pleased to support the open source community by making
 * 蓝鲸智云 - bk-cli (BlueKing - Cli) available.
 * Copyright (C) Tencent. All rights reserved.
 * Licensed under the MIT License (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 *     http://opensource.org/licenses/MIT
 *
 * Unless required by applicable law or agreed to in writing, software distributed under
 * the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND,
 * either express or implied. See the License for the specific language governing permissions and
 * limitations under the License.
 *
 * We undertake not to change the open source license (MIT license) applicable
 * to the current version of the project delivered to anyone in the future.
 */

// Command inttest runs the YAML integration runner inside the test container.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/TencentBlueKing/bk-cli/tests/integration/runner"
)

func main() {
	var config runner.RunnerConfig

	flag.StringVar(&config.BKCLIBin, "bk-cli-bin", os.Getenv("BK_CLI_BIN"), "path to the bk-cli binary under test")
	flag.StringVar(&config.CasesRoot, "cases-root", os.Getenv("CASES_DIR"), "path to the YAML case directory")
	flag.StringVar(&config.ReportDir, "report-dir", os.Getenv("REPORT_DIR"), "path to the report output directory")
	flag.StringVar(&config.FixturesDir, "fixtures-dir", os.Getenv("FIXTURES_DIR"), "path to shared fixtures")
	flag.StringVar(&config.Scenario, "scenario", os.Getenv("SCENARIO"), "run a single scenario id")
	flag.StringVar(&config.CasePath, "case", os.Getenv("CASE"), "run a single case path")
	flag.Parse()

	if config.BKCLIBin == "" || config.CasesRoot == "" || config.ReportDir == "" || config.FixturesDir == "" {
		fmt.Fprintln(os.Stderr, "bk-cli-bin, cases-root, report-dir, and fixtures-dir are required")
		os.Exit(2)
	}

	report, err := runner.Run(config)
	if err != nil {
		if report.Status == "fail" {
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}
