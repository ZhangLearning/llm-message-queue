package priorityqueue

import (
	"context"
	"sync"
	"time"

	"llm-message-queue/pkg/models"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

// QueueManager 管理多个多级队列，提供高级队列操作和监控功能
type QueueManager struct {
	queues      map[string]*MultiLevelQueue
	mu          sync.RWMutex
	config      *QueueManagerConfig
	metrics     *QueueMetrics
	logger      *zap.Logger
	ctx         context.Context
	cancel      context.CancelFunc
	monitorDone chan struct{}
}

// QueueManagerConfig 队列管理器配置
type QueueManagerConfig struct {
	DefaultMaxSize      int
	MonitorInterval     time.Duration
	CleanupInterval     time.Duration
	MaxRetentionPeriod  time.Duration
	EnableMetrics       bool
	EnableAutoScaling   bool
	ScalingThresholds   map[string]int // 队列名称 -> 阈值
	PriorityAdjustRules []PriorityAdjustRule
}

// PriorityAdjustRule 优先级调整规则
type PriorityAdjustRule struct {
	Condition   func(msg *models.Message) bool
	NewPriority models.Priority
	Description string
}

// QueueMetrics Prometheus指标收集
type QueueMetrics struct {
	PendingMessages    *prometheus.GaugeVec
	ProcessingMessages *prometheus.GaugeVec
	CompletedMessages  *prometheus.CounterVec
	FailedMessages     *prometheus.CounterVec
	MessageWaitTime    *prometheus.HistogramVec
	MessageProcessTime *prometheus.HistogramVec
	QueueOperations    *prometheus.CounterVec
}

// NewQueueManager 创建新的队列管理器
func NewQueueManager(config *QueueManagerConfig, logger *zap.Logger) *QueueManager {
	ctx, cancel := context.WithCancel(context.Background())

	manager := &QueueManager{
		queues:      make(map[string]*MultiLevelQueue),
		config:      config,
		logger:      logger,
		ctx:         ctx,
		cancel:      cancel,
		monitorDone: make(chan struct{}),
	}

	if config.EnableMetrics {
		manager.initMetrics()
	}

	return manager
}

// 初始化Prometheus指标
func (qm *QueueManager) initMetrics() {
	qm.metrics = &QueueMetrics{
		PendingMessages: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "llm_queue",
				Subsystem: "messages",
				Name:      "pending_total",
				Help:      "Number of pending messages in queue",
			},
			[]string{"queue", "priority"},
		),
		ProcessingMessages: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "llm_queue",
				Subsystem: "messages",
				Name:      "processing_total",
				Help:      "Number of processing messages in queue",
			},
			[]string{"queue", "priority"},
		),
		CompletedMessages: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "llm_queue",
				Subsystem: "messages",
				Name:      "completed_total",
				Help:      "Total number of completed messages",
			},
			[]string{"queue", "priority"},
		),
		FailedMessages: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "llm_queue",
				Subsystem: "messages",
				Name:      "failed_total",
				Help:      "Total number of failed messages",
			},
			[]string{"queue", "priority"},
		),
		MessageWaitTime: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "llm_queue",
				Subsystem: "messages",
				Name:      "wait_time_seconds",
				Help:      "Time messages spend waiting in queue",
				Buckets:   prometheus.ExponentialBuckets(0.01, 2, 15),
			},
			[]string{"queue", "priority"},
		),
		MessageProcessTime: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "llm_queue",
				Subsystem: "messages",
				Name:      "process_time_seconds",
				Help:      "Time spent processing messages",
				Buckets:   prometheus.ExponentialBuckets(0.1, 2, 15),
			},
			[]string{"queue", "priority"},
		),
		QueueOperations: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "llm_queue",
				Subsystem: "operations",
				Name:      "total",
				Help:      "Total number of queue operations",
			},
			[]string{"queue", "operation"},
		),
	}

	// 注册所有指标
	prometheus.MustRegister(
		qm.metrics.PendingMessages,
		qm.metrics.ProcessingMessages,
		qm.metrics.CompletedMessages,
		qm.metrics.FailedMessages,
		qm.metrics.MessageWaitTime,
		qm.metrics.MessageProcessTime,
		qm.metrics.QueueOperations,
	)
}

