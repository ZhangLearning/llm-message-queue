package priorityqueue

import (
	"container/heap"
	"sync"
	"time"

	"llm-message-queue/pkg/models"
)

type QueueItem struct {
	Message   *models.Message
	Priority  int
	Timestamp time.Time
	Index     int
}

type PriorityQueue []*QueueItem

func (pq PriorityQueue) Len() int { return len(pq) }

func (pq PriorityQueue) Less(i, j int) bool {
	if pq[i].Priority == pq[j].Priority {
		return pq[i].Timestamp.Before(pq[j].Timestamp)
	}
	return pq[i].Priority < pq[j].Priority
}

func (pq PriorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].Index = i
	pq[j].Index = j
}

func (pq *PriorityQueue) Push(x interface{}) {
	n := len(*pq)
	item := x.(*QueueItem)
	item.Index = n
	*pq = append(*pq, item)
}

func (pq *PriorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.Index = -1
	*pq = old[0 : n-1]
	return item
}

type MultiLevelQueue struct {
	queues  map[string]*PriorityQueue
	mu      sync.RWMutex
	maxSize int
	stats   map[string]*QueueStats
	statsMu sync.RWMutex
}

type QueueStats struct {
	PendingCount     int
	ProcessingCount  int
	CompletedCount   int
	FailedCount      int
	TotalWaitTime    time.Duration
	TotalProcessTime time.Duration
	LastUpdate       time.Time
}

func NewMultiLevelQueue(maxSize int) *MultiLevelQueue {
	return &MultiLevelQueue{
		queues:  make(map[string]*PriorityQueue),
		maxSize: maxSize,
		stats:   make(map[string]*QueueStats),
	}
}

func (mlq *MultiLevelQueue) AddQueue(name string) {
	mlq.mu.Lock()
	defer mlq.mu.Unlock()

	if _, exists := mlq.queues[name]; !exists {
		queue := make(PriorityQueue, 0)
		heap.Init(&queue)
		mlq.queues[name] = &queue
		mlq.stats[name] = &QueueStats{
			LastUpdate: time.Now(),
		}
	}
}

func (mlq *MultiLevelQueue) Push(queueName string, message *models.Message, priority int) error {
	mlq.mu.Lock()
	defer mlq.mu.Unlock()

	queue, exists := mlq.queues[queueName]
	if !exists {
		return ErrQueueNotFound
	}

	if mlq.maxSize > 0 && len(*queue) >= mlq.maxSize {
		return ErrQueueFull
	}

	item := &QueueItem{
		Message:   message,
		Priority:  priority,
		Timestamp: time.Now(),
	}

	heap.Push(queue, item)

	mlq.updateStats(queueName, func(stats *QueueStats) {
		stats.PendingCount++
		stats.LastUpdate = time.Now()
	})

	return nil
}

func (mlq *MultiLevelQueue) Pop(queueName string) (*models.Message, error) {
	mlq.mu.Lock()
	defer mlq.mu.Unlock()

	queue, exists := mlq.queues[queueName]
	if !exists || len(*queue) == 0 {
		return nil, ErrQueueEmpty
	}

	item := heap.Pop(queue).(*QueueItem)

	mlq.updateStats(queueName, func(stats *QueueStats) {
		stats.PendingCount--
		stats.ProcessingCount++
		stats.LastUpdate = time.Now()
	})

	return item.Message, nil
}

func (mlq *MultiLevelQueue) Peek(queueName string) (*models.Message, error) {
	mlq.mu.RLock()
	defer mlq.mu.RUnlock()

	queue, exists := mlq.queues[queueName]
	if !exists || len(*queue) == 0 {
		return nil, ErrQueueEmpty
	}

	return (*queue)[0].Message, nil
}

func (mlq *MultiLevelQueue) Size(queueName string) (int, error) {
	mlq.mu.RLock()
	defer mlq.mu.RUnlock()

	queue, exists := mlq.queues[queueName]
	if !exists {
		return 0, ErrQueueNotFound
	}

	return len(*queue), nil
}

func (mlq *MultiLevelQueue) GetStats(queueName string) (*QueueStats, error) {
	mlq.statsMu.RLock()
	defer mlq.statsMu.RUnlock()

	stats, exists := mlq.stats[queueName]
	if !exists {
		return nil, ErrQueueNotFound
	}

	return stats, nil
}

func (mlq *MultiLevelQueue) GetAllStats() map[string]*QueueStats {
	mlq.statsMu.RLock()
	defer mlq.statsMu.RUnlock()

	stats := make(map[string]*QueueStats)
	for name, stat := range mlq.stats {
		stats[name] = stat
	}
	return stats
}

func (mlq *MultiLevelQueue) updateStats(queueName string, updateFunc func(*QueueStats)) {
	mlq.statsMu.Lock()
	defer mlq.statsMu.Unlock()

	if stats, exists := mlq.stats[queueName]; exists {
		updateFunc(stats)
	}
}

func (mlq *MultiLevelQueue) CompleteMessage(queueName string) {
	mlq.updateStats(queueName, func(stats *QueueStats) {
		stats.ProcessingCount--
		stats.CompletedCount++
		stats.LastUpdate = time.Now()
	})
}

func (mlq *MultiLevelQueue) FailMessage(queueName string) {
	mlq.updateStats(queueName, func(stats *QueueStats) {
		stats.ProcessingCount--
		stats.FailedCount++
		stats.LastUpdate = time.Now()
	})
}

var (
	ErrQueueNotFound = &QueueError{Code: "QUEUE_NOT_FOUND", Message: "queue not found"}
	ErrQueueFull     = &QueueError{Code: "QUEUE_FULL", Message: "queue is full"}
	ErrQueueEmpty    = &QueueError{Code: "QUEUE_EMPTY", Message: "queue is empty"}
)

type QueueError struct {
	Code    string
	Message string
}

func (e *QueueError) Error() string {
	return e.Message
}
