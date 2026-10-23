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

// Package monitorapi calls individual Monitor HTTP APIs through the shared request executor.
// The server remains the source of truth for tool contracts, resource ownership and permissions.
package monitorapi

import (
	"errors"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	json "github.com/goccy/go-json"
	"github.com/spf13/cobra"

	"github.com/TencentBlueKing/bk-cli/internal/output"
	syslib "github.com/TencentBlueKing/bk-cli/internal/system"
	systemcmd "github.com/TencentBlueKing/bk-cli/internal/systemcmd"
)

const (
	gatewayName     = "bk-monitor"
	requestIDHeader = "X-Bkapi-Request-Id"
	maxWindow       = 24 * time.Hour
)

// Diagnostic endpoints use the same user-authenticated gateway policy as the tool APIs.
var authConfig = &syslib.AuthConfig{UserVerifiedRequired: true, ResourcePermissionRequired: true}

var (
	supported     = map[string]struct{}{}
	commandFor    = func(string) string { return "" }
	nextFor       = func(string, string) []string { return nil }
	supportedList []string
)

var resourceDecisions = map[string]bool{
	"needs_resource": true, "resource_not_visible": true, "resource_not_found": true,
}

// Server judgments that describe the target rather than a broken request.
var judgments = map[string]bool{
	"needs_resource": true, "resource_not_visible": true, "resource_not_found": true,
	"permission_missing": true, "permission_denied": true, "resource_blocked": true, "retry_later": true,
}

// Endpoint is the reviewed HTTP operation for one read-only command.
type Endpoint struct {
	Method      string            `json:"method"`
	Path        string            `json:"path"`
	QueryParams []string          `json:"query_params"`
	Auth        syslib.AuthConfig `json:"auth"`
}

var endpoints = map[string]Endpoint{}

// SetEndpoints installs routes from the reviewed API catalog, never from user input.
func SetEndpoints(routes map[string]Endpoint) { endpoints = routes }

// Runtime adds invocation-scoped request options without changing shared context or credentials.
type Runtime struct {
	*syslib.Runtime
	Stage   string
	Headers []string
}

// Bind preserves options for every request made by a command, including preflight requests.
func Bind(cmd *cobra.Command, rt *syslib.Runtime) *Runtime {
	stage, _ := cmd.Flags().GetString("stage")
	headers, _ := cmd.Flags().GetStringArray("header")
	return &Runtime{Runtime: rt, Stage: stage, Headers: headers}
}

// Reply is the unwrapped API data plus trace metadata.
type Reply struct {
	Data      any
	RequestID string
	dryRun    *output.Envelope
	envelope  *output.Envelope
}

// IsDryRun reports whether the reply is a local preview without a response.
func (r *Reply) IsDryRun() bool { return r != nil && r.dryRun != nil }

// Decision is a stable, user-facing translation of a local or server judgment.
type Decision struct {
	Code      string
	Message   string
	ExitCode  int
	RequestID string
	Next      []string
	AnyOf     []map[string]string
	Steps     []map[string]any
}

func (d *Decision) Error() string { return d.Code + ": " + d.Message }

// InputError reports a locally fixable input problem.
func InputError(code, message string, next ...string) *Decision {
	return &Decision{Code: code, Message: message, ExitCode: output.ExitCodeUserError, Next: next}
}

// SetCatalog installs the reviewed read-only tool list and command hints owned by the monitor system.
func SetCatalog(tools []string, command func(string) string, next func(string, string) []string) {
	supported = make(map[string]struct{}, len(tools))
	supportedList = append([]string(nil), tools...)
	for _, name := range tools {
		supported[name] = struct{}{}
	}
	commandFor, nextFor = command, next
}

// Supported reports whether a read-only tool is in the reviewed catalog.
func Supported(tool string) bool { _, ok := supported[tool]; return ok }

// SupportedTools returns accepted read-only tool names.
func SupportedTools() []string { return append([]string(nil), supportedList...) }

