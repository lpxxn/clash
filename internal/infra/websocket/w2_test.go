package websocket_test

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

// MockRedis Redis的mock实现
type MockRedis struct {
	data sync.Map
	mu   sync.Mutex
}

// NewMockRedis 创建MockRedis实例
func NewMockRedis() *MockRedis {
	return &MockRedis{}
}

// SetNX 实现SetNX命令
func (m *MockRedis) SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.data.Load(key); exists {
		return false, nil
	}

	m.data.Store(key, value)
	if expiration > 0 {
		go func() {
			time.Sleep(expiration)
			m.data.Delete(key)
		}()
	}
	return true, nil
}

// Set 实现Set命令
func (m *MockRedis) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	m.data.Store(key, value)
	if expiration > 0 {
		go func() {
			time.Sleep(expiration)
			m.data.Delete(key)
		}()
	}
	return nil
}

// Get 实现Get命令
func (m *MockRedis) Get(ctx context.Context, key string) (string, error) {
	if value, ok := m.data.Load(key); ok {
		if str, ok := value.(string); ok {
			return str, nil
		}
		return "", errors.New("value is not string")
	}
	return "", errors.New("key not found")
}

// Del 实现Del命令
func (m *MockRedis) Del(ctx context.Context, key string) error {
	m.data.Delete(key)
	return nil
}

type OrderStatus string

const (
	OrderStatusPending OrderStatus = "pending"
	OrderStatusPaid    OrderStatus = "paid"
	OrderStatusFailed  OrderStatus = "failed"
)

// OrderRequest 订单请求结构体
type OrderRequest struct {
	UserID    int64   `json:"user_id"`
	ProductID int64   `json:"product_id"`
	Quantity  int     `json:"quantity"`
	Amount    float64 `json:"amount"`
	OrderNote string  `json:"order_note"`
}

// Order 订单模型
type Order struct {
	OrderID    string      `json:"order_id"`
	UserID     int64       `json:"user_id"`
	ProductID  int64       `json:"product_id"`
	Quantity   int         `json:"quantity"`
	Amount     float64     `json:"amount"`
	Status     OrderStatus `json:"status"`
	CreateTime time.Time   `json:"create_time"`
	FeatureKey string      `json:"feature_key"`
	OrderNote  string      `json:"order_note"`
}

// RedisInterface Redis接口
type RedisInterface interface {
	SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) (bool, error)
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error
	Get(ctx context.Context, key string) (string, error)
	Del(ctx context.Context, key string) error
}

// OrderService 订单服务
type OrderService struct {
	redis RedisInterface
}

// NewOrderService 创建订单服务实例
func NewOrderService(redis RedisInterface) *OrderService {
	return &OrderService{
		redis: redis,
	}
}

// CreateOrder 创建订单
func (s *OrderService) CreateOrder(ctx context.Context, req *OrderRequest) (string, error) {
	// 1. 参数验证
	if err := s.validateOrderRequest(req); err != nil {
		return "", err
	}

	// 2. 生成订单特征值
	featureKey := s.generateOrderFeature(req)
	orderKey := fmt.Sprintf("order:feature:%s", featureKey)

	// 3. 使用SetNX检查和设置订单标识
	success, err := s.redis.SetNX(ctx, orderKey, "", 24*time.Hour)
	if err != nil {
		return "", fmt.Errorf("redis error: %v", err)
	}

	// 如果设置失败，说明已经存在相同特征的订单
	if !success {
		existingOrderID, err := s.getExistingOrderID(ctx, featureKey)
		if err != nil {
			return "", err
		}
		if existingOrderID != "" {
			return existingOrderID, nil
		}
	}

	// 4. 生成新的订单ID
	orderID := s.generateOrderID()

	// 5. 创建订单记录
	order := &Order{
		OrderID:    orderID,
		UserID:     req.UserID,
		ProductID:  req.ProductID,
		Quantity:   req.Quantity,
		Amount:     req.Amount,
		Status:     OrderStatusPending,
		CreateTime: time.Now(),
		FeatureKey: featureKey,
		OrderNote:  req.OrderNote,
	}

	// 6. 保存订单到数据库
	if err := s.saveOrder(ctx, order); err != nil {
		s.redis.Del(ctx, orderKey)
		return "", fmt.Errorf("save order failed: %v", err)
	}

	// 7. 更新Redis中的订单ID
	err = s.redis.Set(ctx, orderKey, orderID, 24*time.Hour)
	if err != nil {
		// 记录日志但不返回错误
		fmt.Printf("update redis order id failed: %v\n", err)
	}

	return orderID, nil
}

