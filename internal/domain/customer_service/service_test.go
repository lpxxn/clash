package customer_service

import (
	"io"
	"net"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
)

// mockWebsocketConn 模拟websocket连接
type mockWebsocketConn struct {
	closed bool
	*websocket.Conn
}

func (m *mockWebsocketConn) Close() error {
	m.closed = true
	// 不调用底层的websocket.Conn.Close()
	return nil
}

func (m *mockWebsocketConn) WriteMessage(messageType int, data []byte) error {
	return nil
}

func (m *mockWebsocketConn) ReadMessage() (messageType int, p []byte, err error) {
	return websocket.TextMessage, nil, nil
}

func (m *mockWebsocketConn) SetWriteDeadline(t time.Time) error {
	return nil
}

func (m *mockWebsocketConn) SetReadDeadline(t time.Time) error {
	return nil
}

func (m *mockWebsocketConn) SetReadLimit(limit int64) {}

func (m *mockWebsocketConn) EnableWriteCompression(enable bool) {}

func (m *mockWebsocketConn) SetCompressionLevel(level int) error {
	return nil
}

func (m *mockWebsocketConn) WriteControl(messageType int, data []byte, deadline time.Time) error {
	return nil
}

func (m *mockWebsocketConn) NextWriter(messageType int) (io.WriteCloser, error) {
	return nil, nil
}

func (m *mockWebsocketConn) NextReader() (messageType int, r io.Reader, err error) {
	return websocket.TextMessage, nil, nil
}

func (m *mockWebsocketConn) WritePreparedMessage(pm *websocket.PreparedMessage) error {
	return nil
}

func (m *mockWebsocketConn) WriteJSON(v interface{}) error {
	return nil
}

func (m *mockWebsocketConn) ReadJSON(v interface{}) error {
	return nil
}

func (m *mockWebsocketConn) SetPingHandler(h func(appData string) error) {
}

func (m *mockWebsocketConn) SetPongHandler(h func(appData string) error) {
}

func (m *mockWebsocketConn) UnderlyingConn() net.Conn {
	return nil
}

func (m *mockWebsocketConn) Subprotocol() string {
	return ""
}

func TestCustomerService_Basic(t *testing.T) {
	cs := NewCustomerService()
	assert.NotNil(t, cs)
	assert.Empty(t, cs.users)
	assert.Empty(t, cs.staffs)
	assert.Empty(t, cs.groups)
	assert.Empty(t, cs.sessions)
}

func TestCustomerService_ConnectUser(t *testing.T) {
	cs := NewCustomerService()
	conn := &mockWebsocketConn{}

	user := cs.ConnectUser("user1", "TestUser", conn.Conn)
	assert.NotNil(t, user)
	assert.Equal(t, "user1", user.ID)
	assert.Equal(t, "TestUser", user.Name)
	assert.Equal(t, UserStatusOnline, user.Status)
	assert.NotNil(t, user.Conn)
	assert.NotZero(t, user.CreateAt)

	// 验证用户是否已添加到系统中
	assert.Len(t, cs.users, 1)
	assert.Equal(t, user, cs.users["user1"])
}

func TestCustomerService_ConnectStaff(t *testing.T) {
	cs := NewCustomerService()
	conn := &mockWebsocketConn{}

	// 创建客服组
	group := cs.CreateGroup("group1", "TestGroup")
	assert.NotNil(t, group)

	// 测试连接客服
	staff, err := cs.ConnectStaff("staff1", "TestStaff", "group1", conn.Conn)
	assert.NoError(t, err)
	assert.NotNil(t, staff)
	assert.Equal(t, "staff1", staff.ID)
	assert.Equal(t, "TestStaff", staff.Name)
	assert.Equal(t, "group1", staff.GroupID)
	assert.Equal(t, UserStatusOnline, staff.Status)
	assert.NotNil(t, staff.Conn)
	assert.Empty(t, staff.Sessions)

	// 验证客服是否已添加到系统和组中
	assert.Len(t, cs.staffs, 1)
	assert.Equal(t, staff, cs.staffs["staff1"])
	assert.Equal(t, staff, group.Members["staff1"])

	// 测试连接到不存在的组
	_, err = cs.ConnectStaff("staff2", "TestStaff2", "nonexistent", conn.Conn)
	assert.Equal(t, ErrGroupNotFound, err)
}

func TestCustomerService_CreateSession(t *testing.T) {
	cs := NewCustomerService()
	conn := &mockWebsocketConn{}

	// 准备测试数据
	cs.CreateGroup("group1", "TestGroup")
	user := cs.ConnectUser("user1", "TestUser", conn.Conn)
	staff, _ := cs.ConnectStaff("staff1", "TestStaff", "group1", conn.Conn)

	// 测试创建会话
	session, err := cs.CreateSession("user1", "staff1")
	assert.NoError(t, err)
	assert.NotNil(t, session)
	assert.Contains(t, session.ID, "user1_staff1")
	assert.Equal(t, "user1", session.UserID)
	assert.Equal(t, "staff1", session.StaffID)
	assert.Equal(t, SessionStatusActive, session.Status)
	assert.NotZero(t, session.CreateAt)
	assert.NotZero(t, session.UpdateAt)
	assert.Empty(t, session.Messages)

	// 验证会话是否正确关联
	assert.Equal(t, session.ID, user.SessionID)
	assert.Equal(t, UserStatusInSession, user.Status)
	assert.Equal(t, session, staff.Sessions[session.ID])
	assert.Equal(t, session, cs.sessions[session.ID])

	// 测试错误情况
	_, err = cs.CreateSession("nonexistent", "staff1")
	assert.Equal(t, ErrUserNotFound, err)

	_, err = cs.CreateSession("user1", "nonexistent")
	assert.Equal(t, ErrStaffNotFound, err)
}

