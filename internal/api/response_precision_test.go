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

package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/TencentBlueKing/bk-cli/internal/api"
	"github.com/TencentBlueKing/bk-cli/internal/output"
)

var _ = Describe("response number precision", func() {
	DescribeTable(
		"preserves nested and top-level JSON numbers through envelope output",
		func(status int, raw string) {
			resp := &http.Response{
				StatusCode: status,
				Body:       io.NopCloser(strings.NewReader(raw)),
			}
			data, err := api.ParseResponse(resp)
			Expect(err).NotTo(HaveOccurred())
			env := output.APIResponse(status, nil, data)
			Expect(env.OK).To(Equal(status >= 200 && status < 300))
			var out bytes.Buffer
			Expect(env.WriteJSON(&out)).To(Succeed())
			var emitted map[string]json.RawMessage
			Expect(json.Unmarshal(out.Bytes(), &emitted)).To(Succeed())
			var compact bytes.Buffer
			Expect(json.Compact(&compact, emitted["data"])).To(Succeed())
			Expect(compact.String()).To(Equal(raw))
		},
		Entry(
			"nested success",
			200,
			`{"id":9007199254740993,"nested":{"values":[18446744073709551615,-9223372036854775809,0.123456789012345678901,1.234567890123456789e+100]}}`,
		),
		Entry("nested error", 502, `{"error":{"id":18446744073709551615,"values":[9007199254740993]}}`),
		Entry("top-level integer", 200, `18446744073709551615`),
		Entry(
			"numeric strings and escaped quotes",
			200,
			`{"9007199254740993":"18446744073709551615","id":9007199254740993,"s":"\\\"9007199254740993"}`,
		),
		Entry("fast path escaped strings", 200, `{"id":42,"s":"\\\"9007199254740993"}`),
		Entry("mixed numeric types", 200, `[9007199254740993,42,0.1,2.5,0.00123,9007199254740992]`),
		Entry("top-level array", 200, `[9007199254740993,-9223372036854775809,1.25]`),
	)

	DescribeTable("keeps malformed and trailing response bodies as text",
		func(raw string) {
			value, err := api.ParseResponse(&http.Response{Body: io.NopCloser(strings.NewReader(raw))})
			Expect(err).NotTo(HaveOccurred())
			Expect(value).To(Equal(raw))
		},
		Entry("two objects", `{"id":1}{"id":2}`),
		Entry("trailing text", `{"id":9007199254740993} broken`),
		Entry("invalid number", `{"id":01}`),
		Entry("negative leading zero", `{"id":-01}`),
		Entry("missing fraction", `1.`),
		Entry("missing exponent", `1e`),
		Entry("missing exponent digits", `1e+`),
		Entry("missing integer", `-.1`),
		Entry("sign only", `-`),
		Entry("double sign", `1-2`),
		Entry("unterminated string", `{"s":"9007199254740993}`),
		Entry("invalid escape", `{"s":"\q"}`),
	)
	DescribeTable("keeps ordinary numeric types and preserves unsafe numbers", func(raw string, expected any) {
		value, err := api.ParseResponse(&http.Response{Body: io.NopCloser(strings.NewReader(raw))})
		Expect(err).NotTo(HaveOccurred())
		Expect(value).To(Equal(expected))
	},
		Entry("small integer", "42", float64(42)),
		Entry("safe upper boundary", "9007199254740991", float64(9007199254740991)),
		Entry("safe lower boundary", "-9007199254740991", float64(-9007199254740991)),
		Entry("exact large upper integer", "9007199254740992", float64(9007199254740992)),
		Entry("exact large lower integer", "-9007199254740992", float64(-9007199254740992)),
		Entry("rounded large integer", "9007199254740993", json.Number("9007199254740993")),
		Entry("exact large even integer", "9007199254740994", float64(9007199254740994)),
		Entry("exact large exponent", "9.007199254740992e15", float64(9007199254740992)),
		Entry("equivalent decimal spellings", "0.0012300e+2", float64(0.123)),
		Entry("equivalent exponent padding", "1.2300E+0002", float64(123)),
		Entry("negative zero", "-0.0e+1000000000000000000000", math.Copysign(0, -1)),
		Entry("huge exponent", "1e10000000000000000000000", json.Number("1e10000000000000000000000")),
		Entry("exact float minimum", "5e-324", float64(5e-324)),
		Entry("rounded float minimum", "4e-324", json.Number("4e-324")),
		Entry("decimal integer", "2.0", float64(2)),
		Entry("exponent integer", "2e0", float64(2)),
		Entry("short decimal boundary", "0.1234567890123", float64(0.1234567890123)),
		Entry("short tiny decimal", "0.0000000000001", float64(1e-13)),
		Entry("ordinary decimal", "0.1", float64(0.1)),
		Entry("ordinary exponent", "1e-7", float64(1e-7)),
		Entry("decimal precision", "0.10000000000000001", json.Number("0.10000000000000001")),
		Entry("large exponent integer", "9.007199254740993e15", json.Number("9.007199254740993e15")),
		Entry("float overflow", "1e1000", json.Number("1e1000")),
		Entry("float underflow", "1e-1000000", json.Number("1e-1000000")),
		Entry("zero exponent", "0e-1000000", float64(0)),
	)
})
