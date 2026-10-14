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

	"github.com/spf13/cobra"

	"github.com/TencentBlueKing/bk-cli/internal/api"
	"github.com/TencentBlueKing/bk-cli/internal/output"
	syslib "github.com/TencentBlueKing/bk-cli/internal/system"
	systemcmd "github.com/TencentBlueKing/bk-cli/internal/systemcmd"
)

type queryFlags struct {
	options
	config, environment, namespace, logSource, space, expression, stage, body, timeout string
	headers                                                                            []string
}

func newQueryCmd(kind string, deps systemcmd.BuildDeps) *cobra.Command {
	o := &queryFlags{}
	cmd := &cobra.Command{
		Use: kind, Short: queryDescription(kind), Args: cobra.NoArgs,
		Long: "Query configured observability resources through the selected bk-cli context.\n" +
			"--body overrides synthesized query flags; --input accepts a complete UQ JSON file or stdin.\n" +
			"Metric namespace and space validation also apply to complete JSON input.\n" +
			"Results use the bk-cli envelope; failures, including partial UQ results, return nonzero.",
		RunE: func(cmd *cobra.Command, _ []string) error { return runQuery(cmd, kind, o, deps) },
	}
	cmd.Flags().StringVar(
		&o.config,
		"config",
		configPath(),
		"Monitor environment JSON (BK_CLI_MONITOR_CONFIG)",
	)
	cmd.Flags().StringVar(&o.environment, "env", "", "Project environment (BK_CLI_MONITOR_ENV or default_env)")
	cmd.Flags().StringVar(&o.space, "space-uid", "", "Override query space (BK_CLI_MONITOR_SPACE_UID)")
	cmd.Flags().StringVar(&o.input, "input", "", "Complete UQ JSON file, or - for stdin")
	cmd.Flags().StringVar(&o.since, "since", "1h", "Lookback duration")
	cmd.Flags().StringVar(&o.start, "start", "", "Start: Unix seconds or RFC3339")
	cmd.Flags().StringVar(&o.end, "end", "", "End: Unix seconds or RFC3339 (default now)")
	cmd.Flags().StringVar(&o.timeout, "timeout", "", "Request timeout (defaults to context)")
	systemcmd.AddCommonRequestFlags(cmd, &o.stage, &o.body, &o.headers)
	cmd.MarkFlagsMutuallyExclusive("input", "body")
	cmd.MarkFlagsMutuallyExclusive("start", "since")
	if kind == "metrics" || kind == "query" {
		cmd.Flags().StringVar(&o.namespace, "namespace", "", "Metric namespace override")
		cmd.Flags().StringVar(&o.step, "step", "60s", "Query interval")
		cmd.Flags().BoolVar(&o.instant, "instant", false, "Instant query")
	}
	if kind == "query" {
		cmd.Aliases = []string{"promql"}
		cmd.Flags().StringVar(
			&o.expression,
			"promql",
			"",
			"PromQL; namespace={{namespace}} expands to a quoted value",
		)
		cmd.Flags().StringVar(&o.expression, "query", "", "Alias for --promql")
		cmd.MarkFlagsMutuallyExclusive("promql", "query")
		cmd.Example = "  bk-cli monitor query --env dev --promql 'sum(kube_pod_info{namespace={{namespace}}})'"
	} else {
		cmd.Flags().StringVar(&o.table, "table", "", "Override configured result table")
		cmd.Flags().IntVar(&o.limit, "limit", 100, "Maximum results per page")
		cmd.Flags().StringArrayVar(&o.filters, "where", nil, "Equality filter FIELD=VALUE; repeat for AND")
		if kind == "metrics" {
			cmd.Aliases = []string{"queryTs", "query-ts"}
			cmd.Flags().StringVar(&o.field, "field", "", "Metric field")
			cmd.Flags().StringVar(&o.aggregate, "aggregate", "", "Aggregation method, such as sum")
			cmd.Flags().StringVar(&o.groupBy, "group-by", "", "Comma-separated aggregation dimensions")
			cmd.Example = "  bk-cli monitor metrics --env dev --field kube_pod_info --aggregate sum --group-by namespace"
		} else {
			cmd.Flags().StringVar(&o.query, "query", "", "Storage query_string")
			cmd.Flags().IntVar(&o.offset, "from", 0, "Page offset (no automatic pagination)")
			cmd.Flags().StringVar(&o.orderBy, "order-by", "", "Comma-separated sort fields")
			if kind == "logs" {
				cmd.Aliases = []string{"queryRaw", "query-raw"}
				cmd.Flags().StringVar(
					&o.logSource,
					"log-source",
					"",
					"Named log source from log_tables",
				)
				cmd.Example = "  bk-cli monitor logs --env dev --query ERROR --since 30m --limit 20"
			} else {
				cmd.Flags().StringVar(&o.traceID, "id", "", "Trace ID")
				cmd.Flags().StringVar(&o.traceField, "trace-field", "trace_id", "Trace ID field")
				cmd.Example = "  bk-cli monitor trace --env prod --id TRACE_ID --since 1h"
			}
		}
	}
	return cmd
}