// Command returns the CLI command for a tool, if one exists.
func Command(tool string) string { return commandFor(tool) }

// NextCommands returns commands that produce the identifiers required by a tool.
func NextCommands(tool, biz string) []string { return nextFor(tool, biz) }

// Call requests an explicit diagnostic API; business commands use their own Endpoint.
func Call(rt *Runtime, name string, body map[string]any) (*Reply, error) {
	switch name {
	case "lookup_tool", "lookup_tool_schema", "lookup_permissions", "lookup_metadata":
		return request(rt, Endpoint{Method: "POST", Path: "/mcp/" + name + "/", Auth: *authConfig}, body)
	default:
		return nil, InputError("tool_not_allowed", "未知的诊断 API。")
	}
}

func request(rt *Runtime, endpoint Endpoint, args map[string]any) (*Reply, error) {
	query, body := map[string]any{}, map[string]any{}
	for key, value := range args {
		inQuery := endpoint.Method == http.MethodGet || slices.Contains(endpoint.QueryParams, key)
		if inQuery {
			query[key] = value
		} else {
			body[key] = value
		}
	}
	spec := syslib.RequestSpec{
		GatewayName: gatewayName, Method: endpoint.Method,
		Path: endpoint.Path, Stage: rt.Stage, Headers: rt.Headers, AuthConfig: &endpoint.Auth,
	}
	var err error
	if len(query) > 0 {
		spec.ParamsJSON, err = systemcmd.MarshalJSON(query)
		if err != nil {
			return nil, err
		}
	}
	if len(body) > 0 || endpoint.Method != http.MethodGet {
		spec.BodyJSON, err = systemcmd.MarshalJSON(body)
		if err != nil {
			return nil, err
		}
	}
	result, err := syslib.ExecuteRequest(rt.Runtime, spec)
	if err != nil {
		return nil, err
	}
	env := result.Envelope
	if err := systemcmd.EnsureEnvelope(endpoint.Path, env); err != nil {
		return nil, err
	}
	if env.DryRun {
		return &Reply{dryRun: env}, nil
	}
	rid := env.Headers[requestIDHeader]
	wrapped, _ := env.Data.(map[string]any)
	if !env.OK || wrapped == nil || wrapped["result"] != true {
		return nil, classify(env.Status, wrapped, rid)
	}
	if Map(wrapped["meta"])["truncated"] == true {
		return nil, &Decision{
			Code:      "partial_result",
			Message:   "服务端标记结果被截断；请缩小查询范围。",
			ExitCode:  output.ExitCodeSystemError,
			RequestID: rid,
		}
	}
	return &Reply{Data: wrapped["data"], RequestID: rid, envelope: env}, nil
}

