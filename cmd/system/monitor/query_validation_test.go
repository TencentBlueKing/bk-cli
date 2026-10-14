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
	"encoding/json"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("monitor query validation", func() {
	DescribeTable("parses explicit time values", func(raw string, expected int64, valid bool) {
		value, err := timestamp(raw)
		if valid {
			Expect(err).NotTo(HaveOccurred())
			Expect(value).To(Equal(expected))
		} else {
			Expect(err).To(MatchError("time must be Unix seconds or RFC3339"))
		}
	},
		Entry("Unix seconds", "1700000000", int64(1700000000), true),
		Entry("RFC3339 timezone", "2023-11-15T06:13:20+08:00", int64(1700000000), true),
		Entry("zero", "0", int64(0), false),
		Entry("negative", "-1", int64(0), false),
		Entry("milliseconds", "1700000000000", int64(0), false),
		Entry("invalid date", "2023-02-30T00:00:00Z", int64(0), false),
		Entry("before epoch", "1969-12-31T23:59:59Z", int64(0), false),
		Entry("overflow", "999999999999999999999", int64(0), false),
	)
	DescribeTable(
		"validates time ranges against a fixed clock",
		func(o options, start, end, errorText string) {
			actualStart, actualEnd, err := timeRange(o, time.Unix(1700003600, 0))
			if errorText != "" {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(errorText))
				return
			}
			Expect(err).NotTo(HaveOccurred())
			Expect(actualStart).To(Equal(start))
			Expect(actualEnd).To(Equal(end))
		},
		Entry("default clock", options{since: "1h"}, "1700000000", "1700003600", ""),
		Entry("explicit end", options{since: "30m", end: "1700001800"}, "1700000000", "1700001800", ""),
		Entry(
			"explicit start",
			options{start: "2023-11-14T22:13:20Z", end: "1700003600", since: "ignored"},
			"1700000000",
			"1700003600",
			"",
		),
		Entry("invalid end", options{since: "1h", end: "bad"}, "", "", "time must"),
		Entry("invalid start", options{start: "bad"}, "", "", "time must"),
		Entry("invalid duration", options{since: "bad"}, "", "", "invalid duration"),
		Entry("subsecond", options{since: "999ms"}, "", "", "at least 1s"),
		Entry("negative duration", options{since: "-1h"}, "", "", "at least 1s"),
		Entry("zero duration", options{since: "0s"}, "", "", "at least 1s"),
		Entry("nonpositive computed start", options{since: "2h", end: "60"}, "", "", "start must be positive"),
		Entry("equal endpoints", options{start: "1700003600"}, "", "", "before end"),
		Entry("reversed endpoints", options{start: "1700003601"}, "", "", "before end"),
	)
	DescribeTable(
		"validates namespace conditions from JSON",
		func(conditions, errorText string) {
			body, err := decodeObject(
				strings.NewReader(fmt.Sprintf(`{"query_list":[{"conditions":%s}]}`, conditions)),
			)
			Expect(err).NotTo(HaveOccurred())
			err = scopeMetrics(body, "dev-ns")
			if errorText != "" {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(errorText))
				return
			}
			Expect(err).NotTo(HaveOccurred())
			q := body["query_list"].([]any)[0].(map[string]any)
			c := q["conditions"].(map[string]any)
			fields := c["field_list"].([]any)
			namespaces := 0
			for _, raw := range fields {
				f := raw.(map[string]any)
				if f["field_name"] == "namespace" {
					namespaces++
					encoded, err := json.Marshal(f["value"])
					Expect(err).NotTo(HaveOccurred())
					Expect(string(encoded)).To(Equal(`["dev-ns"]`))
				}
			}
			Expect(namespaces).To(Equal(1))
			Expect(c["condition_list"].([]any)).To(HaveLen(len(fields) - 1))
		},
		Entry("empty", `{}`, ""),
		Entry(
			"existing namespace",
			`{"field_list":[{"field_name":"namespace","op":"eq","value":["dev-ns"]}]}`,
			"",
		),
		Entry(
			"additional AND field",
			`{"field_list":[{"field_name":"pod","value":["api"]},{"field_name":"namespace","op":"eq","value":["dev-ns"]}],"condition_list":["and"]}`,
			"",
		),
		Entry("conditions not object", `[]`, "conditions must be an object"),
		Entry("null conditions", `null`, "conditions must be an object"),
		Entry("fields not array", `{"field_list":{}}`, "field_list must be an array"),
		Entry("relations not array", `{"condition_list":"and"}`, "condition_list must be an array"),
		Entry("relation arity", `{"field_list":[{},{}],"condition_list":[]}`, "condition_list length"),
		Entry("OR", `{"field_list":[{},{}],"condition_list":["or"]}`, "AND-only"),
		Entry("non-object field", `{"field_list":[1]}`, "condition must be an object"),
		Entry(
			"namespace values not array",
			`{"field_list":[{"field_name":"namespace","op":"eq","value":"dev-ns"}]}`,
			"namespace condition conflicts",
		),
		Entry(
			"empty namespace values",
			`{"field_list":[{"field_name":"namespace","op":"eq","value":[]}]}`,
			"namespace condition conflicts",
		),
		Entry(
			"multiple namespaces",
			`{"field_list":[{"field_name":"namespace","op":"eq","value":["dev-ns","prod"]}]}`,
			"namespace condition conflicts",
		),
		Entry(
			"other namespace",
			`{"field_list":[{"field_name":"namespace","op":"eq","value":["prod"]}]}`,
			"namespace condition conflicts",
		),
		Entry(
			"non-equality namespace",
			`{"field_list":[{"field_name":"namespace","op":"ne","value":["dev-ns"]}]}`,
			"namespace condition conflicts",
		),
	)
	It("leaves explicitly unscoped metrics unchanged and rejects invalid query containers", func() {
		body := map[string]any{"query_list": "unchanged"}
		Expect(scopeMetrics(body, "")).To(Succeed())
		Expect(body["query_list"]).To(Equal("unchanged"))
		Expect(scopeMetrics(body, "dev-ns")).To(MatchError("query_list must be an array"))
		body["query_list"] = []any{1}
		Expect(scopeMetrics(body, "dev-ns")).To(MatchError("query_list entries must be objects"))
	})
})
