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

package system

import (
	"bytes"
	"io"
	"io/fs"
	"os"
	"strings"
	"testing/fstest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"

	"github.com/TencentBlueKing/bk-cli/internal/output"
	syslib "github.com/TencentBlueKing/bk-cli/internal/system"
	systemcmd "github.com/TencentBlueKing/bk-cli/internal/systemcmd"
)

func testBuildDeps(warnWriter *bytes.Buffer) systemcmd.BuildDeps {
	var writer io.Writer
	if warnWriter != nil {
		writer = warnWriter
	}
	return systemcmd.BuildDeps{
		GetContext: func() string { return "" },
		IsDryRun:   func() bool { return false },
		IsVerbose:  func() bool { return false },
		WarnWriter: writer,
	}
}

var _ = Describe("registerSystemSpecs", func() {
	It("embeds system-local YAML files", func() {
		cmdbData, err := fs.ReadFile(actionsFS, "cmdb/actions.yaml")
		Expect(err).NotTo(HaveOccurred())
		Expect(string(cmdbData)).To(ContainSubstring("name: cmdb"))

		apigatewayData, err := fs.ReadFile(actionsFS, "apigateway/actions.yaml")
		Expect(err).NotTo(HaveOccurred())
		Expect(string(apigatewayData)).To(ContainSubstring("name: apigateway"))

		subsystemData, err := fs.ReadFile(actionsFS, "testdata/subsystem/actions.yaml")
		Expect(err).NotTo(HaveOccurred())
		Expect(string(subsystemData)).To(ContainSubstring("name: subsystem"))
	})

	It("registers a YAML-driven system", func() {
		root := &cobra.Command{Use: "bk-cli"}
		yamlFS := fstest.MapFS{
			"demo/actions.yaml": {
				Data: []byte(`
name: demo
gateway_name: bk-demo
description: "Demo service"
actions:
  - name: list_items
    description: "List items"
    method: GET
    path: "/api/v1/items/"
    authConfig:
      appVerifiedRequired: true
      userVerifiedRequired: false
      resourcePermissionRequired: false
    params:
      - name: keyword
        in: query
        type: string
`),
			},
		}

		err := registerSystemSpecs(root, []systemcmd.SystemSpec{
			{
				Name:        "demo",
				Description: "Demo service",
				YAMLFile:    "demo/actions.yaml",
			},
		}, testBuildDeps(nil), yamlFS)
		Expect(err).NotTo(HaveOccurred())

		actionCmd, _, err := root.Find([]string{"demo", "list_items"})
		Expect(err).NotTo(HaveOccurred())
		Expect(actionCmd.Flag(syslib.ActionStageFlagName)).NotTo(BeNil())
		Expect(actionCmd.Flag(syslib.ActionBodyFlagName)).NotTo(BeNil())
		Expect(actionCmd.Flag(syslib.ActionHeaderFlagName)).NotTo(BeNil())
		Expect(actionCmd.Flag("keyword")).NotTo(BeNil())
	})

	It("surfaces YAML examples and schema hint in action help", func() {
		root := &cobra.Command{Use: "bk-cli"}
		yamlFS := fstest.MapFS{
			"demo/actions.yaml": {
				Data: []byte(`
name: demo
gateway_name: bk-demo
description: "Demo service"
actions:
  - name: update_item
    description: "Update item"
    method: PUT
    path: "/api/v1/items/{id}/"
    body_schema: |
      {
        "type": "object",
        "properties": {
          "name": {"type": "string"}
        }
      }
    body_required: true
    examples:
      - >-
        bk-cli demo update_item --id 42 --body '{"name":"demo"}'
    authConfig:
      appVerifiedRequired: true
      userVerifiedRequired: false
      resourcePermissionRequired: false
    params:
      - name: id
        in: path
        type: string
        required: true
`),
			},
		}

		err := registerSystemSpecs(root, []systemcmd.SystemSpec{
			{
				Name:        "demo",
				Description: "Demo service",
				YAMLFile:    "demo/actions.yaml",
			},
		}, testBuildDeps(nil), yamlFS)
		Expect(err).NotTo(HaveOccurred())

		actionCmd, _, err := root.Find([]string{"demo", "update_item"})
		Expect(err).NotTo(HaveOccurred())

		var help bytes.Buffer
		actionCmd.SetOut(&help)
		Expect(actionCmd.Help()).To(Succeed())

		helpText := help.String()
		usageIndex := strings.Index(helpText, "Usage:")
		examplesIndex := strings.Index(helpText, "Examples:")
		schemaIndex := strings.Index(helpText, "Request body schema:")
		flagsIndex := strings.Index(helpText, "Flags:")
		Expect(usageIndex).NotTo(Equal(-1))
		Expect(examplesIndex).To(BeNumerically(">", usageIndex))
		Expect(schemaIndex).To(BeNumerically(">", examplesIndex))
		Expect(flagsIndex).To(BeNumerically(">", schemaIndex))
		Expect(helpText).To(ContainSubstring(`--body '{"name":"demo"}'`))
		Expect(helpText).To(ContainSubstring("Run with -h --body-schema"))
		Expect(helpText).To(ContainSubstring("[Required] JSON request body"))
		Expect(helpText).NotTo(ContainSubstring("Request body example:"))
		Expect(helpText).NotTo(ContainSubstring(`"properties": {`))
		Expect(actionCmd.Flag(syslib.ActionBodySchemaFlagName)).NotTo(BeNil())
	})

	It("prints only the YAML body schema when help uses --body-schema", func() {
		root := &cobra.Command{Use: "bk-cli"}
		yamlFS := fstest.MapFS{
			"demo/actions.yaml": {
				Data: []byte(`
name: demo
gateway_name: bk-demo
description: "Demo service"
actions:
  - name: update_item
    description: "Update item"
    method: PUT
    path: "/api/v1/items/{id}/"
    body_schema: |
      {
        "type": "object",
        "properties": {
          "name": {"type": "string"}
        }
      }
    authConfig:
      appVerifiedRequired: true
      userVerifiedRequired: false
      resourcePermissionRequired: false
    params:
      - name: id
        in: path
        type: string
        required: true
`),
			},
		}

		err := registerSystemSpecs(root, []systemcmd.SystemSpec{
			{
				Name:        "demo",
				Description: "Demo service",
				YAMLFile:    "demo/actions.yaml",
			},
		}, testBuildDeps(nil), yamlFS)
		Expect(err).NotTo(HaveOccurred())

		var help bytes.Buffer
		root.SetOut(&help)
		root.SetArgs([]string{"demo", "update_item", "-h", "--body-schema"})
		Expect(root.Execute()).To(Succeed())

		Expect(help.String()).To(HavePrefix("Request body schema:\n"))
		Expect(help.String()).To(ContainSubstring(`"name": {"type": "string"}`))
		Expect(help.String()).NotTo(ContainSubstring("Usage:"))
		Expect(help.String()).NotTo(ContainSubstring("Request body example:"))
	})

	It("rejects --body-schema without help before executing a YAML action", func() {
		root := &cobra.Command{
			Use:           "bk-cli",
			SilenceUsage:  true,
			SilenceErrors: true,
		}
		yamlFS := fstest.MapFS{
			"demo/actions.yaml": {
				Data: []byte(`
name: demo
gateway_name: bk-demo
description: "Demo service"
actions:
  - name: update_item
    description: "Update item"
    method: PUT
    path: "/api/v1/items/{id}/"
    body_schema: |
      {"type": "object"}
    authConfig:
      appVerifiedRequired: true
      userVerifiedRequired: false
      resourcePermissionRequired: false
    params:
      - name: id
        in: path
        type: string
        required: true
`),
			},
		}

		err := registerSystemSpecs(root, []systemcmd.SystemSpec{
			{
				Name:        "demo",
				Description: "Demo service",
				YAMLFile:    "demo/actions.yaml",
			},
		}, testBuildDeps(nil), yamlFS)
		Expect(err).NotTo(HaveOccurred())

		root.SetArgs([]string{"demo", "update_item", "--id", "42", "--body-schema"})
		err = root.Execute()
		Expect(err).To(MatchError("--body-schema is a help modifier; use -h --body-schema"))
	})

	It("rejects a missing required body before resolving runtime", func() {
		oldConfigDir, hadConfigDir := os.LookupEnv("BK_CLI_CONFIG_DIR")
		tmpDir := GinkgoT().TempDir()
		Expect(os.Setenv("BK_CLI_CONFIG_DIR", tmpDir)).To(Succeed())
		DeferCleanup(func() {
			if hadConfigDir {
				Expect(os.Setenv("BK_CLI_CONFIG_DIR", oldConfigDir)).To(Succeed())
				return
			}
			Expect(os.Unsetenv("BK_CLI_CONFIG_DIR")).To(Succeed())
		})

		root := &cobra.Command{
			Use:           "bk-cli",
			SilenceUsage:  true,
			SilenceErrors: true,
		}
		yamlFS := fstest.MapFS{
			"demo/actions.yaml": {
				Data: []byte(`
name: demo
gateway_name: bk-demo
description: "Demo service"
actions:
  - name: create_item
    description: "Create item"
    method: POST
    path: "/api/v1/items/"
    body_required: true
    authConfig:
      appVerifiedRequired: true
      userVerifiedRequired: false
      resourcePermissionRequired: false
`),
			},
		}

		err := registerSystemSpecs(root, []systemcmd.SystemSpec{
			{
				Name:        "demo",
				Description: "Demo service",
				YAMLFile:    "demo/actions.yaml",
			},
		}, testBuildDeps(nil), yamlFS)
		Expect(err).NotTo(HaveOccurred())

		root.SetArgs([]string{"demo", "create_item"})
		err = root.Execute()
		Expect(err).To(HaveOccurred())

		cliErr, ok := err.(*output.CLIError)
		Expect(ok).To(BeTrue())
		Expect(cliErr.Code).To(Equal("missing_param"))
		Expect(cliErr.Message).To(ContainSubstring("required parameter --body is missing"))
	})

	It("surfaces YAML header params in --header help without generating a dedicated flag", func() {
		root := &cobra.Command{Use: "bk-cli"}
		yamlFS := fstest.MapFS{
			"demo/actions.yaml": {
				Data: []byte(`
name: demo
gateway_name: bk-demo
description: "Demo service"
actions:
  - name: traceable_action
    description: "Action with documented headers"
    method: GET
    path: "/api/v1/items/"
    authConfig:
      appVerifiedRequired: true
      userVerifiedRequired: false
      resourcePermissionRequired: false
    params:
      - name: keyword
        in: query
        type: string
      - name: X-Request-Id
        in: header
        type: string
        description: "Request ID"
`),
			},
		}

		err := registerSystemSpecs(root, []systemcmd.SystemSpec{
			{
				Name:        "demo",
				Description: "Demo service",
				YAMLFile:    "demo/actions.yaml",
			},
		}, testBuildDeps(nil), yamlFS)
		Expect(err).NotTo(HaveOccurred())

		actionCmd, _, err := root.Find([]string{"demo", "traceable_action"})
		Expect(err).NotTo(HaveOccurred())
		Expect(actionCmd.Flag("keyword")).NotTo(BeNil())
		Expect(actionCmd.Flag("X-Request-Id")).To(BeNil())
		Expect(actionCmd.Flag(syslib.ActionHeaderFlagName)).NotTo(BeNil())
		Expect(
			actionCmd.Flag(syslib.ActionHeaderFlagName).Usage,
		).To(
			ContainSubstring(`--header "X-Request-Id:value_example"`),
		)
	})

	It("registers a Go-implemented system without YAML", func() {
		root := &cobra.Command{Use: "bk-cli"}

		err := registerSystemSpecs(root, []systemcmd.SystemSpec{
			{
				Name:        "logic",
				Description: "Logic service",
				RegisterGoActions: func(parent *cobra.Command, deps systemcmd.BuildDeps) error {
					parent.AddCommand(&cobra.Command{
						Use:   "orchestrate",
						Short: "Run orchestration logic",
					})
					return nil
				},
			},
		}, testBuildDeps(nil), fstest.MapFS{})
		Expect(err).NotTo(HaveOccurred())

		actionCmd, _, err := root.Find([]string{"logic", "orchestrate"})
		Expect(err).NotTo(HaveOccurred())
		Expect(actionCmd.Name()).To(Equal("orchestrate"))
	})

	It("registers a mixed system with YAML and Go actions", func() {
		root := &cobra.Command{Use: "bk-cli"}
		yamlFS := fstest.MapFS{
			"apigateway/actions.yaml": {
				Data: []byte(`
name: apigateway
gateway_name: bk-apigateway
description: "API Gateway"
actions:
  - name: list_gateways
    description: "List gateways"
    method: GET
    path: "/api/v2/open/gateways/"
    authConfig:
      appVerifiedRequired: true
      userVerifiedRequired: false
      resourcePermissionRequired: false
`),
			},
		}

		err := registerSystemSpecs(root, []systemcmd.SystemSpec{
			{
				Name:        "apigateway",
				Description: "API Gateway",
				YAMLFile:    "apigateway/actions.yaml",
				RegisterGoActions: func(parent *cobra.Command, deps systemcmd.BuildDeps) error {
					parent.AddCommand(&cobra.Command{
						Use:   "demo_action",
						Short: "Example Go-implemented action",
					})
					return nil
				},
			},
		}, testBuildDeps(nil), yamlFS)
		Expect(err).NotTo(HaveOccurred())

		systemCmd, _, err := root.Find([]string{"apigateway"})
		Expect(err).NotTo(HaveOccurred())
		Expect(systemCmd.Commands()).To(HaveLen(2))

		yamlCmd, _, err := root.Find([]string{"apigateway", "list_gateways"})
		Expect(err).NotTo(HaveOccurred())
		Expect(yamlCmd.Name()).To(Equal("list_gateways"))

		goCmd, _, err := root.Find([]string{"apigateway", "demo_action"})
		Expect(err).NotTo(HaveOccurred())
		Expect(goCmd.Name()).To(Equal("demo_action"))
	})

	It("registers parent actions and one-level subsystem actions independently", func() {
		root := &cobra.Command{Use: "bk-cli"}
		yamlFS := fstest.MapFS{
			"devops/actions.yaml": {
				Data: []byte(`
name: devops
gateway_name: devops
description: "DevOps parent commands"
actions:
  - name: status
    description: "Show DevOps status"
    method: GET
    path: "/status"
    authConfig:
      appVerifiedRequired: false
      userVerifiedRequired: true
      resourcePermissionRequired: false
`),
			},
			"devops/pipeline/actions.yaml": {
				Data: []byte(`
name: pipeline
gateway_name: devops-pipeline
description: "DevOps pipeline commands"
actions:
  - name: get_build_list
    description: "List pipeline builds"
    method: GET
    path: "/v4/projects/{project_id}/builds"
    authConfig:
      appVerifiedRequired: false
      userVerifiedRequired: true
      resourcePermissionRequired: false
    params:
      - name: project_id
        in: path
        type: string
        required: true
`),
			},
		}

		err := registerSystemSpecs(root, []systemcmd.SystemSpec{
			{
				Name:        "devops",
				Description: "DevOps commands",
				YAMLFile:    "devops/actions.yaml",
				RegisterGoActions: func(parent *cobra.Command, deps systemcmd.BuildDeps) error {
					parent.AddCommand(&cobra.Command{
						Use:   "parent_go",
						Short: "Parent Go action",
					})
					return nil
				},
				Subsystems: []systemcmd.SystemSpec{
					{
						Name:        "pipeline",
						Description: "Pipeline commands",
						YAMLFile:    "devops/pipeline/actions.yaml",
						RegisterGoActions: func(parent *cobra.Command, deps systemcmd.BuildDeps) error {
							parent.AddCommand(&cobra.Command{
								Use:   "start_build",
								Short: "Start a pipeline build",
							})
							return nil
						},
					},
				},
			},
		}, testBuildDeps(nil), yamlFS)
		Expect(err).NotTo(HaveOccurred())

		parentYAML, _, err := root.Find([]string{"devops", "status"})
		Expect(err).NotTo(HaveOccurred())
		Expect(parentYAML.Name()).To(Equal("status"))

		parentGo, _, err := root.Find([]string{"devops", "parent_go"})
		Expect(err).NotTo(HaveOccurred())
		Expect(parentGo.Name()).To(Equal("parent_go"))

		subsystemYAML, _, err := root.Find([]string{"devops", "pipeline", "get_build_list"})
		Expect(err).NotTo(HaveOccurred())
		Expect(subsystemYAML.Name()).To(Equal("get_build_list"))
		Expect(subsystemYAML.Flag("project_id")).NotTo(BeNil())

		subsystemGo, _, err := root.Find([]string{"devops", "pipeline", "start_build"})
		Expect(err).NotTo(HaveOccurred())
		Expect(subsystemGo.Name()).To(Equal("start_build"))
	})

	It("rejects unknown action names under system command groups", func() {
		root := &cobra.Command{
			Use:           "bk-cli",
			SilenceUsage:  true,
			SilenceErrors: true,
		}
		yamlFS := fstest.MapFS{
			"demo/actions.yaml": {
				Data: []byte(`
name: demo
gateway_name: bk-demo
description: "Demo service"
actions:
  - name: list_items
    description: "List items"
    method: GET
    path: "/api/v1/items/"
    authConfig:
      appVerifiedRequired: false
      userVerifiedRequired: false
      resourcePermissionRequired: false
`),
			},
		}

		err := registerSystemSpecs(root, []systemcmd.SystemSpec{
			{
				Name:        "demo",
				Description: "Demo service",
				YAMLFile:    "demo/actions.yaml",
			},
		}, testBuildDeps(nil), yamlFS)
		Expect(err).NotTo(HaveOccurred())

		systemCmd, _, err := root.Find([]string{"demo"})
		Expect(err).NotTo(HaveOccurred())
		Expect(systemCmd.Args).NotTo(BeNil())

		err = systemCmd.Args(systemCmd, []string{"missing_action"})
		Expect(err).To(MatchError(ContainSubstring(`unknown command "missing_action" for "bk-cli demo"`)))
	})

	It("rejects unknown action names under subsystem command groups", func() {
		root := &cobra.Command{
			Use:           "bk-cli",
			SilenceUsage:  true,
			SilenceErrors: true,
		}
		yamlFS := fstest.MapFS{
			"devops/pipeline/actions.yaml": {
				Data: []byte(`
name: pipeline
gateway_name: devops-pipeline
description: "Pipeline commands"
actions:
  - name: get_build_list
    method: GET
    path: "/builds"
    authConfig:
      appVerifiedRequired: false
      userVerifiedRequired: true
      resourcePermissionRequired: false
`),
			},
		}

		err := registerSystemSpecs(root, []systemcmd.SystemSpec{
			{
				Name:        "devops",
				Description: "DevOps commands",
				Subsystems: []systemcmd.SystemSpec{
					{
						Name:        "pipeline",
						Description: "Pipeline commands",
						YAMLFile:    "devops/pipeline/actions.yaml",
					},
				},
			},
		}, testBuildDeps(nil), yamlFS)
		Expect(err).NotTo(HaveOccurred())

		subsystemCmd, _, err := root.Find([]string{"devops", "pipeline"})
		Expect(err).NotTo(HaveOccurred())
		Expect(subsystemCmd.Args).NotTo(BeNil())

		err = subsystemCmd.Args(subsystemCmd, []string{"missing_action"})
		Expect(
			err,
		).To(
			MatchError(ContainSubstring(`unknown command "missing_action" for "bk-cli devops pipeline"`)),
		)
	})

	It("rejects extra positional arguments for YAML actions", func() {
		root := &cobra.Command{
			Use:           "bk-cli",
			SilenceUsage:  true,
			SilenceErrors: true,
		}
		yamlFS := fstest.MapFS{
			"demo/actions.yaml": {
				Data: []byte(`
name: demo
gateway_name: bk-demo
description: "Demo service"
actions:
  - name: list_items
    description: "List items"
    method: GET
    path: "/api/v1/items/"
    authConfig:
      appVerifiedRequired: false
      userVerifiedRequired: false
      resourcePermissionRequired: false
`),
			},
		}

		err := registerSystemSpecs(root, []systemcmd.SystemSpec{
			{
				Name:        "demo",
				Description: "Demo service",
				YAMLFile:    "demo/actions.yaml",
			},
		}, testBuildDeps(nil), yamlFS)
		Expect(err).NotTo(HaveOccurred())

		root.SetArgs([]string{"demo", "list_items", "extra"})
		err = root.Execute()
		Expect(err).To(MatchError(ContainSubstring(`unknown command "extra" for "bk-cli demo list_items"`)))
	})

	It("rejects extra positional arguments for Go actions", func() {
		root := &cobra.Command{
			Use:           "bk-cli",
			SilenceUsage:  true,
			SilenceErrors: true,
		}

		err := registerSystemSpecs(root, []systemcmd.SystemSpec{
			{
				Name:        "logic",
				Description: "Logic service",
				RegisterGoActions: func(parent *cobra.Command, deps systemcmd.BuildDeps) error {
					parent.AddCommand(&cobra.Command{
						Use:   "orchestrate",
						Short: "Run orchestration logic",
						RunE: func(cmd *cobra.Command, args []string) error {
							return nil
						},
					})
					return nil
				},
			},
		}, testBuildDeps(nil), fstest.MapFS{})
		Expect(err).NotTo(HaveOccurred())

		root.SetArgs([]string{"logic", "orchestrate", "extra"})
		err = root.Execute()
		Expect(err).To(MatchError(ContainSubstring(`unknown command "extra" for "bk-cli logic orchestrate"`)))
	})

	It("returns an error when a parent YAML action conflicts with a subsystem name", func() {
		root := &cobra.Command{Use: "bk-cli"}
		yamlFS := fstest.MapFS{
			"devops/actions.yaml": {
				Data: []byte(`
name: devops
gateway_name: devops
description: "DevOps parent commands"
actions:
  - name: pipeline
    description: "Conflicting parent action"
    method: GET
    path: "/pipeline"
    authConfig:
      appVerifiedRequired: false
      userVerifiedRequired: true
      resourcePermissionRequired: false
`),
			},
			"devops/pipeline/actions.yaml": {
				Data: []byte(`
name: pipeline
gateway_name: devops-pipeline
description: "Pipeline commands"
actions:
  - name: get_build_list
    method: GET
    path: "/builds"
    authConfig:
      appVerifiedRequired: false
      userVerifiedRequired: true
      resourcePermissionRequired: false
`),
			},
		}

		err := registerSystemSpecs(root, []systemcmd.SystemSpec{
			{
				Name:     "devops",
				YAMLFile: "devops/actions.yaml",
				Subsystems: []systemcmd.SystemSpec{
					{Name: "pipeline", YAMLFile: "devops/pipeline/actions.yaml"},
				},
			},
		}, testBuildDeps(nil), yamlFS)
		Expect(err).To(MatchError(ContainSubstring(`system "devops" has duplicate child command "pipeline"`)))
	})

	It("returns an error when a parent Go action conflicts with a subsystem name", func() {
		root := &cobra.Command{Use: "bk-cli"}
		yamlFS := fstest.MapFS{
			"devops/pipeline/actions.yaml": {
				Data: []byte(`
name: pipeline
gateway_name: devops-pipeline
description: "Pipeline commands"
actions:
  - name: get_build_list
    method: GET
    path: "/builds"
    authConfig:
      appVerifiedRequired: false
      userVerifiedRequired: true
      resourcePermissionRequired: false
`),
			},
		}

		err := registerSystemSpecs(root, []systemcmd.SystemSpec{
			{
				Name: "devops",
				RegisterGoActions: func(parent *cobra.Command, deps systemcmd.BuildDeps) error {
					parent.AddCommand(&cobra.Command{Use: "pipeline"})
					return nil
				},
				Subsystems: []systemcmd.SystemSpec{
					{Name: "pipeline", YAMLFile: "devops/pipeline/actions.yaml"},
				},
			},
		}, testBuildDeps(nil), yamlFS)
		Expect(err).To(MatchError(ContainSubstring(`system "devops" has duplicate child command "pipeline"`)))
	})

	It("returns an error for duplicate subsystem names under one system", func() {
		root := &cobra.Command{Use: "bk-cli"}

		err := registerSystemSpecs(root, []systemcmd.SystemSpec{
			{
				Name: "devops",
				Subsystems: []systemcmd.SystemSpec{
					{
						Name: "pipeline",
						RegisterGoActions: func(parent *cobra.Command, deps systemcmd.BuildDeps) error {
							parent.AddCommand(&cobra.Command{Use: "list"})
							return nil
						},
					},
					{
						Name: "pipeline",
						RegisterGoActions: func(parent *cobra.Command, deps systemcmd.BuildDeps) error {
							parent.AddCommand(&cobra.Command{Use: "start"})
							return nil
						},
					},
				},
			},
		}, testBuildDeps(nil), fstest.MapFS{})
		Expect(err).To(MatchError(ContainSubstring(`system "devops" has duplicate subsystem "pipeline"`)))
	})

	It("rejects nested subsystem definitions beyond one level", func() {
		root := &cobra.Command{Use: "bk-cli"}

		err := registerSystemSpecs(root, []systemcmd.SystemSpec{
			{
				Name: "devops",
				Subsystems: []systemcmd.SystemSpec{
					{
						Name: "pipeline",
						Subsystems: []systemcmd.SystemSpec{
							{Name: "build"},
						},
					},
				},
			},
		}, testBuildDeps(nil), fstest.MapFS{})
		Expect(
			err,
		).To(
			MatchError(
				ContainSubstring(
					`system "devops.pipeline" cannot define nested subsystems; only one subsystem level is supported`,
				),
			),
		)
	})

	It("registers representative shipped actions for each built-in system", func() {
		root := &cobra.Command{Use: "bk-cli"}

		err := registerSystemSpecs(root, systemCatalog(), testBuildDeps(nil), actionsFS)
		Expect(err).NotTo(HaveOccurred())

		for _, path := range [][]string{
			{"apigateway", "list_gateways"},
			{"apigateway", "demo_action"},
			{"bcs", "cluster_manager", "update_auto_scaling_option"},
			{"bcs", "cluster_manager", "create_cluster"},
			{"bcs", "cluster_manager", "delete_nodes_from_cluster"},
			{"bcs", "cluster_manager", "update_node_group"},
			{"bcs", "cluster_manager", "clean_nodes_in_group_v2"},
			{"paas", "get_deployment_result"},
			{"paas", "module_env_released_info"},
			{"paas", "deploy_with_module"},
			{"cmdb", "search_business"},
			{"cmdb", "create_set"},
			{"job", "get_job_instance_status"},
			{"job", "fast_execute_script"},
			{"sops", "get_template_list"},
			{"sops", "create_task"},
			{"gse", "list_agent_state"},
			{"gse", "list_agent_info"},
			{"devops", "pipeline", "get_build_list"},
			{"devops", "pipeline", "start_build"},
			{"devops", "codecc", "get_task_detail"},
			{"devops", "stream", "trigger"},
			{"nodeman", "install_job"},
			{"nodeman", "get_job_details"},
		} {
			actionCmd, _, findErr := root.Find(path)
			Expect(findErr).NotTo(HaveOccurred(), strings.Join(path, " "))
			Expect(actionCmd).NotTo(BeNil(), strings.Join(path, " "))
		}
	})

	It("registers BCS cluster_manager YAML actions with generated request flags", func() {
		root := &cobra.Command{Use: "bk-cli"}

		err := registerSystemSpecs(root, systemCatalog(), testBuildDeps(nil), actionsFS)
		Expect(err).NotTo(HaveOccurred())

		updateAutoScalingOption, _, err := root.Find([]string{
			"bcs",
			"cluster_manager",
			"update_auto_scaling_option",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(updateAutoScalingOption.Flag("clusterID")).NotTo(BeNil())
		Expect(updateAutoScalingOption.Flag(syslib.ActionBodyFlagName)).NotTo(BeNil())
		Expect(updateAutoScalingOption.Flag(syslib.ActionBodyFlagName).Usage).To(ContainSubstring("[Required]"))
		Expect(updateAutoScalingOption.Flag(syslib.ActionStageFlagName)).NotTo(BeNil())
		Expect(updateAutoScalingOption.Flag(syslib.ActionHeaderFlagName)).NotTo(BeNil())

		deleteNodes, _, err := root.Find([]string{
			"bcs",
			"cluster_manager",
			"delete_nodes_from_cluster",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(deleteNodes.Flag("clusterID")).NotTo(BeNil())
		Expect(deleteNodes.Flag("nodes")).NotTo(BeNil())
		Expect(deleteNodes.Flag("isForce")).NotTo(BeNil())
		Expect(deleteNodes.Flag("onlyDeleteInfo")).NotTo(BeNil())

		updateNodeTemplate, _, err := root.Find([]string{
			"bcs",
			"cluster_manager",
			"update_node_template",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(updateNodeTemplate.Flag("projectID")).NotTo(BeNil())
		Expect(updateNodeTemplate.Flag("nodeTemplateID")).NotTo(BeNil())

		deleteTemplateConfig, _, err := root.Find([]string{
			"bcs",
			"cluster_manager",
			"delete_template_config",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(deleteTemplateConfig.Flag("templateConfigID")).NotTo(BeNil())
		Expect(deleteTemplateConfig.Flag("businessID")).NotTo(BeNil())
		Expect(deleteTemplateConfig.Flag("projectID")).NotTo(BeNil())
	})

	It("registers PaaS YAML actions with generated path flags", func() {
		root := &cobra.Command{Use: "bk-cli"}

		err := registerSystemSpecs(root, systemCatalog(), testBuildDeps(nil), actionsFS)
		Expect(err).NotTo(HaveOccurred())

		getDeploymentResult, _, err := root.Find([]string{
			"paas",
			"get_deployment_result",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(getDeploymentResult.Flag("app_code")).NotTo(BeNil())
		Expect(getDeploymentResult.Flag("module")).NotTo(BeNil())
		Expect(getDeploymentResult.Flag("deployment_id")).NotTo(BeNil())
		Expect(getDeploymentResult.Flag(syslib.ActionBodyFlagName)).NotTo(BeNil())
		Expect(getDeploymentResult.Flag(syslib.ActionStageFlagName)).NotTo(BeNil())
		Expect(getDeploymentResult.Flag(syslib.ActionHeaderFlagName)).NotTo(BeNil())

		releasedInfo, _, err := root.Find([]string{
			"paas",
			"module_env_released_info",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(releasedInfo.Flag("code")).NotTo(BeNil())
		Expect(releasedInfo.Flag("module_name")).NotTo(BeNil())
		Expect(releasedInfo.Flag("environment")).NotTo(BeNil())

		deployWithModule, _, err := root.Find([]string{
			"paas",
			"deploy_with_module",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(deployWithModule.Flag("app_code")).NotTo(BeNil())
		Expect(deployWithModule.Flag("module")).NotTo(BeNil())
		Expect(deployWithModule.Flag("env")).NotTo(BeNil())
	})

	It("does not register empty systems", func() {
		root := &cobra.Command{Use: "bk-cli"}

		err := registerSystemSpecs(root, []systemcmd.SystemSpec{
			{
				Name:        "empty",
				Description: "No actions yet",
			},
		}, testBuildDeps(nil), fstest.MapFS{})
		Expect(err).NotTo(HaveOccurred())
		Expect(root.Commands()).To(BeEmpty())
	})

	It("returns an error for duplicate system names", func() {
		root := &cobra.Command{Use: "bk-cli"}

		err := registerSystemSpecs(root, []systemcmd.SystemSpec{
			{
				Name: "demo",
				RegisterGoActions: func(parent *cobra.Command, deps systemcmd.BuildDeps) error {
					parent.AddCommand(&cobra.Command{Use: "first"})
					return nil
				},
			},
			{
				Name: "demo",
				RegisterGoActions: func(parent *cobra.Command, deps systemcmd.BuildDeps) error {
					parent.AddCommand(&cobra.Command{Use: "second"})
					return nil
				},
			},
		}, testBuildDeps(nil), fstest.MapFS{})
		Expect(err).To(MatchError(ContainSubstring(`system "demo" is already registered`)))
	})

	It("skips conflicting YAML actions and emits a warning", func() {
		root := &cobra.Command{Use: "bk-cli"}
		warnBuf := &bytes.Buffer{}
		yamlFS := fstest.MapFS{
			"demo/actions.yaml": {
				Data: []byte(`
name: demo
gateway_name: bk-demo
description: "Demo service"
actions:
  - name: valid_action
    method: GET
    path: "/api/v1/resources/{id}/"
    authConfig:
      appVerifiedRequired: true
      userVerifiedRequired: false
      resourcePermissionRequired: false
    params:
      - name: id
        in: path
        type: string
        required: true
  - name: broken_action
    method: GET
    path: "/api/v1/resources/{id}/"
    authConfig:
      appVerifiedRequired: true
      userVerifiedRequired: false
      resourcePermissionRequired: false
    params:
      - name: id
        in: path
        type: string
        required: true
      - name: id
        in: query
        type: string
`),
			},
		}

		err := registerSystemSpecs(root, []systemcmd.SystemSpec{
			{
				Name:        "demo",
				Description: "Demo service",
				YAMLFile:    "demo/actions.yaml",
			},
		}, testBuildDeps(warnBuf), yamlFS)
		Expect(err).NotTo(HaveOccurred())

		systemCmd, _, err := root.Find([]string{"demo"})
		Expect(err).NotTo(HaveOccurred())
		Expect(systemCmd.Commands()).To(HaveLen(1))
		Expect(systemCmd.Commands()[0].Name()).To(Equal("valid_action"))
		Expect(warnBuf.String()).To(ContainSubstring("skip registering action demo.broken_action"))
		Expect(warnBuf.String()).To(ContainSubstring("param name conflict \"id\" between path and query"))
	})

	It("skips YAML actions with invalid int defaults and emits a warning", func() {
		root := &cobra.Command{Use: "bk-cli"}
		warnBuf := &bytes.Buffer{}
		yamlFS := fstest.MapFS{
			"demo/actions.yaml": {
				Data: []byte(`
name: demo
gateway_name: bk-demo
description: "Demo service"
actions:
  - name: valid_action
    method: GET
    path: "/api/v1/resources/"
    authConfig:
      appVerifiedRequired: true
      userVerifiedRequired: false
      resourcePermissionRequired: false
  - name: broken_action
    method: GET
    path: "/api/v1/resources/"
    authConfig:
      appVerifiedRequired: true
      userVerifiedRequired: false
      resourcePermissionRequired: false
    params:
      - name: limit
        in: query
        type: int
        default: "abc"
`),
			},
		}

		err := registerSystemSpecs(root, []systemcmd.SystemSpec{
			{
				Name:        "demo",
				Description: "Demo service",
				YAMLFile:    "demo/actions.yaml",
			},
		}, testBuildDeps(warnBuf), yamlFS)
		Expect(err).NotTo(HaveOccurred())

		systemCmd, _, err := root.Find([]string{"demo"})
		Expect(err).NotTo(HaveOccurred())
		Expect(systemCmd.Commands()).To(HaveLen(1))
		Expect(systemCmd.Commands()[0].Name()).To(Equal("valid_action"))
		Expect(warnBuf.String()).To(ContainSubstring("skip registering action demo.broken_action"))
		Expect(warnBuf.String()).To(ContainSubstring(`param "limit"`))
		Expect(warnBuf.String()).To(ContainSubstring(`invalid int default value "abc"`))
	})

	It("returns an error when YAML and Go actions share the same name", func() {
		root := &cobra.Command{Use: "bk-cli"}
		yamlFS := fstest.MapFS{
			"apigateway/actions.yaml": {
				Data: []byte(`
name: apigateway
gateway_name: bk-apigateway
description: "API Gateway"
actions:
  - name: list_gateways
    method: GET
    path: "/api/v2/open/gateways/"
    authConfig:
      appVerifiedRequired: true
      userVerifiedRequired: false
      resourcePermissionRequired: false
`),
			},
		}

		err := registerSystemSpecs(root, []systemcmd.SystemSpec{
			{
				Name:        "apigateway",
				Description: "API Gateway",
				YAMLFile:    "apigateway/actions.yaml",
				RegisterGoActions: func(parent *cobra.Command, deps systemcmd.BuildDeps) error {
					parent.AddCommand(&cobra.Command{
						Use:   "list_gateways",
						Short: "Duplicate action",
					})
					return nil
				},
			},
		}, testBuildDeps(nil), yamlFS)
		Expect(
			err,
		).To(
			MatchError(
				ContainSubstring(`system "apigateway" has duplicate child command "list_gateways"`),
			),
		)
	})

	It("validates the embedded system catalog without warnings", func() {
		root := &cobra.Command{Use: "bk-cli"}
		warnBuf := &bytes.Buffer{}

		err := registerSystemSpecs(root, systemCatalog(), testBuildDeps(warnBuf), actionsFS)
		Expect(err).NotTo(HaveOccurred())
		Expect(warnBuf.String()).To(BeEmpty())

		actionCmd, _, err := root.Find([]string{"apigateway", "list_gateways"})
		Expect(err).NotTo(HaveOccurred())
		Expect(actionCmd.Name()).To(Equal("list_gateways"))

		cmdbCmd, _, err := root.Find([]string{"cmdb", "search_business"})
		Expect(err).NotTo(HaveOccurred())
		Expect(cmdbCmd.Name()).To(Equal("search_business"))

		cmdbYAMLCmd, _, err := root.Find([]string{"cmdb", "get_biz_internal_module"})
		Expect(err).NotTo(HaveOccurred())
		Expect(cmdbYAMLCmd.Name()).To(Equal("get_biz_internal_module"))

		cmdbGoCmd, _, err := root.Find([]string{"cmdb", "search_set"})
		Expect(err).NotTo(HaveOccurred())
		Expect(cmdbGoCmd.Name()).To(Equal("search_set"))
	})
})
