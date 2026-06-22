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

package config

import "strings"

const (
	jobLegacyGatewayName  = "jobv3-cloud"
	paasLegacyGatewayName = "paasv3"
)

// bkTEDomain is injected at build time via:
// -X github.com/TencentBlueKing/bk-cli/internal/config.bkTEDomain=<domain>
var bkTEDomain string

// ResolveGatewayName returns the effective gateway name under the active
// context's URL template.
func ResolveGatewayName(tmpl, gatewayName string) string {
	if gatewayName != "bk-job" && gatewayName != "bkpaas3" {
		return gatewayName
	}

	subdomainTmpl, pathTmpl, ok := legacyGatewayTemplates()
	if !ok {
		return gatewayName
	}

	normalized := strings.TrimRight(NormalizeURLTemplate(strings.TrimSpace(tmpl)), "/")
	if normalized == subdomainTmpl || normalized == pathTmpl {
		switch gatewayName {
		case "bk-job":
			return jobLegacyGatewayName
		case "bkpaas3":
			return paasLegacyGatewayName
		}
	}

	return gatewayName
}

// SetBKTeDomainForTesting temporarily overrides the build-time BK_TE_DOMAIN.
func SetBKTeDomainForTesting(domain string) func() {
	previous := bkTEDomain
	bkTEDomain = strings.TrimSpace(domain)
	return func() {
		bkTEDomain = previous
	}
}

func legacyGatewayTemplates() (string, string, bool) {
	domain := strings.TrimSpace(bkTEDomain)
	if domain == "" {
		return "", "", false
	}

	return "https://{gateway_name}.apigw." + domain,
		"https://bkapi." + domain + "/api/{gateway_name}",
		true
}
