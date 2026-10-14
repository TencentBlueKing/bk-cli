package job

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

var _ = Describe("RunScript", func() {
	var tmpDir string

	BeforeEach(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "bk-cli-shortcut-job-*")
		Expect(err).NotTo(HaveOccurred())
		Expect(os.Setenv("BK_CLI_CONFIG_DIR", tmpDir)).To(Succeed())
	})

	AfterEach(func() {
		Expect(os.Unsetenv("BK_CLI_CONFIG_DIR")).To(Succeed())
		Expect(os.RemoveAll(tmpDir)).To(Succeed())
	})

	It("resolves hosts before dispatching the Job script", func() {
		var paths []string
		runtime := setupShortcutRuntime(false, func(w http.ResponseWriter, r *http.Request) {
			paths = append(paths, r.URL.Path)
			switch {
			case strings.Contains(r.URL.Path, "/bk-cmdb/"):
				_, _ = w.Write(
					[]byte(
						`{"count":1,"info":[{"bk_host_id":101,"bk_host_innerip":"10.0.0.1","bk_cloud_id":0,"bk_host_name":"host-101"}]}`,
					),
				)
			case strings.Contains(r.URL.Path, "/bk-job/"):
				var body map[string]any
				Expect(json.NewDecoder(r.Body).Decode(&body)).To(Succeed())
				Expect(
					body["target_server"],
				).To(
					Equal(map[string]any{"host_id_list": []any{float64(101)}}),
				)
				_, _ = w.Write([]byte(`{"job_instance_id":200}`))
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		})

		result, err := RunScript(runtime, RunScriptInput{
			BizID:          2,
			Hosts:          "10.0.0.1",
			ScriptContent:  "echo hello",
			ScriptLanguage: "shell",
			AccountAlias:   "root",
			Timeout:        7200,
			Stage:          "prod",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Envelope.OK).To(BeTrue())
		Expect(paths).To(Equal([]string{
			"/bk-cmdb/prod/api/v3/open/hosts/app/2/list_hosts",
			"/bk-job/prod/api/v3/fast_execute_script",
		}))
	})

	It("does not dispatch Job when CMDB resolution is partial", func() {
		var jobCalled bool
		runtime := setupShortcutRuntime(false, func(w http.ResponseWriter, r *http.Request) {
			if strings.Contains(r.URL.Path, "/bk-job/") {
				jobCalled = true
			}
			_, _ = w.Write(
				[]byte(
					`{"count":1,"info":[{"bk_host_id":101,"bk_host_innerip":"10.0.0.1","bk_cloud_id":0,"bk_host_name":"host-101"}]}`,
				),
			)
		})

		_, err := RunScript(runtime, RunScriptInput{
			BizID:          2,
			Hosts:          "10.0.0.1,10.0.0.2",
			ScriptContent:  "echo hello",
			ScriptLanguage: "shell",
			Timeout:        7200,
			Stage:          "prod",
		})
		Expect(err).To(MatchError(ContainSubstring("only resolved 1 of 2 requested hosts")))
		Expect(jobCalled).To(BeFalse())
	})

	It("returns ordered dry-run step previews without sending upstream requests", func() {
		runtime := setupShortcutRuntime(true, func(w http.ResponseWriter, r *http.Request) {
			Fail("dry-run must not send upstream requests")
		})

		result, err := RunScript(runtime, RunScriptInput{
			BizID:          2,
			Hosts:          "10.0.0.1",
			ScriptContent:  "echo hello",
			ScriptLanguage: "shell",
			Timeout:        7200,
			Stage:          "prod",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Envelope.DryRun).To(BeTrue())
		data := result.Envelope.Data.(map[string]any)
		Expect(data["shortcut"]).To(Equal("job.+run-script"))
		steps := data["steps"].([]map[string]any)
		Expect(steps).To(HaveLen(2))
		Expect(steps[0]["name"]).To(Equal("resolve_hosts"))
		Expect(steps[1]["name"]).To(Equal("fast_execute_script"))
	})
})

func setupShortcutRuntime(dryRun bool, handler http.HandlerFunc) *syslib.Runtime {
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

	runtime, err := syslib.ResolveRuntime("", dryRun, false)
	Expect(err).NotTo(HaveOccurred())
	return runtime
}
