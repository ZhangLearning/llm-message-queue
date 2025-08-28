package models

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// 错误定义
var (
	ErrConversationNotFound = errors.New("conversation not found")
)

type Priority int

const (
	PriorityRealtime Priority = iota + 1
	PriorityHigh
	PriorityNormal
	PriorityLow
)

func (p Priority) String() string {
	switch p {
	case PriorityRealtime:
		return "realtime"
	case PriorityHigh:
		return "high"
	case PriorityNormal:
		return "normal"
	case PriorityLow:
		return "low"
	default:
		return "unknown"
	}
}

type MessageStatus string

const (
	StatusPending    MessageStatus = "pending"
	StatusProcessing MessageStatus = "processing"
	StatusCompleted  MessageStatus = "completed"
	StatusFailed     MessageStatus = "failed"
	StatusTimeout    MessageStatus = "timeout"
)

type ConversationState string

const (
	ConversationStateActive    ConversationState = "active"
	ConversationStateInactive  ConversationState = "inactive"
	ConversationStateCompleted ConversationState = "completed"
	ConversationStateArchived  ConversationState = "archived"
)

type Message struct {
	ID           string        `json:"id" gorm:"primaryKey"`
	ConversationID string      `json:"conversation_id" gorm:"index"`
	UserID       string        `json:"user_id" gorm:"index"`
	Content      string        `json:"content"`
	Priority     Priority      `json:"priority" gorm:"index"`
	Status       MessageStatus `json:"status" gorm:"index"`
	QueueName    string        `json:"queue_name"`
	RetryCount   int           `json:"retry_count"`
	MaxRetries   int           `json:"max_retries"`
	Timeout      time.Duration `json:"timeout"`
	CreatedAt    time.Time     `json:"created_at"`
	UpdatedAt    time.Time     `json:"updated_at"`
	ScheduledAt  *time.Time    `json:"scheduled_at"`
	CompletedAt  *time.Time    `json:"completed_at"`
	Metadata     map[string]interface{} `json:"metadata" gorm:"serializer:json"`
}

func NewMessage(conversationID, userID, content string, priority Priority) *Message {
	return &Message{
		ID:           uuid.New().String(),
		ConversationID: conversationID,
		UserID:       userID,
		Content:      content,
		Priority:     priority,
		Status:       StatusPending,
		RetryCount:   0,
		MaxRetries:   3,
		Timeout:      30 * time.Second,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		Metadata:     make(map[string]interface{}),
	}
}

type Conversation struct {
	ID             string            `json:"id" gorm:"primaryKey"`
	UserID         string            `json:"user_id" gorm:"index"`
	Title          string            `json:"title"`
	Context        string            `json:"context" gorm:"type:text"`
	Status         string            `json:"status" gorm:"index"`
	State          ConversationState `json:"state" gorm:"index"`
	Priority       Priority          `json:"priority" gorm:"index"`
	MessageCount   int               `json:"message_count"`
	LastActivity   time.Time         `json:"last_activity" gorm:"index"`
	LastActiveTime time.Time         `json:"last_active_time" gorm:"index"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
	CompletedAt    time.Time         `json:"completed_at"`
	Messages       []Message         `json:"messages" gorm:"-"`
	Metadata       map[string]interface{} `json:"metadata" gorm:"serializer:json"`
}

type QueueStats struct {
	QueueName    string    `json:"queue_name"`
	Priority     Priority  `json:"priority"`
	PendingCount int       `json:"pending_count"`
	ProcessingCount int    `json:"processing_count"`
	CompletedCount int     `json:"completed_count"`
	FailedCount  int       `json:"failed_count"`
	AvgWaitTime  time.Duration `json:"avg_wait_time"`
	AvgProcessTime time.Duration `json:"avg_process_time"`
	UpdatedAt    time.Time `json:"updated_at"`
}
