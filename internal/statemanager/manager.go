package statemanager

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"llm-message-queue/pkg/models"

	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"
)

type StateManager struct {
	db    *gorm.DB
	redis *redis.Client
	mu    sync.RWMutex
	cache map[string]*models.Conversation
}

type ConversationState struct {
	ID           string                 `json:"id"`
	UserID       string                 `json:"user_id"`
	Title        string                 `json:"title"`
	Context      string                 `json:"context"`
	Status       string                 `json:"status"`
	Priority     models.Priority        `json:"priority"`
	MessageCount int                    `json:"message_count"`
	LastActivity time.Time              `json:"last_activity"`
	Metadata     map[string]interface{} `json:"metadata"`
	Messages     []*models.Message      `json:"messages,omitempty"`
}

type StateUpdate struct {
	ConversationID string
	Field          string
	Value          interface{}
	Timestamp      time.Time
}

func NewStateManager(db *gorm.DB, redisClient *redis.Client) *StateManager {
	return &StateManager{
		db:    db,
		redis: redisClient,
		cache: make(map[string]*models.Conversation),
	}
}

func (sm *StateManager) CreateConversation(ctx context.Context, userID, title string, priority models.Priority) (*models.Conversation, error) {
	conversation := &models.Conversation{
		ID:           generateConversationID(),
		UserID:       userID,
		Title:        title,
		Status:       "active",
		Priority:     priority,
		MessageCount: 0,
		LastActivity: time.Now(),
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		Metadata:     make(map[string]interface{}),
	}

	if err := sm.db.WithContext(ctx).Create(conversation).Error; err != nil {
		return nil, fmt.Errorf("failed to create conversation: %w", err)
	}

	sm.cacheConversation(conversation)
	sm.cacheToRedis(ctx, conversation)

	return conversation, nil
}

func (sm *StateManager) GetConversation(ctx context.Context, conversationID string) (*models.Conversation, error) {
	sm.mu.RLock()
	if cached, exists := sm.cache[conversationID]; exists {
		sm.mu.RUnlock()
		return cached, nil
	}
	sm.mu.RUnlock()

	conversation, err := sm.getFromRedis(ctx, conversationID)
	if err == nil && conversation != nil {
		sm.cacheConversation(conversation)
		return conversation, nil
	}

	var conversationDB models.Conversation
	if err := sm.db.WithContext(ctx).First(&conversationDB, "id = ?", conversationID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("conversation not found: %s", conversationID)
		}
		return nil, fmt.Errorf("failed to get conversation: %w", err)
	}

	sm.cacheConversation(&conversationDB)
	sm.cacheToRedis(ctx, &conversationDB)

	return &conversationDB, nil
}

func (sm *StateManager) UpdateConversation(ctx context.Context, conversationID string, updates map[string]interface{}) error {
	if err := sm.db.WithContext(ctx).Model(&models.Conversation{}).
		Where("id = ?", conversationID).
		Updates(updates).Error; err != nil {
		return fmt.Errorf("failed to update conversation: %w", err)
	}

	sm.invalidateCache(conversationID)
	sm.invalidateRedis(ctx, conversationID)

	return nil
}

func (sm *StateManager) AddMessage(ctx context.Context, conversationID string, message *models.Message) error {
	conversation, err := sm.GetConversation(ctx, conversationID)
	if err != nil {
		return err
	}

	message.ConversationID = conversationID
	if err := sm.db.WithContext(ctx).Create(message).Error; err != nil {
		return fmt.Errorf("failed to add message: %w", err)
	}

	updates := map[string]interface{}{
		"message_count": conversation.MessageCount + 1,
		"last_activity": time.Now(),
		"updated_at":    time.Now(),
	}

	if message.Status == models.StatusCompleted {
		updates["context"] = conversation.Context + "\n" + message.Content
	}

	return sm.UpdateConversation(ctx, conversationID, updates)
}

