package conversation

import (
	"sync"
	"time"

	"llm-message-queue/pkg/models"

	"go.uber.org/zap"
)

// StateManager 管理对话状态和上下文
type StateManager struct {
	conversations      map[string]*models.Conversation // 对话ID到对话的映射
	activeUsers        map[string][]string             // 用户ID到其活跃对话ID的映射
	conversationTTL    time.Duration                   // 对话状态的生存时间
	cleanupInterval    time.Duration                   // 清理过期对话的间隔
	maxConversations   int                             // 每个用户的最大对话数
	maxContextLength   int                             // 每个对话的最大上下文长度
	maxIdleTime        time.Duration                   // 对话的最大空闲时间
	persistenceEnabled bool                            // 是否启用持久化
	persistenceStore   PersistenceStore                // 持久化存储接口
	logger             *zap.Logger                     // 日志记录器
	mutex              sync.RWMutex                    // 读写锁
}

// PersistenceStore 定义了对话状态持久化的接口
type PersistenceStore interface {
	SaveConversation(conversation *models.Conversation) error
	LoadConversation(conversationID string) (*models.Conversation, error)
	ListUserConversations(userID string) ([]string, error)
	DeleteConversation(conversationID string) error
}

// StateManagerConfig 配置对话状态管理器
type StateManagerConfig struct {
	ConversationTTL    time.Duration
	CleanupInterval    time.Duration
	MaxConversations   int
	MaxContextLength   int
	MaxIdleTime        time.Duration
	PersistenceEnabled bool
	PersistenceStore   PersistenceStore
}

// NewStateManager 创建新的对话状态管理器
func NewStateManager(config StateManagerConfig, logger *zap.Logger) *StateManager {
	if logger == nil {
		logger, _ = zap.NewProduction()
	}

	manager := &StateManager{
		conversations:      make(map[string]*models.Conversation),
		activeUsers:        make(map[string][]string),
		conversationTTL:    config.ConversationTTL,
		cleanupInterval:    config.CleanupInterval,
		maxConversations:   config.MaxConversations,
		maxContextLength:   config.MaxContextLength,
		maxIdleTime:        config.MaxIdleTime,
		persistenceEnabled: config.PersistenceEnabled,
		persistenceStore:   config.PersistenceStore,
		logger:             logger,
	}

	// 启动定期清理过期对话的协程
	go manager.startCleanupTask()

	return manager
}

// GetConversation 获取指定ID的对话，如果不存在则创建
func (sm *StateManager) GetConversation(conversationID string, userID string) (*models.Conversation, error) {
	sm.mutex.RLock()
	conv, exists := sm.conversations[conversationID]
	sm.mutex.RUnlock()

	if exists {
		// 更新最后活跃时间
		sm.mutex.Lock()
		conv.LastActiveTime = time.Now()
		sm.mutex.Unlock()
		return conv, nil
	}

	// 如果启用了持久化，尝试从存储中加载
	if sm.persistenceEnabled && sm.persistenceStore != nil {
		loadedConv, err := sm.persistenceStore.LoadConversation(conversationID)
		if err == nil && loadedConv != nil {
			sm.mutex.Lock()
			sm.conversations[conversationID] = loadedConv
			sm.addToActiveUsers(userID, conversationID)
			sm.mutex.Unlock()
			return loadedConv, nil
		}
	}

	// 创建新对话
	newConv := &models.Conversation{
		ID:             conversationID,
		UserID:         userID,
		CreatedAt:      time.Now(),
		LastActiveTime: time.Now(),
		Messages:       []models.Message{},
		Metadata:       make(map[string]interface{}),
		State:          models.ConversationStateActive,
	}

	sm.mutex.Lock()
	sm.conversations[conversationID] = newConv
	sm.addToActiveUsers(userID, conversationID)
	sm.mutex.Unlock()

	return newConv, nil
}

