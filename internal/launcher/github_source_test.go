package launcher

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestNormalizeGitHubRepo(t *testing.T) {
	for input, want := range map[string]string{
		"":                  githubRepo,
		"  ":                githubRepo,
		"superquail/NEOBOT": githubRepo,
		" HTTPS://GitHub.com/SUPERQUAIL/NeoBot.GIT/ ": githubRepo,
		"Contrib-Org/My_Neo.Bot/":                     "contrib-org/my_neo.bot",
		"https://github.com/Dev_Corp/NeoBot.git":      "dev_corp/neobot",
		"User/.github":                                "user/.github",
	} {
		got, err := normalizeGitHubRepo(input)
		if err != nil || got != want {
			t.Errorf("normalizeGitHubRepo(%q) = %q, %v; want %q", input, got, err, want)
		}
		if again, err := normalizeGitHubRepo(got); err != nil || again != got {
			t.Errorf("normalizing %q again = %q, %v", got, again, err)
		}
	}
	for _, input := range []string{
		"repo", "/owner/repo", "owner/", "owner/.git", "owner/.", "owner/..", "../repo", "owner/repo//",
		"owner/repo/tree/main", "owner/repo.git.git", "owner/repo x", `owner\repo`, "owner/neobot%2fextra",
		"http://github.com/o/r", "ssh://git@github.com/o/r", "git@github.com:o/r.git", "//github.com/o/r",
		"https://example.com/o/r", "https://github.com.evil.example/o/r", "https://github.com@evil.example/o/r",
		"https://user@github.com/o/r", "https://@github.com/o/r", "https://github.com:443/o/r", "https://github.com:/o/r",
		"https://github.com/o/r?ref=main", "https://github.com/o/r#frag", "https://github.com/o/r?", "https://github.com/o/r#",
		"https://github.com//o/r", "https://github.com/o%2fr", "https://github.com/o/%2e%2e", "https://github.com/o/r/extra/",
	} {
		if got, err := normalizeGitHubRepo(input); err == nil {
			t.Errorf("accepted invalid repository %q as %q", input, got)
		}
	}
}

func TestRepositoryBranchIDs(t *testing.T) {
	if got := repositoryBranchID(githubRepo, "feat/QQ-args"); got != "branch-feat-qq-args-1d312c0a" {
		t.Fatalf("upstream installation ID changed to %q", got)
	}
	seen := map[string]bool{}
	for _, source := range [][2]string{
		{"alice/neobot", "main"}, {"bob/neobot", "main"}, {githubRepo, "main"},
		{"alice/neobot", "Main"}, {githubRepo, "alice/neobot/main"},
		{"alice/neobot", "feat/中文"}, {"alice/neobot", strings.Repeat("x", 200)},
	} {
		id := repositoryBranchID(source[0], source[1])
		if !isBranchID(id) || seen[id] || repositoryBranchID(source[0], source[1]) != id {
			t.Fatalf("unsafe, colliding or unstable ID %q for %q", id, source)
		}
		seen[id] = true
	}
}