func runQuery(cmd *cobra.Command, kind string, o *queryFlags, deps systemcmd.BuildDeps) error {
	root, err := readProfiles(o.config, cmd.Flags().Changed("config"))
	if err != nil {
		return inputError(err)
	}
	p, err := selectProfile(root, o.environment, true)
	if err != nil {
		return inputError(err)
	}
	body, err := queryBody(cmd, kind, o, p)
	if err != nil {
		return inputError(err)
	}
	space := p.space(kind, o.space)
	headers, err := scopedHeaders(body, space, o.headers)
	if err != nil {
		return inputError(err)
	}
	runtime, err := systemcmd.ResolveRuntime(deps)
	if err != nil {
		return err
	}
	spec, err := p.request(runtime, kind, o.stage, cmd.Flags().Changed("stage"), body, headers, o.timeout)
	if err != nil {
		return inputError(err)
	}
	result, err := syslib.ExecuteRequest(runtime, spec)
	if err != nil {
		return err
	}
	if err := systemcmd.EnsureEnvelope(kind, result.Envelope); err != nil {
		return err
	}
	env := result.Envelope
	if env.DryRun {
		return env.WriteJSON(cmd.OutOrStdout())
	}
	data, queryErr := queryData(env, runtime, spec.Headers)
	env.Data = data
	if queryErr == nil || data != nil {
		if err := env.WriteJSON(cmd.OutOrStdout()); err != nil {
			return err
		}
	}
	if queryErr != nil {
		return output.SystemError(
			"monitor_query_failed",
			queryErr.Error(),
			"Inspect the query status and narrow the time range",
		)
	}
	return nil
}

func queryBody(cmd *cobra.Command, kind string, o *queryFlags, p profile) (map[string]any, error) {
	namespace := o.namespace
	if namespace == "" {
		namespace = p.MetricsNamespace
	}
	var body map[string]any
	var err error
	switch {
	case o.body != "":
		body, err = decodeObject(strings.NewReader(o.body))
	case o.input != "":
		if err := validateInputFlags(cmd); err != nil {
			return nil, err
		}
		body, err = readInput(o.input, cmd.InOrStdin())
	case kind == "query":
		body, err = buildPromQLBody(o, namespace)
	default:
		if err := selectTable(kind, o, p); err != nil {
			return nil, err
		}
		body, err = buildQuery(kind, o.options, cmd.InOrStdin(), time.Now())
	}
	if err != nil {
		return nil, err
	}
	if kind == "query" {
		return body, validatePromQLBody(body, namespace)
	}
	if err := validateQueryList(body); err != nil {
		return nil, err
	}
	if kind == "metrics" {
		return body, scopeMetrics(body, namespace)
	}
	return body, nil
}

func validateInputFlags(cmd *cobra.Command) error {
	for _, name := range []string{
		"table", "log-source", "query", "promql", "namespace", "since", "start", "end",
		"limit", "from", "where", "order-by", "id", "trace-field", "field", "step", "aggregate", "group-by", "instant",
	} {
		if cmd.Flags().Changed(name) {
			return fmt.Errorf("--input cannot be combined with --%s", name)
		}
	}
	return nil
}

func selectTable(kind string, o *queryFlags, p profile) error {
	if o.logSource != "" {
		if o.table != "" {
			return fmt.Errorf("--log-source and --table are mutually exclusive")
		}
		table, ok := p.LogTables[o.logSource]
		if !ok || table == "" {
			return fmt.Errorf("unknown or unconfigured log source %q", o.logSource)
		}
		o.table = table
	}
	if o.table == "" {
		if kind == "logs" {
			o.table = p.LogTable
		}
		if kind == "trace" {
			o.table = p.TraceTable
		}
	}
	return nil
}

func buildPromQLBody(o *queryFlags, namespace string) (map[string]any, error) {
	expression, err := expandPromQL(o.expression, namespace)
	if err != nil {
		return nil, err
	}
	start, end, err := timeRange(o.options, time.Now())
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"promql":  expression,
		"start":   start,
		"end":     end,
		"step":    o.step,
		"instant": o.instant,
	}, nil
}

func validatePromQLBody(body map[string]any, namespace string) error {
	expression, ok := body["promql"].(string)
	if !ok {
		return fmt.Errorf("PromQL input requires promql")
	}
	expanded, err := expandPromQL(expression, namespace)
	if err != nil {
		return err
	}
	body["promql"] = expanded
	for _, key := range []string{"start", "end", "step"} {
		if value, ok := body[key].(string); !ok || value == "" {
			return fmt.Errorf("PromQL input requires string %s", key)
		}
	}
	stepValue, _ := body["step"].(string)
	step, err := time.ParseDuration(stepValue)
	if err != nil || step <= 0 {
		return fmt.Errorf("--step must be a positive duration")
	}
	return nil
}

