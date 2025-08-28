package tests

import (
	"context"
	"testing"
	"time"

	"llm-message-queue/internal/priorityqueue"
	"llm-message-queue/pkg/config"
	"llm-message-queue/pkg/models"

	"go.uber.org/zap"
)

func TestQueueFactory(t *testing.T) {
	// 创建日志记录器
	logger, _ := zap.NewDevelopment()

	// 创建队列配置
	cfg := &config.QueueConfig{
		DefaultMaxSize:    1000,
		MonitorInterval:   time.Minute,
		CleanupInterval:   time.Hour,
		EnableMetrics:     false, // 禁用指标以避免重复注册
		Worker: config.WorkerConfig{
			MaxBatchSize:    10,
			ProcessInterval: 100 * time.Millisecond,
			MaxConcurrent:   2,
		},
		Retry: config.RetryConfig{
			MaxRetries:      3,
			InitialBackoff:  time.Second,
			MaxBackoff:      time.Minute,
			Factor:          2.0,
		},
	}

	// 创建队列工厂
	factory := priorityqueue.NewQueueFactory(cfg, logger)

	// 测试创建标准队列
	t.Run("CreateStandardQueue", func(t *testing.T) {
		// 创建标准队列管理器
		manager := factory.CreateQueueManager("test-standard", priorityqueue.StandardQueueType)

		// 验证管理器已创建
		if manager == nil {
			t.Fatal("Expected non-nil manager, got nil")
		}

		// 创建队列
		manager.CreateQueue("test-standard", 1000)

		// 验证队列已创建
		stats, err := manager.GetQueueStats("test-standard")
		if err != nil {
			t.Logf("Queue stats error (expected initially): %v", err)
		}

		// 获取管理器
		retrievedManager, exists := factory.GetQueueManager("test-standard")
		if !exists {
			t.Fatal("Failed to get queue manager")
		}

		// 验证获取的是同一个管理器
		if retrievedManager != manager {
			t.Error("Retrieved manager is not the same as created manager")
		}

		// 测试基本操作
		msg := models.NewMessage("conv-factory", "user-factory", "Factory test message", models.PriorityNormal)
		err = manager.PushMessage("test-standard", msg)
		if err != nil {
			t.Fatalf("Failed to push message: %v", err)
		}

		// 验证消息已入队
		stats, err = manager.GetQueueStats("test-standard")
		if err != nil {
			t.Fatalf("Failed to get queue stats: %v", err)
		}
		if stats.PendingCount == 0 {
			t.Error("Expected pending count > 0 after pushing message")
		}
	})

	// 测试创建延迟队列
	t.Run("CreateDelayedQueue", func(t *testing.T) {
		// 创建延迟队列管理器
		manager := factory.CreateQueueManager("test-delayed", priorityqueue.DelayedQueueType)

		// 验证管理器已创建
		if manager == nil {
			t.Fatal("Expected non-nil manager, got nil")
		}

		// 创建队列
		manager.CreateQueue("test-delayed", 1000)

		// 获取延迟队列管理器
		retrievedManager, exists := factory.GetQueueManager("test-delayed")
		if !exists {
			t.Fatal("Failed to get delayed queue manager")
		}
		if retrievedManager != manager {
			t.Error("Retrieved manager is not the same as created manager")
		}

		// 测试延迟消息处理
		msg := models.NewMessage("conv-delayed", "user-delayed", "Delayed test message", models.PriorityNormal)
		delayTime := time.Now().Add(2 * time.Second)
		msg.ScheduledAt = &delayTime

		err := manager.PushMessage("test-delayed", msg)
		if err != nil {
			t.Fatalf("Failed to push delayed message: %v", err)
		}
	})

	// 测试创建死信队列
	t.Run("CreateDeadLetterQueue", func(t *testing.T) {
		// 创建死信队列管理器
		manager := factory.CreateQueueManager("test-deadletter", priorityqueue.DeadLetterQueueType)

		// 验证管理器已创建
		if manager == nil {
			t.Fatal("Expected non-nil manager, got nil")
		}

		// 创建队列
		manager.CreateQueue("test-deadletter", 1000)

		// 获取死信队列管理器
		retrievedManager, exists := factory.GetQueueManager("test-deadletter")
		if !exists {
			t.Fatal("Failed to get dead letter queue manager")
		}
		if retrievedManager != manager {
			t.Error("Retrieved manager is not the same as created manager")
		}

		// 测试死信消息处理
		msg := models.NewMessage("conv-dlq", "user-dlq", "Dead letter test message", models.PriorityNormal)
		err := manager.PushMessage("test-deadletter", msg)
		if err != nil {
			t.Fatalf("Failed to push message to dead letter queue: %v", err)
		}
	})

	// 测试工作器创建
	t.Run("CreateWorkers", func(t *testing.T) {
		// 创建队列管理器
		manager := factory.CreateQueueManager("worker-test", priorityqueue.StandardQueueType)
		if manager == nil {
			t.Fatal("Expected non-nil manager, got nil")
		}

		// 创建队列
		manager.CreateQueue("worker-test", 1000)

		// 创建处理函数
		processFunc := func(ctx context.Context, msg *models.Message) error {
			// 简单处理函数
			return nil
		}

		// 创建工作器
		workers := factory.CreateWorkers("worker-test", 2, processFunc)
		if len(workers) != 2 {
			t.Fatalf("Expected 2 workers, got %d", len(workers))
		}

		// 验证工作器已创建
		for i, worker := range workers {
			if worker == nil {
				t.Fatalf("Worker %d is nil", i)
			}
		}

		// 停止所有工作器以避免goroutine泄漏
		for _, worker := range workers {
			worker.Stop()
		}
		// 等待goroutine完全停止
		time.Sleep(100 * time.Millisecond)

		// 停止管理器
		manager.Stop()
		time.Sleep(100 * time.Millisecond)
	})

	// 测试工厂清理
	t.Run("FactoryCleanup", func(t *testing.T) {
		// 创建测试队列
		manager := factory.CreateQueueManager("cleanup-test", priorityqueue.StandardQueueType)
		if manager == nil {
			t.Fatal("Expected non-nil manager, got nil")
		}

		// 验证队列存在
		_, exists := factory.GetQueueManager("cleanup-test")
		if !exists {
			t.Fatal("Queue should exist before cleanup")
		}

		// 停止管理器
		manager.Stop()
		// 等待goroutine完全停止
		time.Sleep(100 * time.Millisecond)
	})
}