// Execute calls the selected tool's own HTTP operation. No generic execute dispatcher is used.
func Execute(rt *Runtime, tool string, args map[string]any) (*Reply, error) {
	endpoint, found := endpoints[tool]
	if !Supported(tool) || !found {
		return nil, InputError("tool_not_allowed", "该工具没有经过审查的只读 API 路由。")
	}
	args, err := alertRequestArgs(tool, args)
	if err != nil {
		return nil, err
	}
	reply, err := request(rt, endpoint, args)
	if err != nil {
		var decision *Decision
		if errors.As(err, &decision) && resourceDecisions[decision.Code] {
			decision.Next = NextCommands(tool, fmt.Sprint(args["bk_biz_id"]))
		}
		if errors.As(err, &decision) &&
			(decision.Code == "permission_missing" || decision.Code == "permission_denied") {
			decision.Next = []string{
				fmt.Sprintf(
					"bk-cli monitor permission apply --bk_biz_id %v --tool %s",
					args["bk_biz_id"],
					tool,
				),
			}
		}
		return nil, err
	}
	if reply.IsDryRun() {
		return reply, nil
	}
	fail := func(code, message string) (*Reply, error) {
		return nil, &Decision{
			Code:      code,
			Message:   message,
			ExitCode:  output.ExitCodeSystemError,
			RequestID: reply.RequestID,
		}
	}
	if payload, ok := reply.Data.(map[string]any); ok {
		switch {
		case payload["result"] == false:
			return fail("business_error", "底层业务返回失败，不能当作空结果。")
		case payload["is_partial"] == true || Map(payload["meta"])["truncated"] == true:
			return fail("partial_result", "上游只返回部分结果；请缩小查询范围。")
		case tool == "execute_range_query" && payload["status"] == "error":
			return fail("query_error", "指标查询引擎返回错误；检查 PromQL。")
		}
	}
	if tool == "list_alerts" {
		valid, scoped := alertListScopeValid(reply.Data, args["bk_biz_id"])
		if !valid {
			return fail("invalid_response", "告警查询没有返回有效的 alerts 列表；请核对接口契约。")
		}
		if !scoped {
			return fail("response_scope_mismatch", "告警列表的业务归属与请求不一致或无法验证；结果未输出，请核对接口和所选业务。")
		}
	}
	if tool == "find_relations" || tool == "find_relations_range" {
		rows := List(Map(reply.Data)["data"])
		if len(rows) == 0 || len(rows) != len(List(args["query_list"])) {
			return fail("partial_result", "关联查询没有返回与请求数量一致的子项。")
		}
		for _, row := range rows {
			if fmt.Sprint(Map(row)["code"]) != strconv.Itoa(http.StatusOK) {
				return fail("relation_query_failed", "关联查询存在失败子项；根据请求 ID 排查。")
			}
		}
	}
	return reply, nil
}

// StateDecision translates a Unified permission state. requires_resource is neither denial nor grant.
func StateDecision(state map[string]any) *Decision {
	switch state["state"] {
	case "granted", "exempt":
		return nil
	case "requires_resource":
		return &Decision{
			Code:     "needs_resource",
			Message:  "还缺具体资源（例如索引集或指标表）。这不是无权，也不是已授权。",
			ExitCode: output.ExitCodeUserError,
		}
	case "missing":
		return &Decision{
			Code:     "permission_missing",
			Message:  "服务端明确判定无权：下列权限满足其一即可。CLI 不会自动申请。",
			ExitCode: output.ExitCodeUserError,
			AnyOf:    permissionTargets(state),
		}
	}
	return &Decision{
		Code:     "permission_not_granted",
		Message:  "服务端没有给出明确授权，已停止。",
		ExitCode: output.ExitCodeUserError,
	}
}

func permissionTargets(state map[string]any) []map[string]string {
	var targets []map[string]string
	for _, key := range []string{"legacy_permission", "fallback_permission"} {
		item, _ := state[key].(map[string]any)
		if item["action_id"] == nil {
			continue
		}
		target := map[string]string{}
		for _, name := range []string{"system_id", "action_id", "resource_type"} {
			if value, ok := item[name].(string); ok && value != "" {
				target[name] = value
			}
		}
		if url, ok := item["apply_url"].(string); ok &&
			(strings.HasPrefix(url, "https://") || strings.HasPrefix(url, "http://")) {
			target["apply_url"] = url
		}
		targets = append(targets, target)
	}
	return targets
}

type rule struct {
	needles []string
	code    string
	message string
	exit    int
}

