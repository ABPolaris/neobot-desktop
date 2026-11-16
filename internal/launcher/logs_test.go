package launcher

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
)

func TestProcessOutputTraceRetainsStartupAndShutdown(t *testing.T) {
	m := &Manager{bot: &botProcess{}}
	m.logs.reset("2.0.1", "dev")
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		m.watchOutput(m.bot, reader)
	}()
	var output strings.Builder
	output.WriteString("\x1b[32m准备启动\x1b[0m\r\nDashboard URL: http://127.0.0.1:19981/\n")
	for i := 0; i < logBeginning+logRecent; i++ {
		fmt.Fprintf(&output, "消息 %d\n", i)
	}
	output.WriteString(strings.Repeat("X", 4*logLineLimit) + "\n")
	output.WriteString("连接日志 \"APIKey\": \"key-should-not-leak\", Authorization: Bearer token-should-not-leak\nNeoBot 已停止")
	_, err = writer.WriteString(output.String())
	closeErr := writer.Close()
	<-done
	if err != nil || closeErr != nil {
		t.Fatalf("write: %v; close: %v", err, closeErr)
	}
	snapshot := m.Logs(0)
	if m.bot.reportedURL != "http://127.0.0.1:19981/" || snapshot.Version != "2.0.1" || snapshot.Branch != "dev" ||
		snapshot.Omitted != 5 || snapshot.GapAfter != logBeginning || len(snapshot.Lines) != logBeginning+logRecent ||
		!slices.Equal(snapshot.Lines[:2], []string{"准备启动", "Dashboard URL: http://127.0.0.1:19981/"}) {
		t.Fatal("startup output or retained history is incomplete")
	}
	last := snapshot.Lines[len(snapshot.Lines)-3:]
	if !strings.Contains(last[0], "单条输出过长") || strings.Contains(last[1], "should-not-leak") ||
		!strings.Contains(last[1], "[已隐藏]") || last[2] != "NeoBot 已停止" {
		t.Fatal("long lines, redaction or final output was not handled")
	}
	if unchanged := m.Logs(snapshot.Revision); unchanged.Revision != snapshot.Revision || unchanged.Lines != nil {
		t.Fatal("an unchanged revision should not transfer the buffer")
	}
	m.logs.reset("2.1.0a9", "feat/x")
	if next := m.Logs(snapshot.Revision); next.Version != "2.1.0a9" || next.Branch != "feat/x" || len(next.Lines) != 0 {
		t.Fatal("previous process output leaked into the new run")
	}
}
