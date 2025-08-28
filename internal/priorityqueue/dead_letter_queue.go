package priorityqueue

import (
	"sync"
	"time"

	"llm-message-queue/pkg/models"

	"go.uber.org/zap"
)

// DeadLetterItem 死信队列项
type DeadLetterItem struct {
	Message     *models.Message
	FailReason  string
	FailedAt    time.Time
	SourceQueue string
	RetryCount  int
}

// DeadLetterQueue 死信队列，用于存储处理失败的消息
type DeadLetterQueue struct {
	items       []*DeadLetterItem
	mu          sync.RWMutex
	maxSize     int
	logger      *zap.Logger
	handlers    []DeadLetterHandler
	notifyChans []chan<- *DeadLetterItem
}

// DeadLetterHandler 死信处理器函数类型
type DeadLetterHandler func(item *DeadLetterItem) error

// NewDeadLetterQueue 创建新的死信队列
func NewDeadLetterQueue(maxSize int, logger *zap.Logger) *DeadLetterQueue {
	return &DeadLetterQueue{
		items:       make([]*DeadLetterItem, 0),
		maxSize:     maxSize,
		logger:      logger,
		handlers:    make([]DeadLetterHandler, 0),
		notifyChans: make([]chan<- *DeadLetterItem, 0),
	}
}

// AddHandler 添加死信处理器
func (dlq *DeadLetterQueue) AddHandler(handler DeadLetterHandler) {
	dlq.mu.Lock()
	defer dlq.mu.Unlock()

	dlq.handlers = append(dlq.handlers, handler)
}

// AddNotificationChannel 添加通知通道
func (dlq *DeadLetterQueue) AddNotificationChannel(ch chan<- *DeadLetterItem) {
	dlq.mu.Lock()
	defer dlq.mu.Unlock()

	dlq.notifyChans = append(dlq.notifyChans, ch)
}

// Push 将消息推送到死信队列
func (dlq *DeadLetterQueue) Push(message *models.Message, failReason string, sourceQueue string) error {
	dlq.mu.Lock()
	defer dlq.mu.Unlock()

	// 检查队列是否已满
	if dlq.maxSize > 0 && len(dlq.items) >= dlq.maxSize {
		return ErrQueueFull
	}

	// 创建死信项
	item := &DeadLetterItem{
		Message:     message,
		FailReason:  failReason,
		FailedAt:    time.Now(),
		SourceQueue: sourceQueue,
		RetryCount:  message.RetryCount,
	}

	// 添加到队列
	dlq.items = append(dlq.items, item)

	dlq.logger.Warn("Message added to dead letter queue",
		zap.String("messageID", message.ID),
		zap.String("sourceQueue", sourceQueue),
		zap.String("failReason", failReason),
		zap.Int("retryCount", message.RetryCount),
	)

	// 调用所有处理器
	for _, handler := range dlq.handlers {
		go func(h DeadLetterHandler, i *DeadLetterItem) {
			err := h(i)
			if err != nil {
				dlq.logger.Error("Dead letter handler failed",
					zap.String("messageID", i.Message.ID),
					zap.Error(err),
				)
			}
		}(handler, item)
	}

	// 发送通知
	for _, ch := range dlq.notifyChans {
		go func(c chan<- *DeadLetterItem, i *DeadLetterItem) {
			select {
			case c <- i:
				// 成功发送
			default:
				// 通道已满或关闭，忽略
				dlq.logger.Warn("Failed to send notification, channel full or closed",
					zap.String("messageID", i.Message.ID),
				)
			}
		}(ch, item)
	}

	return nil
}

// Get 获取指定索引的死信项
func (dlq *DeadLetterQueue) Get(index int) (*DeadLetterItem, error) {
	dlq.mu.RLock()
	defer dlq.mu.RUnlock()

	if index < 0 || index >= len(dlq.items) {
		return nil, ErrIndexOutOfRange
	}

	return dlq.items[index], nil
}