func (sm *StateManager) GetConversationMessages(ctx context.Context, conversationID string, limit int) ([]*models.Message, error) {
	var messages []*models.Message
	if err := sm.db.WithContext(ctx).
		Where("conversation_id = ?", conversationID).
		Order("created_at DESC").
		Limit(limit).
		Find(&messages).Error; err != nil {
		return nil, fmt.Errorf("failed to get messages: %w", err)
	}
	return messages, nil
}

func (sm *StateManager) GetUserConversations(ctx context.Context, userID string, limit int) ([]*models.Conversation, error) {
	var conversations []*models.Conversation
	if err := sm.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("last_activity DESC").
		Limit(limit).
		Find(&conversations).Error; err != nil {
		return nil, fmt.Errorf("failed to get user conversations: %w", err)
	}
	return conversations, nil
}

func (sm *StateManager) ArchiveConversation(ctx context.Context, conversationID string) error {
	return sm.UpdateConversation(ctx, conversationID, map[string]interface{}{
		"status":     "archived",
		"updated_at": time.Now(),
	})
}

func (sm *StateManager) DeleteConversation(ctx context.Context, conversationID string) error {
	if err := sm.db.WithContext(ctx).Delete(&models.Conversation{}, "id = ?", conversationID).Error; err != nil {
		return fmt.Errorf("failed to delete conversation: %w", err)
	}

	sm.invalidateCache(conversationID)
	sm.invalidateRedis(ctx, conversationID)

	return nil
}

func (sm *StateManager) GetActiveConversations(ctx context.Context, limit int) ([]*models.Conversation, error) {
	var conversations []*models.Conversation
	if err := sm.db.WithContext(ctx).
		Where("status = ?", "active").
		Order("last_activity DESC").
		Limit(limit).
		Find(&conversations).Error; err != nil {
		return nil, fmt.Errorf("failed to get active conversations: %w", err)
	}
	return conversations, nil
}

func (sm *StateManager) UpdateConversationPriority(ctx context.Context, conversationID string, priority models.Priority) error {
	return sm.UpdateConversation(ctx, conversationID, map[string]interface{}{
		"priority":   priority,
		"updated_at": time.Now(),
	})
}

func (sm *StateManager) GetConversationContext(ctx context.Context, conversationID string) (string, error) {
	conversation, err := sm.GetConversation(ctx, conversationID)
	if err != nil {
		return "", err
	}
	return conversation.Context, nil
}

// UpdateMessage updates a message in the database
func (sm *StateManager) UpdateMessage(ctx context.Context, message *models.Message) error {
	if err := sm.db.WithContext(ctx).Save(message).Error; err != nil {
		return fmt.Errorf("failed to update message: %w", err)
	}
	return nil
}

func (sm *StateManager) cacheConversation(conversation *models.Conversation) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.cache[conversation.ID] = conversation
}

func (sm *StateManager) invalidateCache(conversationID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	delete(sm.cache, conversationID)
}

func (sm *StateManager) cacheToRedis(ctx context.Context, conversation *models.Conversation) error {
	if sm.redis == nil {
		return nil
	}

	data, err := json.Marshal(conversation)
	if err != nil {
		return err
	}

	key := fmt.Sprintf("conversation:%s", conversation.ID)
	return sm.redis.Set(ctx, key, data, 24*time.Hour).Err()
}

func (sm *StateManager) getFromRedis(ctx context.Context, conversationID string) (*models.Conversation, error) {
	if sm.redis == nil {
		return nil, fmt.Errorf("redis not configured")
	}

	key := fmt.Sprintf("conversation:%s", conversationID)
	data, err := sm.redis.Get(ctx, key).Result()
	if err != nil {
		return nil, err
	}

	var conversation models.Conversation
	if err := json.Unmarshal([]byte(data), &conversation); err != nil {
		return nil, err
	}

	return &conversation, nil
}

func (sm *StateManager) invalidateRedis(ctx context.Context, conversationID string) error {
	if sm.redis == nil {
		return nil
	}

	key := fmt.Sprintf("conversation:%s", conversationID)
	return sm.redis.Del(ctx, key).Err()
}

func generateConversationID() string {
	return fmt.Sprintf("conv_%d", time.Now().UnixNano())
}