// validateOrderRequest 验证订单请求
func (s *OrderService) validateOrderRequest(req *OrderRequest) error {
	if req == nil {
		return errors.New("order request cannot be nil")
	}
	if req.UserID <= 0 {
		return errors.New("invalid user id")
	}
	if req.ProductID <= 0 {
		return errors.New("invalid product id")
	}
	if req.Quantity <= 0 {
		return errors.New("invalid quantity")
	}
	if req.Amount <= 0 {
		return errors.New("invalid amount")
	}
	return nil
}

// generateOrderFeature 生成订单特征值
func (s *OrderService) generateOrderFeature(req *OrderRequest) string {
	feature := fmt.Sprintf("%d:%d:%d:%.2f:%s",
		req.UserID,
		req.ProductID,
		req.Quantity,
		req.Amount,
		req.OrderNote,
	)

	hash := md5.New()
	hash.Write([]byte(feature))
	return hex.EncodeToString(hash.Sum(nil))
}

// generateOrderID 生成订单ID
func (s *OrderService) generateOrderID() string {
	timestamp := time.Now().UnixNano()
	random := time.Now().UnixNano() % 1000
	orderID := fmt.Sprintf("%d%d", timestamp, random)
	return orderID
}

// getExistingOrderID 获取已存在的订单ID
func (s *OrderService) getExistingOrderID(ctx context.Context, featureKey string) (string, error) {
	orderKey := fmt.Sprintf("order:feature:%s", featureKey)
	orderID, err := s.redis.Get(ctx, orderKey)
	if err != nil {
		return "", err
	}
	return orderID, nil
}

// saveOrder 保存订单到数据库
func (s *OrderService) saveOrder(ctx context.Context, order *Order) error {
	// 这里应该是实际的数据库操作
	// 为了演示，我们只打印订单信息
	orderJSON, _ := json.Marshal(order)
	fmt.Printf("Saving order to database: %s\n", string(orderJSON))
	return nil
}

func TestOrderService(t *testing.T) {
	// 创建MockRedis实例
	mockRedis := NewMockRedis()

	// 创建OrderService实例
	orderService := NewOrderService(mockRedis)

	// 测试用例
	tests := []struct {
		name    string
		req     *OrderRequest
		wantErr bool
	}{
		{
			name: "valid order",
			req: &OrderRequest{
				UserID:    1,
				ProductID: 1,
				Quantity:  1,
				Amount:    100.00,
				OrderNote: "test order",
			},
			wantErr: false,
		},
		{
			name: "invalid user id",
			req: &OrderRequest{
				UserID:    0,
				ProductID: 1,
				Quantity:  1,
				Amount:    100.00,
			},
			wantErr: true,
		},
		{
			name: "duplicate order",
			req: &OrderRequest{
				UserID:    1,
				ProductID: 1,
				Quantity:  1,
				Amount:    100.00,
				OrderNote: "test order",
			},
			wantErr: false,
		},
	}

	ctx := context.Background()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orderID, err := orderService.CreateOrder(ctx, tt.req)

			if (err != nil) != tt.wantErr {
				t.Errorf("CreateOrder() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && orderID == "" {
				t.Error("CreateOrder() returned empty orderID")
			}
		})

		// 等待一小段时间，确保异步操作完成
		time.Sleep(100 * time.Millisecond)
	}
}

func TestValidateOrderRequest(t *testing.T) {
	orderService := NewOrderService(NewMockRedis())

	tests := []struct {
		name    string
		req     *OrderRequest
		wantErr bool
	}{
		{
			name: "valid request",
			req: &OrderRequest{
				UserID:    1,
				ProductID: 1,
				Quantity:  1,
				Amount:    100.00,
			},
			wantErr: false,
		},
		{
			name:    "nil request",
			req:     nil,
			wantErr: true,
		},
		{
			name: "invalid user id",
			req: &OrderRequest{
				UserID:    0,
				ProductID: 1,
				Quantity:  1,
				Amount:    100.00,
			},
			wantErr: true,
		},
		{
			name: "invalid product id",
			req: &OrderRequest{
				UserID:    1,
				ProductID: 0,
				Quantity:  1,
				Amount:    100.00,
			},
			wantErr: true,
		},
		{
			name: "invalid quantity",
			req: &OrderRequest{
				UserID:    1,
				ProductID: 1,
				Quantity:  0,
				Amount:    100.00,
			},
			wantErr: true,
		},
		{
			name: "invalid amount",
			req: &OrderRequest{
				UserID:    1,
				ProductID: 1,
				Quantity:  1,
				Amount:    0,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := orderService.validateOrderRequest(tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateOrderRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
