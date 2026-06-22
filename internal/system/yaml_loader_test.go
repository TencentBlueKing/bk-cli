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
	"testing/fstest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/TencentBlueKing/bk-cli/internal/system"
)

var _ = Describe("LoadFromYAML", func() {
	Context("valid YAML", func() {
		It("parses a complete system definition with in fields", func() {
			data := []byte(`
name: test-system
gateway_name: bk-test
description: "Test system"
actions:
  - name: do_something
    description: "Do something"
    method: GET
    path: "/api/v1/things/{id}/"
    authConfig:
      appVerifiedRequired: true
      userVerifiedRequired: false
      resourcePermissionRequired: false
    params:
      - name: id
        in: path
        type: string
        description: "Thing ID"
        required: true
      - name: verbose
        in: query
        type: bool
        description: "Verbose output"
        required: false
        default: "false"
    examples:
      - "bk-cli test-system do_something --id=123"
`)
			sys, err := system.LoadFromYAML(data)
			Expect(err).NotTo(HaveOccurred())
			Expect(sys.Name).To(Equal("test-system"))
			Expect(sys.GatewayName).To(Equal("bk-test"))
			Expect(sys.Description).To(Equal("Test system"))
			Expect(sys.Actions).To(HaveLen(1))

			action := sys.Actions[0]
			Expect(action.Name).To(Equal("do_something"))
			Expect(action.Method).To(Equal("GET"))
			Expect(action.Path).To(Equal("/api/v1/things/{id}/"))
			Expect(action.AuthConfig).NotTo(BeNil())
			Expect(action.AuthConfig.AppVerifiedRequired).To(BeTrue())
			Expect(action.AuthConfig.UserVerifiedRequired).To(BeFalse())
			Expect(action.AuthConfig.ResourcePermissionRequired).To(BeFalse())
			Expect(action.Params).To(HaveLen(2))
			Expect(action.Params[0].Name).To(Equal("id"))
			Expect(action.Params[0].In).To(Equal("path"))
			Expect(action.Params[0].Required).To(BeTrue())
			Expect(action.Params[1].Type).To(Equal("bool"))
			Expect(action.Params[1].In).To(Equal("query"))
			Expect(action.Params[1].Default).To(Equal("false"))
			Expect(action.Examples).To(HaveLen(1))
		})

		It("parses a minimal system definition", func() {
			data := []byte(`
name: minimal
gateway_name: bk-minimal
actions: []
`)
			sys, err := system.LoadFromYAML(data)
			Expect(err).NotTo(HaveOccurred())
			Expect(sys.Name).To(Equal("minimal"))
			Expect(sys.GatewayName).To(Equal("bk-minimal"))
			Expect(sys.Actions).To(BeEmpty())
		})

		It("accepts path, query, and header params", func() {
			data := []byte(`
name: test-query
gateway_name: bk-test
actions:
  - name: do_thing
    description: "Do thing with query"
    method: GET
    path: "/api/v1/things/"
    authConfig:
      appVerifiedRequired: true
      userVerifiedRequired: false
      resourcePermissionRequired: false
    params:
      - name: keyword
        in: query
        type: string
        description: "Search keyword"
        required: false
      - name: X-Request-Id
        in: header
        type: string
        description: "Request ID"
        required: false
`)
			sys, err := system.LoadFromYAML(data)
			Expect(err).NotTo(HaveOccurred())
			Expect(sys.Actions[0].Params).To(HaveLen(2))
			Expect(sys.Actions[0].Params[0].In).To(Equal("query"))
			Expect(sys.Actions[0].Params[1].In).To(Equal("header"))
		})

		It("parses an action timeout override", func() {
			data := []byte(`
name: test-timeout
gateway_name: bk-test
actions:
  - name: slow_call
    method: GET
    path: "/api/v1/slow/"
    timeout: 180s
    authConfig:
      appVerifiedRequired: true
      userVerifiedRequired: false
      resourcePermissionRequired: false
`)
			sys, err := system.LoadFromYAML(data)
			Expect(err).NotTo(HaveOccurred())
			Expect(sys.Actions).To(HaveLen(1))
			Expect(sys.Actions[0].Timeout).To(Equal("180s"))
		})

		It("parses optional body schema metadata", func() {
			data := []byte(`
name: test-body
gateway_name: bk-test
actions:
  - name: update_thing
    description: "Update thing"
    method: PUT
    path: "/api/v1/things/{id}/"
    body_schema: |
      {
        "type": "object",
        "properties": {
          "name": {"type": "string"}
        }
      }
    body_required: true
    authConfig:
      appVerifiedRequired: true
      userVerifiedRequired: false
      resourcePermissionRequired: false
    params:
      - name: id
        in: path
        type: string
        required: true
`)
			sys, err := system.LoadFromYAML(data)
			Expect(err).NotTo(HaveOccurred())
			Expect(sys.Actions).To(HaveLen(1))
			Expect(sys.Actions[0].BodySchema).To(ContainSubstring(`"name": {"type": "string"}`))
			Expect(sys.Actions[0].BodyRequired).To(BeTrue())
		})
	})

	Context("invalid YAML", func() {
		It("returns error for malformed YAML", func() {
			data := []byte(`{{{invalid yaml`)
			_, err := system.LoadFromYAML(data)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse YAML"))
		})

		It("returns error for missing name", func() {
			data := []byte(`
gateway_name: bk-test
description: "No name"
`)
			_, err := system.LoadFromYAML(data)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("system name is required"))
		})

		It("returns error for missing gateway_name", func() {
			data := []byte(`
name: test-system
description: "No gateway name"
`)
			_, err := system.LoadFromYAML(data)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("gateway_name is required"))
		})

		It("returns error for invalid gateway_name", func() {
			data := []byte(`
name: test-system
gateway_name: bad_gateway
description: "Bad gateway name"
`)
			_, err := system.LoadFromYAML(data)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("gateway_name"))
		})

		It("returns error for missing in field", func() {
			data := []byte(`
name: test-system
gateway_name: bk-test
actions:
  - name: do_thing
    method: GET
    path: "/api/v1/"
    params:
      - name: keyword
        type: string
`)
			_, err := system.LoadFromYAML(data)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("\"in\" is required"))
		})

		It("returns error for body params in YAML actions", func() {
			data := []byte(`
name: test-system
gateway_name: bk-test
actions:
  - name: do_thing
    method: GET
    path: "/api/v1/"
    params:
      - name: keyword
        in: body
        type: string
`)
			_, err := system.LoadFromYAML(data)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unsupported \"in\" value"))
			Expect(err.Error()).To(ContainSubstring("body"))
		})

		It("returns error for path param without matching placeholder", func() {
			data := []byte(`
name: test-system
gateway_name: bk-test
actions:
  - name: do_thing
    method: GET
    path: "/api/v1/things/"
    params:
      - name: id
        in: path
        type: string
        required: true
`)
			_, err := system.LoadFromYAML(data)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no {id} placeholder"))
		})

		It("returns error for actions missing authConfig", func() {
			data := []byte(`
name: test-system
gateway_name: bk-test
actions:
  - name: no_auth_config
    method: GET
    path: "/api/v1/things/"
`)
			_, err := system.LoadFromYAML(data)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(`system "test-system" action "no_auth_config"`))
			Expect(err.Error()).To(ContainSubstring("authConfig is required"))
		})

		It("returns error when resource permission auth is enabled without app auth", func() {
			data := []byte(`
name: test-system
gateway_name: bk-test
actions:
  - name: bad_auth_config
    method: GET
    path: "/api/v1/things/"
    authConfig:
      appVerifiedRequired: false
      userVerifiedRequired: true
      resourcePermissionRequired: true
`)
			_, err := system.LoadFromYAML(data)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(`system "test-system" action "bad_auth_config"`))
			Expect(
				err.Error(),
			).To(
				ContainSubstring("resourcePermissionRequired requires appVerifiedRequired=true"),
			)
		})
	})
})

var _ = Describe("LoadSystemFromFS", func() {
	It("loads a single system definition from a named YAML file", func() {
		yamlFS := fstest.MapFS{
			"actions/demo.yaml": {
				Data: []byte(`
name: demo
gateway_name: bk-demo
description: "Demo service"
actions:
  - name: list_items
    method: GET
    path: "/api/v1/items/"
    authConfig:
      appVerifiedRequired: true
      userVerifiedRequired: false
      resourcePermissionRequired: false
`),
			},
		}

		sys, err := system.LoadSystemFromFS(yamlFS, "actions/demo.yaml")
		Expect(err).NotTo(HaveOccurred())
		Expect(sys.Name).To(Equal("demo"))
		Expect(sys.GatewayName).To(Equal("bk-demo"))
		Expect(sys.Actions).To(HaveLen(1))
		Expect(sys.Actions[0].Name).To(Equal("list_items"))
	})
})
