package conversation

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"llm-message-queue/pkg/models"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// 定义持久化存储的错误类型
var (
	ErrPersistenceFailure = errors.New("failed to persist conversation")
	ErrLoadFailure        = errors.New("failed to load conversation")
	ErrNotFound           = errors.New("conversation not found")
)

// RedisPersistenceStore 使用Redis实现对话状态的持久化
type RedisPersistenceStore struct {
	client     *redis.Client
	prefix     string        // 键前缀
	expiration time.Duration // 过期时间
	logger     *zap.Logger
}

// NewRedisPersistenceStore 创建新的Redis持久化存储
func NewRedisPersistenceStore(client *redis.Client, prefix string, expiration time.Duration, logger *zap.Logger) *RedisPersistenceStore {
	if logger == nil {
		logger, _ = zap.NewProduction()
	}

	return &RedisPersistenceStore{
		client:     client,
		prefix:     prefix,
		expiration: expiration,
		logger:     logger,
	}
}

// SaveConversation 将对话保存到Redis
func (s *RedisPersistenceStore) SaveConversation(conversation *models.Conversation) error {
	data, err := json.Marshal(conversation)
	if err != nil {
		s.logger.Error("Failed to marshal conversation",
			zap.String("conversationID", conversation.ID),
			zap.Error(err))
		return ErrPersistenceFailure
	}

	ctx := context.Background()
	key := s.prefix + conversation.ID

	// 保存对话数据
	err = s.client.Set(ctx, key, data, s.expiration).Err()
	if err != nil {
		s.logger.Error("Failed to save conversation to Redis",
			zap.String("conversationID", conversation.ID),
			zap.Error(err))
		return ErrPersistenceFailure
	}

	// 保存用户到对话的映射
	userKey := s.prefix + "user:" + conversation.UserID
	err = s.client.SAdd(ctx, userKey, conversation.ID).Err()
	if err != nil {
		s.logger.Error("Failed to save user-conversation mapping",
			zap.String("userID", conversation.UserID),
			zap.String("conversationID", conversation.ID),
			zap.Error(err))
		// 不返回错误，因为主要数据已保存成功
	}

	// 设置用户映射的过期时间
	s.client.Expire(ctx, userKey, s.expiration)

	return nil
}

// LoadConversation 从Redis加载对话
func (s *RedisPersistenceStore) LoadConversation(conversationID string) (*models.Conversation, error) {
	ctx := context.Background()
	key := s.prefix + conversationID

	data, err := s.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, ErrNotFound
		}
		s.logger.Error("Failed to load conversation from Redis",
			zap.String("conversationID", conversationID),
			zap.Error(err))
		return nil, ErrLoadFailure
	}

	var conversation models.Conversation
	err = json.Unmarshal(data, &conversation)
	if err != nil {
		s.logger.Error("Failed to unmarshal conversation data",
			zap.String("conversationID", conversationID),
			zap.Error(err))
		return nil, ErrLoadFailure
	}

	return &conversation, nil
}

// ListUserConversations 列出用户的所有对话ID
func (s *RedisPersistenceStore) ListUserConversations(userID string) ([]string, error) {
	ctx := context.Background()
	userKey := s.prefix + "user:" + userID

	conversationIDs, err := s.client.SMembers(ctx, userKey).Result()
	if err != nil {
		if err == redis.Nil {
			return []string{}, nil
		}
		s.logger.Error("Failed to list user conversations from Redis",
			zap.String("userID", userID),
			zap.Error(err))
		return nil, ErrLoadFailure
	}

	return conversationIDs, nil
}

// DeleteConversation 从Redis删除对话
func (s *RedisPersistenceStore) DeleteConversation(conversationID string) error {
	ctx := context.Background()
	key := s.prefix + conversationID

	// 首先获取对话以找到用户ID
	conversation, err := s.LoadConversation(conversationID)
	if err != nil {
		if err == ErrNotFound {
			return nil // 已经不存在，视为成功
		}
		return err
	}

	// 删除对话数据
	err = s.client.Del(ctx, key).Err()
	if err != nil {
		s.logger.Error("Failed to delete conversation from Redis",
			zap.String("conversationID", conversationID),
			zap.Error(err))
		return ErrPersistenceFailure
	}

	// 从用户映射中移除
	userKey := s.prefix + "user:" + conversation.UserID
	s.client.SRem(ctx, userKey, conversationID)

	return nil
}

// PostgresPersistenceStore 使用PostgreSQL实现对话状态的持久化
type PostgresPersistenceStore struct {
	db     *gorm.DB
	logger *zap.Logger
}

