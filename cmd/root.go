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

// Package cmd wires the root Cobra command and top-level CLI flags.
package cmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	flag "github.com/spf13/pflag"

	apicmd "github.com/TencentBlueKing/bk-cli/cmd/api"
	authcmd "github.com/TencentBlueKing/bk-cli/cmd/auth"
	ctxcmd "github.com/TencentBlueKing/bk-cli/cmd/context"
	syscmd "github.com/TencentBlueKing/bk-cli/cmd/system"
	updatecmd "github.com/TencentBlueKing/bk-cli/cmd/update"
	internalapi "github.com/TencentBlueKing/bk-cli/internal/api"
	"github.com/TencentBlueKing/bk-cli/internal/output"
)

const (
	rootCommandPrefix   = "[root] "
	systemCommandPrefix = "[system] "
)

var (
	buildInfo    = BuildInfo{Version: "dev", CommitID: "unknown", BuildTime: "unknown"}
	rootContext  string
	rootDryRun   bool
	rootVerbose  bool
	rootInsecure bool
)

// BuildInfo contains build metadata reported by `bk-cli version`.
type BuildInfo struct {
	Version   string
	CommitID  string
	BuildTime string
}

func normalizeBuildInfo(info BuildInfo) BuildInfo {
	if info.Version == "" {
		info.Version = "dev"
	}
	if info.CommitID == "" {
		info.CommitID = "unknown"
	}
	if info.BuildTime == "" {
		info.BuildTime = "unknown"
	}
	return info
}

// SetBuildInfo sets the CLI build metadata (called from main.go).
func SetBuildInfo(info BuildInfo) {
	buildInfo = normalizeBuildInfo(info)
	internalapi.SetUserAgentVersion(buildInfo.Version)
}

// SetVersion sets only the CLI version while preserving other build metadata.
func SetVersion(v string) {
	info := buildInfo
	info.Version = v
	SetBuildInfo(info)
}

// GetVersion returns the current CLI version.
func GetVersion() string {
	return buildInfo.Version
}

// GetContext returns the --context flag value.
func GetContext() string {
	return rootContext
}

// IsDryRun returns the --dry-run flag value.
func IsDryRun() bool {
	return rootDryRun
}

// IsVerbose returns the -v/--verbose flag value.
func IsVerbose() bool {
	return rootVerbose
}

// IsInsecure returns the --insecure flag value.
func IsInsecure() bool {
	return rootInsecure
}

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bk-cli",
		Short: "BlueKing platform CLI for agents and automation",
		Long: `bk-cli is a command-line tool for interacting with BlueKing platform APIs.
Designed for agents and automation with structured JSON output,
multi-context support, and rich help with examples.

Examples:
  # First-run setup
  bk-cli context init --bk_api_url_tmpl="https://bkapi.example.com/api/{gateway_name}/"

  # Login to store credentials
  bk-cli auth login --bk_app_code="app" --bk_app_secret="secret" --bk_token="tok"

  # Make a raw API call
  bk-cli api bk-apigateway GET /api/v2/open/gateways/

  # Use a system subcommand
  bk-cli apigateway list_gateways --name bk-iam

  # Manage contexts for different deployments
  bk-cli context create clouds --bk_api_url_tmpl="https://bkapi.clouds.example.com/api/{gateway_name}/"
  bk-cli context use clouds

403 guidance:
  - API gateway 403: if X-Bkapi-Error-Code is 1640301 or the message says
    "App has no permission", ask to apply API permission:
    bk_app_code - gateway_name - api_name/method/url
  - Business system 403: if the upstream body returns a business code such as
    bk_error_code 9900403 (IAM permission error), ask to apply business
    permission instead of API permission`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.PersistentFlags().StringVar(&rootContext, "context", "", "Override active context")
	cmd.PersistentFlags().BoolVar(&rootDryRun, "dry-run", false, "Preview without executing")
	cmd.PersistentFlags().BoolVarP(&rootVerbose, "verbose", "v", false, "Detailed logging to stderr")
	cmd.PersistentFlags().BoolVar(
		&rootInsecure,
		"insecure",
		false,
		"Skip TLS certificate verification for HTTPS requests",
	)

	// Add built-in root commands with an explicit help label.
	cmd.AddCommand(markRootCommand(newVersionCmd()))
	cmd.AddCommand(markRootCommand(authcmd.NewAuthCmd()))
	cmd.AddCommand(markRootCommand(apicmd.NewAPICmd(GetContext, IsDryRun, IsVerbose, IsInsecure)))
	cmd.AddCommand(markRootCommand(ctxcmd.NewContextCmd()))
	cmd.AddCommand(markRootCommand(updatecmd.NewUpdateCmd(GetVersion, IsDryRun)))

	// Register YAML-driven system subcommands
	if err := syscmd.RegisterAll(cmd, GetContext, IsDryRun, IsVerbose, IsInsecure); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to load system commands: %v\n", err)
	}

	// Initialize Cobra's built-in top-level commands so help rendering is stable
	// whether callers use cmd.Help() directly or execute with -h/--help.
	cmd.InitDefaultHelpCmd()
	cmd.InitDefaultCompletionCmd()
	markTopLevelRootCommands(cmd)

	return cmd
}

