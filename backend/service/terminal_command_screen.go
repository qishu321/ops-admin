package service

import (
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Readline redraws the editable command when recalling history or completing a
// word. Observe the server's echo, not just the client's arrow/tab bytes.
// This is a bounded single logical line renderer, not a terminal emulator.
type terminalCommandScreen struct {
	line        []rune
	cursor      int
	escape      string
	unicode     []byte
	promptEnd   int
	alternate   bool
	modeVersion uint64
}

var terminalPromptEnd = regexp.MustCompile(`^[^\r\n]*?[$#>%] `)

func (s *terminalCommandScreen) observe(data []byte) {
	for _, b := range data {
		if s.escape != "" {
			s.escape += string(b)
			if strings.HasPrefix(s.escape, "\x1b]") {
				if b == 7 || strings.HasSuffix(s.escape, "\x1b\\") {
					s.escape = ""
				}
				if len(s.escape) > 4096 {
					s.escape = ""
				}
				continue
			}
			if len(s.escape) == 2 && (b == '[' || b == ']' || b == '(' || b == ')') {
				continue
			}
			if strings.HasPrefix(s.escape, "\x1b[") {
				if b >= 0x40 && b <= 0x7e {
					s.csi(s.escape[2:])
					s.escape = ""
				}
			} else {
				s.escape = ""
			}
			if len(s.escape) > 128 {
				s.escape = ""
			}
			continue
		}
		// Full-screen applications use their own input grammar. Keep parsing
		// escape sequences so leaving the alternate screen is detected.
		if s.alternate && b != 27 {
			continue
		}
		switch b {
		case 27:
			s.escape = "\x1b"
		case '\r':
			s.cursor = 0
		case '\n':
			s.line = nil
			s.cursor = 0
			s.promptEnd = 0
		case 8:
			if s.cursor > 0 {
				s.cursor--
			}
		case '\t':
			s.cursor = (s.cursor/8 + 1) * 8
		default:
			if b < 32 || b == 127 {
				continue
			}
			s.unicode = append(s.unicode, b)
			for utf8.FullRune(s.unicode) {
				r, n := utf8.DecodeRune(s.unicode)
				s.unicode = s.unicode[n:]
				s.put(r)
			}
		}
	}
	if s.promptEnd == 0 {
		if end := terminalPromptEnd.FindStringIndex(string(s.line)); end != nil {
			s.promptEnd = utf8.RuneCountInString(string(s.line)[:end[1]])
		}
	}
}
func (s *terminalCommandScreen) put(r rune) {
	if s.cursor < 0 {
		s.cursor = 0
	}
	if s.cursor > 65536 {
		s.cursor = 65536
	}
	for len(s.line) <= s.cursor {
		s.line = append(s.line, ' ')
	}
	s.line[s.cursor] = r
	s.cursor++
}
func (s *terminalCommandScreen) csi(sequence string) {
	final := sequence[len(sequence)-1]
	raw := sequence[:len(sequence)-1]
	if strings.HasPrefix(raw, "?") && (final == 'h' || final == 'l') {
		for _, mode := range strings.Split(raw[1:], ";") {
			if mode != "47" && mode != "1047" && mode != "1049" {
				continue
			}
			if enabled := final == 'h'; enabled != s.alternate {
				s.alternate = enabled
				s.modeVersion++
				s.line, s.unicode = nil, nil
				s.cursor, s.promptEnd = 0, 0
			}
		}
		return
	}
	if strings.ContainsAny(raw, "?<>=!") {
		return
	}
	args := strings.Split(raw, ";")
	arg := func(i, defaultValue int) int {
		if i >= len(args) {
			return defaultValue
		}
		n, err := strconv.Atoi(args[i])
		if err != nil {
			return defaultValue
		}
		return n
	}
	count := arg(0, 1)
	if count < 1 {
		count = 1
	}
	if count > 65536 {
		count = 65536
	}
	switch final {
	case 'C':
		s.cursor += count
	case 'D':
		s.cursor -= count
		if s.cursor < 0 {
			s.cursor = 0
		}
	case 'G', '`':
		s.cursor = count - 1
	case 'H', 'f':
		s.cursor = arg(1, 1) - 1
		if s.cursor < 0 {
			s.cursor = 0
		}
	case 'K':
		switch arg(0, 0) {
		case 0:
			if s.cursor < len(s.line) {
				s.line = s.line[:s.cursor]
			}
		case 1:
			for i := 0; i <= s.cursor && i < len(s.line); i++ {
				s.line[i] = ' '
			}
		case 2:
			s.line = nil
			s.promptEnd = 0
		}
	case 'P':
		if s.cursor < len(s.line) {
			end := s.cursor + count
			if end > len(s.line) {
				end = len(s.line)
			}
			s.line = append(s.line[:s.cursor], s.line[end:]...)
		}
	case 'J':
		if arg(0, 0) == 2 || arg(0, 0) == 3 {
			s.line = nil
			s.cursor = 0
			s.promptEnd = 0
		}
	}
}
func (s *terminalCommandScreen) command() (string, int, bool) {
	if s.promptEnd == 0 || s.promptEnd > len(s.line) {
		return "", 0, false
	}
	command := []rune(strings.TrimRight(string(s.line[s.promptEnd:]), " "))
	cursor := s.cursor - s.promptEnd
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(command) {
		cursor = len(command)
	}
	return string(command), len(string(command[:cursor])), true
}