func TestGitHubRedirectsStayOnGitHub(t *testing.T) {
	for _, test := range []struct {
		client  *http.Client
		address string
		allowed bool
	}{
		{githubAPIHTTP, "https://api.github.com/repositories/123", true},
		{branchDownloadHTTP, "https://codeload.github.com/org/repo/zip/refs/heads/dev", true},
		{githubAPIHTTP, "https://other.example/repos/org/repo", false},
		{branchDownloadHTTP, "https://codeload.github.com.other.example/org/repo", false},
		{branchDownloadHTTP, "http://github.com/org/repo", false},
		{branchDownloadHTTP, "https://user@github.com/org/repo", false},
		{branchDownloadHTTP, "https://github.com:444/org/repo", false},
	} {
		req, err := http.NewRequest(http.MethodGet, test.address, nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := test.client.CheckRedirect(req, nil); (err == nil) != test.allowed {
			t.Errorf("redirect %s: %v; want allowed=%v", test.address, err, test.allowed)
		}
	}
}

func TestGitHubBranchCacheIsolation(t *testing.T) {
	var offline atomic.Bool
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if offline.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		switch r.URL.Path {
		case "/repos/a/neobot/branches":
			_, _ = w.Write([]byte(`[{"name":"main","commit":{"sha":"aaa"}}]`))
		case "/repos/b/neobot/branches":
			_, _ = w.Write([]byte(`[{"name":"main","commit":{"sha":"bbb"}}]`))
		default:
			t.Errorf("unexpected branch list URL %s", r.URL)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	old := githubAPIBase
	githubAPIBase = server.URL
	defer func() { githubAPIBase = old }()

	m := &Manager{branches: []Branch{{Name: "legacy"}}, branchesAt: time.Now()}
	list := func(repo, wantSHA string, wantRequests int32) []Branch {
		t.Helper()
		got, err := m.ListGitHubBranches(repo, false)
		if err != nil || len(got) != 1 || got[0].Commit != wantSHA || requests.Load() != wantRequests {
			t.Fatalf("list %s = %+v, %v, requests=%d; want %s, %d", repo, got, err, requests.Load(), wantSHA, wantRequests)
		}
		return got
	}
	first := list("A/Neobot", "aaa", 1)
	first[0].Commit = "changed by caller"
	list("https://github.com/a/NEOBOT.git/", "aaa", 1)
	list("b/neobot", "bbb", 2)

	m.branchesAt = time.Now().Add(-2 * branchCacheTTL)
	offline.Store(true)
	list("b/neobot", "bbb", 3) // A stale list is still useful for its own repository.
	if got, err := m.ListGitHubBranches("a/neobot", false); err == nil || len(got) != 0 {
		t.Errorf("offline request for a/neobot reused b/neobot's list: %+v, %v", got, err)
	}
	if got, err := m.ListGitHubBranches("b/neobot", true); err == nil || len(got) != 0 {
		t.Errorf("forced refresh hid the fetch error: %+v, %v", got, err)
	}
}

func TestFetchGitHubDefaultBranch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/SuperQuail/NeoBot":
			_, _ = w.Write([]byte(`{"default_branch":"stable/中文+1"}`))
		case "/repos/user/unsafe":
			_, _ = w.Write([]byte(`{"default_branch":"../bad"}`))
		case "/repos/user/empty":
			_, _ = w.Write([]byte(`{}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	old := githubAPIBase
	githubAPIBase = server.URL
	defer func() { githubAPIBase = old }()
	if got, err := fetchGitHubDefaultBranch(context.Background(), githubRepo); err != nil || got != "stable/中文+1" {
		t.Fatalf("default branch = %q, %v", got, err)
	}
	for _, repo := range []string{"user/unsafe", "user/empty", "user/missing"} {
		if got, err := fetchGitHubDefaultBranch(context.Background(), repo); err == nil || got != "" {
			t.Errorf("invented a default for %s: %q, %v", repo, got, err)
		}
	}
}

func TestInstallBranchResolvesAndRecordsSource(t *testing.T) {
	if !supported() {
		t.Skip("分支安装仅支持 Windows x64")
	}
	const repo, defaultBranch = "a/neobot", "Release/中文+1"
	var lookups atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lookups.Add(1)
		if r.URL.RequestURI() != "/repos/a/neobot" {
			t.Errorf("unexpected metadata URL %s", r.URL)
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"default_branch":"Release/中文+1"}`))
	}))
	defer server.Close()
	old := githubAPIBase
	githubAPIBase = server.URL
	defer func() { githubAPIBase = old }()

	m := &Manager{root: t.TempDir(), installed: map[string]Version{}}
	// Stop the job before uv is fetched so these entry-point checks stay offline.
	if err := os.WriteFile(filepath.Join(m.root, "tools"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ input, want string }{{"", defaultBranch}, {" Topic/Manual ", "Topic/Manual"}} {
		if err := m.InstallBranch("https://github.com/A/NeoBot.git/", test.input, false, true); err != nil {
			t.Fatal(err)
		}
		<-m.jobDone
		job := m.Snapshot().Job
		if job.Repository != repo || job.Branch != test.want || job.Version != repositoryBranchID(repo, test.want) || job.State != "failed" {
			t.Fatalf("job for input %q = %+v", test.input, job)
		}
	}
	if lookups.Load() != 1 {
		t.Fatalf("manual input triggered metadata requests: %d lookups", lookups.Load())
	}
	id := repositoryBranchID(repo, defaultBranch)
	m.installed[id] = Version{Name: id, Repository: repo, Branch: defaultBranch}
	m.bot = &botProcess{version: id}
	if err := m.InstallBranch(repo, "", false, true); err == nil || !strings.Contains(err.Error(), "停止") {
		t.Fatalf("allowed a refresh of the running default branch: %v", err)
	}
}
