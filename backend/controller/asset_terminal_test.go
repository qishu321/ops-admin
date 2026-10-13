package controller

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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
