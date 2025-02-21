package websocket

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"clash/internal/domain/customer_service"

	"github.com/gorilla/websocket"
)

// MessageGateway WebSocket消息网关
type MessageGateway struct {
	service  *customer_service.CustomerService
	upgrader websocket.Upgrader
	mu       sync.RWMutex
}

// NewMessageGateway 创建新的消息网关实例
func NewMessageGateway() *MessageGateway {
	return &MessageGateway{
		service: customer_service.NewCustomerService(),
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				return true // 在生产环境中应该根据实际需求设置跨域策略
			},
		},
	}
}

// WSMessage WebSocket消息结构
type WSMessage struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// HandleUserConnection 处理用户WebSocket连接
func (g *MessageGateway) HandleUserConnection(w http.ResponseWriter, r *http.Request) {
	// 从请求中获取用户信息（实际应用中应该从认证token中获取）
	userID := r.URL.Query().Get("user_id")
	name := r.URL.Query().Get("name")
	if userID == "" || name == "" {
		http.Error(w, "Missing user information", http.StatusBadRequest)
		return
	}

	// 升级HTTP连接为WebSocket连接
	conn, err := g.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade connection: %v", err)
		return
	}

	// 注册用户连接
	user := g.service.ConnectUser(userID, name, conn)
	defer g.service.DisconnectUser(userID)

	// 处理用户消息
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			log.Printf("Error reading message from user %s: %v", userID, err)
			break
		}

		var msg WSMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			log.Printf("Error parsing message from user %s: %v", userID, err)
			continue
		}

		// 处理不同类型的消息
		switch msg.Type {
		case "message":
			var payload struct {
				Content string `json:"content"`
			}
			if err := json.Unmarshal(msg.Payload, &payload); err != nil {
				log.Printf("Error parsing message payload: %v", err)
				continue
			}

			// 发送消息
			if user.SessionID != "" {
				message, err := g.service.SendMessage(user.SessionID, userID, payload.Content, customer_service.MessageTypeText)
				if err != nil {
					log.Printf("Error sending message: %v", err)
					continue
				}

				// 转发消息给客服
				g.forwardMessageToStaff(message)
			}
		}
	}
}

// HandleStaffConnection 处理客服WebSocket连接
func (g *MessageGateway) HandleStaffConnection(w http.ResponseWriter, r *http.Request) {
	// 从请求中获取客服信息（实际应用中应该从认证token中获取）
	staffID := r.URL.Query().Get("staff_id")
	name := r.URL.Query().Get("name")
	groupID := r.URL.Query().Get("group_id")
	if staffID == "" || name == "" || groupID == "" {
		http.Error(w, "Missing staff information", http.StatusBadRequest)
		return
	}

	// 升级HTTP连接为WebSocket连接
	conn, err := g.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade connection: %v", err)
		return
	}

	// 注册客服连接
	_, err = g.service.ConnectStaff(staffID, name, groupID, conn)
	if err != nil {
		log.Printf("Failed to connect staff: %v", err)
		conn.Close()
		return
	}
	defer g.service.DisconnectStaff(staffID)

	// 处理客服消息
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			log.Printf("Error reading message from staff %s: %v", staffID, err)
			break
		}

		var msg WSMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			log.Printf("Error parsing message from staff %s: %v", staffID, err)
			continue
		}

		// 处理不同类型的消息
		switch msg.Type {
		case "connect_user":
			var payload struct {
				UserID string `json:"user_id"`
			}
			if err := json.Unmarshal(msg.Payload, &payload); err != nil {
				log.Printf("Error parsing connect_user payload: %v", err)
				continue
			}

			// 创建会话
			session, err := g.service.CreateSession(payload.UserID, staffID)
			if err != nil {
				log.Printf("Error creating session: %v", err)
				continue
			}

			// 通知客服和用户会话已创建
			g.notifySessionCreated(session)

		case "transfer_session":
			var payload struct {
				SessionID  string `json:"session_id"`
				NewStaffID string `json:"new_staff_id"`
			}
			if err := json.Unmarshal(msg.Payload, &payload); err != nil {
				log.Printf("Error parsing transfer_session payload: %v", err)
				continue
			}

			// 转移会话
			if err := g.service.TransferSession(payload.SessionID, payload.NewStaffID); err != nil {
				log.Printf("Error transferring session: %v", err)
				continue
			}

			// 通知相关方会话已转移
			g.notifySessionTransferred(payload.SessionID, staffID, payload.NewStaffID)

		case "message":
			var payload struct {
				SessionID string `json:"session_id"`
				Content   string `json:"content"`
			}
			if err := json.Unmarshal(msg.Payload, &payload); err != nil {
				log.Printf("Error parsing message payload: %v", err)
				continue
			}

			// 发送消息
			message, err := g.service.SendMessage(payload.SessionID, staffID, payload.Content, customer_service.MessageTypeText)
			if err != nil {
				log.Printf("Error sending message: %v", err)
				continue
			}

			// 转发消息给用户
			g.forwardMessageToUser(message)
		}
	}
}

// forwardMessageToStaff 转发消息给客服
func (g *MessageGateway) forwardMessageToStaff(message *customer_service.Message) {
	response := map[string]interface{}{
		"type":    "message",
		"payload": message,
	}
	data, _ := json.Marshal(response)

	staff := g.service.GetStaff(message.ToID)
	if staff != nil {
		staff.Conn.WriteMessage(websocket.TextMessage, data)
	}
}

// forwardMessageToUser 转发消息给用户
func (g *MessageGateway) forwardMessageToUser(message *customer_service.Message) {
	response := map[string]interface{}{
		"type":    "message",
		"payload": message,
	}
	data, _ := json.Marshal(response)

	user := g.service.GetUser(message.ToID)
	if user != nil {
		user.Conn.WriteMessage(websocket.TextMessage, data)
	}
}

// notifySessionCreated 通知会话创建
func (g *MessageGateway) notifySessionCreated(session *customer_service.Session) {
	response := map[string]interface{}{
		"type":    "session_created",
		"payload": session,
	}
	data, _ := json.Marshal(response)

	// 通知用户
	user := g.service.GetUser(session.UserID)
	if user != nil {
		user.Conn.WriteMessage(websocket.TextMessage, data)
	}

	// 通知客服
	staff := g.service.GetStaff(session.StaffID)
	if staff != nil {
		staff.Conn.WriteMessage(websocket.TextMessage, data)
	}
}

// notifySessionTransferred 通知会话转移
func (g *MessageGateway) notifySessionTransferred(sessionID, oldStaffID, newStaffID string) {
	response := map[string]interface{}{
		"type": "session_transferred",
		"payload": map[string]string{
			"session_id":   sessionID,
			"old_staff_id": oldStaffID,
			"new_staff_id": newStaffID,
		},
	}
	data, _ := json.Marshal(response)

	// 获取会话信息
	session := g.service.GetSession(sessionID)
	if session == nil {
		return
	}

	// 通知用户
	user := g.service.GetUser(session.UserID)
	if user != nil {
		user.Conn.WriteMessage(websocket.TextMessage, data)
	}

	// 通知原客服
	oldStaff := g.service.GetStaff(oldStaffID)
	if oldStaff != nil {
		oldStaff.Conn.WriteMessage(websocket.TextMessage, data)
	}

	// 通知新客服
	newStaff := g.service.GetStaff(newStaffID)
	if newStaff != nil {
		newStaff.Conn.WriteMessage(websocket.TextMessage, data)
	}
}
