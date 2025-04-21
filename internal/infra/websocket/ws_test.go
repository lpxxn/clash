package websocket_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
)

type Message struct {
	ID      int    `json:"id"`
	Content string `json:"content"`
	Time    int64  `json:"time"`
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

func TestWebSocketJSONMessage(t *testing.T) {
	// 创建测试消息
	// messages := []Message{
	// 	{ID: 1, Content: "Hello", Time: time.Now().Unix()},
	// 	{ID: 2, Content: "World", Time: time.Now().Unix()},
	// 	{ID: 3, Content: strings.Repeat("Large Message ", 1000), Time: time.Now().Unix()},
	// 	{ID: 4, Content: "Small Message", Time: time.Now().Unix()},
	// }
	// 创建测试消息
	messages := make([]Message, 0, 100)
	for i := 1; i <= 200; i++ {
		messages = append(messages, Message{
			ID:      i,
			Content: fmt.Sprintf("Message-%d-%s", i, strings.Repeat("content", i%5+1)),
			Time:    time.Now().Unix(),
		})
	}

	// 创建 WebSocket 服务器
	var receivedMessages []Message
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()

		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var message Message
			if err := json.Unmarshal(msg, &message); err != nil {
				t.Error(err)
				return
			}
			t.Logf("Received message: %s\n", msg)
			// fmt.Printf("Received message: %s\n", msg)
			mu.Lock()
			receivedMessages = append(receivedMessages, message)
			mu.Unlock()
		}
	}))
	defer server.Close()

	// 创建 WebSocket 客户端
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	// 快速发送所有消息
	for _, msg := range messages {
		data, err := json.Marshal(msg)
		if err != nil {
			t.Fatal(err)
		}
		err = conn.WriteMessage(websocket.TextMessage, data)
		if err != nil {
			t.Fatal(err)
		}
	}

	// 等待消息接收完成
	time.Sleep(time.Second)

	// 验证接收到的消息
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, len(messages), len(receivedMessages), "消息数量不匹配")
	for i, msg := range messages {
		assert.Equal(t, msg.ID, receivedMessages[i].ID, "消息ID不匹配")
		assert.Equal(t, msg.Content, receivedMessages[i].Content, "消息内容不匹配")
		assert.Equal(t, msg.Time, receivedMessages[i].Time, "消息时间不匹配")
	}
}
