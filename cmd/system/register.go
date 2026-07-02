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

// Package system registers system subcommands from Go specs plus optional YAML actions.
package system

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/spf13/cobra"

	syslib "github.com/TencentBlueKing/bk-cli/internal/system"
	systemcmd "github.com/TencentBlueKing/bk-cli/internal/systemcmd"
)

const (
	systemCommandPrefix = "[system] "
	maxSubsystemDepth   = 1
)

//go:embed */actions.yaml */*/actions.yaml
var actionsFS embed.FS

// RegisterAll registers all known systems on the root command.
func RegisterAll(
	parent *cobra.Command,
	getContext func() string,
	isDryRun, isVerbose, isInsecure func() bool,
) error {
	return registerSystemSpecs(parent, systemCatalog(), systemcmd.BuildDeps{
		GetContext: getContext,
		IsDryRun:   isDryRun,
		IsVerbose:  isVerbose,
		IsInsecure: isInsecure,
		WarnWriter: os.Stderr,
	}, actionsFS)
}

func systemCatalog() []systemcmd.SystemSpec {
	return []systemcmd.SystemSpec{
		newApigatewaySystemSpec(),
		newBCSSystemSpec(),
		newPaasSystemSpec(),
		newCMDBSystemSpec(),
		newJobSystemSpec(),
		newSOPSSystemSpec(),
		newGSESystemSpec(),
		newDevopsSystemSpec(),
		newNodemanSystemSpec(),
	}
}

func registerSystemSpecs(
	parent *cobra.Command,
	specs []systemcmd.SystemSpec,
	deps systemcmd.BuildDeps,
	yamlFS fs.FS,
) error {
	return registerCommandGroups(parent, specs, deps, yamlFS, 0, nil)
}

func registerCommandGroups(
	parent *cobra.Command,
	specs []systemcmd.SystemSpec,
	deps systemcmd.BuildDeps,
	yamlFS fs.FS,
	depth int,
	parentPath []string,
) error {
	seenGroups := make(map[string]struct{}, len(specs))

	for _, spec := range specs {
		if spec.Name == "" {
			return fmt.Errorf("system name is required")
		}
		if _, exists := seenGroups[spec.Name]; exists {
			if len(parentPath) == 0 {
				return fmt.Errorf("system %q is already registered", spec.Name)
			}
			return fmt.Errorf(
				"system %q has duplicate subsystem %q",
				commandGroupName(parentPath),
				spec.Name,
			)
		}
		seenGroups[spec.Name] = struct{}{}

		groupPath := append(append([]string{}, parentPath...), spec.Name)
		groupName := commandGroupName(groupPath)

		if depth > maxSubsystemDepth {
			return fmt.Errorf(
				"system %q cannot define nested subsystems; only one subsystem level is supported",
				groupName,
			)
		}
		if depth == maxSubsystemDepth && len(spec.Subsystems) > 0 {
			return fmt.Errorf(
				"system %q cannot define nested subsystems; only one subsystem level is supported",
				groupName,
			)
		}

		groupCmd := &cobra.Command{
			Use:   spec.Name,
			Short: withSystemCommandPrefix(spec.Description),
			Args:  cobra.NoArgs,
		}

		if spec.YAMLFile != "" {
			if err := attachYAMLActions(groupCmd, spec, deps, yamlFS, groupName); err != nil {
				return err
			}
		}

		if spec.RegisterGoActions != nil {
			if err := spec.RegisterGoActions(groupCmd, deps); err != nil {
				return fmt.Errorf("failed to register Go actions for system %q: %w", groupName, err)
			}
		}

		if len(spec.Subsystems) > 0 {
			if err := registerCommandGroups(
				groupCmd,
				spec.Subsystems,
				deps,
				yamlFS,
				depth+1,
				groupPath,
			); err != nil {
				return err
			}
		}

		setDefaultNoArgs(groupCmd)

		if err := validateUniqueChildCommands(groupCmd, groupName); err != nil {
			return err
		}

		if len(groupCmd.Commands()) == 0 {
			continue
		}

		if err := addUniqueChildCommand(parent, groupCmd, commandGroupName(parentPath)); err != nil {
			return err
		}
	}

	return nil
}

func commandGroupName(path []string) string {
	if len(path) == 0 {
		return "root"
	}
	return strings.Join(path, ".")
}

func withSystemCommandPrefix(short string) string {
	if short == "" || strings.HasPrefix(short, systemCommandPrefix) {
		return short
	}
	return systemCommandPrefix + short
}

func attachYAMLActions(
	parent *cobra.Command,
	spec systemcmd.SystemSpec,
	deps systemcmd.BuildDeps,
	yamlFS fs.FS,
	commandGroup string,
) error {
	sys, err := syslib.LoadSystemFromFS(yamlFS, spec.YAMLFile)
	if err != nil {
		return fmt.Errorf("failed to load action definitions for system %q: %w", commandGroup, err)
	}
	if sys.Name != spec.Name {
		return fmt.Errorf(
			"system spec %q does not match YAML system %q in %s",
			commandGroup,
			sys.Name,
			spec.YAMLFile,
		)
	}
	if parent.Short == "" {
		parent.Short = sys.Description
	}

	for i := range sys.Actions {
		action := &sys.Actions[i]
		inputSpec, err := syslib.BuildActionInputSpec(action)
		if err != nil {
			if deps.WarnWriter != nil {
				_, _ = fmt.Fprintf(
					deps.WarnWriter,
					"warning: skip registering action %s.%s: %s\n",
					commandGroup,
					action.Name,
					err,
				)
			}
			continue
		}

		if err := addUniqueChildCommand(
			parent,
			buildYAMLActionCmd(sys, action, inputSpec, deps),
			commandGroup,
		); err != nil {
			return err
		}
	}

	return nil
}