// AddMessage 向对话添加新消息并更新状态
func (sm *StateManager) AddMessage(conversationID string, message models.Message) error {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	conv, exists := sm.conversations[conversationID]
	if !exists {
		return models.ErrConversationNotFound
	}

	// 添加消息到对话
	conv.Messages = append(conv.Messages, message)
	conv.LastActiveTime = time.Now()

	// 如果超过最大上下文长度，移除最旧的消息
	if sm.maxContextLength > 0 && len(conv.Messages) > sm.maxContextLength {
		excess := len(conv.Messages) - sm.maxContextLength
		conv.Messages = conv.Messages[excess:]
	}

	// 如果启用了持久化，保存对话
	if sm.persistenceEnabled && sm.persistenceStore != nil {
		err := sm.persistenceStore.SaveConversation(conv)
		if err != nil {
			sm.logger.Error("Failed to persist conversation",
				zap.String("conversationID", conversationID),
				zap.Error(err))
		}
	}

	return nil
}

// UpdateConversationState 更新对话状态
func (sm *StateManager) UpdateConversationState(conversationID string, state models.ConversationState) error {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	conv, exists := sm.conversations[conversationID]
	if !exists {
		return models.ErrConversationNotFound
	}

	conv.State = state
	// 如果对话已完成或已归档，更新相关时间戳
	if state == models.ConversationStateCompleted || state == models.ConversationStateArchived {
		conv.CompletedAt = time.Now()
	}

	// 如果启用了持久化，保存对话
	if sm.persistenceEnabled && sm.persistenceStore != nil {
		err := sm.persistenceStore.SaveConversation(conv)
		if err != nil {
			sm.logger.Error("Failed to persist conversation state",
				zap.String("conversationID", conversationID),
				zap.Error(err))
		}
	}

	return nil
}

// GetUserConversations 获取用户的所有对话
func (sm *StateManager) GetUserConversations(userID string) ([]*models.Conversation, error) {
	sm.mutex.RLock()
	conversationIDs, exists := sm.activeUsers[userID]
	sm.mutex.RUnlock()

	if !exists || len(conversationIDs) == 0 {
		// 如果启用了持久化，尝试从存储中加载
		if sm.persistenceEnabled && sm.persistenceStore != nil {
			ids, err := sm.persistenceStore.ListUserConversations(userID)
			if err != nil {
				return nil, err
			}
			conversationIDs = ids
		} else {
			return []*models.Conversation{}, nil
		}
	}

	result := make([]*models.Conversation, 0, len(conversationIDs))

	for _, id := range conversationIDs {
		sm.mutex.RLock()
		conv, exists := sm.conversations[id]
		sm.mutex.RUnlock()

		if exists {
			result = append(result, conv)
		} else if sm.persistenceEnabled && sm.persistenceStore != nil {
			// 尝试从存储中加载
			loadedConv, err := sm.persistenceStore.LoadConversation(id)
			if err == nil && loadedConv != nil {
				sm.mutex.Lock()
				sm.conversations[id] = loadedConv
				sm.mutex.Unlock()
				result = append(result, loadedConv)
			}
		}
	}

	return result, nil
}

// DeleteConversation 删除对话
func (sm *StateManager) DeleteConversation(conversationID string) error {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	conv, exists := sm.conversations[conversationID]
	if !exists {
		return models.ErrConversationNotFound
	}

	// 从用户的活跃对话列表中移除
	userID := conv.UserID
	convIDs, userExists := sm.activeUsers[userID]
	if userExists {
		newConvIDs := make([]string, 0, len(convIDs))
		for _, id := range convIDs {
			if id != conversationID {
				newConvIDs = append(newConvIDs, id)
			}
		}
		sm.activeUsers[userID] = newConvIDs
	}

	// 从内存中删除对话
	delete(sm.conversations, conversationID)

	// 如果启用了持久化，从存储中删除
	if sm.persistenceEnabled && sm.persistenceStore != nil {
		err := sm.persistenceStore.DeleteConversation(conversationID)
		if err != nil {
			sm.logger.Error("Failed to delete conversation from persistence store",
				zap.String("conversationID", conversationID),
				zap.Error(err))
			return err
		}
	}

	return nil
}

// UpdateConversationMetadata 更新对话元数据
func (sm *StateManager) UpdateConversationMetadata(conversationID string, metadata map[string]interface{}) error {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	conv, exists := sm.conversations[conversationID]
	if !exists {
		return models.ErrConversationNotFound
	}

	// 更新元数据
	for key, value := range metadata {
		conv.Metadata[key] = value
	}

	// 如果启用了持久化，保存对话
	if sm.persistenceEnabled && sm.persistenceStore != nil {
		err := sm.persistenceStore.SaveConversation(conv)
		if err != nil {
			sm.logger.Error("Failed to persist conversation metadata",
				zap.String("conversationID", conversationID),
				zap.Error(err))
		}
	}

	return nil
}