// Start 启动队列管理器
func (qm *QueueManager) Start() {
	go qm.monitorQueues()
}

// Stop 停止队列管理器
func (qm *QueueManager) Stop() {
	qm.cancel()
	<-qm.monitorDone
}

// CreateQueue 创建新队列
func (qm *QueueManager) CreateQueue(name string, maxSize int) {
	qm.mu.Lock()
	defer qm.mu.Unlock()

	if _, exists := qm.queues[name]; !exists {
		if maxSize <= 0 {
			maxSize = qm.config.DefaultMaxSize
		}

		mlq := NewMultiLevelQueue(maxSize)
		mlq.AddQueue(name) // 添加队列到MultiLevelQueue
		qm.queues[name] = mlq
		qm.logger.Info("Created new queue", zap.String("queue", name), zap.Int("maxSize", maxSize))

		if qm.config.EnableMetrics {
			qm.metrics.QueueOperations.WithLabelValues(name, "create").Inc()
		}
	}
}

// DeleteQueue 删除队列
func (qm *QueueManager) DeleteQueue(name string) error {
	qm.mu.Lock()
	defer qm.mu.Unlock()

	if _, exists := qm.queues[name]; !exists {
		return ErrQueueNotFound
	}

	delete(qm.queues, name)
	qm.logger.Info("Deleted queue", zap.String("queue", name))

	if qm.config.EnableMetrics {
		qm.metrics.QueueOperations.WithLabelValues(name, "delete").Inc()
	}

	return nil
}

// PushMessage 将消息推送到指定队列
func (qm *QueueManager) PushMessage(queueName string, message *models.Message) error {
	qm.mu.RLock()
	queue, exists := qm.queues[queueName]
	qm.mu.RUnlock()

	if !exists {
		return ErrQueueNotFound
	}

	// 应用优先级调整规则
	qm.applyPriorityRules(message)

	// 推送消息到队列
	err := queue.Push(queueName, message, int(message.Priority))
	if err != nil {
		return err
	}

	qm.logger.Debug("Pushed message to queue",
		zap.String("queue", queueName),
		zap.String("messageID", message.ID),
		zap.Int("priority", int(message.Priority)),
	)

	if qm.config.EnableMetrics {
		qm.metrics.QueueOperations.WithLabelValues(queueName, "push").Inc()
		qm.metrics.PendingMessages.WithLabelValues(
			queueName,
			message.Priority.String(),
		).Inc()
	}

	return nil
}

// BatchPushMessages 批量推送消息到队列
func (qm *QueueManager) BatchPushMessages(queueName string, messages []*models.Message) (int, error) {
	qm.mu.RLock()
	queue, exists := qm.queues[queueName]
	qm.mu.RUnlock()

	if !exists {
		return 0, ErrQueueNotFound
	}

	successCount := 0
	for _, msg := range messages {
		// 应用优先级调整规则
		qm.applyPriorityRules(msg)

		// 推送消息到队列
		err := queue.Push(queueName, msg, int(msg.Priority))
		if err != nil {
			continue
		}

		successCount++

		if qm.config.EnableMetrics {
			qm.metrics.PendingMessages.WithLabelValues(
				queueName,
				msg.Priority.String(),
			).Inc()
		}
	}

	qm.logger.Info("Batch pushed messages to queue",
		zap.String("queue", queueName),
		zap.Int("total", len(messages)),
		zap.Int("success", successCount),
	)

	if qm.config.EnableMetrics {
		qm.metrics.QueueOperations.WithLabelValues(queueName, "batch_push").Inc()
	}

	return successCount, nil
}

