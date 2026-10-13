package controller

import (
	"io"
	"sync"

	"github.com/gorilla/websocket"
)

func writeAssetTerminalOutput(conn *websocket.Conn, writeMu *sync.Mutex, reader io.Reader) error {
	buf := make([]byte, 4096)
	for {
		n, err := reader.Read(buf)
		if n > 0 {
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