func TestCustomerService_TransferSession(t *testing.T) {
	cs := NewCustomerService()
	conn := &mockWebsocketConn{}

	// 准备测试数据
	cs.CreateGroup("group1", "TestGroup")
	cs.ConnectUser("user1", "TestUser", conn.Conn)
	staff1, _ := cs.ConnectStaff("staff1", "TestStaff1", "group1", conn.Conn)
	staff2, _ := cs.ConnectStaff("staff2", "TestStaff2", "group1", conn.Conn)
	session, _ := cs.CreateSession("user1", "staff1")

	// 测试转移会话
	err := cs.TransferSession(session.ID, "staff2")
	assert.NoError(t, err)
	assert.Equal(t, "staff2", session.StaffID)
	assert.NotContains(t, staff1.Sessions, session.ID)
	assert.Contains(t, staff2.Sessions, session.ID)

	// 测试错误情况
	err = cs.TransferSession("nonexistent", "staff2")
	assert.Equal(t, ErrSessionNotFound, err)

	err = cs.TransferSession(session.ID, "nonexistent")
	assert.Equal(t, ErrStaffNotFound, err)
}

func TestCustomerService_SendMessage(t *testing.T) {
	cs := NewCustomerService()
	conn := &mockWebsocketConn{}

	// 准备测试数据
	cs.CreateGroup("group1", "TestGroup")
	cs.ConnectUser("user1", "TestUser", conn.Conn)
	cs.ConnectStaff("staff1", "TestStaff", "group1", conn.Conn)
	session, _ := cs.CreateSession("user1", "staff1")

	// 测试用户发送消息
	userMsg, err := cs.SendMessage(session.ID, "user1", "Hello", MessageTypeText)
	assert.NoError(t, err)
	assert.NotNil(t, userMsg)
	assert.Equal(t, session.ID, userMsg.SessionID)
	assert.Equal(t, "user1", userMsg.FromID)
	assert.Equal(t, "staff1", userMsg.ToID)
	assert.Equal(t, "Hello", userMsg.Content)
	assert.Equal(t, MessageTypeText, userMsg.Type)

	// 测试客服发送消息
	staffMsg, err := cs.SendMessage(session.ID, "staff1", "Hi", MessageTypeText)
	assert.NoError(t, err)
	assert.NotNil(t, staffMsg)
	assert.Equal(t, session.ID, staffMsg.SessionID)
	assert.Equal(t, "staff1", staffMsg.FromID)
	assert.Equal(t, "user1", staffMsg.ToID)
	assert.Equal(t, "Hi", staffMsg.Content)

	// 验证消息是否已添加到会话中
	assert.Len(t, session.Messages, 2)
	assert.Equal(t, userMsg, session.Messages[0])
	assert.Equal(t, staffMsg, session.Messages[1])

	// 测试错误情况
	_, err = cs.SendMessage("nonexistent", "user1", "Hello", MessageTypeText)
	assert.Equal(t, ErrSessionNotFound, err)

	_, err = cs.SendMessage(session.ID, "nonexistent", "Hello", MessageTypeText)
	assert.Equal(t, ErrInvalidOperation, err)
}

func TestCustomerService_DisconnectUser(t *testing.T) {
	cs := NewCustomerService()
	conn := &mockWebsocketConn{}

	// 准备测试数据
	cs.ConnectUser("user1", "TestUser", conn.Conn)

	// 测试断开连接
	cs.DisconnectUser("user1")
	assert.Empty(t, cs.users)
	assert.True(t, conn.closed)

	// 测试断开不存在的用户
	cs.DisconnectUser("nonexistent") // 不应该panic
}

func TestCustomerService_DisconnectStaff(t *testing.T) {
	cs := NewCustomerService()
	conn := &mockWebsocketConn{}

	// 准备测试数据
	cs.CreateGroup("group1", "TestGroup")
	cs.ConnectUser("user1", "TestUser", conn.Conn)
	cs.ConnectStaff("staff1", "TestStaff", "group1", conn.Conn)
	session, _ := cs.CreateSession("user1", "staff1")

	// 测试断开连接
	cs.DisconnectStaff("staff1")
	assert.Empty(t, cs.staffs)
	assert.True(t, conn.closed)
	assert.Empty(t, cs.groups["group1"].Members)
	assert.Equal(t, SessionStatusClosed, cs.sessions[session.ID].Status)

	// 测试断开不存在的客服
	cs.DisconnectStaff("nonexistent") // 不应该panic
}

func TestCustomerService_GetMethods(t *testing.T) {
	cs := NewCustomerService()
	conn := &mockWebsocketConn{}

	// 准备测试数据
	cs.CreateGroup("group1", "TestGroup")
	user := cs.ConnectUser("user1", "TestUser", conn.Conn)
	staff, _ := cs.ConnectStaff("staff1", "TestStaff", "group1", conn.Conn)
	session, _ := cs.CreateSession("user1", "staff1")

	// 测试获取方法
	assert.Equal(t, user, cs.GetUser("user1"))
	assert.Equal(t, staff, cs.GetStaff("staff1"))
	assert.Equal(t, session, cs.GetSession(session.ID))

	// 测试获取不存在的对象
	assert.Nil(t, cs.GetUser("nonexistent"))
	assert.Nil(t, cs.GetStaff("nonexistent"))
	assert.Nil(t, cs.GetSession("nonexistent"))
}
