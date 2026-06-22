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

package system_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"

	"github.com/TencentBlueKing/bk-cli/internal/system"
)

var _ = Describe("BuildActionInputSpec", func() {
	It("builds generated flags for path and query params", func() {
		action := &system.Action{
			Name: "list_resources",
			Params: []system.Param{
				{Name: "gateway_name", In: "path", Type: "string", Required: true},
				{Name: "keyword", In: "query", Type: "string"},
			},
		}

		spec, err := system.BuildActionInputSpec(action)
		Expect(err).NotTo(HaveOccurred())
		Expect(spec.GeneratedFlags).To(HaveLen(2))
		Expect(spec.GeneratedFlags[0].FlagName).To(Equal("gateway_name"))
		Expect(spec.GeneratedFlags[1].FlagName).To(Equal("keyword"))
	})

	It("rejects duplicate path params", func() {
		action := &system.Action{
			Name: "broken",
			Params: []system.Param{
				{Name: "id", In: "path", Type: "string"},
				{Name: "id", In: "path", Type: "string"},
			},
		}

		_, err := system.BuildActionInputSpec(action)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("duplicate path param \"id\""))
	})

	It("rejects duplicate query params", func() {
		action := &system.Action{
			Name: "broken",
			Params: []system.Param{
				{Name: "id", In: "query", Type: "string"},
				{Name: "id", In: "query", Type: "string"},
			},
		}

		_, err := system.BuildActionInputSpec(action)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("duplicate query param \"id\""))
	})

	It("rejects path and query name conflicts", func() {
		action := &system.Action{
			Name: "broken",
			Params: []system.Param{
				{Name: "id", In: "path", Type: "string"},
				{Name: "id", In: "query", Type: "string"},
			},
		}

		_, err := system.BuildActionInputSpec(action)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("between path and query"))
	})

	It("rejects built-in flag name conflicts", func() {
		action := &system.Action{
			Name:   "broken",
			Params: []system.Param{{Name: "body", In: "query", Type: "string"}},
		}

		_, err := system.BuildActionInputSpec(action)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("collide with built-in CLI flag --body"))
	})

	It("rejects insecure flag name conflicts", func() {
		action := &system.Action{
			Name:   "broken",
			Params: []system.Param{{Name: "insecure", In: "query", Type: "bool"}},
		}

		_, err := system.BuildActionInputSpec(action)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("collide with built-in CLI flag --insecure"))
	})

	It("rejects body schema help modifier flag conflicts", func() {
		action := &system.Action{
			Name:   "broken",
			Params: []system.Param{{Name: "body-schema", In: "query", Type: "string"}},
		}

		_, err := system.BuildActionInputSpec(action)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("collide with built-in CLI flag --body-schema"))
	})

	It("keeps header params as help-only inputs", func() {
		action := &system.Action{
			Name: "documented_headers",
			Params: []system.Param{
				{Name: "X-Request-Id", In: "header", Type: "string"},
				{Name: "keyword", In: "query", Type: "string"},
			},
		}

		spec, err := system.BuildActionInputSpec(action)
		Expect(err).NotTo(HaveOccurred())
		Expect(spec.GeneratedFlags).To(HaveLen(1))
		Expect(spec.GeneratedFlags[0].FlagName).To(Equal("keyword"))
		Expect(spec.HeaderParams).To(HaveLen(1))
		Expect(spec.HeaderParams[0].Name).To(Equal("X-Request-Id"))
	})

	It("rejects unsupported generated param locations", func() {
		action := &system.Action{
			Name:   "broken",
			Params: []system.Param{{Name: "payload", In: "body", Type: "string"}},
		}

		_, err := system.BuildActionInputSpec(action)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("only path and query params can generate CLI flags"))
	})

	It("rejects invalid int default values during input spec build", func() {
		action := &system.Action{
			Name: "broken",
			Params: []system.Param{
				{Name: "limit", In: "query", Type: "int", Default: "abc"},
			},
		}

		_, err := system.BuildActionInputSpec(action)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(`param "limit"`))
		Expect(err.Error()).To(ContainSubstring(`invalid int default value "abc"`))
	})
})

