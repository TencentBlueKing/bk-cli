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
    def generic_gateway(gateway: str, stage: str, tail: str):
        negative = maybe_negative_response(gateway, stage, tail)
        if negative is not None:
            return negative

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
            requested_ips = set()
            host_filter = payload.get("host_property_filter") or {}
            filter_rules = host_filter.get("rules") or []
            for rule in filter_rules:
                if rule.get("field") == "bk_host_innerip":
                    requested_ips.update(rule.get("value") or [])
                for nested in rule.get("rules") or []:
                    if nested.get("field") == "bk_host_innerip":
                        requested_ips.update(nested.get("value") or [])
            if requested_ips:
                hosts = [host for host in hosts if host["bk_host_innerip"] in requested_ips]
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
