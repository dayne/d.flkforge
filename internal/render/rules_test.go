package render

import (
	"strings"
	"testing"

	"github.com/dayne/d.flkforge/internal/recipe"
)

func findings(t *testing.T, body string) []Finding {
	t.Helper()
	r, err := recipe.Parse([]byte("app = \"flkt-x\"\n" + body))
	if err != nil {
		t.Fatal(err)
	}
	res, err := Render(r, catalog(t))
	if err != nil {
		t.Fatal(err)
	}
	return res.Findings
}

func has(fs []Finding, rule string) bool {
	for _, f := range fs {
		if f.Rule == rule {
			return true
		}
	}
	return false
}

const ubuntu = "[[use]]\nsnippet = \"base-ubuntu\"\n"
const wrapper = "[[use]]\nsnippet = \"run-wrapper\"\nparams = { command = [\"/srv\"] }\n"

func TestRules(t *testing.T) {
	for _, tc := range []struct {
		name, recipe, rule string
		want               bool
	}{
		{"wrapper without the outer net", ubuntu + wrapper, "wrapper-restart", true},
		{"wrapper with the outer net", ubuntu + wrapper + "[[use]]\nsnippet = \"restart-outer-net\"\n", "wrapper-restart", false},
		{"exec mode needs no net", ubuntu + "[[use]]\nsnippet = \"run-wrapper\"\nparams = { command = [\"/srv\"], exec = true }\n", "wrapper-restart", false},
		{"kill_timeout under STOP_TIMEOUT", ubuntu + "[[use]]\nsnippet = \"env\"\nparams = { vars = { STOP_TIMEOUT = \"100\" } }\n[[use]]\nsnippet = \"kill-timeout\"\nparams = { seconds = 60 }\n", "kill-timeout-margin", true},
		{"the wrapper's own margin", ubuntu + wrapper, "kill-timeout-margin", false},
		{"before_deploy, no check", ubuntu + "[[use]]\nsnippet = \"before-deploy\"\n", "before-deploy-check", true},
		{"before_deploy with a check", ubuntu + "[[use]]\nsnippet = \"before-deploy\"\n[[use]]\nsnippet = \"check-log\"\nparams = { match = \"up\" }\n", "before-deploy-check", false},
		{"udp only, no check", ubuntu + "[[use]]\nsnippet = \"service\"\nparams = { port = 2456, protocol = \"udp\" }\n", "udp-only-check", true},
		{"udp with a log check", ubuntu + "[[use]]\nsnippet = \"service\"\nparams = { port = 2456, protocol = \"udp\" }\n[[use]]\nsnippet = \"check-log\"\nparams = { match = \"up\" }\n", "udp-only-check", false},
		{"tcp service", ubuntu + "[[use]]\nsnippet = \"service\"\nparams = { port = 25565 }\n", "udp-only-check", false},
		{"log check ending in $", ubuntu + "[[use]]\nsnippet = \"check-log\"\nparams = { match = \"^ready$\" }\n", "log-check-dollar", true},
		{"log check, escaped $", ubuntu + "[[use]]\nsnippet = \"check-log\"\nparams = { match = \"costs \\\\$\" }\n", "log-check-dollar", false},
		{"alpine user gets setpriv", "[[use]]\nsnippet = \"base-alpine\"\n[[use]]\nsnippet = \"user-1000\"\n", "busybox-user", false},
		{"stdin null, console stop", ubuntu + "[[use]]\nsnippet = \"run-wrapper\"\nparams = { command = [\"/srv\"], stop = \"console:stop\" }\n[[use]]\nsnippet = \"stdin-null\"\n", "stdin-null-console", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := has(findings(t, tc.recipe), tc.rule); got != tc.want {
				t.Errorf("%s: got %v, want %v", tc.rule, got, tc.want)
			}
		})
	}
}

func TestUserSnippet(t *testing.T) {
	r, _ := recipe.Parse([]byte("app = \"flkt-x\"\n" + ubuntu + "[[use]]\nsnippet = \"user-1000\"\nparams = { name = \"steam\" }\n"))
	if _, err := Render(r, catalog(t)); err == nil || !strings.Contains(err.Error(), "already has uid 1000 as ubuntu") {
		t.Errorf("renaming the base's user: got %v", err)
	}
	r, _ = recipe.Parse([]byte("app = \"flkt-x\"\n[[use]]\nsnippet = \"base-alpine\"\n[[use]]\nsnippet = \"user-1000\"\n"))
	res, err := Render(r, catalog(t))
	if err != nil {
		t.Fatal(err)
	}
	df := string(res.File("Dockerfile").Content)
	for _, want := range []string{"apk add --no-cache setpriv", "adduser -D -u 1000 app", "USER app"} {
		if !strings.Contains(df, want) {
			t.Errorf("missing %q in\n%s", want, df)
		}
	}
}

func TestSteamcmd(t *testing.T) {
	r, _ := recipe.Parse([]byte("app = \"flkt-x\"\n" + ubuntu + "[[use]]\nsnippet = \"steamcmd\"\nparams = { app_id = 896660, platform = \"windows\" }\n"))
	res, err := Render(r, catalog(t))
	if err != nil {
		t.Fatal(err)
	}
	df := string(res.File("Dockerfile").Content)
	if strings.Index(df, "AS steam") > strings.Index(df, "FROM docker.io/library/ubuntu") {
		t.Error("the steam stage must come before the final stage")
	}
	for _, want := range []string{"+@sSteamCmdForcePlatformType windows", "app_update 896660", "COPY --from=steam /server /server"} {
		if !strings.Contains(df, want) {
			t.Errorf("missing %q", want)
		}
	}
	if !strings.Contains(string(res.File("flk.toml").Content), `args = { STEAM_BUILDID = "latest" }`) {
		t.Error("no build arg")
	}
	pending := false
	for _, s := range res.Steps {
		pending = pending || s.Kind == "pending" && strings.Contains(s.Text, "isn't pinned")
	}
	if !pending {
		t.Error("an unpinned build should be a 'not in effect yet' line")
	}
}
