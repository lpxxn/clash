package customer_service

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// UserStatus 用户状态
type UserStatus int

const (
	UserStatusOffline UserStatus = iota
	UserStatusOnline
	UserStatusInSession
)

// User 表示连接到系统的用户
type User struct {
	ID        string
	Name      string
	Status    UserStatus
	Conn      *websocket.Conn
	CreateAt  time.Time
	SessionID string
	mu        sync.RWMutex
}

// CSGroup 客服组
type CSGroup struct {
	ID      string
	Name    string
	Members map[string]*CSStaff
	mu      sync.RWMutex
}

// CSStaff 客服人员
type CSStaff struct {
	ID       string
	Name     string
	GroupID  string
	Status   UserStatus
	Conn     *websocket.Conn
	Sessions map[string]*Session // 当前处理的会话列表
	mu       sync.RWMutex
}

// Session 会话
type Session struct {
	ID        string
	UserID    string
	StaffID   string
	Status    SessionStatus
	CreateAt  time.Time
	UpdateAt  time.Time
	Messages  []*Message
	mu        sync.RWMutex
}

// SessionStatus 会话状态
type SessionStatus int

const (
	SessionStatusWaiting SessionStatus = iota
	SessionStatusActive
	SessionStatusClosed
)

// Message 消息
type Message struct {
	ID        string
	SessionID string
	FromID    string
	ToID      string
	Content   string
	Type      MessageType
	CreateAt  time.Time
}

// MessageType 消息类型
type MessageType int

const (
	MessageTypeText MessageType = iota
	MessageTypeImage
	MessageTypeSystem
)