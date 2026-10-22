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

// Package mcp implements the server-first MCP command without gateway context.
package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/TencentBlueKing/bk-cli/internal/config"
	"github.com/TencentBlueKing/bk-cli/internal/mcpclient"
	"github.com/TencentBlueKing/bk-cli/internal/output"
)

type options struct {
	config, body                                  string
	timeout                                       time.Duration
	check, tools, help, dryRun, verbose, insecure bool
	context                                       string
	server, tool                                  string
	named                                         []mcpclient.NamedArgument
}

const generalHelp = `Usage:
  bk-cli mcp [--config path] <server> [--check | --tools | <tool> [arguments]]

Configuration: $BK_CLI_CONFIG_DIR/mcp.json, otherwise ~/.bk-cli/mcp.json.
Only the selected server's configured headers are used; context credentials are never loaded.

Examples:
  bk-cli mcp report-prod --check
  bk-cli mcp report-prod --tools
  bk-cli mcp report-prod search --help
  bk-cli mcp report-prod search --body '{"keyword":"gateway"}'
  bk-cli mcp report-prod search --body @arguments.json
  bk-cli mcp report-prod search --body - < arguments.json
  bk-cli mcp report-prod search --body '{}' --dry-run

Options: --config, --timeout (default 60s), --jq, --verbose/-v, --insecure, --dry-run.
Remote tools may have side effects. Discovery success does not prove business permissions.
`

// NewCommand registers a local routing entrypoint; construction never reads
// configuration, credentials or remote schemas.
func NewCommand() *cobra.Command {
	o := &options{}
	cmd := &cobra.Command{
		Use:                "mcp",
		Short:              "Discover and call configured remote MCP tools",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		SilenceErrors:      true,
	}
	flags := cmd.Flags()
	flags.StringVar(&o.config, "config", "", "Use exactly this MCP JSON file")
	flags.StringVar(&o.body, "body", "", "Complete arguments JSON, @file, or - for stdin")
	flags.DurationVar(&o.timeout, "timeout", config.DefaultTimeout, "Total remote operation timeout")
	flags.BoolVar(&o.check, "check", false, "Check connection, protocol and complete tool discovery")
	flags.BoolVar(&o.tools, "tools", false, "List all tools and schemas")
	flags.BoolVarP(&o.help, "help", "h", false, "Show help without calling a tool")
	flags.BoolVar(&o.dryRun, "dry-run", false, "Preview a complete body without network access")
	flags.BoolVarP(&o.verbose, "verbose", "v", false, "Write masked diagnostics to stderr")
	flags.BoolVar(&o.insecure, "insecure", false, "Skip HTTPS certificate verification")
	flags.StringVar(&o.context, "context", "", "Unsupported for MCP")
	output.AddJQFlag(cmd)
	cmd.SetHelpFunc(func(cmd *cobra.Command, _ []string) { _, _ = fmt.Fprint(cmd.OutOrStdout(), generalHelp) })
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if err := o.parse(cmd, args); err != nil {
			return localError(cmd, err)
		}
		return o.run(cmd)
	}
	return cmd
}

func invalidArguments() *mcpclient.Error {
	return &mcpclient.Error{Code: "invalid_mcp_arguments", Message: "Invalid MCP command arguments; use --help"}
}

func (o *options) parse(cmd *cobra.Command, args []string) error {
	common := make([]string, 0)
	positionals := make([]string, 0, 2)
	seen := make(map[string]bool)
	for i := 0; i < len(args); i++ {
		token := args[i]
		if token == "--" {
			positionals = append(positionals, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(token, "-") {
			positionals = append(positionals, token)
			continue
		}
		name, value, hasValue := strings.Cut(strings.TrimPrefix(token, "--"), "=")
		var flag *pflag.Flag
		if strings.HasPrefix(token, "--") {
			flag = cmd.Flags().Lookup(name)
		} else {
			short, shortValue, equals := strings.Cut(strings.TrimPrefix(token, "-"), "=")
			if len(short) != 1 {
				return invalidArguments()
			}
			flag = cmd.Flags().ShorthandLookup(short)
			value, hasValue = shortValue, equals
		}
		if flag != nil {
			if seen[flag.Name] {
				return invalidArguments()
			}
			seen[flag.Name] = true
			if !hasValue {
				if flag.NoOptDefVal != "" {
					value = flag.NoOptDefVal
				} else {
					if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
						return invalidArguments()
					}
					i++
					value = args[i]
				}
			}
			common = append(common, "--"+flag.Name+"="+value)
			continue
		}
		if !strings.HasPrefix(token, "--") || mcpclient.ReservedParameter(name) {
			return &mcpclient.Error{Code: "unsupported_option", Message: "Option is not supported by MCP"}
		}
		if !hasValue {
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return invalidArguments()
			}
			i++
			value = args[i]
		}
		o.named = append(o.named, mcpclient.NamedArgument{Name: name, Value: value})
	}
	if err := cmd.Flags().Parse(common); err != nil {
		return invalidArguments()
	}
	return o.validateMode(cmd, positionals)
}

