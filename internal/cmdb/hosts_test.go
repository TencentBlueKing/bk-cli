package cmdb

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"

	json "github.com/goccy/go-json"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/TencentBlueKing/bk-cli/internal/config"
	"github.com/TencentBlueKing/bk-cli/internal/credential"
	syslib "github.com/TencentBlueKing/bk-cli/internal/system"
)

var _ = Describe("ResolveBizHosts", func() {
	var tmpDir string

	BeforeEach(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "bk-cli-internal-cmdb-*")
		Expect(err).NotTo(HaveOccurred())
		Expect(os.Setenv("BK_CLI_CONFIG_DIR", tmpDir)).To(Succeed())
	})

	AfterEach(func() {
		Expect(os.Unsetenv("BK_CLI_CONFIG_DIR")).To(Succeed())
		Expect(os.RemoveAll(tmpDir)).To(Succeed())
	})

	It("parses and deduplicates host IP tokens", func() {
		hosts, err := ParseHostIPs("10.0.0.1,27:10.0.0.2 10.0.0.1")
		Expect(err).NotTo(HaveOccurred())
		Expect(hosts).To(Equal([]HostIP{
			{IP: "10.0.0.1", CloudID: 0},
			{IP: "10.0.0.2", CloudID: 27},
		}))
	})

	It("rejects malformed host IP tokens", func() {
		_, err := ParseHostIPs("10.0.0.1,bad-token")
		Expect(err).To(MatchError(ContainSubstring("host_ips contains an invalid host entry")))
	})

	It("builds the host lookup request body", func() {
		spec, parsedHosts, err := BuildBizHostsRequest(ResolveBizHostsInput{
			BizID:   2,
			Hosts:   "10.0.0.1,27:10.0.0.2",
			Stage:   "prod",
			Headers: []string{"X-Test:true"},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(parsedHosts).To(HaveLen(2))
		Expect(spec.GatewayName).To(Equal("bk-cmdb"))
		Expect(spec.Method).To(Equal("POST"))
		Expect(spec.Path).To(Equal("/api/v3/open/hosts/app/2/list_hosts"))
		Expect(spec.Stage).To(Equal("prod"))
		Expect(spec.Headers).To(Equal([]string{"X-Test:true"}))

		var body map[string]any
		Expect(json.Unmarshal([]byte(spec.BodyJSON), &body)).To(Succeed())
		Expect(body["fields"]).To(ContainElements("bk_host_id", "bk_host_innerip", "bk_cloud_id", "bk_host_name"))
		Expect(body).To(HaveKey("host_property_filter"))
	})

	It("resolves all requested hosts", func() {
		runtime := setupCMDBRuntime(func(w http.ResponseWriter, r *http.Request) {
			Expect(r.URL.Path).To(Equal("/bk-cmdb/prod/api/v3/open/hosts/app/2/list_hosts"))
			_, _ = w.Write([]byte(`{"count":2,"info":[{"bk_host_id":101,"bk_host_innerip":"10.0.0.1","bk_cloud_id":0,"bk_host_name":"host-101"},{"bk_host_id":102,"bk_host_innerip":"10.0.0.2","bk_cloud_id":27,"bk_host_name":"host-102"}]}`))
		})

		result, err := ResolveBizHosts(runtime, ResolveBizHostsInput{BizID: 2, Hosts: "10.0.0.1,27:10.0.0.2", Stage: "prod"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.HostIDs).To(Equal([]int64{101, 102}))
		Expect(result.Hosts).To(HaveLen(2))
	})

	It("fails when no hosts match", func() {
		runtime := setupCMDBRuntime(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"count":0,"info":[]}`))
		})
		_, err := ResolveBizHosts(runtime, ResolveBizHostsInput{BizID: 2, Hosts: "10.0.0.1", Stage: "prod"})
		Expect(err).To(MatchError(ContainSubstring("no hosts matched")))
	})

	It("fails when only some requested hosts match", func() {
		runtime := setupCMDBRuntime(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"count":1,"info":[{"bk_host_id":101,"bk_host_innerip":"10.0.0.1","bk_cloud_id":0,"bk_host_name":"host-101"}]}`))
		})
		_, err := ResolveBizHosts(runtime, ResolveBizHostsInput{BizID: 2, Hosts: "10.0.0.1,10.0.0.2", Stage: "prod"})
		Expect(err).To(MatchError(ContainSubstring("only resolved 1 of 2 requested hosts")))
	})
})

func setupCMDBRuntime(handler http.HandlerFunc) *syslib.Runtime {
	server := httptest.NewServer(handler)
	DeferCleanup(server.Close)

	cfg := &config.Config{
		BkAPIURLTmpl: strings.TrimRight(server.URL, "/") + "/{gateway_name}",
		UserKey:      "bk_token",
	}
	Expect(cfg.Save(config.ConfigPath("default"))).To(Succeed())
	Expect(config.SetActiveContext("default")).To(Succeed())
	key, err := credential.DeriveKey()
	Expect(err).NotTo(HaveOccurred())
	Expect(credential.Save(config.CredentialsPath("default"), &credential.Credential{
		Type:        credential.TypeAccessToken,
		AccessToken: "token-123",
	}, key)).To(Succeed())

	runtime, err := syslib.ResolveRuntime("", false, false)
	Expect(err).NotTo(HaveOccurred())
	return runtime
}
