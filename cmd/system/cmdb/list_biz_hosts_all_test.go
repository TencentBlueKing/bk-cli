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

package cmdb

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"

	json "github.com/goccy/go-json"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	systemtest "github.com/TencentBlueKing/bk-cli/cmd/system/testutil"
)

var _ = Describe("cmdb list_biz_hosts_all", func() {
	var tmpDir string

	BeforeEach(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "bk-cli-cmdb-list-biz-hosts-all-*")
		Expect(err).NotTo(HaveOccurred())
		Expect(os.Setenv("BK_CLI_CONFIG_DIR", tmpDir)).To(Succeed())
	})

	AfterEach(func() {
		Expect(os.Unsetenv("BK_CLI_CONFIG_DIR")).To(Succeed())
		Expect(os.RemoveAll(tmpDir)).To(Succeed())
	})

	It("aggregates hosts across pages", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(r.Body)
			Expect(err).NotTo(HaveOccurred())

			var payload map[string]any
			Expect(json.Unmarshal(body, &payload)).To(Succeed())
			page := payload["page"].(map[string]any)
			start := int(page["start"].(float64))

			w.Header().Set("Content-Type", "application/json")
			switch start {
			case 0:
				_, _ = w.Write([]byte(`{"count":3,"info":[{"bk_host_id":1},{"bk_host_id":2}]}`))
			case 2:
				_, _ = w.Write([]byte(`{"count":3,"info":[{"bk_host_id":3}]}`))
			default:
				Fail("unexpected page start")
			}
		}))
		DeferCleanup(server.Close)

		Expect(systemtest.SetupTestContext(server.URL)).To(Succeed())

		cmd := newListBizHostsAllCmd(systemtest.BuildDeps(false))
		Expect(cmd.Flags().Set("bk_biz_id", "2")).To(Succeed())
		Expect(cmd.Flags().Set("page_limit", "2")).To(Succeed())

		stdout, err := systemtest.CaptureCommandStdout(func() error {
			return cmd.RunE(cmd, nil)
		})
		Expect(err).NotTo(HaveOccurred())

		var env map[string]any
		Expect(json.Unmarshal([]byte(stdout), &env)).To(Succeed())
		data := env["data"].(map[string]any)
		Expect(data["count"]).To(BeNumerically("==", 3))
		Expect(data["info"]).To(Equal([]any{
			map[string]any{"bk_host_id": float64(1)},
			map[string]any{"bk_host_id": float64(2)},
			map[string]any{"bk_host_id": float64(3)},
		}))
	})

	It("deduplicates overlapping hosts across pages", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(r.Body)
			Expect(err).NotTo(HaveOccurred())

			var payload map[string]any
			Expect(json.Unmarshal(body, &payload)).To(Succeed())
			page := payload["page"].(map[string]any)
			start := int(page["start"].(float64))

			w.Header().Set("Content-Type", "application/json")
			switch start {
			case 0:
				_, _ = w.Write([]byte(`{"count":4,"info":[{"bk_host_id":1},{"bk_host_id":2}]}`))
			case 2:
				_, _ = w.Write([]byte(`{"count":4,"info":[{"bk_host_id":2},{"bk_host_id":3}]}`))
			case 4:
				_, _ = w.Write([]byte(`{"count":4,"info":[]}`))
			default:
				Fail("unexpected page start")
			}
		}))
		DeferCleanup(server.Close)

		Expect(systemtest.SetupTestContext(server.URL)).To(Succeed())

		cmd := newListBizHostsAllCmd(systemtest.BuildDeps(false))
		Expect(cmd.Flags().Set("bk_biz_id", "2")).To(Succeed())
		Expect(cmd.Flags().Set("page_limit", "2")).To(Succeed())

		stdout, err := systemtest.CaptureCommandStdout(func() error {
			return cmd.RunE(cmd, nil)
		})
		Expect(err).NotTo(HaveOccurred())

		var env map[string]any
		Expect(json.Unmarshal([]byte(stdout), &env)).To(Succeed())
		data := env["data"].(map[string]any)
		Expect(data["count"]).To(BeNumerically("==", 3))
		Expect(data["info"]).To(Equal([]any{
			map[string]any{"bk_host_id": float64(1)},
			map[string]any{"bk_host_id": float64(2)},
			map[string]any{"bk_host_id": float64(3)},
		}))
	})

	It("fails when a page adds no new hosts", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(r.Body)
			Expect(err).NotTo(HaveOccurred())

			var payload map[string]any
			Expect(json.Unmarshal(body, &payload)).To(Succeed())
			page := payload["page"].(map[string]any)
			start := int(page["start"].(float64))

			w.Header().Set("Content-Type", "application/json")
			switch start {
			case 0:
				_, _ = w.Write([]byte(`{"count":4,"info":[{"bk_host_id":1},{"bk_host_id":2}]}`))
			case 2:
				_, _ = w.Write([]byte(`{"count":4,"info":[{"bk_host_id":1},{"bk_host_id":2}]}`))
			default:
				Fail("unexpected page start")
			}
		}))
		DeferCleanup(server.Close)

		Expect(systemtest.SetupTestContext(server.URL)).To(Succeed())

		cmd := newListBizHostsAllCmd(systemtest.BuildDeps(false))
		Expect(cmd.Flags().Set("bk_biz_id", "2")).To(Succeed())
		Expect(cmd.Flags().Set("page_limit", "2")).To(Succeed())

		_, err := systemtest.CaptureCommandStdout(func() error {
			return cmd.RunE(cmd, nil)
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("received a page with no new hosts"))
	})

	It("shows the initial paginated request and pagination metadata in dry-run", func() {
		Expect(systemtest.SetupTestContext("https://bkapi.example.com/api")).To(Succeed())

		cmd := newListBizHostsAllCmd(systemtest.BuildDeps(true))
		Expect(cmd.Flags().Set("bk_biz_id", "2")).To(Succeed())
		Expect(cmd.Flags().Set("page_limit", "200")).To(Succeed())

		stdout, err := systemtest.CaptureCommandStdout(func() error {
			return cmd.RunE(cmd, nil)
		})
		Expect(err).NotTo(HaveOccurred())

		var env map[string]any
		Expect(json.Unmarshal([]byte(stdout), &env)).To(Succeed())
		Expect(env["dry_run"]).To(BeTrue())

		request := env["request"].(map[string]any)
		Expect(request["url"]).To(Equal(
			"https://bkapi.example.com/api/bk-cmdb/prod/api/v3/open/hosts/app/2/list_hosts",
		))

		body := request["body"].(map[string]any)
		page := body["page"].(map[string]any)
		Expect(page["start"]).To(BeNumerically("==", 0))
		Expect(page["limit"]).To(BeNumerically("==", 200))

		data := env["data"].(map[string]any)
		pagination := data["pagination"].(map[string]any)
		Expect(pagination["aggregates_all_pages"]).To(BeTrue())
		Expect(pagination["page_limit"]).To(BeNumerically("==", 200))
	})

	It("parses the paged response payload", func() {
		count, info, err := parsePagedInfoResponse("list_biz_hosts_all", map[string]any{
			"count": float64(2),
			"info": []any{
				map[string]any{"bk_host_id": float64(1)},
				map[string]any{"bk_host_id": float64(2)},
			},
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(count).To(Equal(2))
		Expect(info).To(Equal([]any{
			map[string]any{"bk_host_id": float64(1)},
			map[string]any{"bk_host_id": float64(2)},
		}))
	})

	It("rejects a paged response without numeric count", func() {
		_, _, err := parsePagedInfoResponse("list_biz_hosts_all", map[string]any{
			"count": "2",
			"info":  []any{},
		})

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("missing numeric count"))
	})
})