// Remove 移除指定索引的死信项
func (dlq *DeadLetterQueue) Remove(index int) error {
	dlq.mu.Lock()
	defer dlq.mu.Unlock()

	if index < 0 || index >= len(dlq.items) {
		return ErrIndexOutOfRange
	}

	// 记录被移除的消息ID
	messageID := dlq.items[index].Message.ID

	// 移除项目
	dlq.items = append(dlq.items[:index], dlq.items[index+1:]...)

	dlq.logger.Info("Removed message from dead letter queue",
		zap.String("messageID", messageID),
		zap.Int("index", index),
	)

	return nil
}

// Size 获取队列大小
func (dlq *DeadLetterQueue) Size() int {
	dlq.mu.RLock()
	defer dlq.mu.RUnlock()

	return len(dlq.items)
}

// GetAll 获取所有死信项
func (dlq *DeadLetterQueue) GetAll() []*DeadLetterItem {
	dlq.mu.RLock()
	defer dlq.mu.RUnlock()

	// 创建副本以避免并发问题
	result := make([]*DeadLetterItem, len(dlq.items))
	copy(result, dlq.items)

	return result
}

// Clear 清空队列
func (dlq *DeadLetterQueue) Clear() {
	dlq.mu.Lock()
	defer dlq.mu.Unlock()

	dlq.items = make([]*DeadLetterItem, 0)

	dlq.logger.Info("Cleared dead letter queue")
}

// Requeue 将死信项重新入队到原始队列
func (dlq *DeadLetterQueue) Requeue(index int, queueManager *QueueManager) error {
	dlq.mu.Lock()
	defer dlq.mu.Unlock()

	if index < 0 || index >= len(dlq.items) {
		return ErrIndexOutOfRange
	}

	item := dlq.items[index]

	// 重置重试计数
	item.Message.RetryCount = 0

	// 重新入队
	err := queueManager.PushMessage(item.SourceQueue, item.Message)
	if err != nil {
		return err
	}

	// 从死信队列中移除
	dlq.items = append(dlq.items[:index], dlq.items[index+1:]...)

	dlq.logger.Info("Requeued message from dead letter queue",
		zap.String("messageID", item.Message.ID),
		zap.String("sourceQueue", item.SourceQueue),
	)

	return nil
}

// BatchRequeue 批量将死信项重新入队
func (dlq *DeadLetterQueue) BatchRequeue(indices []int, queueManager *QueueManager) (int, error) {
	if len(indices) == 0 {
		return 0, nil
	}

	successCount := 0

	// 对索引进行排序（从大到小），以便从后向前删除，避免索引变化
	sortIndices(indices)

	dlq.mu.Lock()
	defer dlq.mu.Unlock()

	for _, index := range indices {
		if index < 0 || index >= len(dlq.items) {
			continue
		}

		item := dlq.items[index]

		// 重置重试计数
		item.Message.RetryCount = 0

		// 重新入队
		err := queueManager.PushMessage(item.SourceQueue, item.Message)
		if err != nil {
			continue
		}

		// 从死信队列中移除
		dlq.items = append(dlq.items[:index], dlq.items[index+1:]...)
		successCount++

		dlq.logger.Info("Requeued message from dead letter queue",
			zap.String("messageID", item.Message.ID),
			zap.String("sourceQueue", item.SourceQueue),
		)
	}

	return successCount, nil
}

// 对索引进行排序（从大到小）
func sortIndices(indices []int) {
	// 简单的冒泡排序
	for i := 0; i < len(indices); i++ {
		for j := i + 1; j < len(indices); j++ {
			if indices[i] < indices[j] {
				indices[i], indices[j] = indices[j], indices[i]
			}
		}
	}
}

// 自定义错误
var (
	ErrIndexOutOfRange = &QueueError{Code: "INDEX_OUT_OF_RANGE", Message: "index out of range"}
)