func (o *options) validateMode(cmd *cobra.Command, positionals []string) error {
	if cmd.Flags().Changed("context") ||
		(cmd.InheritedFlags().Lookup("context") != nil && cmd.InheritedFlags().Lookup("context").Changed) {
		return &mcpclient.Error{
			Code:    "unsupported_option",
			Message: "--context is not supported by MCP; authentication comes only from server headers",
		}
	}
	if len(positionals) > 2 || o.timeout <= 0 {
		return invalidArguments()
	}
	if len(positionals) > 0 {
		o.server = positionals[0]
	}
	if len(positionals) > 1 {
		o.tool = positionals[1]
	}
	modes := 0
	if o.check {
		modes++
	}
	if o.tools {
		modes++
	}
	if o.tool != "" {
		modes++
	}
	if modes > 1 || ((o.check || o.tools) && (cmd.Flags().Changed("body") || len(o.named) > 0 || o.dryRun)) {
		return invalidArguments()
	}
	if o.tool == "" && (len(o.named) > 0 || cmd.Flags().Changed("body") || o.dryRun) {
		return invalidArguments()
	}
	if o.server == "" && modes > 0 {
		return invalidArguments()
	}
	return nil
}

func (o *options) run(cmd *cobra.Command) error {
	if o.server == "" {
		_, err := fmt.Fprint(cmd.OutOrStdout(), generalHelp)
		return err
	}
	server, err := mcpclient.LoadServer(o.config, o.server)
	if err != nil {
		return localError(cmd, err)
	}
	if o.tool == "" && (o.help || (!o.check && !o.tools)) {
		_, err := fmt.Fprintf(
			cmd.OutOrStdout(),
			"Server: %s\nConfig: %s\n\n%s",
			server.Name,
			server.ConfigFile,
			generalHelp,
		)
		return err
	}
	headers, err := server.ResolveHeaders()
	if err != nil {
		return localError(cmd, err)
	}
	if o.dryRun && !o.help {
		if !cmd.Flags().Changed("body") || len(o.named) > 0 {
			return localError(cmd, invalidArguments())
		}
		args, err := mcpclient.ReadArguments(o.body, cmd.InOrStdin())
		if err != nil {
			return localError(cmd, err)
		}
		env := output.DryRun(
			&output.DryRunRequest{
				Method:  "POST",
				URL:     server.URL,
				Headers: server.MaskedHeaders(),
				Body:    map[string]any{"tool": o.tool, "arguments": args},
			},
		)
		env.Data = map[string]any{
			"server":          o.server,
			"tool":            o.tool,
			"verified_remote": false,
			"unverified": []string{
				"tool existence",
				"schema",
				"authentication validity",
				"business permissions",
			},
		}
		return writeResult(cmd, env)
	}
	if o.verbose {
		masked, marshalErr := json.Marshal(server.MaskedHeaders())
		if marshalErr != nil {
			return localError(cmd, marshalErr)
		}
		_, _ = fmt.Fprintf(
			cmd.ErrOrStderr(),
			"MCP server=%s config=%s headers=%s\n",
			server.Name,
			server.ConfigFile,
			masked,
		)
	}
	return o.execute(cmd, server, headers)
}

