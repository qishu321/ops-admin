package service

import (
	"bytes"
	"io"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// TerminalCommandFilter checks input before forwarding an execution delimiter.
// Interactive editing, history and completion are forwarded normally. Only
// configured, enabled matches may reject a command; editing is not a deny rule.
// Remote Readline echo supplies the actual history/completion text.
type TerminalCommandFilter struct {
	mu            sync.Mutex
	screen        terminalCommandScreen
	observed      bool
	remotePending bool
	remoteReady   bool
	changed       chan struct{}

	line         []byte
	paste        []byte
	cursor       int
	escape       string
	previousCR   bool
	pending      string
	lastOutputAt time.Time
	check        func(string) error
}

func (s *Service) NewTerminalCommandFilter() *TerminalCommandFilter {
	return &TerminalCommandFilter{check: func(command string) error {
		rules, err := s.enabledCommandFilters()
		if err != nil {
			return err
		}
		return matchCommandFilters(rules, "terminal", command)
	}}
}
func (f *TerminalCommandFilter) reset() {
	f.line = nil
	f.paste = nil
	f.cursor = 0
	f.escape = ""
	f.pending = ""
	f.remotePending = false
	f.remoteReady = false
}
func (f *TerminalCommandFilter) Feed(input []byte) ([]byte, []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.syncRemoteLine()

	out := []byte{}
	notices := []string{}
	for _, c := range input {
		if f.paste != nil {
			if c == 3 {
				out = append(out, c)
				f.reset()
				continue
			}
			f.paste = append(f.paste, c)
			if len(f.paste) > 1048576 {
				out = append(out, 3)
				notices = append(notices, "粘贴内容过长，请通过脚本执行提交")
				f.reset()
				continue
			}
			if bytes.HasSuffix(f.paste, []byte("\x1b[201~")) {
				text := f.paste[6 : len(f.paste)-6]
				next := append(append(append([]byte{}, f.line[:f.cursor]...), text...), f.line[f.cursor:]...)
				if err := f.check(f.pending + string(next)); err != nil {
					out = append(out, 3)
					notices = append(notices, err.Error())
					f.reset()
				} else {
					out = append(out, f.paste...)
					f.line = next
					f.cursor += len(text)
					f.paste = nil
				}
			}
			continue
		}
		if f.escape != "" && (c == '\r' || c == '\n' || c == 4 || c == 3) {
			f.escape = ""
		}
		if f.previousCR && c == '\n' {
			f.previousCR = false
			out = append(out, c)
			continue
		}
		f.previousCR = false
		if f.escape != "" {
			f.escape += string(c)
			if len(f.escape) == 2 && (c == '[' || c == 'O') {
				continue
			}
			if c >= 0x40 && c <= 0x7e || len(f.escape) > 32 {
				if f.escape == "\x1b[200~" {
					f.paste = []byte(f.escape)
					f.escape = ""
					continue
				}
				out = append(out, []byte(f.escape)...)
				switch f.escape {
				case "\x1b[A", "\x1b[B", "\x1bOA", "\x1bOB":
					f.remotePending = true
					f.remoteReady = false
				case "\x1b[D", "\x1bOD":
					if f.cursor > 0 {
						_, n := utf8.DecodeLastRune(f.line[:f.cursor])
						f.cursor -= n
					}
				case "\x1b[C", "\x1bOC":
					if f.cursor < len(f.line) {
						_, n := utf8.DecodeRune(f.line[f.cursor:])
						f.cursor += n
					}
				case "\x1b[H", "\x1bOH", "\x1b[1~":
					f.cursor = 0
				case "\x1b[F", "\x1bOF", "\x1b[4~":
					f.cursor = len(f.line)
				case "\x1b[3~":
					if f.cursor < len(f.line) {
						_, n := utf8.DecodeRune(f.line[f.cursor:])
						f.line = append(f.line[:f.cursor], f.line[f.cursor+n:]...)
					}
				case "\x1b[200~", "\x1b[201~": // bracketed paste wrappers
				default:
				}
				f.escape = ""
			}
			continue
		}
		if c == '\r' || c == '\n' || c == 4 {
			command := f.pending + string(f.line)
			err := f.check(command)
			if err != nil {
				out = append(out, 3)
				notices = append(notices, err.Error())
				f.reset()
			} else {
				out = append(out, c)
				if shellInputContinues(command) {
					f.pending = command + "\n"
					f.line = nil
					f.cursor = 0
				} else {
					f.reset()
				}
			}
			f.previousCR = c == '\r'
			continue
		}
		if c != 27 {
			out = append(out, c)
		}
		switch c {
		case 9, 14, 16, 18:
			f.remotePending = true
			f.remoteReady = false
		case 27:
			f.escape = "\x1b"
		case 3:
			f.reset()
		case 1:
			f.cursor = 0
		case 5:
			f.cursor = len(f.line)
		case 21:
			f.line = append([]byte{}, f.line[f.cursor:]...)
			f.cursor = 0
		case 11:
			f.line = f.line[:f.cursor]
		case 8, 127:
			if f.cursor > 0 {
				_, n := utf8.DecodeLastRune(f.line[:f.cursor])
				f.line = append(f.line[:f.cursor-n], f.line[f.cursor:]...)
				f.cursor -= n
			}
		case 12, 17, 19: // redraw and flow control do not edit the command
		default:
			if c >= 32 {
				f.line = append(f.line, 0)
				copy(f.line[f.cursor+1:], f.line[f.cursor:])
				f.line[f.cursor] = c
				f.cursor++
			}
		}
		if len(f.line)+len(f.pending) > 65536 {
			f.line = nil
			f.pending = ""
			f.cursor = 0
		}
	}
	return out, notices
}
func shellInputContinues(command string) bool {
	var quote byte
	escaped := false
	for i := 0; i < len(command); i++ {
		c := command[i]
		if escaped {
			escaped = false
			continue
		}
		if c == '\\' && quote != '\'' {
			escaped = true
			continue
		}
		if quote != 0 {
			if c == quote {
				quote = 0
			}
		} else if c == '\'' || c == '"' || c == '`' {
			quote = c
		}
	}
	return quote != 0 || escaped || strings.HasSuffix(strings.TrimSpace(command), "|")
}

// ObserveOutput consumes the same SSH bytes sent to xterm. It never modifies the
// output stream or persists terminal content.
func (f *TerminalCommandFilter) ObserveOutput(data []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.observed = true
	f.lastOutputAt = time.Now()
	f.screen.observe(data)
	if f.remotePending {
		_, _, ok := f.screen.command()
		f.remoteReady = ok
	}
	if f.changed != nil {
		close(f.changed)
	}
	f.changed = make(chan struct{})
}
func (f *TerminalCommandFilter) syncRemoteLine() {
	if !f.remotePending || !f.remoteReady {
		return
	}
	if command, cursor, ok := f.screen.command(); ok {
		f.line = []byte(command)
		f.cursor = cursor
		f.remotePending = false
		f.remoteReady = false
	}
}
func (f *TerminalCommandFilter) waitRemoteLine() {
	deadline := time.NewTimer(500 * time.Millisecond)
	defer deadline.Stop()
	for {
		f.mu.Lock()
		if !f.remotePending || !f.observed {
			f.mu.Unlock()
			return
		}
		// A Readline redraw may be split over several SSH reads. Wait for a short
		// quiet period so a prompt/partial redraw is not mistaken for the final line.
		settle := 15*time.Millisecond - time.Since(f.lastOutputAt)
		if f.remoteReady && settle <= 0 {
			f.mu.Unlock()
			return
		}
		ready := f.remoteReady
		if f.changed == nil {
			f.changed = make(chan struct{})
		}
		changed := f.changed
		f.mu.Unlock()
		if !ready {
			select {
			case <-changed:
			case <-deadline.C:
				return
			}
			continue
		}
		timer := time.NewTimer(settle)
		select {
		case <-changed:
		case <-timer.C:
		case <-deadline.C:
			timer.Stop()
			return
		}
		timer.Stop()
	}
}

// Flush editing keys before waiting for their server-side echo. A WebSocket
// frame may contain both an arrow key and Enter; buffering them together would
// let Enter reach the shell before history had been checked.
func (f *TerminalCommandFilter) WriteInput(input []byte, writer io.Writer) ([]string, error) {
	notices := []string{}
	write := func(data []byte) error {
		if len(data) == 0 {
			return nil
		}
		f.waitRemoteLine()
		out, messages := f.Feed(data)
		notices = append(notices, messages...)
		if len(out) > 0 {
			_, err := writer.Write(out)
			return err
		}
		return nil
	}
	start := 0
	for i, c := range input {
		if c != '\r' && c != '\n' && c != 4 {
			continue
		}
		if err := write(input[start:i]); err != nil {
			return notices, err
		}
		f.waitRemoteLine()
		if err := write(input[i : i+1]); err != nil {
			return notices, err
		}
		start = i + 1
	}
	if err := write(input[start:]); err != nil {
		return notices, err
	}
	return notices, nil
}
