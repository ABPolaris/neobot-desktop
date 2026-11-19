package launcher

// 宿主（NeoBot 自身）日志的读取。日志由 loguru 写在 <data_dir>/logs/ 下，
// 运行中一直在增长，单个文件可达 10MB，所以这里一律按行扫描、只保留需要的那一段。

import (
	"bufio"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	// hostLogPrefix 是 loguru 写的宿主日志文件名本体。
	hostLogPrefix = "neobot.log"
	// hostLogStem / hostLogSuffix 描述轮转后的文件名：loguru 把时间戳插在扩展名之前，
	// 于是 neobot.log 被改成 neobot.2026-11-19_10-00-00.log，两者前缀并不相同。
	hostLogStem   = "neobot."
	hostLogSuffix = ".log"
	// hostLogDefaultLines / hostLogMaxLines 限制一次返回的行数，界面按 500 行分页。
	hostLogDefaultLines = 500
	hostLogMaxLines     = 2000
	// hostLogLineLimit 限制单行长度：一条超长日志不该把界面和内存撑爆。
	hostLogLineLimit = 8192
)

// HostLogFile 是宿主（NeoBot 自身）日志目录里的一个文件。
type HostLogFile struct {
	Name  string `json:"name"`
	Bytes int64  `json:"bytes"`
	At    int64  `json:"at"`
}

// HostLogPage 是宿主日志的一段内容。
type HostLogPage struct {
	File  string   `json:"file"`
	Lines []string `json:"lines"`
	Start int      `json:"start"`
	Total int      `json:"total"`
}

// HostLogFiles 列出 <data_dir>/logs/neobot.log* 及其轮转文件，按时间倒序。
func (m *Manager) HostLogFiles() ([]HostLogFile, error) {
	dir := m.hostLogDir()
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		// 还没运行过 NeoBot：不是错误，界面显示「暂无日志」即可。
		return []HostLogFile{}, nil
	}
	if err != nil {
		return nil, errors.New("无法读取 NeoBot 日志目录，请检查数据目录权限后重试")
	}
	files := make([]HostLogFile, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !isHostLogName(name) {
			continue
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		files = append(files, HostLogFile{Name: name, Bytes: info.Size(), At: info.ModTime().Unix()})
	}
	sort.Slice(files, func(i, j int) bool {
		if files[i].At == files[j].At {
			return files[i].Name > files[j].Name
		}
		return files[i].At > files[j].At
	})
	return files, nil
}

// isHostLogName 判断文件名是不是宿主日志：neobot.log 本体，或轮转出来的
// neobot.<时间戳>.log。拼接路径之前先用它挡住其它文件。
func isHostLogName(name string) bool {
	return strings.HasPrefix(name, hostLogStem) && strings.HasSuffix(name, hostLogSuffix)
}

// hostLogDir 返回宿主日志目录。运行中优先用正在运行的版本的数据目录，
// 否则用下次启动的版本，和 Snapshot.DataDir 的口径保持一致。
func (m *Manager) hostLogDir() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	inUse := m.state.Selected
	if m.bot != nil {
		inUse = m.bot.version
	}
	return filepath.Join(m.dataDir(inUse), "logs")
}

// ReadHostLog 读取宿主日志的第 start 行起的 count 行。
// 日志是 DEBUG 级、单文件可达 10MB，必须按块读，不能整文件载入内存。
// 输出前用 redactLogLine 再脱敏一次，防止上游脱敏遗漏。
//
// start 为负数时读取最后 count 行：界面第一次打开只需要最新的尾部，
// 这样不必为了知道总行数而把文件扫两遍。返回的 Start 是页内第一行的行号，
// Total 是文件总行数，界面据此往前翻页。
func (m *Manager) ReadHostLog(name string, start, count int) (HostLogPage, error) {
	if count <= 0 {
		count = hostLogDefaultLines
	}
	if count > hostLogMaxLines {
		count = hostLogMaxLines
	}
	path, err := m.hostLogPath(name)
	if err != nil {
		return HostLogPage{}, err
	}
	page, err := readHostLogPage(path, start, count)
	if err != nil {
		return HostLogPage{}, err
	}
	page.File = name
	return page, nil
}

// hostLogPath 把界面传来的文件名解析成日志路径，并挡住路径穿越。
func (m *Manager) hostLogPath(name string) (string, error) {
	// 只接受纯粹的日志文件名：分隔符、盘符或 .. 一律拒绝，先挡在拼接路径之前。
	if name == "" || name != filepath.Base(name) || strings.ContainsAny(name, `/\`) || !isHostLogName(name) {
		return "", errors.New("日志文件名无效，请刷新列表后重试")
	}
	path := filepath.Join(m.hostLogDir(), name)
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", errors.New("这个日志文件已经不存在，请刷新列表后重试")
	}
	if err != nil {
		return "", errors.New("无法读取这个日志文件，请刷新列表后重试")
	}
	// 只读普通文件：软链接可能指向数据目录之外的文件。
	if !info.Mode().IsRegular() {
		return "", errors.New("这个日志文件不可读，请刷新列表后重试")
	}
	return path, nil
}

// readHostLogPage 逐行扫描日志文件。start 小于 0 时只保留最后 count 行（环形缓冲），
// start 之前的内容读完就丢，不会留在内存里。
func readHostLogPage(path string, start, count int) (HostLogPage, error) {
	file, err := os.Open(path)
	if err != nil {
		return HostLogPage{}, errors.New("无法读取这个日志文件，请刷新列表后重试")
	}
	defer file.Close()

	reader := bufio.NewReaderSize(file, 64<<10)
	lines := make([]string, 0, count)
	tail := start < 0
	next := 0 // 环形缓冲下一次覆盖的位置
	total := 0
	for {
		raw, readErr := reader.ReadString('\n')
		if len(raw) > 0 {
			// 去掉行尾换行：最后一行可能没有换行符；ReadString 只按 \n 切，\r 自己剥。
			line := cleanHostLogLine(strings.TrimRight(raw, "\r\n"))
			if tail {
				if len(lines) < count {
					lines = append(lines, line)
				} else {
					lines[next] = line
					next = (next + 1) % count
				}
			} else if total >= start && total < start+count {
				lines = append(lines, line)
			}
			total++
		}
		if readErr == nil {
			continue
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		return HostLogPage{}, errors.New("读取日志失败，请稍后重试")
	}

	page := HostLogPage{Total: total, Lines: lines}
	if tail {
		if next > 0 {
			// 缓冲已经绕过一圈，把最早写入的那段挪回前面。
			page.Lines = append(append(make([]string, 0, count), lines[next:]...), lines[:next]...)
		}
		page.Start = total - len(page.Lines)
		if page.Start < 0 {
			page.Start = 0
		}
		return page, nil
	}
	page.Start = start
	if page.Start > total {
		page.Start = total
	}
	return page, nil
}

// cleanHostLogLine 清掉 ANSI 转义和控制字符，再脱敏一次，最后限制单行长度。
func cleanHostLogLine(line string) string {
	line = redactLogLine(printableLine(line))
	if len(line) <= hostLogLineLimit {
		return line
	}
	// 截断要落在 UTF-8 边界上，否则 JSON 里会多出乱码字符。
	cut := line[:hostLogLineLimit]
	for len(cut) > 0 && !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1]
	}
	return cut + "…（本行过长已截断）"
}
