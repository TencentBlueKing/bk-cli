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
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/TencentBlueKing/bk-cli/internal/config"
	"github.com/TencentBlueKing/bk-cli/internal/output"
	syslib "github.com/TencentBlueKing/bk-cli/internal/system"
	systemcmd "github.com/TencentBlueKing/bk-cli/internal/systemcmd"
)

func newEnvsCmd(deps systemcmd.BuildDeps) *cobra.Command {
	path := configPath()
	cmd := &cobra.Command{
		Use: "envs", Short: "Show configured query environments", Args: cobra.NoArgs,
		Example: "  bk-cli monitor envs --config ./monitor.json",
		RunE: func(cmd *cobra.Command, _ []string) error {
			runtime, err := systemcmd.ResolveRuntime(deps)
			if err != nil {
				return err
			}
			root, err := readProfiles(path, cmd.Flags().Changed("config"))
			if err != nil {
				return inputError(err)
			}
			return output.SuccessData(map[string]any{
				"context":     runtime.ContextName,
				"default_env": root.DefaultEnv, "environments": root.Environments,
			}).WriteJSON(cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&path, "config", configPath(), "Monitor environment JSON")
	return cmd
}

type check struct {
	Environment string `json:"environment"`
	Resource    string `json:"resource"`
	Source      string `json:"source,omitempty"`
	Status      string `json:"status"`
	Rows        int    `json:"rows"`
	TraceID     string `json:"trace_id,omitempty"`
	Error       string `json:"error,omitempty"`
}

type probe struct{ kind, source, table, space, expression string }

func newStatusCmd(deps systemcmd.BuildDeps) *cobra.Command {
	var path, environment, stage, timeout string
	var headers []string
	var local bool
	cmd := &cobra.Command{
		Use: "status", Aliases: []string{"verify"}, Args: cobra.NoArgs,
		Short: "Check credentials and all configured query resources",
		Long: "Check every configured environment unless --env is explicit.\n" +
			"Checks continue after a failure; summaries contain counts and trace IDs, never log or span payloads.\n" +
			"empty means a successful query without recent data. --local does not contact the gateway.",
		Example: "  bk-cli monitor status\n  bk-cli monitor status --env dev\n  bk-cli monitor status --local",
		RunE: func(cmd *cobra.Command, _ []string) error {
			runtime, err := systemcmd.ResolveRuntime(deps)
			if err != nil {
				return err
			}
			root, err := readProfiles(path, cmd.Flags().Changed("config"))
			if err != nil {
				return inputError(err)
			}
			names := environmentNames(root, environment)
			if local {
				_, credentialErr := os.Stat(config.CredentialsPath(runtime.ContextName))
				return output.SuccessData(map[string]any{
					"context": runtime.ContextName, "local_only": true, "verified": false,
					"credential_file_present": credentialErr == nil, "environments": names,
				}).WriteJSON(cmd.OutOrStdout())
			}
			return verifyProfiles(cmd, runtime, root, names, stage, timeout, headers)
		},
	}
	cmd.Flags().StringVar(&path, "config", configPath(), "Monitor environment JSON")
	cmd.Flags().StringVar(&environment, "env", "", "Check only this environment (otherwise all)")
	cmd.Flags().BoolVar(&local, "local", false, "Inspect local setup without gateway calls")
	cmd.Flags().StringVar(&timeout, "timeout", "15s", "Timeout for each resource check")
	systemcmd.AddCommonRequestFlagsWithoutBody(cmd, &stage, &headers)
	return cmd
}

func environmentNames(root profile, environment string) []string {
	if environment != "" {
		return []string{environment}
	}
	names := make([]string, 0, len(root.Environments))
	for name := range root.Environments {
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) == 0 {
		return []string{""}
	}
	return names
}

func resourceProbes(p profile) []probe {
	probes := []probe{{kind: "query", expression: "vector(1)", space: p.space("query", "")}}
	names := make([]string, 0, len(p.LogTables))
	for name := range p.LogTables {
		names = append(names, name)
	}
	sort.Strings(names)
	covered := false
	for _, name := range names {
		table := p.LogTables[name]
		probes = append(probes, probe{kind: "logs", source: name, table: table, space: p.space("logs", "")})
		covered = covered || table == p.LogTable
	}
	if p.LogTable != "" && !covered {
		probes = append(
			probes,
			probe{kind: "logs", source: "default", table: p.LogTable, space: p.space("logs", "")},
		)
	}
	if p.TraceTable != "" {
		probes = append(probes, probe{kind: "trace", table: p.TraceTable, space: p.space("trace", "")})
	}
	if p.MetricsNamespace != "" {
		namespace := strconv.Quote(p.MetricsNamespace)
		probes = append(probes,
			probe{kind: "metrics", space: p.space("metrics", "")},
			probe{
				kind: "query", space: p.space("query", ""),
				expression: "sum(kube_pod_info{namespace=" + namespace + "})",
			},
		)
	}
	return probes
}

func verifyProfiles(cmd *cobra.Command, runtime *syslib.Runtime, root profile, names []string,
	stage, timeout string, headers []string,
) error {
	checks := []check{}
	summary := map[string]int{"passed": 0, "empty": 0, "failed": 0, "total": 0}
	record := func(c check) { checks = append(checks, c); summary[c.Status]++; summary["total"]++ }
	for _, name := range names {
		p, err := selectProfile(root, name, false)
		if err != nil {
			record(check{Environment: name, Resource: "config", Status: "failed", Error: err.Error()})
			continue
		}
		probes := resourceProbes(p)
		for _, probe := range probes {
			spec, err := probeRequest(
				runtime,
				p,
				probe,
				stage,
				cmd.Flags().Changed("stage"),
				timeout,
				headers,
			)
			if err != nil {
				record(
					check{
						Environment: name,
						Resource:    probe.kind,
						Source:      probe.source,
						Status:      "failed",
						Error:       err.Error(),
					},
				)
				continue
			}
			result, err := syslib.ExecuteRequest(runtime, spec)
			if err == nil {
				err = systemcmd.EnsureEnvelope("status", result.Envelope)
			}
			if err != nil {
				record(
					check{
						Environment: name,
						Resource:    probe.kind,
						Source:      probe.source,
						Status:      "failed",
						Error:       err.Error(),
					},
				)
				continue
			}
			if runtime.DryRun {
				result.Envelope.Data = map[string]any{
					"environment": name,
					"resource":    probe.kind,
					"note":        "Preview of first check; status checks all configured environments without --env",
				}
				return result.Envelope.WriteJSON(cmd.OutOrStdout())
			}
			data, err := queryData(result.Envelope, runtime, spec.Headers)
			c := check{Environment: name, Resource: probe.kind, Source: probe.source, Status: "passed"}
			if err == nil {
				c.Rows, c.TraceID, err = probeSummary(data, probe)
			}
			if err != nil {
				c.Status = "failed"
				c.Error = err.Error()
			} else if c.Rows == 0 {
				c.Status = "empty"
			}
			record(c)
		}
	}
	env := output.SuccessData(map[string]any{
		"context": runtime.ContextName, "verified": summary["failed"] == 0,
		"checks": checks, "summary": summary,
		"note": "empty means a successful query without recent data; each environment is checked separately",
	})
	if err := env.WriteJSON(cmd.OutOrStdout()); err != nil {
		return err
	}
	if summary["failed"] > 0 {
		return output.SystemError(
			"monitor_verification_failed",
			fmt.Sprintf(
				"%d of %d resource checks failed",
				summary["failed"],
				summary["total"],
			),
			"Inspect data.checks",
		)
	}
	return nil
}

func probeRequest(runtime *syslib.Runtime, p profile, probe probe, stage string, explicitStage bool,
	timeout string, headers []string,
) (syslib.RequestSpec, error) {
	var body map[string]any
	var err error
	kind := probe.kind
	if kind == "query" {
		now := time.Now().Unix()
		body = map[string]any{
			"promql": probe.expression, "start": strconv.FormatInt(now-900, 10),
			"end": strconv.FormatInt(now, 10), "step": "60s", "instant": probe.expression == "vector(1)",
		}
	} else {
		if kind == "trace" {
			kind = "logs"
		}
		body, err = buildQuery(kind, options{
			table: probe.table, field: "kube_pod_info", since: "1h",
			limit: 1, step: "60s", aggregate: "sum", groupBy: "namespace",
		}, nil, time.Now())
		if err != nil {
			return syslib.RequestSpec{}, err
		}
		if kind == "metrics" {
			if err := scopeMetrics(body, p.MetricsNamespace); err != nil {
				return syslib.RequestSpec{}, err
			}
		}
	}
	scoped, err := scopedHeaders(body, probe.space, headers)
	if err != nil {
		return syslib.RequestSpec{}, err
	}
	return p.request(runtime, probe.kind, stage, explicitStage, body, scoped, timeout)
}

func probeSummary(data map[string]any, probe probe) (int, string, error) {
	traceID, _ := data["trace_id"].(string)
	if probe.kind == "logs" || probe.kind == "trace" {
		list, ok := data["list"].([]any)
		if !ok {
			return 0, traceID, fmt.Errorf("raw query response is missing list")
		}
		return len(list), traceID, nil
	}
	series, ok := data["series"].([]any)
	if !ok {
		return 0, traceID, fmt.Errorf("metric query response is missing series")
	}
	rows := 0
	matched := false
	for _, raw := range series {
		s, ok := raw.(map[string]any)
		if !ok {
			return 0, traceID, fmt.Errorf("invalid query series")
		}
		values, ok := s["values"].([]any)
		if !ok {
			return 0, traceID, fmt.Errorf("query series is missing values")
		}
		rows += len(values)
		columns, _ := s["columns"].([]any)
		for i, column := range columns {
			if column != "_value" {
				continue
			}
			for _, rawRow := range values {
				row, ok := rawRow.([]any)
				if ok && len(row) > i && row[i] == json.Number("1") {
					matched = true
				}
			}
		}
	}
	if probe.expression == "vector(1)" && !matched {
		return 0, traceID, fmt.Errorf("vector(1) did not return the expected value")
	}
	return rows, traceID, nil
}