func markRootCommand(cmd *cobra.Command) *cobra.Command {
	if cmd == nil {
		return nil
	}
	if cmd.Short != "" && !strings.HasPrefix(cmd.Short, rootCommandPrefix) {
		cmd.Short = rootCommandPrefix + cmd.Short
	}
	return cmd
}

func markTopLevelRootCommands(parent *cobra.Command) {
	for _, child := range parent.Commands() {
		if strings.HasPrefix(child.Short, systemCommandPrefix) {
			continue
		}
		markRootCommand(child)
	}
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show version build information",
		Long: `Show the CLI version and build metadata.

Examples:
  bk-cli version`,
		RunE: func(cmd *cobra.Command, args []string) error {
			data := map[string]any{
				"version":    buildInfo.Version,
				"commit_id":  buildInfo.CommitID,
				"build_time": buildInfo.BuildTime,
			}
			return output.SuccessData(data).WriteJSON(cmd.OutOrStdout())
		},
	}
}

// Execute runs the root command.
func Execute() error {
	return executeRoot(newRootCmd(), os.Args[1:])
}

func executeRoot(root *cobra.Command, args []string) error {
	args = normalizeSystemCommandBoolArgs(root, args)
	if err := validateSystemCommandArgs(root, args); err != nil {
		return err
	}

	root.SetArgs(args)
	return root.Execute()
}

func normalizeSystemCommandBoolArgs(root *cobra.Command, args []string) []string {
	cmd, remainingArgs, err := root.Find(args)
	if err != nil || !isSystemCommandPath(cmd) || len(remainingArgs) == 0 {
		return args
	}

	commandArgsLen := len(args) - len(remainingArgs)
	if commandArgsLen < 0 || !stringSlicesEqual(args[commandArgsLen:], remainingArgs) {
		return args
	}

	normalizedRemainingArgs, changed := normalizeBoolFlagValues(cmd, remainingArgs)
	if !changed {
		return args
	}

	normalizedArgs := make([]string, 0, commandArgsLen+len(normalizedRemainingArgs))
	normalizedArgs = append(normalizedArgs, args[:commandArgsLen]...)
	normalizedArgs = append(normalizedArgs, normalizedRemainingArgs...)
	return normalizedArgs
}

