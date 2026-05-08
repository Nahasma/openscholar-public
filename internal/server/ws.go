package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/coder/websocket"
	"github.com/Nahasma/openscholar-public/internal/permission"
	"github.com/Nahasma/openscholar-public/internal/pubsub"
)

// clientMessage is sent from the browser to the server.
type clientMessage struct {
	Type      string `json:"type"`      // "message", "cancel"
	SessionID string `json:"sessionId"`
	Text      string `json:"text"`
}

// serverMessage is sent from the server to the browser.
type serverMessage struct {
	Type      string `json:"type"`
	SessionID string `json:"sessionId,omitempty"`
	Text      string `json:"text,omitempty"`
	ToolName  string `json:"toolName,omitempty"`
	ToolID    string `json:"toolId,omitempty"`
	Content   string `json:"content,omitempty"`
	Error     string `json:"error,omitempty"`
	Done      bool   `json:"done,omitempty"`
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // Allow any origin in dev
	})
	if err != nil {
		log.Printf("WebSocket accept error: %v", err)
		return
	}
	defer conn.CloseNow()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	var wsMu sync.Mutex
	sendMsg := func(msg serverMessage) {
		data, _ := json.Marshal(msg)
		wsMu.Lock()
		defer wsMu.Unlock()
		conn.Write(ctx, websocket.MessageText, data)
	}

	// Subscribe to message events
	msgCh := s.app.Messages.Subscribe(ctx)
	agentCh := s.app.CoderAgent.Subscribe(ctx)

	// Forward broker events to WebSocket
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-msgCh:
				if !ok {
					return
				}
				if event.Type == pubsub.UpdatedEvent {
					msg := event.Payload
					// Send content deltas
					text := msg.Content()
					if text.String() != "" {
						sendMsg(serverMessage{
							Type:      "content_delta",
							SessionID: msg.SessionID,
							Text:      text.String(),
						})
					}
					// Send tool calls
					for _, tc := range msg.ToolCalls() {
						sendMsg(serverMessage{
							Type:      "tool_use_start",
							SessionID: msg.SessionID,
							ToolName:  tc.Name,
							ToolID:    tc.ID,
						})
					}
				}
			case event, ok := <-agentCh:
				if !ok {
					return
				}
				if event.Payload.Done {
					sendMsg(serverMessage{
						Type: "complete",
						Done: true,
					})
				}
				if event.Payload.Error != nil {
					sendMsg(serverMessage{
						Type:  "error",
						Error: event.Payload.Error.Error(),
					})
				}
			}
		}
	}()

	// Read client messages
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return
		}

		var msg clientMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			sendMsg(serverMessage{Type: "error", Error: "invalid message format"})
			continue
		}

		switch msg.Type {
		case "message":
			if msg.SessionID == "" || msg.Text == "" {
				sendMsg(serverMessage{Type: "error", Error: "sessionId and text are required"})
				continue
			}

			// Set default permission mode for web sessions
			s.app.Permissions.SetSessionMode(msg.SessionID, permission.ModeDefault)

			_, err := s.app.CoderAgent.Run(ctx, msg.SessionID, msg.Text)
			if err != nil {
				sendMsg(serverMessage{Type: "error", Error: fmt.Sprintf("agent error: %v", err)})
			}

		case "cancel":
			if msg.SessionID != "" {
				s.app.CoderAgent.Cancel(msg.SessionID)
			}
		}
	}
}