func scopedHeaders(body map[string]any, space string, headers []string) ([]string, error) {
	if strings.TrimSpace(space) == "" {
		return nil, fmt.Errorf("configure a query space or pass --space-uid")
	}
	if value, exists := body["space_uid"]; exists {
		if value != space {
			return nil, fmt.Errorf("input space_uid conflicts with configured space")
		}
		delete(body, "space_uid")
	}
	parsed, err := api.ParseHeaderFlags(headers)
	if err != nil {
		return nil, err
	}
	found := false
	for key, value := range parsed {
		if strings.EqualFold(key, "X-Bk-Scope-Skip-Space") {
			return nil, fmt.Errorf("X-Bk-Scope-Skip-Space is not supported")
		}
		if strings.EqualFold(key, "X-Bk-Scope-Space-Uid") {
			if value != space {
				return nil, fmt.Errorf("space header conflicts with configured space")
			}
			found = true
		}
	}
	if !found {
		headers = append(append([]string{}, headers...), "X-Bk-Scope-Space-Uid:"+space)
	}
	return headers, nil
}

func inputError(err error) error {
	return output.UserError("invalid_monitor_query", err.Error(), "See: bk-cli monitor --help")
}

func queryData(env *output.Envelope, runtime *syslib.Runtime, headers []string) (map[string]any, error) {
	raw, err := json.Marshal(env.Data)
	if err != nil {
		return nil, fmt.Errorf("invalid query response")
	}
	obj, err := decodeObject(strings.NewReader(string(raw)))
	if err != nil {
		return nil, fmt.Errorf("server returned invalid query JSON object")
	}
	secrets := responseSecrets(runtime, headers)
	for name, value := range env.Headers {
		if strings.EqualFold(name, "X-Bkapi-Authorization") {
			env.Headers[name] = "[REDACTED]"
		} else {
			env.Headers[name] = redactString(value, secrets)
		}
	}
	if !env.OK {
		return nil, fmt.Errorf("query HTTP status %d", env.Status)
	}
	if rejected(obj) {
		return nil, fmt.Errorf("query rejected by upstream")
	}
	if result, ok := obj["result"].(bool); ok && result {
		nested, ok := obj["data"].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("APIGW success response is missing query data")
		}
		obj = nested
		if rejected(obj) {
			return nil, fmt.Errorf("query rejected by Unify Query")
		}
	}
	// Interpret gateway and UQ control fields before redacting payload keys.
	// A credential can itself equal a protocol key such as "data" or "code".
	partial := isPartial(obj)
	redactQueryStrings(obj, secrets)
	if partial {
		return obj, fmt.Errorf("partial query result; inspect stdout data.status")
	}
	return obj, nil
}

func rejected(obj map[string]any) bool {
	if result, ok := obj["result"].(bool); ok && !result {
		return true
	}
	value, exists := obj["error"]
	return exists && value != nil && value != ""
}

func isPartial(obj map[string]any) bool {
	if partial, ok := obj["is_partial"].(bool); ok && partial {
		return true
	}
	status, ok := obj["status"].(map[string]any)
	return ok && status["code"] != nil && status["code"] != ""
}

func responseSecrets(runtime *syslib.Runtime, headers []string) []string {
	values := []string{}
	if runtime.Credential != nil {
		c := runtime.Credential
		values = append(values, c.BkAppSecret, c.BkToken, c.BkTicket, c.AccessToken)
	}
	parsed, _ := api.ParseHeaderFlags(headers)
	for name, value := range parsed {
		if strings.EqualFold(name, "Authorization") || strings.EqualFold(name, "Proxy-Authorization") {
			values = append(values, value)
			_, token, _ := strings.Cut(value, " ")
			values = append(values, token)
		}
		if strings.EqualFold(name, "X-Bkapi-Authorization") {
			values = append(values, value)
			var auth map[string]any
			if json.Unmarshal([]byte(value), &auth) == nil {
				for key, field := range auth {
					switch key {
					case "bk_app_secret", "bk_token", "bk_ticket", "access_token":
						if secret, ok := field.(string); ok {
							values = append(values, secret)
						}
					}
				}
			}
		}
	}
	return values
}

func redactQueryStrings(value any, secrets []string) {
	switch node := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(node))
		for key := range node {
			keys = append(keys, key)
		}
		for _, key := range keys {
			item := node[key]
			if text, ok := item.(string); ok {
				node[key] = redactString(text, secrets)
			} else {
				redactQueryStrings(item, secrets)
			}
			safeKey := redactString(key, secrets)
			if safeKey != key {
				node[safeKey] = node[key]
				delete(node, key)
			}
		}
	case []any:
		for i, item := range node {
			if text, ok := item.(string); ok {
				node[i] = redactString(text, secrets)
			} else {
				redactQueryStrings(item, secrets)
			}
		}
	}
}

func redactString(text string, secrets []string) string {
	for _, value := range secrets {
		if value != "" {
			text = strings.ReplaceAll(text, value, "[REDACTED]")
		}
	}
	return text
}

func queryDescription(kind string) string {
	switch kind {
	case "logs":
		return "Search logs with keywords and exact filters"
	case "trace":
		return "Find spans by trace ID"
	case "metrics":
		return "Query structured metrics with namespace filters"
	default:
		return "Run native PromQL range or instant queries"
	}
}
