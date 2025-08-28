package priorityqueue

import (
	"context"
	"sync"
	"time"

	"llm-message-queue/pkg/models"

	"go.uber.org/zap"
)

// Worker 队列工作器，负责从队列中获取消息并处理
type Worker struct {
	ID              string
	queueManager    *QueueManager
	queueName       string
	maxBatchSize    int
	processInterval time.Duration
	maxConcurrent   int
	processFunc     ProcessFunc
	backoffStrategy BackoffStrategy
	logger          *zap.Logger
	ctx             context.Context
	cancel          context.CancelFunc
	wg              sync.WaitGroup
	semaphore       chan struct{}
	metrics         *WorkerMetrics
	metricsMutex    sync.Mutex // 保护metrics字段的互斥锁
}

// ProcessFunc 消息处理函数类型
type ProcessFunc func(ctx context.Context, message *models.Message) error

// BackoffStrategy 重试退避策略接口
type BackoffStrategy interface {
	NextBackoff(retryCount int) time.Duration
	MaxRetries() int
}

// WorkerMetrics 工作器指标
type WorkerMetrics struct {
	ProcessedCount   int64
	SuccessCount     int64
	FailureCount     int64
	RetryCount       int64
	TotalProcessTime time.Duration
	LastProcessTime  time.Duration
}

// WorkerConfig 工作器配置
type WorkerConfig struct {
	ID              string
	QueueName       string
	MaxBatchSize    int
	ProcessInterval time.Duration
	MaxConcurrent   int
	BackoffStrategy BackoffStrategy
}

// NewWorker 创建新的队列工作器
func NewWorker(config *WorkerConfig, queueManager *QueueManager, processFunc ProcessFunc, logger *zap.Logger) *Worker {
	ctx, cancel := context.WithCancel(context.Background())

	return &Worker{
		ID:              config.ID,
		queueManager:    queueManager,
		queueName:       config.QueueName,
		maxBatchSize:    config.MaxBatchSize,
		processInterval: config.ProcessInterval,
		maxConcurrent:   config.MaxConcurrent,
		processFunc:     processFunc,
		backoffStrategy: config.BackoffStrategy,
		logger:          logger.With(zap.String("workerID", config.ID)),
		ctx:             ctx,
		cancel:          cancel,
		semaphore:       make(chan struct{}, config.MaxConcurrent),
		metrics:         &WorkerMetrics{},
	}
}

// Start 启动工作器
func (w *Worker) Start() {
	w.logger.Info("Starting worker",
		zap.String("queue", w.queueName),
		zap.Int("maxConcurrent", w.maxConcurrent),
		zap.Int("maxBatchSize", w.maxBatchSize),
	)

	w.wg.Add(1)
	go w.processLoop()
}

// Stop 停止工作器
func (w *Worker) Stop() {
	w.logger.Info("Stopping worker")
	w.cancel()
	w.wg.Wait()
}

// GetMetrics 获取工作器指标
func (w *Worker) GetMetrics() *WorkerMetrics {
	w.metricsMutex.Lock()
	defer w.metricsMutex.Unlock()
	return w.metrics
}

// 主处理循环
func (w *Worker) processLoop() {
	defer w.wg.Done()

	ticker := time.NewTicker(w.processInterval)
	defer ticker.Stop()

	for {
		select {
		case <-w.ctx.Done():
			w.logger.Info("Worker process loop stopped")
			return

		case <-ticker.C:
			w.processBatch()
		}
	}
}

// 批量处理消息
func (w *Worker) processBatch() {
	// 尝试从队列中获取一批消息
	messages, err := w.queueManager.BatchPopMessages(w.queueName, w.maxBatchSize)
	if err != nil {
		if err != ErrQueueEmpty {
			w.logger.Error("Failed to pop messages from queue", zap.Error(err))
		}
		return
	}

	if len(messages) == 0 {
		return
	}

	w.logger.Debug("Processing message batch", zap.Int("count", len(messages)))

	// 并发处理消息
	for _, msg := range messages {
		// 使用信号量控制并发数
		w.semaphore <- struct{}{}

		msg := msg // 创建副本以避免闭包问题
		w.wg.Add(1)

		go func() {
			defer w.wg.Done()
			defer func() { <-w.semaphore }()

			w.processMessage(msg)
		}()
	}
}

