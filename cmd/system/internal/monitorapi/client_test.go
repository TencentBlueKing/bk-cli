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

package monitorapi

import (
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Monitor API decisions", func() {
	It("keeps requires_resource separate from permission denial", func() {
		decision := StateDecision(map[string]any{"state": "requires_resource"})
		Expect(decision.Code).To(Equal("needs_resource"))
		Expect(decision.AnyOf).To(BeEmpty())
	})

	It("lists either permission and drops unsafe apply links", func() {
		decision := StateDecision(map[string]any{
			"state": "missing",
			"legacy_permission": map[string]any{
				"system_id": "bk_monitorv3",
				"action_id": "using_log_mcp",
				"apply_url": "https://iam.invalid/apply",
			},
			"fallback_permission": map[string]any{
				"system_id": "bk_log_search",
				"action_id": "view_business_v2",
				"apply_url": "javascript:alert(1)",
			},
		})
		Expect(decision.Code).To(Equal("permission_missing"))
		Expect(decision.AnyOf).To(HaveLen(2))
		Expect(decision.AnyOf[0]).To(HaveKey("apply_url"))
		Expect(decision.AnyOf[1]).NotTo(HaveKey("apply_url"))
	})

	DescribeTable(
		"classifies server failures without echoing upstream text",
		func(status int, body map[string]any, code string) {
			decision := classify(status, body, "request-1")
			Expect(decision.Code).To(Equal(code))
			Expect(decision.RequestID).To(Equal("request-1"))
			Expect(decision.Message).NotTo(ContainSubstring("SECRET"))
		},
		Entry(
			"metadata block",
			200,
			map[string]any{"detail": []any{"incomplete catalog metadata SECRET"}},
			"resource_blocked",
		),
		Entry(
			"invisible resource",
			200,
			map[string]any{"message": "not uniquely visible SECRET"},
			"resource_not_visible",
		),
		Entry(
			"missing resource",
			200,
			map[string]any{"message": "table does not exist SECRET"},
			"resource_not_found",
		),
		Entry(
			"native missing index",
			200,
			map[string]any{"message": "No LogIndexSet matches the given query. SECRET"},
			"resource_not_found",
		),
		Entry("unpublished API", 404, nil, "api_not_found"),
		Entry(
			"native missing alert",
			200,
			map[string]any{"code": 3324002, "message": "SECRET"},
			"resource_not_found",
		),
		Entry(
			"native missing event",
			404,
			map[string]any{"code": "3324002", "message": "SECRET"},
			"resource_not_found",
		),
		Entry("IAM unavailable", 200, map[string]any{"code": 503, "message": "SECRET"}, "retry_later"),
		Entry(
			"bounded result",
			400,
			map[string]any{"data": map[string]any{"message": "查询结果过大 SECRET"}},
			"response_too_large",
		),
		Entry(
			"structured denial",
			200,
			map[string]any{"code": 403, "detail": map[string]any{"state": "missing"}},
			"permission_missing",
		),
		Entry(
			"schema rejection",
			200,
			map[string]any{"detail": map[string]any{"tool_args": "SECRET"}},
			"invalid_arguments",
		),
		Entry(
			"log extraction policy missing",
			200,
			map[string]any{"message": "code 3626305 SECRET"},
			"extract_policy_missing",
		),
		Entry("unknown business failure", 200, map[string]any{"message": "SECRET"}, "business_error"),
	)

	It("sends only resource fields declared by the permission contract", func() {
		schema := map[string]any{"permission": map[string]any{"fallback_permission": map[string]any{
			"resource_type": "indices", "resource_arg": "index_set_id",
		}}}
		context, unused := ResourceContext(
			schema,
			map[string]any{"index_set_id": 1, "target_type": "index_set", "table_id": "x.y"},
		)
		Expect(context).To(Equal(map[string]any{"index_set_id": 1, "target_type": "index_set"}))
		Expect(unused).To(Equal([]string{"table_id"}))
	})

	It("uses the installed catalog for allowlist and next commands", func() {
		SetCatalog(
			[]string{"get_index_set_fields"},
			func(name string) string { return "bk-cli monitor log " + name },
			func(string, string) []string {
				return []string{"bk-cli monitor log list_index_sets --bk_biz_id 100100"}
			},
		)
		Expect(Supported("get_index_set_fields")).To(BeTrue())
		Expect(Supported("create_dashboard")).To(BeFalse())
		Expect(
			NextCommands("get_index_set_fields", "100100"),
		).To(
			Equal([]string{"bk-cli monitor log list_index_sets --bk_biz_id 100100"}),
		)
		Expect(IsResourceDecision("resource_not_found")).To(BeTrue())
		Expect(IsResourceDecision("permission_missing")).To(BeFalse())
	})

	It("only reports server judgments as check results", func() {
		_, ok := IsJudgment(&Decision{Code: "resource_blocked"})
		Expect(ok).To(BeTrue())
		_, ok = IsJudgment(&Decision{Code: "business_error"})
		Expect(ok).To(BeFalse())
		_, ok = IsJudgment(errors.New("network"))
		Expect(ok).To(BeFalse())
	})

	It("bounds relative windows", func() {
		start, end, err := Window("15m", time.Unix(1800000000, 0))
		Expect(err).NotTo(HaveOccurred())
		Expect(start).To(Equal("1799999100"))
		Expect(end).To(Equal("1800000000"))
		_, _, err = Window("25h", time.Unix(1800000000, 0))
		Expect(err).To(MatchError(ContainSubstring("invalid_since")))
	})
})
