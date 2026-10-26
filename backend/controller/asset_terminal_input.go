package controller

import (
	"encoding/json"
	"errors"

	"github.com/gorilla/websocket"
)

const assetTerminalProtocolV2 = "ops-admin-terminal-v2"

// Explicitly negotiated framing keeps pasted JSON/UTF-8 input separate from
// window controls. Older clients retain the original raw input protocol.
func assetTerminalInput(kind int, message []byte, protocol string, resize func(rows, cols int) error) ([]byte, error) {
	if protocol != assetTerminalProtocolV2 || kind == websocket.BinaryMessage {
		return message, nil
	}
	if kind != websocket.TextMessage || len(message) > 512 {
		return nil, errors.New("invalid terminal control frame")
	}
	var control struct {
		Type string `json:"type"`
		Rows int    `json:"rows"`
		Cols int    `json:"cols"`
	}
	if err := json.Unmarshal(message, &control); err != nil || control.Type != "resize" || control.Rows < 1 || control.Rows > 1000 || control.Cols < 2 || control.Cols > 1000 {
		return nil, errors.New("invalid terminal window size")
	}
	return nil, resize(control.Rows, control.Cols)
}