// Upstream text is matched only to choose a stable code; it is never echoed.
var rules = []rule{
	{
		[]string{"access_token expired", "access_token is invalid"},
		"auth_failed",
		"登录凭据无效或已过期；请重新登录 bk-cli auth login。",
		output.ExitCodeUserError,
	},
	{
		[]string{"verified gateway user", "tenant does not match", "authenticated tenant"},
		"identity_unverified",
		"网关用户或租户身份未通过校验；检查登录凭据与环境。",
		output.ExitCodeUserError,
	},
	{
		[]string{"incomplete catalog metadata"},
		"resource_blocked",
		"服务端资源元数据不完整或不受支持。这不是无权；需要修服务端目录，CLI 不绕过。",
		output.ExitCodeSystemError,
	},
	{
		[]string{"permission service is unavailable", "metadata is unavailable", "did not return a"},
		"retry_later",
		"权限或资源元数据服务暂不可用：未授权，也不代表无权。稍后重试。",
		output.ExitCodeSystemError,
	},
	{
		[]string{"not uniquely visible", "does not belong", "does not match the requested business"},
		"resource_not_visible",
		"该资源不在目标业务的可见目录中。请从列表命令重新选择，不要换业务或猜 ID。",
		output.ExitCodeUserError,
	},
	{
		[]string{"does not exist", "no logindexset matches the given query"},
		"resource_not_found",
		"资源不存在。请从列表命令返回值中选择。",
		output.ExitCodeUserError,
	},
	{
		[]string{"3626305"},
		"extract_policy_missing",
		"当前业务还没有可用的日志提取策略；请由管理员在日志平台的日志提取配置中添加策略。",
		output.ExitCodeUserError,
	},
	{[]string{"tool_args"}, "invalid_arguments", "参数不符合服务端工具契约；可加 --dry-run 查看将发送的请求。", output.ExitCodeUserError},
}

func classify(status int, body map[string]any, rid string) *Decision {
	if detail, ok := body["detail"].(map[string]any); ok && detail["state"] != nil {
		if decision := StateDecision(detail); decision != nil {
			decision.RequestID = rid
			return decision
		}
	}
	code := fmt.Sprint(body["code"])
	if code == "3324002" {
		return &Decision{
			Code: "resource_not_found", Message: "指定业务下未找到该告警或事件；请从同一业务的列表重新选择。",
			ExitCode: output.ExitCodeUserError, RequestID: rid,
		}
	}
	raw, _ := json.Marshal(body)
	text := strings.ToLower(string(raw))
	for _, item := range rules {
		for _, needle := range item.needles {
			if strings.Contains(text, needle) {
				return &Decision{
					Code:      item.code,
					Message:   item.message,
					ExitCode:  item.exit,
					RequestID: rid,
				}
			}
		}
	}
	switch {
	case status == http.StatusNotFound:
		return &Decision{
			Code:      "api_not_found",
			Message:   "请求路径返回 404；请核对 stage、网关前台路径、后端映射和服务路由/版本。",
			ExitCode:  output.ExitCodeSystemError,
			RequestID: rid,
		}
	case code == "503" || status == 503:
		return &Decision{
			Code:      "retry_later",
			Message:   "服务端暂不可用；请求未授权。稍后重试。",
			ExitCode:  output.ExitCodeSystemError,
			RequestID: rid,
		}
	case code == "403" || status == 401 || status == 403:
		return &Decision{
			Code:      "permission_denied",
			Message:   "请求被拒绝。运行 bk-cli monitor permission check 查看服务端判定；CLI 不自动申请权限。",
			ExitCode:  output.ExitCodeUserError,
			RequestID: rid,
		}
	}
	return &Decision{
		Code:      "business_error",
		Message:   "服务端没有返回成功；根据请求 ID 排查，不能当作空结果。",
		ExitCode:  output.ExitCodeSystemError,
		RequestID: rid,
	}
}

// PrintFailure prints a Decision as a structured stderr envelope; other errors were printed by shared layers.
func PrintFailure(cmd *cobra.Command, err error, details ...map[string]any) error {
	var decision *Decision
	if !errors.As(err, &decision) {
		return err
	}
	data := map[string]any{"decision": decision.Code}
	for _, detail := range details {
		maps.Copy(data, detail)
	}
	if decision.RequestID != "" {
		data["request_id"] = decision.RequestID
	}
	if len(decision.Next) > 0 {
		data["next"] = decision.Next
	}
	if len(decision.AnyOf) > 0 {
		data["any_of_permissions"] = decision.AnyOf
	}
	if len(decision.Steps) > 0 {
		data["steps"] = decision.Steps
	}
	_ = (&output.Envelope{
		OK:    false,
		Data:  data,
		Error: &output.ErrorDetail{Code: decision.Code, Message: decision.Message},
	}).WriteJSON(cmd.ErrOrStderr())
	return &output.CLIError{ExitCode: decision.ExitCode, Code: decision.Code, Message: decision.Message}
}