func normalizeBoolFlagValues(cmd *cobra.Command, args []string) ([]string, bool) {
	cmd.InitDefaultHelpFlag()

	normalized := make([]string, 0, len(args))
	changed := false

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			normalized = append(normalized, args[i:]...)
			break
		}
		if strings.HasPrefix(arg, "--") {
			name, hasValue := splitLongFlag(arg)
			knownFlag := lookupLongFlag(cmd, name)
			if hasValue || knownFlag == nil || !isBoolFlag(knownFlag) || i+1 >= len(args) {
				normalized = append(normalized, arg)
				continue
			}
			if _, err := strconv.ParseBool(args[i+1]); err != nil {
				normalized = append(normalized, arg)
				continue
			}

			normalized = append(normalized, arg+"="+args[i+1])
			changed = true
			i++
			continue
		}
		if strings.HasPrefix(arg, "-") && arg != "-" && len(arg) == 2 {
			knownFlag := lookupShortFlag(cmd, arg[1:])
			if knownFlag == nil || !isBoolFlag(knownFlag) || i+1 >= len(args) {
				normalized = append(normalized, arg)
				continue
			}
			if _, err := strconv.ParseBool(args[i+1]); err != nil {
				normalized = append(normalized, arg)
				continue
			}

			normalized = append(normalized, arg+"="+args[i+1])
			changed = true
			i++
			continue
		}

		normalized = append(normalized, arg)
	}

	return normalized, changed
}

func isBoolFlag(f *flag.Flag) bool {
	return f != nil && f.NoOptDefVal != ""
}

func stringSlicesEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func validateSystemCommandArgs(root *cobra.Command, args []string) error {
	cmd, remainingArgs, err := root.Find(args)
	if err != nil {
		return err
	}
	if !isSystemCommandPath(cmd) {
		return nil
	}

	positionals, ok := remainingPositionals(cmd, remainingArgs)
	if !ok {
		return nil
	}
	if len(positionals) == 0 {
		return nil
	}

	return fmt.Errorf("unknown command %q for %q", positionals[0], cmd.CommandPath())
}

func isSystemCommandPath(cmd *cobra.Command) bool {
	for current := cmd; current != nil; current = current.Parent() {
		if strings.HasPrefix(current.Short, systemCommandPrefix) {
			return true
		}
	}
	return false
}

func remainingPositionals(cmd *cobra.Command, args []string) ([]string, bool) {
	cmd.InitDefaultHelpFlag()

	positionals := make([]string, 0, len(args))
	skipNext := false

	for i, arg := range args {
		if skipNext {
			skipNext = false
			continue
		}
		if arg == "--" {
			positionals = append(positionals, args[i+1:]...)
			break
		}
		if strings.HasPrefix(arg, "--") {
			name, hasValue := splitLongFlag(arg)
			knownFlag := lookupLongFlag(cmd, name)
			if knownFlag == nil {
				return nil, false
			}
			if !hasValue && knownFlag.NoOptDefVal == "" {
				skipNext = true
			}
			continue
		}
		if strings.HasPrefix(arg, "-") && arg != "-" {
			if len(arg) != 2 {
				return nil, false
			}
			knownFlag := lookupShortFlag(cmd, arg[1:])
			if knownFlag == nil {
				return nil, false
			}
			if knownFlag.NoOptDefVal == "" {
				skipNext = true
			}
			continue
		}

		positionals = append(positionals, arg)
	}

	return positionals, true
}

func splitLongFlag(arg string) (string, bool) {
	name := strings.TrimPrefix(arg, "--")
	if before, _, found := strings.Cut(name, "="); found {
		return before, true
	}
	return name, false
}

func lookupLongFlag(cmd *cobra.Command, name string) *flag.Flag {
	return cmd.Flag(name)
}

func lookupShortFlag(cmd *cobra.Command, shorthand string) *flag.Flag {
	for current := cmd; current != nil; current = current.Parent() {
		if f := current.Flags().ShorthandLookup(shorthand); f != nil {
			return f
		}
		if f := current.PersistentFlags().ShorthandLookup(shorthand); f != nil {
			return f
		}
	}
	return nil
}