func addUniqueChildCommand(parent, child *cobra.Command, systemName string) error {
	if child == nil {
		return fmt.Errorf("system %q cannot register a nil child command", systemName)
	}

	childName := child.Name()
	if childName == "" {
		return fmt.Errorf("system %q cannot register a child command with an empty name", systemName)
	}

	for _, existing := range parent.Commands() {
		if existing.Name() == childName {
			return fmt.Errorf("system %q has duplicate child command %q", systemName, childName)
		}
	}

	parent.AddCommand(child)
	return nil
}

func validateUniqueChildCommands(parent *cobra.Command, systemName string) error {
	seen := make(map[string]struct{}, len(parent.Commands()))
	for _, child := range parent.Commands() {
		childName := child.Name()
		if childName == "" {
			return fmt.Errorf("system %q cannot register a child command with an empty name", systemName)
		}
		if _, exists := seen[childName]; exists {
			return fmt.Errorf("system %q has duplicate child command %q", systemName, childName)
		}
		seen[childName] = struct{}{}
	}

	return nil
}

func buildYAMLActionCmd(
	sys *syslib.System,
	action *syslib.Action,
	inputSpec *syslib.ActionInputSpec,
	deps systemcmd.BuildDeps,
) *cobra.Command {
	var examples strings.Builder
	for _, ex := range action.Examples {
		examples.WriteString("  ")
		examples.WriteString(ex)
		examples.WriteByte('\n')
	}

	stage := "prod"
	body := ""
	headers := []string{}
	bodySchemaHelp := false
	bodyUsage := "[Optional] JSON request body"
	if action.BodyRequired {
		bodyUsage = "[Required] JSON request body"
	}

	cmd := &cobra.Command{
		Use:     action.Name,
		Short:   action.Description,
		Long:    action.Description,
		Example: examples.String(),
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if bodySchemaHelp {
				return fmt.Errorf(
					"--%s is a help modifier; use -h --%s",
					syslib.ActionBodySchemaFlagName,
					syslib.ActionBodySchemaFlagName,
				)
			}

			if err := syslib.ValidateRequiredBody(action, cmd); err != nil {
				return err
			}

			runtime, err := systemcmd.ResolveRuntime(deps)
			if err != nil {
				return err
			}

			return syslib.RunAction(action, inputSpec, sys.GatewayName, cmd, runtime, stage)
		},
	}

	cmd.Flags().StringVar(&stage, syslib.ActionStageFlagName, "prod", "[Optional] API gateway stage")
	cmd.Flags().StringVar(&body, syslib.ActionBodyFlagName, "", bodyUsage)
	cmd.Flags().StringArrayVar(
		&headers,
		syslib.ActionHeaderFlagName,
		nil,
		buildHeaderFlagUsage(inputSpec),
	)
	if action.BodySchema != "" {
		cmd.Flags().BoolVar(
			&bodySchemaHelp,
			syslib.ActionBodySchemaFlagName,
			false,
			"Show request body schema; use with -h",
		)
		cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
			if bodySchemaHelp {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Request body schema:")
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), action.BodySchema)
				return
			}

			writeYAMLActionHelpWithSchemaHint(cmd)
		})
	}

	syslib.RegisterActionFlags(cmd, inputSpec)

	return cmd
}

func writeYAMLActionHelpWithSchemaHint(cmd *cobra.Command) {
	out := cmd.OutOrStdout()

	_, _ = fmt.Fprintf(out, "Usage:\n  %s\n", cmd.UseLine())

	if cmd.Example != "" {
		_, _ = fmt.Fprintf(out, "\nExamples:\n%s", cmd.Example)
	}

	_, _ = fmt.Fprintln(out, "\nRequest body schema:")
	_, _ = fmt.Fprintln(out, "  Run with -h --body-schema to show the full schema.")

	if cmd.HasAvailableLocalFlags() {
		_, _ = fmt.Fprintf(
			out,
			"\nFlags:\n%s\n",
			strings.TrimRight(cmd.LocalFlags().FlagUsages(), "\n"),
		)
	}

	if cmd.HasAvailableInheritedFlags() {
		_, _ = fmt.Fprintf(
			out,
			"\nGlobal Flags:\n%s\n",
			strings.TrimRight(cmd.InheritedFlags().FlagUsages(), "\n"),
		)
	}
}

func buildHeaderFlagUsage(inputSpec *syslib.ActionInputSpec) string {
	usage := "[Optional] Additional headers (key:value, repeatable; auth/tenant overrides allowed)"
	if inputSpec == nil || len(inputSpec.HeaderParams) == 0 {
		return usage
	}

	examples := make([]string, 0, len(inputSpec.HeaderParams))
	for _, p := range inputSpec.HeaderParams {
		examples = append(examples, fmt.Sprintf(`--header "%s:value_example"`, p.Name))
	}

	return usage + ". YAML header params: " + strings.Join(examples, ", ")
}

func setDefaultNoArgs(cmd *cobra.Command) {
	if cmd == nil {
		return
	}
	if cmd.Args == nil {
		cmd.Args = cobra.NoArgs
	}
	for _, child := range cmd.Commands() {
		setDefaultNoArgs(child)
	}
}
