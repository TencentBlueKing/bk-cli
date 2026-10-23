# TencentBlueKing is pleased to support the open source community by making
# 蓝鲸智云 - bk-cli (BlueKing - Cli) available.
# Copyright (C) Tencent. All rights reserved.
# Licensed under the MIT License (the "License"); you may not use this file except
# in compliance with the License. You may obtain a copy of the License at
#
#     http://opensource.org/licenses/MIT
#
# Unless required by applicable law or agreed to in writing, software distributed under
# the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND,
# either express or implied. See the License for the specific language governing permissions and
# limitations under the License.
#
# We undertake not to change the open source license (MIT license) applicable
# to the current version of the project delivered to anyone in the future.

from __future__ import annotations

import json

from typing import Any

from flask import Flask, jsonify, request

NEGATIVE_SCENARIOS: dict[str, tuple[int, dict[str, Any]]] = {
    "SYSGO-002": (
        500,
        {
            "error": "pipeline_start_failed",
            "message": "Synthetic upstream failure for integration coverage",
        },
    ),
    "CMDB-NEG-001": (
        409,
        {
            "error": "transfer_conflict",
            "message": "Synthetic CMDB transfer conflict",
        },
    ),
    "CMDB-NEG-002": (
        400,
        {
            "error": "delete_rejected",
            "message": "Synthetic CMDB delete rejection",
        },
    ),
    "JOB-NEG-001": (
        502,
        {
            "error": "job_dispatch_failed",
            "message": "Synthetic JOB dispatch failure",
        },
    ),
    "NODEMAN-NEG-001": (
        500,
        {
            "error": "install_job_failed",
            "message": "Synthetic NodeMan install failure",
        },
    ),
    "SOPS-NEG-001": (
        409,
        {
            "error": "task_operation_rejected",
            "message": "Synthetic SOPS task rejection",
        },
    ),
}


