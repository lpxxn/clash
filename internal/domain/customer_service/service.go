package customer_service

import (
	"errors"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var (
	ErrUserNotFound     = errors.New("user not found")
	ErrStaffNotFound    = errors.New("staff not found")
	ErrSessionNotFound  = errors.New("session not found")
	ErrGroupNotFound    = errors.New("group not found")
	ErrInvalidOperation = errors.New("invalid operation")
)

// CustomerService 客服系统服务
type CustomerService struct {
	users    map[string]*User    // 在线用户列表
	staffs   map[string]*CSStaff // 在线客服列表
	groups   map[string]*CSGroup // 客服组列表
	sessions map[string]*Session // 活动会话列表
	mu       sync.RWMutex
}

// NewCustomerService 创建新的客服系统服务实例
func NewCustomerService() *CustomerService {
	return &CustomerService{
		users:    make(map[string]*User),
		staffs:   make(map[string]*CSStaff),
		groups:   make(map[string]*CSGroup),
		sessions: make(map[string]*Session),
	}
}

// ConnectUser 处理用户WebSocket连接
func (cs *CustomerService) ConnectUser(userID, name string, conn *websocket.Conn) *User {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	user := &User{
		ID:       userID,
		Name:     name,
		Status:   UserStatusOnline,
		Conn:     conn,
		CreateAt: time.Now(),
	}
	cs.users[userID] = user
	return user
}

// ConnectStaff 处理客服WebSocket连接
func (cs *CustomerService) ConnectStaff(staffID, name, groupID string, conn *websocket.Conn) (*CSStaff, error) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	group, exists := cs.groups[groupID]
	if !exists {
		return nil, ErrGroupNotFound
	}

	staff := &CSStaff{
		ID:       staffID,
		Name:     name,
		GroupID:  groupID,
		Status:   UserStatusOnline,
		Conn:     conn,
		Sessions: make(map[string]*Session),
	}

	cs.staffs[staffID] = staff
	group.Members[staffID] = staff
	return staff, nil
}

// CreateGroup 创建客服组
func (cs *CustomerService) CreateGroup(groupID, name string) *CSGroup {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	group := &CSGroup{
		ID:      groupID,
		Name:    name,
		Members: make(map[string]*CSStaff),
	}
	cs.groups[groupID] = group
	return group
}

// CreateSession 创建会话
func (cs *CustomerService) CreateSession(userID, staffID string) (*Session, error) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	user, exists := cs.users[userID]
	if !exists {
		return nil, ErrUserNotFound
	}

	staff, exists := cs.staffs[staffID]
	if !exists {
		return nil, ErrStaffNotFound
	}

	session := &Session{
		ID:       userID + "_" + staffID + "_" + time.Now().Format("20060102150405"),
		UserID:   userID,
		StaffID:  staffID,
		Status:   SessionStatusActive,
		CreateAt: time.Now(),
		UpdateAt: time.Now(),
		Messages: make([]*Message, 0),
	}

	cs.sessions[session.ID] = session
	staff.Sessions[session.ID] = session
	user.SessionID = session.ID
	user.Status = UserStatusInSession

	return session, nil
}

// TransferSession 转移会话给其他客服
func (cs *CustomerService) TransferSession(sessionID, newStaffID string) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	session, exists := cs.sessions[sessionID]
	if !exists {
		return ErrSessionNotFound
	}

	newStaff, exists := cs.staffs[newStaffID]
	if !exists {
		return ErrStaffNotFound
	}

	oldStaff, exists := cs.staffs[session.StaffID]
	if !exists {
		return ErrStaffNotFound
	}

	// 从原客服的会话列表中移除
	delete(oldStaff.Sessions, sessionID)

	// 更新会话信息
	session.StaffID = newStaffID
	session.UpdateAt = time.Now()

	// 添加到新客服的会话列表
	newStaff.Sessions[sessionID] = session

	return nil
}

// SendMessage 发送消息
func (cs *CustomerService) SendMessage(sessionID, fromID, content string, msgType MessageType) (*Message, error) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	session, exists := cs.sessions[sessionID]
	if !exists {
		return nil, ErrSessionNotFound
	}

	msg := &Message{
		ID:        sessionID + "_" + time.Now().Format("20060102150405"),
		SessionID: sessionID,
		FromID:    fromID,
		ToID:      "", // 根据fromID是用户还是客服来设置
		Content:   content,
		Type:      msgType,
		CreateAt:  time.Now(),
	}

	// 设置接收者ID
	if fromID == session.UserID {
		msg.ToID = session.StaffID
	} else if fromID == session.StaffID {
		msg.ToID = session.UserID
	} else {
		return nil, ErrInvalidOperation
	}

	session.Messages = append(session.Messages, msg)
	session.UpdateAt = time.Now()

	return msg, nil
}

// DisconnectUser 处理用户断开连接
func (cs *CustomerService) DisconnectUser(userID string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if user, exists := cs.users[userID]; exists {
		user.Status = UserStatusOffline
		if user.Conn != nil {
			user.Conn.Close()
		}
		delete(cs.users, userID)
	}
}

// DisconnectStaff 处理客服断开连接
func (cs *CustomerService) DisconnectStaff(staffID string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if staff, exists := cs.staffs[staffID]; exists {
		staff.Status = UserStatusOffline
		if staff.Conn != nil {
			staff.Conn.Close()
		}
		
		// 从所属组中移除
		if group, exists := cs.groups[staff.GroupID]; exists {
			delete(group.Members, staffID)
		}

		// 关闭该客服的所有会话
		for sessionID := range staff.Sessions {
			if session, exists := cs.sessions[sessionID]; exists {
				session.Status = SessionStatusClosed
				session.UpdateAt = time.Now()
			}
		}

		delete(cs.staffs, staffID)
	}
}

// GetUser 获取用户信息
func (cs *CustomerService) GetUser(userID string) *User {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	if user, exists := cs.users[userID]; exists {
		return user
	}
	return nil
}

// GetStaff 获取客服信息
func (cs *CustomerService) GetStaff(staffID string) *CSStaff {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	if staff, exists := cs.staffs[staffID]; exists {
		return staff
	}
	return nil
}

// GetSession 获取会话信息
func (cs *CustomerService) GetSession(sessionID string) *Session {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	if session, exists := cs.sessions[sessionID]; exists {
		return session
	}
	return nil
}