var _ = Describe("RegisterActionFlags", func() {
	It("registers string flags with default values", func() {
		cmd := &cobra.Command{Use: "test"}
		spec := &system.ActionInputSpec{
			GeneratedFlags: []system.ActionFlag{
				{
					Param: system.Param{
						Name:        "keyword",
						Type:        "string",
						Default:     "hello",
						Description: "Search keyword",
					},
					FlagName: "keyword",
				},
			},
		}

		system.RegisterActionFlags(cmd, spec)

		f := cmd.Flag("keyword")
		Expect(f).NotTo(BeNil())
		Expect(f.DefValue).To(Equal("hello"))
		Expect(f.Usage).To(Equal("Search keyword"))
	})

	It("registers bool flags", func() {
		cmd := &cobra.Command{Use: "test"}
		spec := &system.ActionInputSpec{
			GeneratedFlags: []system.ActionFlag{
				{Param: system.Param{Name: "fuzzy", Type: "bool", Default: "true"}, FlagName: "fuzzy"},
				{Param: system.Param{Name: "exact", Type: "bool", Default: ""}, FlagName: "exact"},
			},
		}

		system.RegisterActionFlags(cmd, spec)

		Expect(cmd.Flag("fuzzy")).NotTo(BeNil())
		Expect(cmd.Flag("fuzzy").DefValue).To(Equal("true"))
		Expect(cmd.Flag("exact")).NotTo(BeNil())
		Expect(cmd.Flag("exact").DefValue).To(Equal("false"))
	})

	It("registers int flags with default values", func() {
		cmd := &cobra.Command{Use: "test"}
		spec := &system.ActionInputSpec{
			GeneratedFlags: []system.ActionFlag{
				{Param: system.Param{Name: "limit", Type: "int", Default: "500"}, FlagName: "limit"},
				{Param: system.Param{Name: "offset", Type: "int", Default: ""}, FlagName: "offset"},
			},
		}

		system.RegisterActionFlags(cmd, spec)

		Expect(cmd.Flag("limit")).NotTo(BeNil())
		Expect(cmd.Flag("limit").DefValue).To(Equal("500"))
		Expect(cmd.Flag("offset")).NotTo(BeNil())
		Expect(cmd.Flag("offset").DefValue).To(Equal("0"))
	})

	It("marks required flags", func() {
		cmd := &cobra.Command{Use: "test"}
		spec := &system.ActionInputSpec{
			GeneratedFlags: []system.ActionFlag{
				{Param: system.Param{Name: "id", Type: "string", Required: true}, FlagName: "id"},
				{Param: system.Param{Name: "name", Type: "string", Required: false}, FlagName: "name"},
			},
		}

		system.RegisterActionFlags(cmd, spec)

		idAnnotations := cmd.Flag("id").Annotations
		Expect(idAnnotations).To(HaveKey(cobra.BashCompOneRequiredFlag))
		nameAnnotations := cmd.Flag("name").Annotations
		Expect(nameAnnotations).To(BeEmpty())
	})

	It("registers mixed types in one spec", func() {
		cmd := &cobra.Command{Use: "test"}
		spec := &system.ActionInputSpec{
			GeneratedFlags: []system.ActionFlag{
				{
					Param:    system.Param{Name: "gateway_name", Type: "string", Required: true},
					FlagName: "gateway_name",
				},
				{Param: system.Param{Name: "fuzzy", Type: "bool"}, FlagName: "fuzzy"},
				{Param: system.Param{Name: "limit", Type: "int", Default: "10"}, FlagName: "limit"},
			},
		}

		system.RegisterActionFlags(cmd, spec)

		Expect(cmd.Flag("gateway_name")).NotTo(BeNil())
		Expect(cmd.Flag("fuzzy")).NotTo(BeNil())
		Expect(cmd.Flag("limit")).NotTo(BeNil())
		Expect(cmd.Flag("limit").DefValue).To(Equal("10"))
	})

	It("handles empty input spec without panic", func() {
		cmd := &cobra.Command{Use: "test"}
		spec := &system.ActionInputSpec{}

		system.RegisterActionFlags(cmd, spec)

		Expect(cmd.Flags().NFlag()).To(Equal(0))
	})
})
