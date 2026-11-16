package launcher

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"slices"
	"strconv"
	"time"
)

type SetupStatus struct {
	Revision     string      `json:"revision"`
	EnvRevision  string      `json:"env_revision"`
	DashboardURL string      `json:"dashboard_url"`
	QQ           QQStatus    `json:"qq"`
	Model        ModelStatus `json:"model"`
	InstanceID   string      `json:"instance_id"`
}

type QQStatus struct {
	Bound        bool   `json:"bound"`
	Connected    bool   `json:"connected"`
	Nickname     string `json:"nickname"`
	Phase        string `json:"phase"`
	QRURL        string `json:"qr_url"`
	ExpiresIn    int    `json:"expires_in"`
	NeedsRestart bool   `json:"needs_restart"`
}

type ModelStatus struct {
	Provider     string `json:"provider"`
	ModelName    string `json:"model_name"`
	Configured   bool   `json:"configured"`
	Applied      bool   `json:"applied"`
	NeedsRestart bool   `json:"needs_restart"`
}

type SaveResult struct {
	Saved        bool `json:"saved"`
	Applied      bool `json:"applied"`
	NeedsRestart bool `json:"needs_restart"`
}

type discovery struct {
	Schema     int    `json:"schema_version"`
	URL        string `json:"url"`
	Token      string `json:"token"`
	PID        int    `json:"pid"`
	DataDir    string `json:"data_dir"`
	InstanceID string `json:"instance_id"`
}

var controlHTTP = &http.Client{
	Timeout:       30 * time.Second,
	Transport:     &http.Transport{DialContext: (&net.Dialer{Timeout: 2 * time.Second}).DialContext},
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

func localURL(value string, control bool) bool {
	u, err := url.Parse(value)
	if err != nil || u.Scheme != "http" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return false
	}
	host := u.Hostname()
	port, err := strconv.Atoi(u.Port())
	if err != nil || port < 1 || port > 65535 {
		return false
	}
	if control {
		return host == "127.0.0.1" && u.Path == ""
	}
	return host == "127.0.0.1" || host == "::1"
}

func readDiscovery(data string, ownsPID func(int) bool) (discovery, error) {
	var d discovery
	err := readJSON(filepath.Join(data, ".neobot", "control.json"), &d)
	if err != nil || d.Schema != 1 || !localURL(d.URL, true) || d.Token == "" || d.InstanceID == "" ||
		!ownsPID(d.PID) || !samePath(d.DataDir, data) {
		return discovery{}, errors.New("NeoBot 本机接口尚未就绪")
	}
	return d, nil
}

func (d discovery) call(ctx context.Context, path string, payload any, out any) error {
	method := http.MethodGet
	var body []byte
	if payload != nil {
		method = http.MethodPost
		var err error
		body, err = json.Marshal(payload)
		if err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, d.URL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+d.Token)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := controlHTTP.Do(req)
	if err != nil {
		return errors.New("无法连接 NeoBot 本机接口")
	}
	defer resp.Body.Close()
	var result struct {
		Schema int             `json:"schema_version"`
		OK     bool            `json:"ok"`
		Data   json.RawMessage `json:"data"`
		Error  struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&result) != nil || result.Schema != 1 {
		return errors.New("NeoBot 本机接口返回无效")
	}
	if resp.StatusCode != http.StatusOK || !result.OK {
		if result.Error.Message != "" {
			return errors.New(result.Error.Message)
		}
		return errors.New("NeoBot 未完成此操作")
	}
	var identity struct {
		ID string `json:"instance_id"`
	}
	if json.Unmarshal(result.Data, &identity) != nil || identity.ID != d.InstanceID {
		return errors.New("NeoBot 实例已变化，请重试")
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(result.Data, out)
}

func (d discovery) supported(ctx context.Context) (bool, error) {
	var response struct {
		Capabilities []string `json:"capabilities"`
	}
	err := d.call(ctx, "/v1/status", nil, &response)
	return slices.Contains(response.Capabilities, "desktop.setup.v1"), err
}

func (m *Manager) setupCall(path string, fields map[string]any, out any) error {
	m.mu.Lock()
	bot, supported := m.bot, m.setupSupported && !m.stopping
	m.mu.Unlock()
	if bot == nil || !supported {
		return errors.New("当前实例不支持桌面快捷配置")
	}
	d, err := readDiscovery(bot.data, bot.ownsPID)
	if err != nil {
		return err
	}
	return d.call(context.Background(), path, fields, out)
}

func (m *Manager) BindQQ(revision string, refresh bool) error {
	return m.setupCall("/v1/setup/qq/bind", map[string]any{"revision": revision, "refresh": refresh}, nil)
}

func (m *Manager) SaveQQ(revision, appID, secret string) (SaveResult, error) {
	var result SaveResult
	err := m.setupCall("/v1/setup/qq/credentials", map[string]any{
		"revision": revision, "app_id": appID, "app_secret": secret,
	}, &result)
	return result, err
}

func (m *Manager) SaveModel(revision, envRevision, key, base, model string) (SaveResult, error) {
	var result SaveResult
	err := m.setupCall("/v1/setup/model", map[string]any{
		"revision": revision, "env_revision": envRevision, "api_key": key, "base_url": base, "model_name": model,
	}, &result)
	return result, err
}
