package service

import (
	"strings"
	"testing"
	"time"
)

func TestTerminalVimInputDoesNotWaitForShellHistoryEcho(t *testing.T) {
	f := historyFilter()
	historyPromptReady(f)
	// Vim's alternate-screen setup may be split over SSH packets.
	for _, b := range []byte("\x1b[?1049h\x1b[2J\x1b[HapiVersion: v1") {
		f.ObserveOutput([]byte{b})
	}
	var wire strings.Builder
	start := time.Now()
	for _, input := range []string{"\x1b[A", "\x1b[B", "i", "rm is file content", "\t", "\r", "\x1b", ":wq\r"} {
		if notices, err := f.WriteInput([]byte(input), &wire); err != nil || len(notices) != 0 {
			t.Fatalf("editor keystrokes were treated as shell commands: %v %v", notices, err)
		}
		if elapsed := time.Since(start); elapsed >= 200*time.Millisecond {
			t.Fatalf("Vim input still waits for a nonexistent shell prompt: %v", elapsed)
		}
	}
	if wire.String() != "\x1b[A\x1b[Birm is file content\t\r\x1b:wq\r" {
		t.Fatalf("editor input changed: %q", wire.String())
	}
	t.Logf("editor input sequence forwarded in %v", time.Since(start))
	// Leaving Vim must restore normal command filtering, including history.
	f.ObserveOutput([]byte("\x1b[?1049l\r\n" + historyPrompt))
	f.Feed([]byte("\x1b[A"))
	f.ObserveOutput([]byte("rm -f protected.txt"))
	out, notices := f.Feed([]byte("\r"))
	if string(out) != "\x03" || len(notices) != 1 {
		t.Fatalf("history filtering was not restored after Vim: %q %v", out, notices)
	}
}

func TestTerminalAlternateScreenModesRestoreShellFiltering(t *testing.T) {
	for _, mode := range []string{"47", "1047", "1049", "1;1049"} {
		t.Run(mode, func(t *testing.T) {
			f := historyFilter()
			historyPromptReady(f)
			f.Feed([]byte("\x1b[A")) // Pending history must not leak into the editor.
			f.ObserveOutput([]byte("\x1b[?" + mode + "h"))
			text := []byte("rm is just file content\r\n\x1b[A\t")
			out, notices := f.Feed(text)
			if string(out) != string(text) || len(notices) != 0 {
				t.Fatalf("editor input filtered: %q %v", out, notices)
			}
			f.ObserveOutput([]byte("\x1b[?" + mode + "l\r\n" + historyPrompt))
			out, notices = f.Feed([]byte("rm file\r"))
			if len(notices) != 1 || strings.Contains(string(out), "\r") {
				t.Fatalf("shell filter did not resume: %q %v", out, notices)
			}
		})
	}
}
