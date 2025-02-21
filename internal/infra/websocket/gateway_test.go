package websocket

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
)

func TestMessageGateway(t *testing.T) {
	// 创建消息网关实例
	gateway := NewMessageGateway()

	// 创建测试服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/user") {
			gateway.HandleUserConnection(w, r)
		} else if strings.Contains(r.URL.Path, "/staff") {
			gateway.HandleStaffConnection(w, r)
		}
	}))
	defer server.Close()

	// 创建客服组
	gateway.service.CreateGroup("group1", "测试客服组")

	// 连接客服WebSocket
	staffURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/staff?staff_id=staff1&name=客服1&group_id=group1"
	staffConn, _, err := websocket.DefaultDialer.Dial(staffURL, nil)
	assert.NoError(t, err)
	defer staffConn.Close()

	// 连接用户WebSocket
	userURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/user?user_id=user1&name=用户1"
	userConn, _, err := websocket.DefaultDialer.Dial(userURL, nil)
	assert.NoError(t, err)
	defer userConn.Close()

	// 客服发起连接用户请求
	connectUserMsg := WSMessage{
		Type: "connect_user",
		Payload: json.RawMessage(`{"user_id":"user1"}`),
	}
	data, _ := json.Marshal(connectUserMsg)
	err = staffConn.WriteMessage(websocket.TextMessage, data)
	assert.NoError(t, err)

	// 等待会话创建通知
	_, message, err := staffConn.ReadMessage()
	assert.NoError(t, err)
	var sessionCreatedMsg map[string]interface{}
	err = json.Unmarshal(message, &sessionCreatedMsg)
	assert.NoError(t, err)
	assert.Equal(t, "session_created", sessionCreatedMsg["type"])

	// 等待用户也收到会话创建通知
	_, message, err = userConn.ReadMessage()
	assert.NoError(t, err)
	var userSessionMsg map[string]interface{}
	err = json.Unmarshal(message, &userSessionMsg)
	assert.NoError(t, err)
	assert.Equal(t, "session_created", userSessionMsg["type"])

	// 用户发送消息
	userMsg := WSMessage{
		Type: "message",
		Payload: json.RawMessage(`{"content":"你好，客服"}`),
	}
	data, _ = json.Marshal(userMsg)
	err = userConn.WriteMessage(websocket.TextMessage, data)
	assert.NoError(t, err)

	// 客服接收消息
	_, message, err = staffConn.ReadMessage()
	assert.NoError(t, err)
	var receivedMsg map[string]interface{}
	err = json.Unmarshal(message, &receivedMsg)
	assert.NoError(t, err)
	assert.Equal(t, "message", receivedMsg["type"])

	// 客服回复消息
	staffMsg := WSMessage{
		Type: "message",
		Payload: json.RawMessage(`{"session_id":"user1_staff1_" + time.Now().Format("20060102150405"), "content":"你好，我是客服1"}`),
	}
	data, _ = json.Marshal(staffMsg)
	err = staffConn.WriteMessage(websocket.TextMessage, data)
	assert.NoError(t, err)

	// 用户接收消息
	_, message, err = userConn.ReadMessage()
	assert.NoError(t, err)
	var receivedStaffMsg map[string]interface{}
	err = json.Unmarshal(message, &receivedStaffMsg)
	assert.NoError(t, err)
	assert.Equal(t, "message", receivedStaffMsg["type"])

	// 等待一段时间确保消息都已处理
	time.Sleep(time.Second)
}