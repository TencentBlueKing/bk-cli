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

package monitor

import (
	"bytes"
	"net/http"
	"net/http/httptest"

	json "github.com/goccy/go-json"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	systemtest "github.com/TencentBlueKing/bk-cli/cmd/system/testutil"
)

// All IDs below are synthetic and are used only by the local HTTP fixtures.
var _ = Describe("Legacy space permission responses", func() {
	DescribeTable(
		"requires an explicit decision for the exact tool, action and business",
		func(change func(map[string]any, map[string]any), valid bool) {
			permission := map[string]any{
				"action_id":     "using_metrics_mcp",
				"resource_type": "space",
				"resource_arg":  "bk_biz_id",
			}
			scope := map[string]any{
				"authorized": true,
				"action_id":  "using_metrics_mcp",
				"resource":   map[string]any{"bk_biz_id": "100100"},
			}
			change(scope, permission)
			result := normalizeLegacyPermissionState(scope, permission, "100100")
			Expect(result["state"] == "granted" || result["state"] == "missing").To(Equal(valid))
			Expect(scope).NotTo(HaveKey("state"))
		},
		Entry("grant", func(_, _ map[string]any) {}, true),
		Entry("denial", func(s, _ map[string]any) { s["authorized"] = false }, true),
		Entry("missing decision", func(s, _ map[string]any) { delete(s, "authorized") }, false),
		Entry("nonboolean decision", func(s, _ map[string]any) { s["authorized"] = "true" }, false),
		Entry(
			"wrong business",
			func(s, _ map[string]any) { s["resource"] = map[string]any{"bk_biz_id": 200200} },
			false,
		),
		Entry("missing business", func(s, _ map[string]any) { delete(s, "resource") }, false),
		Entry("wrong action", func(s, _ map[string]any) { s["action_id"] = "other" }, false),
		Entry("native schema", func(_, p map[string]any) { p["mode"] = "mcp_or_native" }, false),
		Entry("missing schema action", func(_, p map[string]any) { delete(p, "action_id") }, false),
		Entry("resource scope", func(_, p map[string]any) { p["resource_type"] = "indices" }, false),
	)

	DescribeTable("checks the complete preflight without executing SQL",
		func(summary, scopeAuth bool, scopeBiz string, wantReady bool) {
			calls := []string{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				name := facade(r.URL.Path)
				calls = append(calls, name)
				var data any
				switch name {
				case "lookup_tool":
					data = map[string]any{
						"tools": []any{
							map[string]any{
								"name":                  "execute_sql_query",
								"risk":                  "query",
								"requires_confirmation": false,
							},
						},
					}
				case "lookup_tool_schema":
					data = map[string]any{
						"permission": map[string]any{
							"action_id":     "using_metrics_mcp",
							"resource_type": "space",
							"resource_arg":  "bk_biz_id",
						},
					}
				case "lookup_permissions":
					data = map[string]any{
						"authorized": summary,
						"scopes": []any{
							map[string]any{
								"tool_name":  "execute_sql_query",
								"action_id":  "using_metrics_mcp",
								"authorized": scopeAuth,
								"resource":   map[string]any{"bk_biz_id": scopeBiz},
							},
						},
					}
				default:
					Fail("preflight must not execute a business request")
				}
				Expect(
					json.NewEncoder(w).Encode(map[string]any{"result": true, "data": data}),
				).To(
					Succeed(),
				)
			}))
			DeferCleanup(server.Close)
			Expect(systemtest.SetupTestContext(server.URL)).To(Succeed())
			cmd := newPermissionCheckCmd(systemtest.BuildDeps(false))
			setFlags(cmd, "bk_biz_id", "100100", "tool", "execute_sql_query")
			cmd.SetErr(&bytes.Buffer{})
			_, err := systemtest.CaptureCommandStdout(func() error { return cmd.RunE(cmd, nil) })
			if wantReady {
				Expect(err).NotTo(HaveOccurred())
			} else {
				Expect(err).To(HaveOccurred())
			}
			Expect(calls).To(Equal([]string{"lookup_tool", "lookup_tool_schema", "lookup_permissions"}))
		},
		Entry("consistent grant", true, true, "100100", true),
		Entry("consistent denial", false, false, "100100", false),
		Entry("conflicting grant", true, false, "100100", false),
		Entry("conflicting denial", false, true, "100100", false),
		Entry("wrong business", true, true, "200200", false),
	)
})
