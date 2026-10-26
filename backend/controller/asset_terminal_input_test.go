package controller

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/crypto/ssh"
)

func TestAssetTerminalInputFraming(t *testing.T) {
	literal := []byte(`{"type":"resize","rows":48,"cols":123}`)
	for _, test := range []struct {
		name, protocol  string
		kind            int
		data            []byte
		resize, invalid bool
	}{
		{"legacy pasted JSON", "", websocket.TextMessage, literal, false, false},
		{"modern pasted JSON", assetTerminalProtocolV2, websocket.BinaryMessage, literal, false, false},
		{"window control", assetTerminalProtocolV2, websocket.TextMessage, literal, true, false},
		{"zero rows", assetTerminalProtocolV2, websocket.TextMessage, []byte(`{"type":"resize","rows":0,"cols":120}`), false, true},
		{"oversized columns", assetTerminalProtocolV2, websocket.TextMessage, []byte(`{"type":"resize","rows":48,"cols":1001}`), false, true},
		{"unknown control", assetTerminalProtocolV2, websocket.TextMessage, []byte(`{"type":"input","data":"echo hi"}`), false, true},
		{"malformed control", assetTerminalProtocolV2, websocket.TextMessage, []byte(`hostname`), false, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			input, err := assetTerminalInput(test.kind, test.data, test.protocol, func(rows, cols int) error {
				calls++
				if rows != 48 || cols != 123 {
					t.Errorf("swapped/invalid dimensions: %dx%d", rows, cols)
				}
				return nil
			})
			if (err != nil) != test.invalid || (calls == 1) != test.resize {
				t.Fatalf("input=%q calls=%d err=%v", input, calls, err)
			}
			if test.resize && len(input) != 0 {
				t.Fatal("resize was forwarded as shell input")
			}
			if !test.invalid && !test.resize && !bytes.Equal(input, test.data) {
				t.Fatal("input bytes changed")
			}
		})
	}
	want := errors.New("SSH closed")
	if _, err := assetTerminalInput(websocket.TextMessage, literal, assetTerminalProtocolV2, func(int, int) error { return want }); !errors.Is(err, want) {
		t.Fatal("SSH resize error swallowed")
	}
}

func TestAssetTerminalWindowControlReachesRealSSHTransport(t *testing.T) {
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatal(err)
	}
	config := &ssh.ServerConfig{NoClientAuth: true}
	config.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	sizes := make(chan [2]uint32, 4)
	input := make(chan []byte, 1)
	literal := []byte("echo '{\"type\":\"resize\"}'\r")
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		server, channels, requests, err := ssh.NewServerConn(conn, config)
		if err != nil {
			return
		}
		defer server.Close()
		go ssh.DiscardRequests(requests)
		for incoming := range channels {
			channel, requests, err := incoming.Accept()
			if err != nil {
				return
			}
			defer channel.Close()
			go func() {
				data := make([]byte, len(literal))
				if _, err := io.ReadFull(channel, data); err == nil {
					input <- data
				}
			}()
			for request := range requests {
				if request.Type == "window-change" {
					var size struct{ Cols, Rows, Width, Height uint32 }
					if ssh.Unmarshal(request.Payload, &size) == nil {
						sizes <- [2]uint32{size.Rows, size.Cols}
					}
				}
				if request.WantReply {
					_ = request.Reply(true, nil)
				}
			}
		}
	}()
	client, err := ssh.Dial("tcp", listener.Addr().String(), &ssh.ClientConfig{User: "test", HostKeyCallback: ssh.InsecureIgnoreHostKey(), Timeout: 3 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	session, err := client.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	stdin, err := session.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := session.RequestPty("xterm-256color", 34, 150, nil); err != nil {
		t.Fatal(err)
	}
	if err := session.Shell(); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			kind, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			data, err = assetTerminalInput(kind, data, assetTerminalProtocolV2, session.WindowChange)
			if err != nil {
				return
			}
			if len(data) > 0 {
				if _, err := stdin.Write(data); err != nil {
					return
				}
			}
		}
	}))
	defer server.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	for _, size := range [][2]uint32{{48, 123}, {72, 160}} {
		if err := conn.WriteJSON(map[string]any{"type": "resize", "rows": size[0], "cols": size[1]}); err != nil {
			t.Fatal(err)
		}
		select {
		case actual := <-sizes:
			if actual != size {
				t.Fatalf("SSH window: %v, want %v", actual, size)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("SSH window-change not delivered")
		}
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, literal); err != nil {
		t.Fatal(err)
	}
	select {
	case actual := <-input:
		if !bytes.Equal(actual, literal) {
			t.Fatalf("control frame polluted stdin: %q", actual)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("input not delivered to SSH")
	}
}