// PopMessage 从队列中弹出最高优先级的消息
func (qm *QueueManager) PopMessage(queueName string) (*models.Message, error) {
	qm.mu.RLock()
	queue, exists := qm.queues[queueName]
	qm.mu.RUnlock()

	if !exists {
		return nil, ErrQueueNotFound
	}

	message, err := queue.Pop(queueName)
	if err != nil {
		return nil, err
	}

	qm.logger.Debug("Popped message from queue",
		zap.String("queue", queueName),
		zap.String("messageID", message.ID),
		zap.Int("priority", int(message.Priority)),
	)

	if qm.config.EnableMetrics {
		qm.metrics.QueueOperations.WithLabelValues(queueName, "pop").Inc()
		qm.metrics.PendingMessages.WithLabelValues(
			queueName,
			message.Priority.String(),
		).Dec()
		qm.metrics.ProcessingMessages.WithLabelValues(
			queueName,
			message.Priority.String(),
		).Inc()
	}

	return message, nil
}

// BatchPopMessages 批量弹出消息
func (qm *QueueManager) BatchPopMessages(queueName string, count int) ([]*models.Message, error) {
	qm.mu.RLock()
	queue, exists := qm.queues[queueName]
	qm.mu.RUnlock()

	if !exists {
		return nil, ErrQueueNotFound
	}

	result := make([]*models.Message, 0, count)
	for i := 0; i < count; i++ {
		message, err := queue.Pop(queueName)
		if err != nil {
			break
		}

		result = append(result, message)

		if qm.config.EnableMetrics {
			qm.metrics.PendingMessages.WithLabelValues(
				queueName,
				message.Priority.String(),
			).Dec()
			qm.metrics.ProcessingMessages.WithLabelValues(
				queueName,
				message.Priority.String(),
			).Inc()
		}
	}

	qm.logger.Debug("Batch popped messages from queue",
		zap.String("queue", queueName),
		zap.Int("requested", count),
		zap.Int("actual", len(result)),
	)

	if qm.config.EnableMetrics {
		qm.metrics.QueueOperations.WithLabelValues(queueName, "batch_pop").Inc()
	}

	return result, nil
}

// CompleteMessage 标记消息处理完成
func (qm *QueueManager) CompleteMessage(queueName string, messageID string, processingTime time.Duration) {
	qm.mu.RLock()
	queue, exists := qm.queues[queueName]
	qm.mu.RUnlock()

	if !exists {
		return
	}

	queue.CompleteMessage(queueName)

	qm.logger.Debug("Completed message",
		zap.String("queue", queueName),
		zap.String("messageID", messageID),
		zap.Duration("processingTime", processingTime),
	)

	if qm.config.EnableMetrics {
		// 由于我们没有保存消息的优先级信息，这里使用"unknown"作为标签
		// 在实际实现中，可以通过状态管理器获取消息的完整信息
		qm.metrics.ProcessingMessages.WithLabelValues(queueName, "unknown").Dec()
		qm.metrics.CompletedMessages.WithLabelValues(queueName, "unknown").Inc()
		qm.metrics.MessageProcessTime.WithLabelValues(queueName, "unknown").Observe(processingTime.Seconds())
	}
}

// FailMessage 标记消息处理失败
func (qm *QueueManager) FailMessage(queueName string, messageID string, err error) {
	qm.mu.RLock()
	queue, exists := qm.queues[queueName]
	qm.mu.RUnlock()

	if !exists {
		return
	}

	queue.FailMessage(queueName)

	qm.logger.Warn("Failed message",
		zap.String("queue", queueName),
		zap.String("messageID", messageID),
		zap.Error(err),
	)

	if qm.config.EnableMetrics {
		// 同上，使用"unknown"作为标签
		qm.metrics.ProcessingMessages.WithLabelValues(queueName, "unknown").Dec()
		qm.metrics.FailedMessages.WithLabelValues(queueName, "unknown").Inc()
	}
}