def create_app(test_config: dict[str, Any] | None = None) -> Flask:
    app = Flask(__name__)
    if test_config:
        app.config.update(test_config)

    def active_scenario() -> str:
        return request.headers.get("X-Mock-Scenario", "").strip()

    @app.before_request
    def verify_personal_auth():
        # Exercise the shared auth path without echoing credentials into reports.
        if active_scenario() == "PERSONAL-TOKEN":
            if (
                request.headers.get("Authorization") != "Bearer integration-personal"
                or request.headers.get("X-Bkapi-Authorization")
                or request.headers.get("X-Bk-Tenant-Id") != "personal-tenant"
            ):
                return jsonify({"error": "unexpected_personal_auth"}), 401
        return None

    def request_body() -> Any:
        payload = request.get_json(silent=True)
        if payload is not None:
            return payload
        raw = request.get_data(cache=True, as_text=True).strip()
        if raw:
            return raw
        return None

    def echo_payload(gateway: str, stage: str, tail: str) -> dict[str, Any]:
        return {
            "gateway": gateway,
            "stage": stage,
            "method": request.method,
            "path": f"/{gateway}/{stage}/{tail}",
            "query": request.args.to_dict(flat=True),
            "body": request_body(),
            "scenario": active_scenario() or "default",
        }

    def maybe_negative_response(gateway: str, stage: str, tail: str):
        scenario = active_scenario()
        response = NEGATIVE_SCENARIOS.get(scenario)
        if response is None:
            return None

        status, payload = response
        data = dict(payload)
        data.update(
            {
                "gateway": gateway,
                "stage": stage,
                "path": f"/{gateway}/{stage}/{tail}",
                "method": request.method,
                "query": request.args.to_dict(flat=True),
                "body": request_body(),
                "scenario": scenario,
            }
        )
        return jsonify(data), status

    @app.get("/healthz")
    def healthz():
        return jsonify({"ok": True, "service": "mock_api"})

    @app.get("/bk-apigateway/prod/api/v2/open/gateways/")
    def list_gateways():
        name = request.args.get("name", "")
        keyword = request.args.get("keyword", "")
        payload = {
            "count": 1,
            "items": [
                {
                    "name": name or "integration-gateway",
                    "keyword": keyword,
                    "description": "Synthetic gateway for integration testing",
                }
            ],
            "echo": {
                "query": request.args.to_dict(flat=True),
                "scenario": active_scenario() or "default",
            },
        }
        return jsonify(payload)

    @app.get("/bk-apigateway/prod/api/v2/open/gateways/<gateway_name>/resources/")
    def list_gateway_apis(gateway_name: str):
        payload = {
            "count": 1,
            "items": [
                {
                    "name": "list_apps",
                    "gateway_name": gateway_name,
                    "keyword": request.args.get("keyword", ""),
                }
            ],
            "echo": {
                "query": request.args.to_dict(flat=True),
                "scenario": active_scenario() or "default",
            },
        }
        return jsonify(payload)

    @app.get("/bk-apigateway/prod/api/v2/open/gateways/<gateway_name>/resources/<api_name>/")
    def retrieve_gateway_api_details(gateway_name: str, api_name: str):
        return jsonify(
            {
                "gateway_name": gateway_name,
                "api_name": api_name,
                "stage_name": request.args.get("stage_name", "prod"),
                "schema": {"method": "GET", "path": f"/api/{api_name}"},
                "scenario": active_scenario() or "default",
            }
        )

    @app.post("/bk-apigateway/prod/api/v2/open/gateways/<gateway_name>/permissions/apply/")
    def apply_gateway_permission(gateway_name: str):
        payload = request.get_json(silent=True) or {}
        data = {
            "record_id": 9007199254740993,
            "approval_url": "https://approval.example.com/records/9007199254740993",
            "approvers": ["gateway-admin"],
            "gateway_name": gateway_name,
            "submitted": payload,
        }
        return jsonify({"code": 0, "message": "OK", "data": data})

    @app.post("/devops/prod/v4/apigw-user/projects/<project_id>/build_start")
    def start_build(project_id: str):
        scenario = active_scenario()
        request_body_json = request.get_json(silent=True) or {}
        pipeline_id = request.args.get("pipelineId", "")

        if scenario == "SYSGO-002":
            return (
                jsonify(
                    {
                        "error": "pipeline_start_failed",
                        "message": "Synthetic upstream failure for integration coverage",
                        "projectId": project_id,
                        "pipelineId": pipeline_id,
                    }
                ),
                500,
            )

        return (
            jsonify(
                {
                    "build_id": f"build-{project_id}-{pipeline_id or 'unknown'}",
                    "projectId": project_id,
                    "pipelineId": pipeline_id,
                    "received_body": request_body_json,
                    "scenario": scenario or "default",
                    "status": "started",
                }
            ),
            201,
        )

    @app.route(
        "/<gateway>/<stage>/<path:tail>",
        methods=["GET", "POST", "PUT", "PATCH", "DELETE"],
    )
    @app.route("/alternate/<gateway>/<stage>/<path:tail>", methods=["GET", "POST"])
    def generic_gateway(gateway: str, stage: str, tail: str):
        negative = maybe_negative_response(gateway, stage, tail)
        if negative is not None:
            return negative

        if gateway == "bk-monitor" and active_scenario() == "MONITOR-012":
            if stage != "testing" or request.headers.get("X-Example") != "preflight":
                return jsonify({"result": False, "message": "preflight request options missing"}), 403

        if gateway == "bk-monitor" and tail == "mcp/lookup_tool/":
            payload = request_body() or {}
            return jsonify(
                {
                    "result": True,
                    "data": {
                        "tools": [
                            {
                                "name": payload.get("tool_name") or "list_alerts",
                                "risk": "query",
                                "requires_confirmation": False,
                                "permission_state": "missing",
                            }
                        ]
                    },
                }
            )

        if gateway == "bk-monitor" and tail == "mcp/lookup_tool_schema/":
            return jsonify(
                {
                    "result": True,
                    "data": {
                        "catalog_version": "integration",
                        "permission": {
                            "mode": "mcp_then_native",
                            "fallback_permission": {
                                "resource_type": "indices" if (request_body() or {}).get("tool_name") == "get_index_set_fields" else "space",
                                "resource_arg": "index_set_id" if (request_body() or {}).get("tool_name") == "get_index_set_fields" else "bk_biz_id",
                            },
                        },
                    },
                }
            )

        if gateway == "bk-monitor" and tail == "mcp/lookup_permissions/":
            payload = request_body() or {}
            tool_name = payload.get("tool_name")
            # Two synthetic V2 evidence sources for MONITOR-009.
            if payload.get("bk_biz_id") in ("10007", "10008"):
                return jsonify({"result": True, "data": {"authorized": True, "scopes": [{"tool_name": tool_name, "state": "granted"}]}})
            # Synthetic scope mismatch for MONITOR-007; never a real business ID.
            if payload.get("bk_biz_id") == "900007":
                return jsonify({"result": True, "data": {"authorized": True, "scopes": [{"tool_name": "different_tool", "state": "granted"}]}})
            # Fixed synthetic profile resources; list_alerts remains denied for MONITOR-004.
            if payload.get("bk_biz_id") == "100100" and (
                tool_name == "list_time_series_metrics"
                or (tool_name == "get_index_set_fields" and (payload.get("resource_context") or {}).get("index_set_id") == 101003)
            ):
                return jsonify({"result": True, "data": {"authorized": True, "scopes": [{"tool_name": tool_name, "state": "granted"}]}})
            return jsonify(
                {
                    "result": True,
                    "data": {
                        "authorized": False,
                        "scopes": [
                            {
                                "tool_name": tool_name,
                                "state": "missing",
                                "authorized": False,
                                "legacy_permission": {
                                    "system_id": "bk_monitorv3",
                                    "action_id": "using_alarm_mcp",
                                    "resource_type": "space",
                                    "apply_url": "https://iam.example.com/apply/mcp",
                                },
                                "fallback_permission": {
                                    "system_id": "bk_monitorv3",
                                    "action_id": "view_event_v2",
                                    "resource_type": "space",
                                    "apply_url": "https://iam.example.com/apply/native",
                                },
                            }
                        ],
                    },
                }
            )

        if gateway == "bk-monitor" and tail.startswith("mcp/"):
            tool_name = tail.removeprefix("mcp/").strip("/")
            if tool_name == "execute_tool":
                return jsonify({"result": False, "message": "generic dispatch is not supported"}), 404
            args = request_body() or {}
            if request.method == "GET":
                args = dict(request.args)
                for key, value in list(args.items()):
                    if key != "bk_biz_id":
                        try:
                            args[key] = json.loads(value)
                        except (TypeError, ValueError):
                            pass
            if active_scenario() == "MONITOR-012" and tool_name == "list_time_series_metrics":
                if args.get("table_id") == "missing.table":
                    return jsonify({"result": False, "message": "table does not exist"})
            if active_scenario() == "MONITOR-013":
                if tool_name in ("list_alerts", "get_alert_top_n"):
                    if args.get("bk_biz_ids") != [str(args.get("bk_biz_id"))]:
                        return jsonify({"result": False, "message": "single-business array missing"}), 400
                    if tool_name == "get_alert_top_n":
                        return jsonify({"result": True, "data": {"scope": args["bk_biz_ids"]}})
                    biz = "100200" if request.headers.get("X-Example") == "foreign-scope" else args["bk_biz_id"]
                    return jsonify({"result": True, "data": {"alerts": [{"id": "sample-alert", "bk_biz_id": biz}], "total": 1}})
                if tool_name == "get_alert_info":
                    if args.get("id") == "missing-alert":
                        return jsonify({"result": False, "code": 3324002, "message": "synthetic alert not found"})
                    return jsonify({"result": True, "data": {"id": args["id"], "bk_biz_id": args["bk_biz_id"]}})
            if tool_name in ("find_relations", "find_relations_range"):
                code = 400 if args.get("bk_biz_id") == "900008" else 200
                items = [{"code": code, "target_list": []} for _ in args.get("query_list", [])]
                return jsonify({"result": True, "data": {"data": items}})
            return jsonify({"result": True, "data": {
                "path": f"/{gateway}/{stage}/{tail}", "request_path": request.path,
                "tool_args": args,
                **({"alerts": []} if tool_name == "list_alerts" else {}),
                "headers": {key: request.headers.get(key) for key in ("X-Example", "X-Bk-Tenant-Id")},
            }})

        if gateway == "bk-cmdb" and tail == "api/v3/open/hosts/modules/read":
            return jsonify(
                [
                    {
                        "bk_biz_id": 2,
                        "bk_host_id": 101,
                        "bk_module_ids": [201, 202],
                    }
                ]
            )

        if gateway == "bk-cmdb" and (
            tail.endswith("/list_hosts")
            or tail.endswith("/list_hosts_topo")
            or tail.endswith("/list_hosts_without_app")
            or tail.endswith("/list_resource_pool_hosts")
        ):
            payload = request.get_json(silent=True) or {}
            page = payload.get("page") or {}
            start = int(page.get("start", 0))
            limit = int(page.get("limit", 500))
            hosts = [
                {
                    "bk_host_id": 101,
                    "bk_host_innerip": "10.0.0.1",
                    "bk_cloud_id": 0,
                    "bk_host_name": "host-101",
                },
                {
                    "bk_host_id": 102,
                    "bk_host_innerip": "10.0.0.2",
                    "bk_cloud_id": 0,
                    "bk_host_name": "host-102",
                },
            ]
            if active_scenario() == "CMDB-007":
                hosts = [
                    {"bk_host_id": 9007199254740992},
                    {"bk_host_id": 9007199254740993},
                    {"bk_host_id": 9223372036854775807},
                ]
            return jsonify(
                {
                    "count": len(hosts),
                    "info": hosts[start : start + limit],
                    "echo": echo_payload(gateway, stage, tail),
                }
            )

        return jsonify(echo_payload(gateway, stage, tail))

    return app


if __name__ == "__main__":
    application = create_app()
    application.run(host="0.0.0.0", port=8080)
