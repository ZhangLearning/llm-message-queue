package priorityqueue

import (
	"container/heap"
	"context"
	"sync"
	"time"

	"llm-message-queue/pkg/models"

	"go.uber.org/zap"
)

// DelayedItem 延迟队列项
type DelayedItem struct {
	Message *models.Message
	ReadyAt time.Time
	Index   int
}

// DelayedQueue 延迟队列，用于处理需要在特定时间执行的消息
type DelayedQueue struct {
	items     []*DelayedItem
	mu        sync.RWMutex
	pushCh    chan *DelayedItem
	readyCh   chan *models.Message
	quitCh    chan struct{}
	ctx       context.Context
	cancel    context.CancelFunc
	processFn func(*models.Message) error
	logger    *zap.Logger
}

// 实现heap.Interface所需的方法
func (dq *DelayedQueue) Len() int { return len(dq.items) }

func (dq *DelayedQueue) Less(i, j int) bool {
	return dq.items[i].ReadyAt.Before(dq.items[j].ReadyAt)
}

func (dq *DelayedQueue) Swap(i, j int) {
	dq.items[i], dq.items[j] = dq.items[j], dq.items[i]
	dq.items[i].Index = i
	dq.items[j].Index = j
}

func (dq *DelayedQueue) Push(x interface{}) {
	n := len(dq.items)
	item := x.(*DelayedItem)
	item.Index = n
	dq.items = append(dq.items, item)
}

func (dq *DelayedQueue) Pop() interface{} {
	old := dq.items
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.Index = -1
	dq.items = old[0 : n-1]
	return item
}

// NewDelayedQueue 创建新的延迟队列
func NewDelayedQueue(processFn func(*models.Message) error, logger *zap.Logger) *DelayedQueue {
	ctx, cancel := context.WithCancel(context.Background())

	dq := &DelayedQueue{
		items:     make([]*DelayedItem, 0),
		pushCh:    make(chan *DelayedItem),
		readyCh:   make(chan *models.Message),
		quitCh:    make(chan struct{}),
		ctx:       ctx,
		cancel:    cancel,
		processFn: processFn,
		logger:    logger,
	}

	// 初始化堆
	heap.Init(dq)

	return dq
}

// Start 启动延迟队列处理
func (dq *DelayedQueue) Start() {
	go dq.run()
	go dq.processReadyItems()
}

// Stop 停止延迟队列处理
func (dq *DelayedQueue) Stop() {
	dq.cancel()
	close(dq.quitCh)
}

// Schedule 调度消息在指定时间执行
func (dq *DelayedQueue) Schedule(message *models.Message, readyAt time.Time) {
	item := &DelayedItem{
		Message: message,
		ReadyAt: readyAt,
	}

	dq.pushCh <- item
}

// ScheduleAfter 调度消息在指定延迟后执行
func (dq *DelayedQueue) ScheduleAfter(message *models.Message, delay time.Duration) {
	readyAt := time.Now().Add(delay)
	dq.Schedule(message, readyAt)
}

// 主运行循环
func (dq *DelayedQueue) run() {
	var timer *time.Timer
	var nextItem *DelayedItem

	for {
		// 获取下一个要执行的项目
		dq.mu.Lock()
		if dq.Len() > 0 && (nextItem == nil || dq.items[0].ReadyAt.Before(nextItem.ReadyAt)) {
			nextItem = dq.items[0]
		}
		dq.mu.Unlock()

		var nextReadyAt time.Time
		if nextItem != nil {
			nextReadyAt = nextItem.ReadyAt
		}

		// 设置定时器
		if timer != nil {
			timer.Stop()
		}

		var waitDuration time.Duration
		now := time.Now()

		if nextItem != nil {
			if now.After(nextReadyAt) || now.Equal(nextReadyAt) {
				// 项目已准备好执行
				dq.mu.Lock()
				if dq.Len() > 0 && dq.items[0].ReadyAt.Before(now.Add(time.Millisecond)) {
					item := heap.Pop(dq).(*DelayedItem)
					dq.mu.Unlock()

					dq.readyCh <- item.Message
					nextItem = nil
					continue
				}
				dq.mu.Unlock()
			} else {
				// 计算等待时间
				waitDuration = nextReadyAt.Sub(now)
			}
		} else {
			// 没有项目，等待新的项目
			waitDuration = 24 * time.Hour
		}

		timer = time.NewTimer(waitDuration)

		select {
		case <-dq.ctx.Done():
			if timer != nil {
				timer.Stop()
			}
			return

		case item := <-dq.pushCh:
			// 新项目入队
			dq.mu.Lock()
			heap.Push(dq, item)
			dq.mu.Unlock()

			dq.logger.Debug("Scheduled new delayed item",
				zap.String("messageID", item.Message.ID),
				zap.Time("readyAt", item.ReadyAt),
			)

		case <-timer.C:
			// 定时器触发，检查是否有准备好的项目
			dq.mu.Lock()
			if dq.Len() > 0 && dq.items[0].ReadyAt.Before(time.Now().Add(time.Millisecond)) {
				item := heap.Pop(dq).(*DelayedItem)
				dq.mu.Unlock()

				dq.logger.Debug("Item ready for processing",
					zap.String("messageID", item.Message.ID),
				)

				dq.readyCh <- item.Message
				nextItem = nil
			} else {
				dq.mu.Unlock()
			}
		}
	}
}

// 处理准备好的项目
func (dq *DelayedQueue) processReadyItems() {
	for {
		select {
		case <-dq.ctx.Done():
			return

		case msg := <-dq.readyCh:
			// 处理准备好的消息
			dq.logger.Debug("Processing ready message", zap.String("messageID", msg.ID))

			err := dq.processMessage(msg)
			if err != nil {
				dq.logger.Error("Failed to process delayed message",
					zap.String("messageID", msg.ID),
					zap.Error(err),
				)
			}
		}
	}
}

// 处理单个消息
func (dq *DelayedQueue) processMessage(msg *models.Message) error {
	if dq.processFn != nil {
		return dq.processFn(msg)
	}
	return nil
}

// Size 获取队列大小
func (dq *DelayedQueue) Size() int {
	dq.mu.RLock()
	defer dq.mu.RUnlock()
	return dq.Len()
}

// Peek 查看下一个要执行的消息，但不移除
func (dq *DelayedQueue) Peek() (*models.Message, time.Time, bool) {
	dq.mu.RLock()
	defer dq.mu.RUnlock()

	if dq.Len() == 0 {
		return nil, time.Time{}, false
	}

	item := dq.items[0]
	return item.Message, item.ReadyAt, true
}

// Clear 清空队列
func (dq *DelayedQueue) Clear() {
	dq.mu.Lock()
	defer dq.mu.Unlock()

	dq.items = make([]*DelayedItem, 0)
	heap.Init(dq)
}
