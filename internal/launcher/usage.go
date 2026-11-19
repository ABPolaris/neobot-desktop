package launcher

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// VersionUsage 是单个已安装版本占用的磁盘空间。
type VersionUsage struct {
	Name  string `json:"name"`
	Label string `json:"label"`
	Bytes int64  `json:"bytes"`
}

// Usage 是启动器数据根目录下各类内容的磁盘占用。换版本、清理缓存和卸载版本都依赖它。
type Usage struct {
	Scanning  bool           `json:"scanning"`
	ScannedAt int64          `json:"scanned_at"`
	Versions  []VersionUsage `json:"versions"`
	Python    int64          `json:"python"`
	Cache     int64          `json:"cache"`
	Backups   int64          `json:"backups"`
	Data      int64          `json:"data"`
	Isolated  int64          `json:"isolated"`
	Logs      int64          `json:"logs"`
	Temp      int64          `json:"temp"`
	Tools     int64          `json:"tools"`
	Total     int64          `json:"total"`
	Error     string         `json:"error"`
}

// RefreshUsage 异步统计磁盘占用，结果写入 Snapshot.Usage 并触发 launcher:changed。
// 统计可能耗时数秒（要遍历几百 MB 的虚拟环境），因此不得在持锁状态下遍历。
func (m *Manager) RefreshUsage() error {
	m.mu.Lock()
	if m.usageBusy {
		// 上一次统计还没结束：界面上已经是「正在统计」，这次直接复用它即将写回的结果。
		m.mu.Unlock()
		return nil
	}
	m.usageBusy = true
	m.usage = Usage{Scanning: true}
	root := m.root
	// 已安装版本在锁内取一份拷贝；遍历各自的版本目录放在锁外，否则界面会跟着卡住。
	installed := make([]Version, 0, len(m.installed))
	for _, version := range m.installed {
		installed = append(installed, version)
	}
	m.mu.Unlock()
	// notify 必须放在释放锁之后：changed 回调里会取 Snapshot，持锁调用会自己等自己。
	m.notify()

	go func() {
		next := usageScan(root, installed)
		m.mu.Lock()
		m.usageBusy = false
		m.usage = next
		closing := m.closing
		m.mu.Unlock()
		// 关闭过程中的通知没有意义，还可能碰到已经拆掉的事件通道。
		if !closing {
			m.notify()
		}
	}()
	return nil
}

// CleanUsage 清理一类可再生的数据，返回释放的字节数。
// target 取值："cache"（uv 缓存）、"tmp"（临时目录）、"backups"（历史快照）。
// 运行中或安装任务进行中必须拒绝，避免删掉正在使用的文件。
func (m *Manager) CleanUsage(target string) (int64, error) {
	m.mu.Lock()
	switch {
	case m.root == "":
		// 数据目录为空时 filepath.Join 会得到相对路径，删除会落到进程当前目录上。
		m.mu.Unlock()
		return 0, errors.New("数据目录尚未就绪，请重新打开启动器后再清理")
	case m.closing:
		m.mu.Unlock()
		return 0, errors.New("启动器正在关闭，请重新打开后再清理")
	case m.job.State == "running":
		m.mu.Unlock()
		return 0, errors.New("安装或更新任务正在进行，请等待任务结束后再清理")
	case m.preparing || m.stopping:
		m.mu.Unlock()
		return 0, errors.New("NeoBot 正在启动或停止，请等待完成后再清理")
	case m.bot != nil:
		m.mu.Unlock()
		return 0, errors.New("请先停止 NeoBot 再清理，运行中删除这些文件夹会让正在运行的程序出错")
	}
	// python/ 与 versions/ 永远不在这里：重新下载要几百 MB，只能由版本管理单独处理。
	var dir string
	switch target {
	case "cache":
		dir = filepath.Join(m.root, "cache")
	case "tmp":
		dir = filepath.Join(m.root, "tmp")
	case "backups":
		dir = filepath.Join(m.root, "backups")
	default:
		m.mu.Unlock()
		return 0, errors.New("未知的清理目标，只能清理缓存、临时文件或历史快照")
	}
	m.mu.Unlock()

	// 先量再删，返回的必须是删除前的占用。
	before := usageDirSize(dir)
	if target == "backups" {
		// 快照记录留在 settings.json 里会指向已经删掉的目录：界面提示「没有兼容快照」
		// 却又显示快照存在。先清记录再删文件，删一半失败最多留下一些无人引用的目录。
		m.mu.Lock()
		previous := m.state.Backups
		m.state.Backups = map[string]string{}
		err := m.persist()
		if err != nil {
			m.state.Backups = previous
		}
		m.mu.Unlock()
		if err != nil {
			return 0, fmt.Errorf("无法保存清理结果，快照未删除，请检查数据目录权限后重试：%w", err)
		}
	}
	if err := os.RemoveAll(dir); err != nil {
		return 0, fmt.Errorf("无法清理%s，请关闭占用该文件夹的程序后重试：%w", usageCleanLabel(target), err)
	}
	// 删除后重新统计一次，让界面立刻显示清理后的占用。
	_ = m.RefreshUsage()
	return before, nil
}