// ConversationModel 是对话的数据库模型
type ConversationModel struct {
	ID             string `gorm:"primaryKey"`
	UserID         string `gorm:"index"`
	CreatedAt      time.Time
	LastActiveTime time.Time
	CompletedAt    *time.Time
	State          string
	Messages       []byte // JSON序列化的消息数组
	Metadata       []byte // JSON序列化的元数据
}

// NewPostgresPersistenceStore 创建新的PostgreSQL持久化存储
func NewPostgresPersistenceStore(db *gorm.DB, logger *zap.Logger) (*PostgresPersistenceStore, error) {
	if logger == nil {
		logger, _ = zap.NewProduction()
	}

	// 自动迁移表结构
	err := db.AutoMigrate(&ConversationModel{})
	if err != nil {
		logger.Error("Failed to migrate conversation table", zap.Error(err))
		return nil, err
	}

	return &PostgresPersistenceStore{
		db:     db,
		logger: logger,
	}, nil
}

// SaveConversation 将对话保存到PostgreSQL
func (s *PostgresPersistenceStore) SaveConversation(conversation *models.Conversation) error {
	// 序列化消息和元数据
	messagesJSON, err := json.Marshal(conversation.Messages)
	if err != nil {
		s.logger.Error("Failed to marshal conversation messages",
			zap.String("conversationID", conversation.ID),
			zap.Error(err))
		return ErrPersistenceFailure
	}

	metadataJSON, err := json.Marshal(conversation.Metadata)
	if err != nil {
		s.logger.Error("Failed to marshal conversation metadata",
			zap.String("conversationID", conversation.ID),
			zap.Error(err))
		return ErrPersistenceFailure
	}

	// 创建数据库模型
	model := ConversationModel{
		ID:             conversation.ID,
		UserID:         conversation.UserID,
		CreatedAt:      conversation.CreatedAt,
		LastActiveTime: conversation.LastActiveTime,
		State:          string(conversation.State),
		Messages:       messagesJSON,
		Metadata:       metadataJSON,
	}

	if !conversation.CompletedAt.IsZero() {
		model.CompletedAt = &conversation.CompletedAt
	}

	// 保存到数据库（使用Upsert）
	result := s.db.Save(&model)
	if result.Error != nil {
		s.logger.Error("Failed to save conversation to PostgreSQL",
			zap.String("conversationID", conversation.ID),
			zap.Error(result.Error))
		return ErrPersistenceFailure
	}

	return nil
}

// LoadConversation 从PostgreSQL加载对话
func (s *PostgresPersistenceStore) LoadConversation(conversationID string) (*models.Conversation, error) {
	var model ConversationModel
	result := s.db.First(&model, "id = ?", conversationID)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		s.logger.Error("Failed to load conversation from PostgreSQL",
			zap.String("conversationID", conversationID),
			zap.Error(result.Error))
		return nil, ErrLoadFailure
	}

	// 反序列化消息和元数据
	var messages []models.Message
	err := json.Unmarshal(model.Messages, &messages)
	if err != nil {
		s.logger.Error("Failed to unmarshal conversation messages",
			zap.String("conversationID", conversationID),
			zap.Error(err))
		return nil, ErrLoadFailure
	}

	var metadata map[string]interface{}
	err = json.Unmarshal(model.Metadata, &metadata)
	if err != nil {
		s.logger.Error("Failed to unmarshal conversation metadata",
			zap.String("conversationID", conversationID),
			zap.Error(err))
		return nil, ErrLoadFailure
	}

	// 创建对话模型
	conversation := &models.Conversation{
		ID:             model.ID,
		UserID:         model.UserID,
		CreatedAt:      model.CreatedAt,
		LastActiveTime: model.LastActiveTime,
		State:          models.ConversationState(model.State),
		Messages:       messages,
		Metadata:       metadata,
	}

	if model.CompletedAt != nil {
		conversation.CompletedAt = *model.CompletedAt
	}

	return conversation, nil
}

// ListUserConversations 列出用户的所有对话ID
func (s *PostgresPersistenceStore) ListUserConversations(userID string) ([]string, error) {
	var ids []string
	result := s.db.Model(&ConversationModel{}).Where("user_id = ?", userID).Pluck("id", &ids)
	if result.Error != nil {
		s.logger.Error("Failed to list user conversations from PostgreSQL",
			zap.String("userID", userID),
			zap.Error(result.Error))
		return nil, ErrLoadFailure
	}

	return ids, nil
}

// DeleteConversation 从PostgreSQL删除对话
func (s *PostgresPersistenceStore) DeleteConversation(conversationID string) error {
	result := s.db.Delete(&ConversationModel{}, "id = ?", conversationID)
	if result.Error != nil {
		s.logger.Error("Failed to delete conversation from PostgreSQL",
			zap.String("conversationID", conversationID),
			zap.Error(result.Error))
		return ErrPersistenceFailure
	}

	return nil
}
