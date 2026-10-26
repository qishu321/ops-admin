package controller

import (
	"io"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const assetTerminalIdleCloseCode = 4001

type assetTerminalTiming struct {
	idle, heartbeat, peerTimeout, sshTimeout time.Duration
}

var defaultAssetTerminalTiming = assetTerminalTiming{
	idle: 60 * time.Minute, heartbeat: 25 * time.Second,
	peerTimeout: 90 * time.Second, sshTimeout: 10 * time.Second,
}

// Only real terminal bytes are activity. WebSocket pong and SSH keepalive
// traffic must never turn an abandoned terminal into an unlimited session.
type assetTerminalActivity struct {
	mu      sync.Mutex
	last    time.Time
	wake    chan struct{}
	expired bool
}

func newAssetTerminalActivity() *assetTerminalActivity {
	return &assetTerminalActivity{last: time.Now(), wake: make(chan struct{}, 1)}
}

func (a *assetTerminalActivity) observe(data []byte) {
	if len(data) == 0 {
		return
	}
	a.mu.Lock()
	if a.expired {
		a.mu.Unlock()
		return
	}
	a.last = time.Now()
	a.mu.Unlock()
	select {
	case a.wake <- struct{}{}:
	default:
	}
}

func (a *assetTerminalActivity) remaining(now time.Time, idle time.Duration) time.Duration {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.last.Add(idle).Sub(now)
}

func (a *assetTerminalActivity) expireIfIdle(now time.Time, idle time.Duration) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if now.Before(a.last.Add(idle)) {
		return false
	}
	a.expired = true
	return true
}

// Call before starting ReadMessage: Gorilla's read deadline/pong handler belong
// to its single reader. WriteControl is safe alongside terminal output writes.
func startAssetTerminalMaintenance(conn *websocket.Conn, activity *assetTerminalActivity, timing assetTerminalTiming, keepSSHAlive func() error, closeSSH func()) func() {
	done := make(chan struct{})
	var stopOnce, closeOnce sync.Once
	stop := func() { stopOnce.Do(func() { close(done) }) }
	shutdown := func(code int, reason string) {
		closeOnce.Do(func() {
			stop()
			_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(code, reason), time.Now().Add(time.Second))
			_ = conn.Close()
			closeSSH()
		})
	}
	_ = conn.SetReadDeadline(time.Now().Add(timing.peerTimeout))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(timing.peerTimeout))
	})
	go func() {
		timer := time.NewTimer(timing.idle)
		defer timer.Stop()
		for {
			select {
			case <-done:
				return
			case <-activity.wake:
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(max(time.Nanosecond, activity.remaining(time.Now(), timing.idle)))
			case <-timer.C:
				if !activity.expireIfIdle(time.Now(), timing.idle) {
					timer.Reset(max(time.Nanosecond, activity.remaining(time.Now(), timing.idle)))
					continue
				}
				shutdown(assetTerminalIdleCloseCode, "terminal idle for 60 minutes")
				return
			}
		}
	}()
	go func() {
		ticker := time.NewTicker(timing.heartbeat)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(timing.sshTimeout)); err != nil {
					shutdown(websocket.CloseGoingAway, "terminal transport unavailable")
					return
				}
				// At most one outstanding SSH request; closing the owned SSH
				// client on failure also releases a blocked SendRequest.
				result := make(chan error, 1)
				go func() { result <- keepSSHAlive() }()
				timer := time.NewTimer(timing.sshTimeout)
				select {
				case <-done:
					timer.Stop()
					return
				case err := <-result:
					timer.Stop()
					if err != nil {
						shutdown(websocket.CloseGoingAway, "SSH transport unavailable")
						return
					}
				case <-timer.C:
					shutdown(websocket.CloseGoingAway, "SSH keepalive timeout")
					return
				}
			}
		}
	}()
	return stop
}

func writeAssetTerminalOutput(conn *websocket.Conn, writeMu *sync.Mutex, reader io.Reader, observers ...func([]byte)) error {
	buf := make([]byte, 4096)
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			for _, observe := range observers {
				observe(buf[:n])
			}
			writeMu.Lock()
			// SSH is a byte stream: reads can split UTF-8 characters or contain
			// arbitrary bytes. Text messages would make browsers close the socket.
			writeErr := conn.WriteMessage(websocket.BinaryMessage, buf[:n])
			writeMu.Unlock()
			if writeErr != nil {
				return writeErr
			}
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}