// usageScan 统计 root 下各类内容的占用。它不持锁也不改 Manager 状态，遍历全部在锁外做。
// 单个文件或目录读不到时按 0 算，不中断整次统计：这里只是给用户看的大概数字。
func usageScan(root string, installed []Version) Usage {
	usage := Usage{ScannedAt: time.Now().Unix()}
	if _, err := os.Stat(root); err != nil {
		usage.Error = "无法读取数据目录，请检查文件夹权限后重新统计"
		return usage
	}
	usage.Python = usageDirSize(filepath.Join(root, "python"))
	usage.Cache = usageDirSize(filepath.Join(root, "cache"))
	usage.Tools = usageDirSize(filepath.Join(root, "tools"))
	usage.Temp = usageDirSize(filepath.Join(root, "tmp"))
	usage.Backups = usageDirSize(filepath.Join(root, "backups"))
	dataDir := filepath.Join(root, "data")
	isolatedDir := filepath.Join(root, "isolated")
	usage.Data = usageDirSize(dataDir)
	usage.Isolated = usageDirSize(isolatedDir)
	// 日志单独算一类：启动器日志在 root/logs，宿主日志在各数据目录的 logs/ 下。
	// 后者本来已经算进 data/isolated，这里先量出来再从父目录扣掉，否则总计会重复计算。
	usage.Logs = usageDirSize(filepath.Join(root, "logs"))
	if size := usageDirSize(filepath.Join(dataDir, "logs")); size > 0 {
		usage.Logs += size
		usage.Data -= size
	}
	if entries, err := os.ReadDir(isolatedDir); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			if size := usageDirSize(filepath.Join(isolatedDir, entry.Name(), "logs")); size > 0 {
				usage.Logs += size
				usage.Isolated -= size
			}
		}
	}
	usage.Versions = make([]VersionUsage, 0, len(installed))
	for _, version := range installed {
		usage.Versions = append(usage.Versions, VersionUsage{
			Name:  version.Name,
			Label: version.Label(),
			Bytes: usageDirSize(filepath.Join(root, "versions", version.Name)),
		})
	}
	// 占用最大的版本排在最前面；大小相同时按名字排，保证多次统计的顺序稳定。
	sort.Slice(usage.Versions, func(i, j int) bool {
		if usage.Versions[i].Bytes != usage.Versions[j].Bytes {
			return usage.Versions[i].Bytes > usage.Versions[j].Bytes
		}
		return usage.Versions[i].Name < usage.Versions[j].Name
	})
	usage.Total = usage.Python + usage.Cache + usage.Tools + usage.Temp +
		usage.Backups + usage.Data + usage.Isolated + usage.Logs
	for _, version := range usage.Versions {
		usage.Total += version.Bytes
	}
	return usage
}

// usageDirSize 递归累加目录下普通文件的大小。目录不存在或遍历出错一律跳过并返回已统计
// 的部分：统计只是为了显示，不能因为某个文件被占用或没有权限就让整次统计失败。
func usageDirSize(dir string) int64 {
	var total int64
	_ = filepath.WalkDir(dir, func(_ string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			// 进不去的子目录直接跳过，不要中断整棵树的遍历。
			if entry != nil && entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		// 目录本身不占实际空间；符号链接不跟随，避免重复计算或绕进环里。
		if entry.IsDir() || !entry.Type().IsRegular() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		total += info.Size()
		return nil
	})
	return total
}

// usageCleanLabel 给出清理目标在提示文案里的中文名字。
func usageCleanLabel(target string) string {
	switch target {
	case "cache":
		return "下载缓存"
	case "tmp":
		return "临时文件"
	case "backups":
		return "历史快照"
	default:
		return "目录"
	}
}
