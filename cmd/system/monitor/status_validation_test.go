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
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("monitor resource summaries", func() {
	DescribeTable(
		"validates rows and vector evidence",
		func(kind, expression, raw string, rows int, errorText string) {
			data, err := decodeObject(strings.NewReader(raw))
			Expect(err).NotTo(HaveOccurred())
			actual, trace, err := probeSummary(data, probe{kind: kind, expression: expression})
			if errorText != "" {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(errorText))
				return
			}
			Expect(err).NotTo(HaveOccurred())
			Expect(actual).To(Equal(rows))
			Expect(trace).To(Equal("query-trace"))
		},
		Entry("raw rows", "logs", "", `{"list":[{},{}],"trace_id":"query-trace"}`, 2, ""),
		Entry("empty trace", "trace", "", `{"list":[],"trace_id":"query-trace"}`, 0, ""),
		Entry("missing list", "logs", "", `{}`, 0, "missing list"),
		Entry("missing series", "metrics", "", `{}`, 0, "missing series"),
		Entry("invalid series", "metrics", "", `{"series":[1]}`, 0, "invalid query series"),
		Entry("missing values", "metrics", "", `{"series":[{}]}`, 0, "missing values"),
		Entry(
			"metrics rows",
			"metrics",
			"",
			`{"series":[{"values":[[1],[2]]}],"trace_id":"query-trace"}`,
			2,
			"",
		),
		Entry("empty metrics", "metrics", "", `{"series":[],"trace_id":"query-trace"}`, 0, ""),
		Entry(
			"vector match",
			"query",
			"vector(1)",
			`{"series":[{"columns":["time","_value"],"values":[[1700000000,1]]}],"trace_id":"query-trace"}`,
			1,
			"",
		),
		Entry("vector no column", "query", "vector(1)", `{"series":[{"values":[[1]]}]}`, 0, "expected value"),
		Entry(
			"vector no rows",
			"query",
			"vector(1)",
			`{"series":[{"columns":["_value"],"values":[]}]}`,
			0,
			"expected value",
		),
		Entry(
			"vector malformed rows",
			"query",
			"vector(1)",
			`{"series":[{"columns":["time","_value"],"values":[[],1,[1,2],[1,"1"]]}]}`,
			0,
			"expected value",
		),
	)
	It("sorts named sources, deduplicates the default and retains optional resource checks", func() {
		p := profile{
			SpaceUID:         "shared",
			LogSpaceUID:      "logs",
			MetricsSpaceUID:  "metrics",
			LogTable:         "a",
			LogTables:        map[string]string{"z": "b", "a": "a"},
			TraceTable:       "traces",
			MetricsNamespace: "dev",
		}
		probes := resourceProbes(p)
		Expect(probes).To(HaveLen(6))
		Expect(probes[0].expression).To(Equal("vector(1)"))
		Expect(probes[1].source).To(Equal("a"))
		Expect(probes[2].source).To(Equal("z"))
		Expect(probes[1].space).To(Equal("logs"))
		Expect(probes[3].kind).To(Equal("trace"))
		Expect(probes[3].space).To(Equal("shared"))
		Expect(probes[4].kind).To(Equal("metrics"))
		Expect(probes[4].space).To(Equal("metrics"))
		Expect(probes[5].expression).To(Equal(`sum(kube_pod_info{namespace="dev"})`))
		p.LogTable = "c"
		Expect(resourceProbes(p)[3].source).To(Equal("default"))
		Expect(resourceProbes(profile{})).To(Equal([]probe{{kind: "query", expression: "vector(1)"}}))
		Expect(environmentNames(profile{}, "")).To(Equal([]string{""}))
	})
	It("accepts numeric vector results independent of their original spelling", func() {
		// queryData re-decodes JSON with UseNumber before summaries consume it.
		data := map[string]any{
			"series": []any{
				map[string]any{"columns": []any{"_value"}, "values": []any{[]any{json.Number("1")}}},
			},
		}
		rows, _, err := probeSummary(data, probe{kind: "query", expression: "vector(1)"})
		Expect(err).NotTo(HaveOccurred())
		Expect(rows).To(Equal(1))
	})
})
