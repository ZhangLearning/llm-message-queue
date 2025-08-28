package priorityqueue

import (
	"sync"

	"llm-message-queue/pkg/config"
	"llm-message-queue/pkg/models"

	"go.uber.org/zap"
)

// QueueType 队列类型
type QueueType string

const (
	StandardQueueType   QueueType = "standard"
	DelayedQueueType    QueueType = "delayed"
	DeadLetterQueueType QueueType = "dead_letter"
	PriorityQueueType   QueueType = "priority"
)

// QueueFactory 队列工厂，负责创建和管理不同类型的队列
type QueueFactory struct {
	managers map[string]*QueueManager
	workers  map[string][]*Worker
	config   *config.QueueConfig
	logger   *zap.Logger
	mu       sync.RWMutex
}

// NewQueueFactory 创建新的队列工厂
func NewQueueFactory(config *config.QueueConfig, logger *zap.Logger) *QueueFactory {
	return &QueueFactory{
		managers: make(map[string]*QueueManager),
		workers:  make(map[string][]*Worker),
		config:   config,
		logger:   logger,
	}
}

// CreateQueueManager 创建队列管理器
func (qf *QueueFactory) CreateQueueManager(name string, queueType QueueType) *QueueManager {
	qf.mu.Lock()
	defer qf.mu.Unlock()

	// 检查是否已存在
	if manager, exists := qf.managers[name]; exists {
		return manager
	}

	// 根据队列类型创建不同配置的队列管理器
	config := qf.createManagerConfig(queueType)

	// 创建队列管理器
	manager := NewQueueManager(config, qf.logger.With(
		zap.String("component", "queue_manager"),
		zap.String("queue", name),
		zap.String("type", string(queueType)),
	))

	// 添加到管理器映射
	qf.managers[name] = manager

	// 启动队列管理器
	manager.Start()

	qf.logger.Info("Created queue manager",
		zap.String("name", name),
		zap.String("type", string(queueType)),
	)

	return manager
}

// GetQueueManager 获取队列管理器
func (qf *QueueFactory) GetQueueManager(name string) (*QueueManager, bool) {
	qf.mu.RLock()
	defer qf.mu.RUnlock()

	manager, exists := qf.managers[name]
	return manager, exists
}

// CreateWorkers 为指定队列创建工作器
func (qf *QueueFactory) CreateWorkers(queueName string, count int, processFunc ProcessFunc) []*Worker {
	qf.mu.Lock()
	defer qf.mu.Unlock()

	// 获取队列管理器
	manager, exists := qf.managers[queueName]
	if !exists {
		qf.logger.Error("Cannot create workers for non-existent queue", zap.String("queue", queueName))
		return nil
	}

	// 创建工作器
	workers := make([]*Worker, count)
	for i := 0; i < count; i++ {
		workerConfig := &WorkerConfig{
			ID:              qf.generateWorkerID(queueName, i),
			QueueName:       queueName,
			MaxBatchSize:    qf.config.Worker.MaxBatchSize,
			ProcessInterval: qf.config.Worker.ProcessInterval,
			MaxConcurrent:   qf.config.Worker.MaxConcurrent,
			BackoffStrategy: NewExponentialBackoff(
				qf.config.Retry.InitialBackoff,
				qf.config.Retry.MaxBackoff,
				qf.config.Retry.Factor,
				qf.config.Retry.MaxRetries,
			),
		}

		worker := NewWorker(workerConfig, manager, processFunc, qf.logger.With(
			zap.String("component", "worker"),
			zap.String("queue", queueName),
		))

		workers[i] = worker

		// 启动工作器
		worker.Start()
	}

	// 保存工作器引用
	qf.workers[queueName] = append(qf.workers[queueName], workers...)

	qf.logger.Info("Created workers for queue",
		zap.String("queue", queueName),
		zap.Int("count", count),
	)

	return workers
}

// StopAll 停止所有队列管理器和工作器
func (qf *QueueFactory) StopAll() {
	qf.mu.Lock()
	defer qf.mu.Unlock()

	// 停止所有工作器
	for queueName, workers := range qf.workers {
		for _, worker := range workers {
			worker.Stop()
		}
		qf.logger.Info("Stopped all workers", zap.String("queue", queueName))
	}

	// 停止所有队列管理器
	for name, manager := range qf.managers {
		manager.Stop()
		qf.logger.Info("Stopped queue manager", zap.String("name", name))
	}

	// 清空映射
	qf.workers = make(map[string][]*Worker)
	qf.managers = make(map[string]*QueueManager)
}

// GetWorkerStats 获取所有工作器的统计信息
func (qf *QueueFactory) GetWorkerStats() map[string][]WorkerMetrics {
	qf.mu.RLock()
	defer qf.mu.RUnlock()

	result := make(map[string][]WorkerMetrics)

	for queueName, workers := range qf.workers {
		queueStats := make([]WorkerMetrics, len(workers))

		for i, worker := range workers {
			queueStats[i] = *worker.GetMetrics()
		}

		result[queueName] = queueStats
	}

	return result
}

// 根据队列类型创建管理器配置
func (qf *QueueFactory) createManagerConfig(queueType QueueType) *QueueManagerConfig {
	baseConfig := &QueueManagerConfig{
		DefaultMaxSize:     qf.config.DefaultMaxSize,
		MonitorInterval:    qf.config.MonitorInterval,
		CleanupInterval:    qf.config.CleanupInterval,
		MaxRetentionPeriod: qf.config.MaxRetentionPeriod,
		EnableMetrics:      qf.config.EnableMetrics,
		EnableAutoScaling:  qf.config.EnableAutoScaling,
		ScalingThresholds:  qf.config.ScalingThresholds,
	}

	// 根据队列类型添加特定配置
	switch queueType {
	case DelayedQueueType:
		// 延迟队列特定配置
		// ...

	case DeadLetterQueueType:
		// 死信队列特定配置
		// ...

	case PriorityQueueType:
		// 优先级队列特定配置
		baseConfig.PriorityAdjustRules = qf.createPriorityRules()
	}

	return baseConfig
}

// 创建优先级调整规则
func (qf *QueueFactory) createPriorityRules() []PriorityAdjustRule {
	// 这里可以从配置中加载规则
	// 示例规则
	return []PriorityAdjustRule{
		{
			Condition: func(msg *models.Message) bool {
				// 检查消息元数据中是否有VIP标记
				_, isVIP := msg.Metadata["vip_user"]
				return isVIP
			},
			NewPriority: models.PriorityHigh,
			Description: "VIP用户消息提升优先级",
		},
		{
			Condition: func(msg *models.Message) bool {
				// 检查消息内容长度是否超过阈值
				return len(msg.Content) > 10000
			},
			NewPriority: models.PriorityLow,
			Description: "超长消息降低优先级",
		},
	}
}

// 生成工作器ID
func (qf *QueueFactory) generateWorkerID(queueName string, index int) string {
	return queueName + "-worker-" + string(index)
}