// 处理单个消息
func (w *Worker) processMessage(message *models.Message) {
	start := time.Now()

	// 创建带超时的上下文
	ctx, cancel := context.WithTimeout(w.ctx, message.Timeout)
	defer cancel()

	w.logger.Debug("Processing message",
		zap.String("messageID", message.ID),
		zap.Int("priority", int(message.Priority)),
	)

	// 调用处理函数
	err := w.processFunc(ctx, message)
	processingTime := time.Since(start)

	// 更新指标
	w.updateMetrics(err, processingTime)

	if err != nil {
		// 处理失败，根据重试策略决定是否重试
		w.handleFailure(message, err)
	} else {
		// 处理成功
		w.handleSuccess(message, processingTime)
	}
}

// 处理成功的消息
func (w *Worker) handleSuccess(message *models.Message, processingTime time.Duration) {
	w.logger.Debug("Message processed successfully",
		zap.String("messageID", message.ID),
		zap.Duration("processingTime", processingTime),
	)

	// 标记消息为已完成
	w.queueManager.CompleteMessage(w.queueName, message.ID, processingTime)
}

// 处理失败的消息
func (w *Worker) handleFailure(message *models.Message, err error) {
	w.logger.Warn("Failed to process message",
		zap.String("messageID", message.ID),
		zap.Error(err),
		zap.Int("retryCount", message.RetryCount),
	)

	// 检查是否可以重试
	if message.RetryCount < w.backoffStrategy.MaxRetries() {
		// 增加重试计数
		message.RetryCount++

		// 计算下次重试时间
		backoff := w.backoffStrategy.NextBackoff(message.RetryCount)
		nextRetry := time.Now().Add(backoff)
		message.ScheduledAt = &nextRetry

		w.logger.Debug("Scheduling message for retry",
			zap.String("messageID", message.ID),
			zap.Int("retryCount", message.RetryCount),
			zap.Duration("backoff", backoff),
			zap.Time("nextRetry", nextRetry),
		)

		// 重新入队
		// 注意：实际实现中，应该将消息放入延迟队列或使用定时任务调度
		// 这里简化处理，直接重新入队
		w.queueManager.PushMessage(w.queueName, message)
	} else {
		// 超过最大重试次数，标记为失败
		w.logger.Error("Message exceeded max retry attempts",
			zap.String("messageID", message.ID),
			zap.Int("maxRetries", w.backoffStrategy.MaxRetries()),
		)

		w.queueManager.FailMessage(w.queueName, message.ID, err)
	}
}

// 更新工作器指标
func (w *Worker) updateMetrics(err error, processingTime time.Duration) {
	w.metricsMutex.Lock()
	defer w.metricsMutex.Unlock()
	
	w.metrics.ProcessedCount++
	w.metrics.TotalProcessTime += processingTime
	w.metrics.LastProcessTime = processingTime

	if err != nil {
		w.metrics.FailureCount++
	} else {
		w.metrics.SuccessCount++
	}
}

// 指数退避策略
type ExponentialBackoff struct {
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	Factor         float64
	maxRetries     int
}

func NewExponentialBackoff(initial, max time.Duration, factor float64, maxRetries int) *ExponentialBackoff {
	return &ExponentialBackoff{
		InitialBackoff: initial,
		MaxBackoff:     max,
		Factor:         factor,
		maxRetries:     maxRetries,
	}
}

func (eb *ExponentialBackoff) NextBackoff(retryCount int) time.Duration {
	if retryCount <= 0 {
		return eb.InitialBackoff
	}

	// 计算指数退避时间
	backoff := float64(eb.InitialBackoff)
	for i := 1; i < retryCount; i++ {
		backoff *= eb.Factor
	}

	if backoff > float64(eb.MaxBackoff) {
		return eb.MaxBackoff
	}

	return time.Duration(backoff)
}

func (eb *ExponentialBackoff) MaxRetries() int {
	return eb.maxRetries
}

// 固定间隔退避策略
type FixedBackoff struct {
	Backoff    time.Duration
	maxRetries int
}

func NewFixedBackoff(backoff time.Duration, maxRetries int) *FixedBackoff {
	return &FixedBackoff{
		Backoff:    backoff,
		maxRetries: maxRetries,
	}
}

func (fb *FixedBackoff) NextBackoff(retryCount int) time.Duration {
	return fb.Backoff
}

func (fb *FixedBackoff) MaxRetries() int {
	return fb.maxRetries
}
