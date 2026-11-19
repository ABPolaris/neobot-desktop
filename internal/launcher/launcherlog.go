package launcher

// 启动器自身日志落盘。sessionLog.add 把每一行交给 m.logs.sink，由这里异步写文件。
// 目录：<root>/logs/launcher-<纳秒>.log；只保留最近 10 个文件，单个文件写到上限就换新的。
//
// 为什么异步：sink 是在 sessionLog 的锁里被调用的，而 sessionLog.add 又跑在读
// NeoBot stdout 的 goroutine 上。同步写文件在杀软/U 盘上会慢到几百毫秒，直接把
// 子进程的输出读卡住，看起来就是「NeoBot 启动到一半不动了」。

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

const (
	// launcherLogKeep 是保留的启动器日志个数：超出的按修改时间从旧到新删掉。
	launcherLogKeep = 10
	// launcherLogPrefix / launcherLogSuffix 标识本启动器写的日志文件。
	launcherLogPrefix = "launcher-"
	launcherLogSuffix = ".log"
	// launcherLogMaxBytes 是单个日志文件的上限：常驻托盘几周也不会写满磁盘。
	launcherLogMaxBytes = 4 << 20
	// launcherLogQueue 是写入队列长度。队列满时宁可丢日志，也不阻塞子进程输出。
	launcherLogQueue = 1024
)

// launcherLogMu 只保护 m.logState 这个字段本身。
//
// 这里不能再取 m.mu：sessionLog.add 是持着日志锁调用 sink 的，而管理器的其它路径
// （例如 Start）会先取 m.mu 再写日志，反过来加锁必然死锁。
var launcherLogMu sync.Mutex

// launcherLogState 是一个日志文件 + 它的写入协程。
type launcherLogState struct {
	mu     sync.Mutex
	file   *os.File
	size   int64
	queue  chan string
	done   chan struct{}
	closed bool
}

// launcherLogDir 返回启动器日志目录。
func (m *Manager) launcherLogDir() string { return filepath.Join(m.root, "logs") }

// startLauncherLog 打开本次运行的日志文件并启动写入协程。失败时静默降级为只写内存，
// 不能因为磁盘问题让启动器起不来。
func (m *Manager) startLauncherLog() {
	dir := m.launcherLogDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		m.closeLauncherLog()
		return
	}
	file, name, err := openLauncherLog(dir)
	if err != nil {
		m.closeLauncherLog()
		return
	}
	state := &launcherLogState{file: file, queue: make(chan string, launcherLogQueue), done: make(chan struct{})}
	launcherLogMu.Lock()
	previous := m.logState
	m.logState = state
	launcherLogMu.Unlock()
	if previous != nil {
		previous.shutdown()
	}
	// 新文件已经就位再清理旧文件：先删后建，一旦创建失败就白丢了上一次运行的日志。
	pruneLauncherLogs(dir, name)
	go state.run(dir)
}

// openLauncherLog 创建一个新的日志文件。
func openLauncherLog(dir string) (*os.File, string, error) {
	name := launcherLogPrefix + timeID() + launcherLogSuffix
	file, err := os.OpenFile(filepath.Join(dir, name), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	return file, name, err
}

// run 是唯一的写盘协程：顺序消费队列，超过体积上限就换一个新文件。
func (s *launcherLogState) run(dir string) {
	defer close(s.done)
	for line := range s.queue {
		// 只在取文件句柄和累加体积时持锁：真正写盘必须放锁外，
		// 否则慢盘上一次写就会把生产者（子进程输出读取）一起卡住。
		// s.file 只有本协程会改（rotate 也在这里调用），所以本地副本是安全的。
		s.mu.Lock()
		file := s.file
		s.mu.Unlock()
		if file == nil {
			continue
		}
		written, err := file.WriteString(line + "\n")
		s.mu.Lock()
		s.size += int64(written)
		overflow := err != nil || s.size >= launcherLogMaxBytes
		s.mu.Unlock()
		if !overflow {
			continue
		}
		// 写失败或到达上限：关掉旧的，换一个新文件继续写。
		s.rotate(dir)
	}
	s.mu.Lock()
	file := s.file
	s.file = nil
	s.mu.Unlock()
	if file != nil {
		_ = file.Close()
	}
}

// rotate 换一个新的日志文件，并清理旧的。
func (s *launcherLogState) rotate(dir string) {
	s.mu.Lock()
	old := s.file
	s.file = nil
	s.mu.Unlock()
	if old != nil {
		_ = old.Close()
	}
	file, name, err := openLauncherLog(dir)
	if err != nil {
		return
	}
	s.mu.Lock()
	s.file, s.size = file, 0
	s.mu.Unlock()
	pruneLauncherLogs(dir, name)
}

// write 把一行放进写入队列。队列满时直接丢弃，绝不阻塞调用方。
func (s *launcherLogState) write(line string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	select {
	case s.queue <- line:
	default:
	}
}

// shutdown 结束写入协程并关闭文件。可以重复调用。
func (s *launcherLogState) shutdown() {
	s.mu.Lock()
	if !s.closed {
		s.closed = true
		close(s.queue)
	}
	s.mu.Unlock()
	<-s.done
}

// pruneLauncherLogs 只保留最新的 launcherLogKeep 个日志文件，其余删除。
// keep 是本次新建的文件名，无论如何都保留。任何错误都忽略：清理日志不能影响启动。
func pruneLauncherLogs(dir, keep string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	type launcherLogFile struct {
		name string
		at   int64
	}
	files := make([]launcherLogFile, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, launcherLogPrefix) || !strings.HasSuffix(name, launcherLogSuffix) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, launcherLogFile{name: name, at: info.ModTime().UnixNano()})
	}
	if len(files) <= launcherLogKeep {
		return
	}
	// 新的在前：修改时间相同时按文件名（纳秒时间戳）兜底，删的顺序才是确定的。
	sort.Slice(files, func(i, j int) bool {
		if files[i].at == files[j].at {
			return files[i].name > files[j].name
		}
		return files[i].at > files[j].at
	})
	for index, file := range files {
		if index < launcherLogKeep || file.name == keep {
			continue
		}
		_ = os.Remove(filepath.Join(dir, file.name))
	}
}

// appendLauncherLog 写一行到启动器日志。调用方持有日志锁，这里只取指针后立刻返回。
func (m *Manager) appendLauncherLog(line string) {
	launcherLogMu.Lock()
	state := m.logState
	launcherLogMu.Unlock()
	if state == nil {
		return
	}
	// 去掉控制字符：一条日志必须占住一行，否则输出里的换行会把日志文件冲乱。
	state.write(printableLine(line))
}

// closeLauncherLog 结束写入协程并关闭文件。可以重复调用。
func (m *Manager) closeLauncherLog() {
	launcherLogMu.Lock()
	state := m.logState
	m.logState = nil
	launcherLogMu.Unlock()
	if state != nil {
		state.shutdown()
	}
}
