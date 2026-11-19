package launcher

import (
	"bufio"
	"errors"
	"io"
	"regexp"
	"strings"
	"sync"
	"unicode"
)

const (
	logBeginning = 120
	logRecent    = 360
	logLineLimit = 2048
)

// Keep the beginning of a run available even after normal output fills the tail.
// Output revisions are independent of launcher status updates.
type sessionLog struct {
	mu       sync.Mutex
	revision uint64
	total    uint64
	version  string
	branch   string
	first    []string
	recent   [logRecent]string
	// sink 把每一行同时写给启动器日志文件；为 nil 时只保留内存缓冲。
	sink func(string)
}

type LogSnapshot struct {
	Revision uint64   `json:"revision"`
	Version  string   `json:"version"`
	Branch   string   `json:"branch,omitempty"`
	Lines    []string `json:"lines"`
	Omitted  uint64   `json:"omitted"`
	GapAfter int      `json:"gap_after"`
}

func (l *sessionLog) reset(version, branch string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.revision++
	l.total, l.version, l.branch, l.first = 0, version, branch, nil
	clear(l.recent[:])
}

func (l *sessionLog) add(line string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.total < logBeginning {
		l.first = append(l.first, line)
	} else {
		l.recent[(l.total-logBeginning)%logRecent] = line
	}
	l.total++
	l.revision++
	if l.sink != nil {
		l.sink(line)
	}
}

func (m *Manager) Logs(revision uint64) LogSnapshot {
	l := &m.logs
	l.mu.Lock()
	defer l.mu.Unlock()
	if revision == l.revision && revision != 0 {
		return LogSnapshot{Revision: revision}
	}
	first := uint64(len(l.first))
	tail := min(l.total-first, logRecent)
	omitted := l.total - first - tail
	lines := make([]string, 0, first+tail)
	lines = append(lines, l.first...)
	for i := omitted; i < omitted+tail; i++ {
		lines = append(lines, l.recent[i%logRecent])
	}
	return LogSnapshot{Revision: l.revision, Version: l.version, Branch: l.branch, Lines: lines, Omitted: omitted, GapAfter: len(l.first)}
}

var (
	terminalEscape = regexp.MustCompile(`\x1b(?:\[[0-?]*[ -/]*[@-~]|\][^\x07\x1b]*(?:\x07|\x1b\\))`)
	logCredentials = regexp.MustCompile(`(?i)((?:api[-_]?key|app[-_]?secret|access[-_]?token|control[-_]?token|token|password|passwd|secret|authorization|cookie)(?:["'])?\s*[:=]\s*)(?:"(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*'|(?:bearer\s+)?[^\s,;&}\]]+)`)
	logBearer      = regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~+/=-]+`)
	logURLPassword = regexp.MustCompile(`(?i)([a-z][a-z0-9+.-]*://)[^/\s:@]+:[^/\s@]+@`)
)

func printableLine(line string) string {
	line = terminalEscape.ReplaceAllString(line, "")
	return strings.Map(func(r rune) rune {
		if r != '\t' && unicode.IsControl(r) {
			return -1
		}
		return r
	}, line)
}

func redactLogLine(line string) string {
	line = logCredentials.ReplaceAllString(line, "${1}[已隐藏]")
	line = logBearer.ReplaceAllString(line, "Bearer [已隐藏]")
	return logURLPassword.ReplaceAllString(line, "${1}[已隐藏]@")
}

// A very long line must not stop the reader or swallow subsequent errors.
func readOutputLines(r io.Reader, write func(string)) {
	buf := bufio.NewReaderSize(r, logLineLimit+2) // allow CRLF
	for {
		line, err := buf.ReadSlice('\n')
		if errors.Is(err, bufio.ErrBufferFull) {
			write("[NeoBot Desktop] 单条输出过长，请查看日志文件。")
			for errors.Is(err, bufio.ErrBufferFull) {
				_, err = buf.ReadSlice('\n')
			}
		} else if len(line) != 0 {
			write(printableLine(string(line)))
		}
		if err != nil {
			return
		}
	}
}
