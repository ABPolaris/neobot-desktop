package launcher

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"
)

const (
	githubRepo     = "SuperQuail/NeoBot"
	branchCacheTTL = 10 * time.Minute
	branchAPILimit = int64(1 << 20)
	branchMaxPages = 3
)

var (
	githubAPIBase     = "https://api.github.com"
	githubArchiveBase = "https://github.com"
	githubAPIHTTP     = &http.Client{
		Timeout: 10 * time.Second, CheckRedirect: githubRedirect("api.github.com"),
	}
	branchDownloadHTTP = &http.Client{
		Timeout: 10 * time.Minute, CheckRedirect: githubRedirect("github.com", "codeload.github.com"),
	}
	githubOwnerPattern   = regexp.MustCompile(`^[A-Za-z0-9_-]{1,39}$`)
	githubNamePattern    = regexp.MustCompile(`^[A-Za-z0-9._-]{1,104}$`) // Up to 100 characters plus an optional .git.
	errInvalidGitHubRepo = errors.New("请输入 GitHub 仓库根地址（https://github.com/owner/repo）或 owner/repo")
)

func githubRedirect(hosts ...string) func(*http.Request, []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		if len(via) < 10 && req.URL.Scheme == "https" && req.URL.User == nil {
			for _, host := range hosts {
				if strings.EqualFold(req.URL.Host, host) {
					return nil
				}
			}
		}
		return errors.New("无效的 GitHub 重定向")
	}
}

// normalizeGitHubRepo returns the same key for a clone URL and its owner/repo shorthand.
func normalizeGitHubRepo(input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return githubRepo, nil
	}
	if strings.Contains(input, "://") {
		u, err := url.Parse(input)
		if err != nil || u.Scheme != "https" || !strings.EqualFold(u.Host, "github.com") || u.User != nil || strings.ContainsAny(input, "?#") {
			return "", errInvalidGitHubRepo
		}
		// Keep escapes visible to validation, including encoded slashes and dots.
		input = strings.TrimPrefix(u.EscapedPath(), "/")
	}
	owner, name, ok := strings.Cut(strings.TrimSuffix(input, "/"), "/")
	if !ok || !githubOwnerPattern.MatchString(owner) || !githubNamePattern.MatchString(name) {
		return "", errInvalidGitHubRepo
	}
	name = strings.TrimSuffix(strings.ToLower(name), ".git")
	if len(name) > 100 || strings.Trim(name, ".") == "" || strings.HasSuffix(name, ".git") {
		return "", errInvalidGitHubRepo
	}
	repository := strings.ToLower(owner) + "/" + name
	if strings.EqualFold(repository, githubRepo) {
		return githubRepo, nil
	}
	return repository, nil
}

// repositoryBranchID takes a normalized repository and the case-sensitive Git ref.
func repositoryBranchID(repository, branch string) string {
	if repository == githubRepo {
		return branchID(branch) // Keep existing installations, data and selections addressable.
	}
	sum := sha256.Sum256([]byte(repository + "\x00" + branch))
	// The legacy slugger collapses double hyphens, reserving this namespace for forks.
	return fmt.Sprintf("branch-fork--%x-%x", sum[:16], sum[16:20])
}

type Branch struct {
	Name   string `json:"name"`
	Commit string `json:"commit,omitempty"`
}

func (m *Manager) ListGitHubBranches(repository string, force bool) ([]Branch, error) {
	repository, err := normalizeGitHubRepo(repository)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	var cached []Branch
	if !force && repository == m.branchRepo {
		cached = slices.Clone(m.branches)
	}
	fresh := len(cached) > 0 && time.Since(m.branchesAt) < branchCacheTTL
	m.mu.Unlock()
	if fresh {
		return cached, nil
	}
	branches, err := fetchGitHubBranches(context.Background(), repository)
	if err != nil {
		if len(cached) > 0 {
			return cached, nil
		}
		return nil, err
	}
	m.mu.Lock()
	m.branchRepo, m.branches, m.branchesAt = repository, slices.Clone(branches), time.Now()
	m.mu.Unlock()
	return branches, nil
}

// readGitHubRepoAPI bounds and decodes a response. Pagination only uses the Link
// header to decide whether to request another page on the same repository.
func readGitHubRepoAPI(ctx context.Context, resource, subject string, out any) (http.Header, error) {
	address := strings.TrimRight(githubAPIBase, "/") + "/repos/" + resource
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, fmt.Errorf("无法获取%s，请检查网络后重试", subject)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "neobot-desktop")
	resp, err := githubAPIHTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("无法获取%s，请检查网络后重试", subject)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests || (resp.StatusCode == http.StatusForbidden &&
		(resp.Header.Get("X-RateLimit-Remaining") == "0" || resp.Header.Get("Retry-After") != "")) {
		return nil, fmt.Errorf("GitHub %s暂时不可用（接口限流），可直接输入分支名", subject)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub 返回 HTTP %d，无法获取%s", resp.StatusCode, subject)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, branchAPILimit)).Decode(out); err != nil {
		return nil, fmt.Errorf("无法解析 GitHub %s", subject)
	}
	return resp.Header, nil
}

func fetchGitHubDefaultBranch(ctx context.Context, repository string) (string, error) {
	var meta struct {
		DefaultBranch string `json:"default_branch"`
	}
	if _, err := readGitHubRepoAPI(ctx, repository, "默认分支", &meta); err != nil {
		return "", err
	}
	if !validBranchName(meta.DefaultBranch) {
		return "", errors.New("仓库没有返回可安装的默认分支，请直接输入分支名")
	}
	return meta.DefaultBranch, nil
}

func fetchGitHubBranches(ctx context.Context, repository string) ([]Branch, error) {
	var result []Branch
	seen := make(map[string]bool)
	for page := 1; page <= branchMaxPages; page++ {
		var items []struct {
			Name   string `json:"name"`
			Commit struct {
				SHA string `json:"sha"`
			} `json:"commit"`
		}
		resource := fmt.Sprintf("%s/branches?per_page=100&page=%d", repository, page)
		headers, err := readGitHubRepoAPI(ctx, resource, "分支列表", &items)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			if !validBranchName(item.Name) || seen[item.Name] {
				continue
			}
			seen[item.Name] = true
			result = append(result, Branch{Name: item.Name, Commit: item.Commit.SHA})
		}
		if !strings.Contains(headers.Get("Link"), `rel="next"`) {
			break
		}
	}
	if len(result) == 0 {
		return nil, errors.New("没有找到可用的 GitHub 分支")
	}
	sort.Slice(result, func(i, j int) bool {
		if (result[i].Name == "main") != (result[j].Name == "main") {
			return result[i].Name == "main"
		}
		return result[i].Name < result[j].Name
	})
	return result, nil
}