func (o *options) execute(cmd *cobra.Command, server *mcpclient.Server, headers http.Header) error {
	interrupted, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(interrupted, o.timeout)
	defer cancel()
	client := mcpclient.NewClient(ctx, server.URL, headers, o.insecure)
	defer client.Close()
	data := map[string]any{"server": o.server, "complete": false}
	checks := map[string]string{
		"config":     "passed",
		"connection": "not_run",
		"protocol":   "not_run",
		"tools":      "not_run",
	}
	if o.check {
		data["checks"] = checks
		data["config_file"] = server.ConfigFile
		data["transport"] = "http"
	}
	if err := client.Connect(); err != nil {
		checks["connection"] = "failed"
		if client.Connected() {
			checks["connection"] = "passed"
			checks["protocol"] = "failed"
		}
		return remoteFailure(cmd, client, data, err)
	}
	checks["connection"] = "passed"
	checks["protocol"] = "passed"
	data["protocol_version"] = client.ProtocolVersion()
	tools, err := client.Discover()
	if err != nil {
		checks["tools"] = "failed"
		return remoteFailure(cmd, client, data, err)
	}
	checks["tools"] = "passed"
	if o.check || o.tools {
		data["complete"] = true
		data["tool_count"] = len(tools)
		if o.tools {
			data["tools"] = tools
			data["arguments_help"] = "Reserved or complex properties require --body; use <tool> --help"
		} else {
			source := "none"
			if len(headers) > 0 {
				source = "server_headers"
			}
			data["authentication"] = map[string]any{"source": source, "discovery_accepted": true}
		}
		return writeResult(cmd, output.SuccessData(data))
	}
	tool := findTool(tools, o.tool)
	if tool == nil {
		return localError(
			cmd,
			&mcpclient.Error{
				Code:    "mcp_tool_not_found",
				Message: "Tool not found after complete discovery",
			},
		)
	}
	schema, _ := tool["inputSchema"].(map[string]any)
	if o.help {
		allowed := make(map[string]bool)
		for _, parameter := range mcpclient.Parameters(schema) {
			allowed[parameter.Name] = true
		}
		for _, argument := range o.named {
			if !allowed[argument.Name] {
				return localError(cmd, invalidArguments())
			}
			delete(allowed, argument.Name)
		}
		return toolHelp(cmd, tool, schema)
	}
	var body map[string]any
	if cmd.Flags().Changed("body") {
		body, err = mcpclient.ReadArguments(o.body, cmd.InOrStdin())
		if err != nil {
			return localError(cmd, err)
		}
	}
	arguments, err := mcpclient.BuildArguments(schema, o.named, body)
	if err != nil {
		return localError(cmd, err)
	}
	data = map[string]any{"server": o.server, "tool": o.tool}
	result, err := client.Call(o.tool, arguments)
	if result != nil {
		data["result"] = result
	}
	if err != nil {
		if client.CallAttempted() && !client.ResultKnown() {
			data["execution_state"] = "unknown"
		}
		return remoteFailure(cmd, client, data, err)
	}
	return writeResult(cmd, output.SuccessData(data))
}

func findTool(tools []map[string]any, name string) map[string]any {
	for _, tool := range tools {
		if tool["name"] == name {
			return tool
		}
	}
	return nil
}

func localError(cmd *cobra.Command, err error) error {
	var detail *mcpclient.Error
	if !errors.As(err, &detail) {
		detail = invalidArguments()
	}
	if writeErr := output.Err(
		detail.Code,
		detail.Message,
		"Run with --help for usage",
	).WriteJSON(
		cmd.ErrOrStderr(),
	); writeErr != nil {
		return writeErr
	}
	return &output.CLIError{ExitCode: 1, Code: detail.Code, Message: detail.Message}
}

func remoteFailure(cmd *cobra.Command, client *mcpclient.Client, data map[string]any, err error) error {
	var detail *mcpclient.Error
	if !errors.As(err, &detail) {
		detail = &mcpclient.Error{Code: "mcp_protocol_error", Message: "MCP operation failed"}
	}
	hint := "Inspect the selected server configuration and result"
	if data["execution_state"] == "unknown" {
		hint = "The server may have executed the tool. Verify through a business query before retrying"
	}
	env := output.Err(detail.Code, detail.Message, hint)
	env.Data = data
	env.Headers = client.Headers()
	if err := writeResult(cmd, env); err != nil {
		return err
	}
	return &output.CLIError{ExitCode: 2, Code: detail.Code, Message: detail.Message}
}

func writeResult(cmd *cobra.Command, env *output.Envelope) error {
	if err := output.WriteCommandEnvelope(cmd, env); err != nil {
		var cli *output.CLIError
		if errors.As(err, &cli) {
			return err
		}
		_ = output.Err(
			"output_error",
			"Result output failed; the request may already have completed",
			"Do not retry without checking the original result",
		).WriteJSON(
			cmd.ErrOrStderr(),
		)
		return &output.CLIError{ExitCode: 2, Code: "output_error", Message: "Result output failed"}
	}
	return nil
}

func toolHelp(cmd *cobra.Command, tool, schema map[string]any) error {
	_, _ = fmt.Fprintf(
		cmd.OutOrStdout(),
		"Tool: %s\nDescription: %v\n\nParameters (only explicit values are sent):\n",
		tool["name"],
		tool["description"],
	)
	for _, parameter := range mcpclient.Parameters(schema) {
		_, _ = fmt.Fprintf(
			cmd.OutOrStdout(),
			"  --%s <%s> required=%t %s\n",
			parameter.Name,
			parameter.Type,
			parameter.Required,
			parameter.Description,
		)
	}
	_, _ = fmt.Fprint(
		cmd.OutOrStdout(),
		"\nExamples: --body '{}' | --body @arguments.json | --body -\n"+
			"Reserved/complex properties require --body. Body replaces named arguments completely.\n"+
			"Input schema (includes required fields, enum and examples):\n",
	)
	encoder := json.NewEncoder(cmd.OutOrStdout())
	encoder.SetIndent("", "  ")
	return encoder.Encode(schema)
}
