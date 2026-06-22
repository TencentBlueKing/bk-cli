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

package runner_test

import (
	"errors"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/TencentBlueKing/bk-cli/tests/integration/runner"
)

var _ = Describe("YAML case runner", func() {
	Describe("LoadCases", func() {
		It("loads valid YAML cases and sorts them by id", func() {
			root := GinkgoT().TempDir()
			Expect(os.MkdirAll(filepath.Join(root, "api"), 0o755)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(root, "api", "B-002-second.yaml"), []byte(`
id: B-002
name: second
steps:
  - name: noop
    command: ["version"]
    expect:
      exit_code: 0
      checks:
        - path: ok
          equals: true
`), 0o644)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(root, "api", "A-001-first.yaml"), []byte(`
id: A-001
name: first
steps:
  - name: noop
    command: ["version"]
    expect:
      exit_code: 0
      checks:
        - path: ok
          equals: true
`), 0o644)).To(Succeed())

			cases, err := runner.LoadCases(root)
			Expect(err).NotTo(HaveOccurred())
			Expect(cases).To(HaveLen(2))
			Expect(cases[0].ID).To(Equal("A-001"))
			Expect(cases[1].ID).To(Equal("B-002"))
		})

		It("rejects duplicate case ids", func() {
			root := GinkgoT().TempDir()
			Expect(os.WriteFile(filepath.Join(root, "first.yaml"), []byte(`
id: API-001
name: first
steps:
  - name: noop
    command: ["version"]
    expect:
      exit_code: 0
      checks:
        - path: ok
          equals: true
`), 0o644)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(root, "second.yaml"), []byte(`
id: API-001
name: second
steps:
  - name: noop
    command: ["version"]
    expect:
      exit_code: 0
      checks:
        - path: ok
          equals: true
`), 0o644)).To(Succeed())

			_, err := runner.LoadCases(root)
			Expect(err).To(MatchError(ContainSubstring("duplicate case id")))
		})
	})

	Describe("SelectCases", func() {
		It("filters by scenario id and case path", func() {
			root := GinkgoT().TempDir()
			Expect(os.MkdirAll(filepath.Join(root, "api"), 0o755)).To(Succeed())
			firstPath := filepath.Join(root, "api", "API-001.yaml")
			secondPath := filepath.Join(root, "api", "API-002.yaml")
			Expect(os.WriteFile(firstPath, []byte(`
id: API-001
name: first
steps:
  - name: noop
    command: ["version"]
    expect:
      exit_code: 0
      checks:
        - path: ok
          equals: true
`), 0o644)).To(Succeed())
			Expect(os.WriteFile(secondPath, []byte(`
id: API-002
name: second
steps:
  - name: noop
    command: ["version"]
    expect:
      exit_code: 0
      checks:
        - path: ok
          equals: true
`), 0o644)).To(Succeed())

			cases, err := runner.LoadCases(root)
			Expect(err).NotTo(HaveOccurred())

			selected, err := runner.SelectCases(cases, root, "API-002", "")
			Expect(err).NotTo(HaveOccurred())
			Expect(selected).To(HaveLen(1))
			Expect(selected[0].ID).To(Equal("API-002"))

			selected, err = runner.SelectCases(cases, root, "", filepath.Join("api", "API-001.yaml"))
			Expect(err).NotTo(HaveOccurred())
			Expect(selected).To(HaveLen(1))
			Expect(selected[0].Path).To(Equal(firstPath))
		})
	})

	Describe("EvaluateExpectations", func() {
		It("supports stderr JSON fallback and structured checks", func() {
			result := runner.ExecutionResult{
				Command:  []string{"bk-cli", "auth", "check"},
				ExitCode: 1,
				Stderr:   `{"ok":false,"error":{"code":"no_credentials","message":"No credentials found"}}`,
			}

			err := runner.EvaluateExpectations(result, runner.Expectation{
				ExitCode: runner.ExitCodeExpectation{Mode: runner.ExitCodeNonZero},
				Checks: []runner.Check{
					{Path: "ok", Equals: false},
					{Path: "error.code", Equals: "no_credentials"},
					{Path: "error.message", Contains: "No credentials"},
				},
			})
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns the declared mismatch category for failed assertions", func() {
			result := runner.ExecutionResult{
				Command:  []string{"bk-cli", "devops", "start_build"},
				ExitCode: 0,
				Stdout:   `{"ok":true,"status":500,"data":{"error":"unexpected"}}`,
			}

			err := runner.EvaluateExpectations(result, runner.Expectation{
				ExitCode: runner.ExitCodeExpectation{Mode: runner.ExitCodeExact, Value: 0},
				Category: runner.CategoryMockMismatch,
				Checks: []runner.Check{
					{Path: "status", Equals: 201},
				},
			})
			Expect(err).To(HaveOccurred())

			var mismatch *runner.ExpectationError
			Expect(err).To(MatchError(ContainSubstring("status to equal 201")))
			Expect(errors.As(err, &mismatch)).To(BeTrue())
			Expect(mismatch.Category).To(Equal(runner.CategoryMockMismatch))
		})
	})
})
