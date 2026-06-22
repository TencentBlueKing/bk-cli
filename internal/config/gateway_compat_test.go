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

package config_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/TencentBlueKing/bk-cli/internal/config"
)

var _ = Describe("ResolveGatewayName", func() {
	It("keeps gateway names unchanged when BK_TE_DOMAIN is not configured", func() {
		restore := config.SetBKTeDomainForTesting("")
		DeferCleanup(restore)

		Expect(config.ResolveGatewayName(
			"https://{gateway_name}.apigw.te.example",
			"bk-job",
		)).To(Equal("bk-job"))
		Expect(config.ResolveGatewayName(
			"https://{gateway_name}.apigw.te.example",
			"bkpaas3",
		)).To(Equal("bkpaas3"))
	})

	It("maps bk-job to jobv3-cloud for the injected subdomain template", func() {
		restore := config.SetBKTeDomainForTesting("te.example")
		DeferCleanup(restore)

		Expect(config.ResolveGatewayName(
			"https://{gateway_name}.apigw.te.example",
			"bk-job",
		)).To(Equal("jobv3-cloud"))
	})

	It("maps bk-job to jobv3-cloud for the injected path template", func() {
		restore := config.SetBKTeDomainForTesting("te.example")
		DeferCleanup(restore)

		Expect(config.ResolveGatewayName(
			"https://bkapi.te.example/api/{gateway_name}",
			"bk-job",
		)).To(Equal("jobv3-cloud"))
	})

	It("maps bk-job to jobv3-cloud for the injected path template with a trailing slash", func() {
		restore := config.SetBKTeDomainForTesting("te.example")
		DeferCleanup(restore)

		Expect(config.ResolveGatewayName(
			"https://bkapi.te.example/api/{gateway_name}/",
			"bk-job",
		)).To(Equal("jobv3-cloud"))
	})

	It("maps bkpaas3 to paasv3 for the injected subdomain template", func() {
		restore := config.SetBKTeDomainForTesting("te.example")
		DeferCleanup(restore)

		Expect(config.ResolveGatewayName(
			"https://{gateway_name}.apigw.te.example",
			"bkpaas3",
		)).To(Equal("paasv3"))
	})

	It("maps bkpaas3 to paasv3 for the injected path template with a trailing slash", func() {
		restore := config.SetBKTeDomainForTesting("te.example")
		DeferCleanup(restore)

		Expect(config.ResolveGatewayName(
			"https://bkapi.te.example/api/{gateway_name}/",
			"bkpaas3",
		)).To(Equal("paasv3"))
	})

	It("does not rewrite other gateway names", func() {
		restore := config.SetBKTeDomainForTesting("te.example")
		DeferCleanup(restore)

		Expect(config.ResolveGatewayName(
			"https://{gateway_name}.apigw.te.example",
			"bk-cmdb",
		)).To(Equal("bk-cmdb"))
	})
})
