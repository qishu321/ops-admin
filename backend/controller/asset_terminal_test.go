package controller

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/gorilla/websocket"
)

func TestAssetTerminalOutputPreservesSplitUTF8(t *testing.T) {
	// The first read ends inside a Chinese character; later reads also split emoji.
	output := []byte(strings.Repeat("x", 4095) + strings.Repeat("当前集群启动中 🚀\r\n", 3000) + "root@host:~# ")
	if !utf8.Valid(output) {
		t.Fatal("invalid fixture")
	}
	errCh := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			errCh <- err
			return
		}
		defer conn.Close()
		var mu sync.Mutex
		errCh <- writeAssetTerminalOutput(conn, &mu, bytes.NewReader(output))
	}))
	defer server.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var got []byte
	for len(got) < len(output) {
		kind, payload, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("terminal disconnected after %d bytes: %v", len(got), err)
		}
		// Browsers reject a text message with incomplete UTF-8 (Gorilla does not).
		if kind == websocket.TextMessage && !utf8.Valid(payload) {
			t.Fatal("browser would disconnect: text message contains a split UTF-8 character")
		}
		if kind != websocket.BinaryMessage {
			t.Fatalf("terminal byte stream must use binary messages, got %d", kind)
		}
		got = append(got, payload...)
	}
	if !bytes.Equal(got, output) {
		t.Fatal("terminal output lost or changed bytes")
	}
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
}

func TestAssetTerminalIdleBoundaryAndRealActivityReset(t *testing.T) {
	if defaultAssetTerminalTiming.idle != 60*time.Minute {
		t.Fatal("idle limit must be 60 minutes")
	}
	activity := newAssetTerminalActivity()
	start := time.Now()
	activity.last = start
	if got := activity.remaining(start.Add(59*time.Minute+59*time.Second), defaultAssetTerminalTiming.idle); got != time.Second {
		t.Fatalf("closed before 60 minutes: %v", got)
	}
	if got := activity.remaining(start.Add(60*time.Minute), defaultAssetTerminalTiming.idle); got != 0 {
		t.Fatalf("idle boundary: %v", got)
	}
	activity.observe(nil)
	if !activity.last.Equal(start) {
		t.Fatal("empty message reset idle timer")
	}
	for _, data := range [][]byte{[]byte("input"), []byte("server output")} {
		activity.last = time.Now().Add(-59 * time.Minute)
		activity.observe(data)
		remaining := activity.remaining(time.Now(), defaultAssetTerminalTiming.idle)
		if remaining < 60*time.Minute-time.Second {
			t.Fatalf("real input/output did not reset idle clock: %v", remaining)
		}
	}
	if activity.expireIfIdle(activity.last.Add(59*time.Minute), defaultAssetTerminalTiming.idle) {
		t.Fatal("expired active session early")
	}
	last := activity.last
	if !activity.expireIfIdle(last.Add(60*time.Minute), defaultAssetTerminalTiming.idle) {
		t.Fatal("did not expire at idle boundary")
	}
	activity.observe([]byte("too late"))
	if !activity.last.Equal(last) {
		t.Fatal("an already expired session was revived")
	}
}

func terminalMaintenanceFixture(t *testing.T, timing assetTerminalTiming, keepAlive func() error, closeSSH func()) (*websocket.Conn, <-chan struct{}) {
	t.Helper()
	done := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(done)
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close()
		defer closeSSH()
		activity := newAssetTerminalActivity()
		stop := startAssetTerminalMaintenance(conn, activity, timing, keepAlive, closeSSH)
		defer stop()
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			activity.observe(data)
		}
	}))
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		server.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close(); server.Close() })
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	return conn, done
}

func TestAssetTerminalHeartbeatsDoNotExtendIdleSession(t *testing.T) {
	var keepAlives atomic.Int32
	timing := assetTerminalTiming{idle: 120 * time.Millisecond, heartbeat: 10 * time.Millisecond, peerTimeout: time.Second, sshTimeout: 100 * time.Millisecond}
	conn, done := terminalMaintenanceFixture(t, timing, func() error { keepAlives.Add(1); return nil }, func() {})
	// ReadMessage automatically responds to Ping. There are no application
	// messages: pings/pongs must not be forwarded to the shell or xterm.
	_, data, err := conn.ReadMessage()
	var closed *websocket.CloseError
	if !errors.As(err, &closed) || closed.Code != assetTerminalIdleCloseCode {
		t.Fatalf("expected idle close, got %s %v", data, err)
	}
	if keepAlives.Load() == 0 {
		t.Fatal("SSH heartbeat never ran")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("idle handler leaked")
	}
}

func TestAssetTerminalBlockedSSHHeartbeatClosesOwnedTransport(t *testing.T) {
	release := make(chan struct{})
	finished := make(chan struct{})
	var once sync.Once
	closeSSH := func() { once.Do(func() { close(release) }) }
	timing := assetTerminalTiming{idle: time.Second, heartbeat: 10 * time.Millisecond, peerTimeout: time.Second, sshTimeout: 30 * time.Millisecond}
	conn, done := terminalMaintenanceFixture(t, timing, func() error { <-release; close(finished); return nil }, closeSSH)
	_, _, err := conn.ReadMessage()
	var closed *websocket.CloseError
	if !errors.As(err, &closed) || closed.Code == assetTerminalIdleCloseCode {
		t.Fatalf("transport failure misreported as idle expiry: %v", err)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("blocked heartbeat leaked handler")
	}
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("blocked SSH request was not released")
	}
}

func TestAssetTerminalMissingPongsReleasesSession(t *testing.T) {
	timing := assetTerminalTiming{idle: time.Second, heartbeat: 10 * time.Millisecond, peerTimeout: 60 * time.Millisecond, sshTimeout: 30 * time.Millisecond}
	var closed atomic.Bool
	_, done := terminalMaintenanceFixture(t, timing, func() error { return nil }, func() { closed.Store(true) })
	// Deliberately do not read on the client, so no Pong is generated.
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("unresponsive browser session retained")
	}
	if !closed.Load() {
		t.Fatal("SSH session was not cleaned up")
	}
}

func TestAssetTerminalOutputPreservesBytesReturnedWithEOF(t *testing.T) {
	errCh := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			errCh <- err
			return
		}
		defer conn.Close()
		var mu sync.Mutex
		errCh <- writeAssetTerminalOutput(conn, &mu, &terminalEOFReader{})
	}))
	defer server.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, payload, err := conn.ReadMessage()
	if err != nil || !bytes.Equal(payload, []byte{0xff, 0x1b, '[', 'm'}) {
		t.Fatalf("final raw bytes = %v, err = %v", payload, err)
	}
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
}

type terminalEOFReader struct{}

func (*terminalEOFReader) Read(p []byte) (int, error) {
	return copy(p, []byte{0xff, 0x1b, '[', 'm'}), io.EOF
}