// GetConversationContext 获取对话上下文（最近的N条消息）
func (sm *StateManager) GetConversationContext(conversationID string, limit int) ([]models.Message, error) {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	conv, exists := sm.conversations[conversationID]
	if !exists {
		return nil, models.ErrConversationNotFound
	}

	messages := conv.Messages
	if limit > 0 && limit < len(messages) {
		// 返回最近的limit条消息
		return messages[len(messages)-limit:], nil
	}

	return messages, nil
}

// addToActiveUsers 将对话添加到用户的活跃对话列表
// 注意：调用此方法前必须持有写锁
func (sm *StateManager) addToActiveUsers(userID string, conversationID string) {
	convIDs, exists := sm.activeUsers[userID]
	if !exists {
		sm.activeUsers[userID] = []string{conversationID}
		return
	}

	// 检查是否已存在
	for _, id := range convIDs {
		if id == conversationID {
			return
		}
	}

	// 添加到列表
	sm.activeUsers[userID] = append(convIDs, conversationID)

	// 如果超过最大对话数，移除最旧的对话
	if sm.maxConversations > 0 && len(sm.activeUsers[userID]) > sm.maxConversations {
		excess := len(sm.activeUsers[userID]) - sm.maxConversations
		oldestIDs := sm.activeUsers[userID][:excess]
		sm.activeUsers[userID] = sm.activeUsers[userID][excess:]

		// 将最旧的对话标记为归档
		for _, oldID := range oldestIDs {
			if oldConv, exists := sm.conversations[oldID]; exists {
				oldConv.State = models.ConversationStateArchived
				oldConv.CompletedAt = time.Now()

				// 如果启用了持久化，保存对话
				if sm.persistenceEnabled && sm.persistenceStore != nil {
					err := sm.persistenceStore.SaveConversation(oldConv)
					if err != nil {
						sm.logger.Error("Failed to persist archived conversation",
							zap.String("conversationID", oldID),
							zap.Error(err))
					}
				}
			}
		}
	}
}

// startCleanupTask 启动定期清理过期对话的任务
func (sm *StateManager) startCleanupTask() {
	ticker := time.NewTicker(sm.cleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		sm.cleanupExpiredConversations()
	}
}

// cleanupExpiredConversations 清理过期的对话
func (sm *StateManager) cleanupExpiredConversations() {
	now := time.Now()
	expiredIDs := []string{}

	// 找出过期的对话
	sm.mutex.RLock()
	for id, conv := range sm.conversations {
		// 检查TTL
		if sm.conversationTTL > 0 {
			age := now.Sub(conv.CreatedAt)
			if age > sm.conversationTTL {
				expiredIDs = append(expiredIDs, id)
				continue
			}
		}

		// 检查空闲时间
		if sm.maxIdleTime > 0 {
			idleTime := now.Sub(conv.LastActiveTime)
			if idleTime > sm.maxIdleTime {
				expiredIDs = append(expiredIDs, id)
				continue
			}
		}

		// 检查已完成或已归档的对话
		if conv.State == models.ConversationStateCompleted || conv.State == models.ConversationStateArchived {
			if conv.CompletedAt.Add(24 * time.Hour).Before(now) { // 已完成/归档超过24小时
				expiredIDs = append(expiredIDs, id)
			}
		}
	}
	sm.mutex.RUnlock()

	// 删除过期的对话
	for _, id := range expiredIDs {
		sm.DeleteConversation(id)
		sm.logger.Debug("Cleaned up expired conversation", zap.String("conversationID", id))
	}
}

// GetStats 获取状态管理器的统计信息
func (sm *StateManager) GetStats() map[string]interface{} {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	stats := make(map[string]interface{})
	stats["total_conversations"] = len(sm.conversations)
	stats["total_users"] = len(sm.activeUsers)

	// 统计不同状态的对话数量
	stateCount := make(map[string]int)
	for _, conv := range sm.conversations {
		stateCount[string(conv.State)]++
	}
	stats["conversation_states"] = stateCount

	return stats
}