// GetQueueStats 获取队列统计信息
func (qm *QueueManager) GetQueueStats(queueName string) (*QueueStats, error) {
	qm.mu.RLock()
	queue, exists := qm.queues[queueName]
	qm.mu.RUnlock()

	if !exists {
		return nil, ErrQueueNotFound
	}

	return queue.GetStats(queueName)
}

// GetAllQueueStats 获取所有队列的统计信息
func (qm *QueueManager) GetAllQueueStats() map[string]*QueueStats {
	result := make(map[string]*QueueStats)

	qm.mu.RLock()
	for name, queue := range qm.queues {
		stats, err := queue.GetStats(name)
		if err == nil {
			result[name] = stats
		}
	}
	qm.mu.RUnlock()

	return result
}

// 应用优先级调整规则
func (qm *QueueManager) applyPriorityRules(message *models.Message) {
	for _, rule := range qm.config.PriorityAdjustRules {
		if rule.Condition(message) {
			oldPriority := message.Priority
			message.Priority = rule.NewPriority

			qm.logger.Debug("Adjusted message priority",
				zap.String("messageID", message.ID),
				zap.Int("oldPriority", int(oldPriority)),
				zap.Int("newPriority", int(message.Priority)),
				zap.String("rule", rule.Description),
			)
			break
		}
	}
}

// 监控队列状态
func (qm *QueueManager) monitorQueues() {
	defer close(qm.monitorDone)

	monitorTicker := time.NewTicker(qm.config.MonitorInterval)
	cleanupTicker := time.NewTicker(qm.config.CleanupInterval)

	defer monitorTicker.Stop()
	defer cleanupTicker.Stop()

	for {
		select {
		case <-qm.ctx.Done():
			return

		case <-monitorTicker.C:
			qm.updateQueueMetrics()

			// 如果启用了自动扩缩容，检查队列长度并调整资源
			if qm.config.EnableAutoScaling {
				qm.checkQueueThresholds()
			}

		case <-cleanupTicker.C:
			// 清理长时间未处理的消息
			qm.cleanupStaleMessages()
		}
	}
}

// 更新队列指标
func (qm *QueueManager) updateQueueMetrics() {
	if !qm.config.EnableMetrics {
		return
	}

	qm.mu.RLock()
	defer qm.mu.RUnlock()

	for name, queue := range qm.queues {
		stats, err := queue.GetStats(name)
		if err != nil {
			continue
		}

		// 这里我们使用"all"作为优先级标签，因为QueueStats没有按优先级细分
		// 在实际实现中，可以扩展QueueStats以按优先级细分统计信息
		qm.metrics.PendingMessages.WithLabelValues(name, "all").Set(float64(stats.PendingCount))
		qm.metrics.ProcessingMessages.WithLabelValues(name, "all").Set(float64(stats.ProcessingCount))
	}
}

// 检查队列阈值并触发扩缩容
func (qm *QueueManager) checkQueueThresholds() {
	qm.mu.RLock()
	defer qm.mu.RUnlock()

	for name, queue := range qm.queues {
		threshold, exists := qm.config.ScalingThresholds[name]
		if !exists {
			continue
		}

		size, err := queue.Size(name)
		if err != nil {
			continue
		}

		if size > threshold {
			// 队列长度超过阈值，可以触发扩容
			// 这里只是记录日志，实际实现中可以调用外部系统进行扩容
			qm.logger.Info("Queue threshold exceeded, scaling up recommended",
				zap.String("queue", name),
				zap.Int("size", size),
				zap.Int("threshold", threshold),
			)
		}
	}
}

// 清理长时间未处理的消息
func (qm *QueueManager) cleanupStaleMessages() {
	// 实际实现中，这里可以与状态管理器配合，清理长时间未处理的消息
	// 例如，将超时的消息标记为失败，或者重新入队
	qm.logger.Debug("Cleaning up stale messages")
}