// Print writes a compact success envelope, or the dry-run request preview.
func Print(cmd *cobra.Command, reply *Reply, data any) error {
	return output.WriteCommandEnvelope(cmd, reply.Envelope(data))
}

// Envelope returns the response or unchanged request preview for local decoration.
func (r *Reply) Envelope(data any) *output.Envelope {
	if r.envelope != nil {
		env := *r.envelope
		env.Data = data
		return &env
	}
	if r.IsDryRun() {
		return r.dryRun
	}
	env := &output.Envelope{OK: true, Data: data}
	if r.RequestID != "" {
		env.Headers = map[string]string{requestIDHeader: r.RequestID}
	}
	return env
}

// Window converts --since into the Unix-second API strings.
func Window(since string, now time.Time) (string, string, error) {
	duration, err := time.ParseDuration(since)
	if err != nil || duration <= 0 || duration > maxWindow {
		return "", "", InputError("invalid_since", "--since 使用 15m、1h 这类格式，且不超过 24h。")
	}
	end := now.Unix()
	return strconv.FormatInt(end-int64(duration.Seconds()), 10), strconv.FormatInt(end, 10), nil
}

// RequireBiz rejects a missing business with the command that discovers it.
func RequireBiz(biz string) error {
	if strings.TrimSpace(biz) == "" {
		return InputError(
			"needs_business",
			"需要 --bk_biz_id；不知道时先按名称查询。",
			"bk-cli monitor space search_spaces --space_name <名称>",
		)
	}
	return nil
}

// Run resolves the shared runtime and prints Decisions as structured errors.
func Run(
	deps systemcmd.BuildDeps,
	fn func(*cobra.Command, *Runtime) error,
) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, _ []string) error {
		rt, err := systemcmd.ResolveRuntime(deps)
		if err != nil {
			return err
		}
		return PrintFailure(cmd, fn(cmd, Bind(cmd, rt)))
	}
}

// ResourceContext keeps only resource fields declared by the tool's own permission contract.
func ResourceContext(schema, values map[string]any) (map[string]any, []string) {
	spec := Map(Map(schema["permission"])["fallback_permission"])
	conditional := Map(spec["conditional"])
	allowed := map[string]bool{}
	keys := []any{spec["resource_arg"], spec["target_arg"], conditional["arg"]}
	keys = append(keys, List(conditional["all_args"])...)
	for _, key := range keys {
		if name, ok := key.(string); ok && name != "" && name != "bk_biz_id" {
			allowed[name] = true
		}
	}
	if spec["resource_type"] == "indices" {
		allowed["target_type"] = true
	}
	context, unused := map[string]any{}, []string{}
	for key, value := range values {
		if allowed[key] {
			context[key] = value
		} else {
			unused = append(unused, key)
		}
	}
	sort.Strings(unused)
	return context, unused
}

// IsResourceDecision reports whether the next step is to choose a real resource.
func IsResourceDecision(code string) bool { return resourceDecisions[code] }

// IsJudgment reports whether a decision is a server judgment that check should report.
func IsJudgment(err error) (*Decision, bool) {
	var decision *Decision
	if errors.As(err, &decision) && judgments[decision.Code] {
		return decision, true
	}
	return nil, false
}

// Map returns value as a JSON object, or nil.
func Map(value any) map[string]any { m, _ := value.(map[string]any); return m }

// List returns value as a JSON array, or nil.
func List(value any) []any { l, _ := value.([]any); return l }
