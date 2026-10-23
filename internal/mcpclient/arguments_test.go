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

package mcpclient

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("SECRET") }

var _ = Describe("MCP arguments", func() {
	It("accepts the exact input limit and rejects one byte more", func() {
		body := "{}" + strings.Repeat(" ", MaxInputBytes-2)
		_, err := ReadArguments(body, nil)
		Expect(err).NotTo(HaveOccurred())
		_, err = ReadArguments(body+" ", nil)
		Expect(err).To(HaveOccurred())
	})
	It("leaves root-level composite schemas on the body path", func() {
		schema := map[string]any{
			"oneOf":      []any{},
			"properties": map[string]any{"x": map[string]any{"type": "string"}},
		}
		Expect(Parameters(schema)).To(BeEmpty())
		_, err := BuildArguments(schema, nil, map[string]any{"x": "ok"})
		Expect(err).NotTo(HaveOccurred())
	})
	It("rejects numeric validation beyond its explicit resource bounds", func() {
		schema := map[string]any{"properties": map[string]any{"x": map[string]any{"type": "number"}}}
		for _, value := range []string{strings.Repeat("1", 4097), "1e10001", "1e999999999999999999999999"} {
			_, err := BuildArguments(schema, []NamedArgument{{"x", value}}, nil)
			Expect(err).To(HaveOccurred())
		}
	})
	DescribeTable(
		"compares structured enum values exactly",
		func(value string, valid bool) {
			enum := []any{map[string]any{"n": json.Number("1")}, []any{json.Number("2"), true}, nil}
			schema := map[string]any{"properties": map[string]any{"x": map[string]any{"enum": enum}}}
			body, err := ReadArguments(`{"x":`+value+`}`, nil)
			Expect(err).NotTo(HaveOccurred())
			_, err = BuildArguments(schema, nil, body)
			if valid {
				Expect(err).NotTo(HaveOccurred())
			} else {
				Expect(err).To(HaveOccurred())
			}
		},
		Entry("object", `{"n":1.0}`, true),
		Entry("array", `[2.0,true]`, true),
		Entry("null", `null`, true),
		Entry("wrong key", `{"other":1}`, false),
		Entry("extra key", `{"n":1,"extra":0}`, false),
		Entry("wrong numeric type", `{"n":"1"}`, false),
		Entry("wrong number", `{"n":2}`, false),
		Entry(
			"wrong element",
			`[2,false]`,
			false,
		),
		Entry("wrong length", `[2]`, false),
		Entry("scalar", `false`, false),
	)
	It("rejects invalid UTF-8 instead of replacing data", func() {
		_, err := ReadArguments("{\"value\":\""+string([]byte{0xff})+"\"}", nil)
		Expect(err).To(HaveOccurred())
	})
	It("rejects invalid UTF-8 named strings unless body overrides them", func() {
		schema := map[string]any{"properties": map[string]any{"text": map[string]any{"type": "string"}}}
		named := []NamedArgument{{"text", string([]byte{0xff})}}
		_, err := BuildArguments(schema, named, nil)
		Expect(err).To(MatchError(ContainSubstring("invalid_mcp_arguments")))
		body := map[string]any{"text": "valid"}
		arguments, err := BuildArguments(schema, named, body)
		Expect(err).NotTo(HaveOccurred())
		Expect(arguments).To(Equal(body))
	})
	It("preserves exact number literals and shell-sensitive data across every body source", func() {
		body := `{"big":9007199254740993,"decimal":0.123456789012345678901,"exponent":1.234567890123456789e99,"text":"00123 中文 $(whoami) ` + "`x`" + `\n\"\\","zero":0,"false":false,"empty":""}`
		path := filepath.Join(GinkgoT().TempDir(), "args.json")
		Expect(os.WriteFile(path, []byte(body), 0o600)).To(Succeed())
		for _, source := range []string{body, "@" + path, "-"} {
			args, err := ReadArguments(source, strings.NewReader(body))
			Expect(err).NotTo(HaveOccurred())
			Expect(args["big"]).To(Equal(json.Number("9007199254740993")))
			Expect(args["decimal"]).To(Equal(json.Number("0.123456789012345678901")))
			Expect(args["exponent"]).To(Equal(json.Number("1.234567890123456789e99")))
			encoded, err := json.Marshal(args)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(encoded)).To(ContainSubstring(`"big":9007199254740993`))
		}
	})
	DescribeTable(
		"rejects malformed or ambiguous bodies",
		func(body string) {
			_, err := ReadArguments(body, strings.NewReader(body))
			Expect(err).To(MatchError(ContainSubstring("invalid_mcp_arguments")))
		},
		Entry("empty", ""),
		Entry("array", `[]`),
		Entry("null", `null`),
		Entry("string", `"secret"`),
		Entry("number", `42`),
		Entry(
			"bad JSON",
			`{"x":`,
		),
		Entry("duplicate", `{"x":1,"x":2}`),
		Entry("nested duplicate", `{"x":[{"a":1,"a":2}]}`),
		Entry(
			"trailing JSON",
			`{} {}`,
		),
		Entry("trailing junk", `{} SECRET`),
		Entry("missing file", "@/missing-mcp-arguments-42"),
		Entry("no file path", "@"),
		Entry("deep", strings.Repeat(`{"x":`, 130)+`0`+strings.Repeat(`}`, 130)),
	)
	It("propagates read failure safely and bounds input", func() {
		_, err := ReadArguments("-", failingReader{})
		Expect(err).To(MatchError(ContainSubstring("invalid_mcp_arguments")))
		Expect(err.Error()).NotTo(ContainSubstring("SECRET"))
		_, err = ReadArguments("-", nil)
		Expect(err).To(HaveOccurred())
		_, err = ReadArguments(
			"-",
			io.LimitReader(strings.NewReader(strings.Repeat(" ", MaxInputBytes+1)), MaxInputBytes+1),
		)
		Expect(err).To(HaveOccurred())
	})
	It("maps only explicit fields without injecting defaults", func() {
		schema := map[string]any{"type": "object", "properties": map[string]any{
			"s": map[string]any{"type": "string"}, "i": map[string]any{"type": "integer"},
			"n": map[string]any{"type": "number"}, "b": map[string]any{"type": "boolean"},
			"a": map[string]any{"type": "array"}, "o": map[string]any{"type": "object"},
			"absent": map[string]any{"type": "boolean", "default": true},
		}}
		args, err := BuildArguments(
			schema,
			[]NamedArgument{
				{"s", "00123"},
				{"i", "9007199254740993"},
				{"n", "1.234567890123456789e-12"},
				{"b", "false"},
				{"a", "[1,2]"},
				{"o", `{"zero":0}`},
			},
			nil,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(args).To(HaveLen(6))
		Expect(args["s"]).To(Equal("00123"))
		Expect(args["i"]).To(Equal(json.Number("9007199254740993")))
		Expect(args["n"]).To(Equal(json.Number("1.234567890123456789e-12")))
		Expect(args["b"]).To(BeFalse())
		Expect(args).NotTo(HaveKey("absent"))
	})
	It("uses body as a complete override but still rejects unknown or repeated names", func() {
		schema := map[string]any{
			"type":       "object",
			"required":   []any{"count"},
			"properties": map[string]any{"count": map[string]any{"type": "integer"}},
		}
		body := map[string]any{"count": json.Number("0")}
		args, err := BuildArguments(schema, []NamedArgument{{"count", "invalid overridden value"}}, body)
		Expect(err).NotTo(HaveOccurred())
		Expect(args).To(Equal(body))
		_, err = BuildArguments(schema, []NamedArgument{{"typo", "1"}}, body)
		Expect(err).To(HaveOccurred())
		_, err = BuildArguments(schema, []NamedArgument{{"count", "1"}, {"count", "2"}}, body)
		Expect(err).To(HaveOccurred())
		_, err = BuildArguments(schema, nil, map[string]any{})
		Expect(err).To(HaveOccurred())
	})
	It("keeps reserved and ambiguous fields accessible through body", func() {
		schema := map[string]any{"properties": map[string]any{
			"body":     map[string]any{"type": "string"},
			"list":     map[string]any{"type": "string"},
			"bad name": map[string]any{"type": "string"},
			"union": map[string]any{
				"type": []any{"string", "null"},
			},
			"ref":  map[string]any{"$ref": "https://example.com/schema", "type": "string"},
			"flag": map[string]any{"type": "boolean", "description": "A flag"},
		}, "required": []any{"body", "flag"}}
		fields := Parameters(schema)
		Expect(
			fields,
		).To(
			Equal([]Parameter{{Name: "flag", Type: "boolean", Description: "A flag", Required: true}}),
		)
		body := map[string]any{
			"body":     "value",
			"list":     "value",
			"flag":     false,
			"bad name": "x",
			"union":    nil,
			"ref":      "x",
		}
		_, err := BuildArguments(schema, nil, body)
		Expect(err).NotTo(HaveOccurred())
		_, err = BuildArguments(schema, []NamedArgument{{"body", "value"}}, nil)
		Expect(err).To(HaveOccurred())
	})
	DescribeTable(
		"checks basic types and enums on final values",
		func(kind, value string, valid bool) {
			schema := map[string]any{"properties": map[string]any{"x": map[string]any{"type": kind}}}
			_, err := BuildArguments(schema, []NamedArgument{{"x", value}}, nil)
			if valid {
				Expect(err).NotTo(HaveOccurred())
			} else {
				Expect(err).To(HaveOccurred())
			}
		},
		Entry("zero", "integer", "0", true),
		Entry("integral exponent", "integer", "1e3", true),
		Entry(
			"fraction",
			"integer",
			"1.1",
			false,
		),
		Entry("large negative exponent", "integer", "1e-999999999", false),
		Entry(
			"true",
			"boolean",
			"true",
			true,
		),
		Entry("bool token", "boolean", "yes", false),
		Entry("null", "object", "null", false),
		Entry(
			"wrong collection",
			"array",
			"{}",
			false,
		),
		Entry("number string", "number", `"3"`, false),
		Entry("invalid number", "number", "01", false),
	)
	It("compares enum numbers without float rounding, recursively validates objects and arrays", func() {
		schema := map[string]any{
			"properties": map[string]any{"items": map[string]any{"type": "array", "items": map[string]any{
				"type":     "object",
				"required": []any{"n"},
				"properties": map[string]any{
					"n": map[string]any{
						"type": "number",
						"enum": []any{json.Number("9007199254740993")},
					},
				},
			}}},
		}
		for _, test := range []struct {
			value string
			valid bool
		}{{`[{"n":9007199254740993}]`, true}, {`[{"n":9007199254740993.0}]`, true}, {`[{"n":9007199254740992}]`, false}, {`[{}]`, false}, {`["x"]`, false}} {
			_, err := BuildArguments(schema, []NamedArgument{{"items", test.value}}, nil)
			if test.valid {
				Expect(err).NotTo(HaveOccurred())
			} else {
				Expect(err).To(HaveOccurred())
			}
		}
	})
})
