package tests

import (
	"context"
	"testing"
	"time"

	"llm-message-queue/internal/priorityqueue"
	"llm-message-queue/pkg/models"

	"go.uber.org/zap"
)

func TestMultiLevelQueue(t *testing.T) {
	// 创建多级队列
	multiQueue := priorityqueue.NewMultiLevelQueue(1000)

	// 添加队列
	multiQueue.AddQueue("realtime")
	multiQueue.AddQueue("high")
	multiQueue.AddQueue("normal")
	multiQueue.AddQueue("low")

	// 测试Push和Pop
	t.Run("PushAndPop", func(t *testing.T) {
		// 创建测试消息
		msg1 := models.NewMessage("conv1", "user1", "Test message 1", models.PriorityHigh)
		msg2 := models.NewMessage("conv1", "user1", "Test message 2", models.PriorityRealtime)
		msg3 := models.NewMessage("conv2", "user2", "Test message 3", models.PriorityNormal)

		// 推送消息到队列
		err := multiQueue.Push("high", msg1, int(models.PriorityHigh))
		if err != nil {
			t.Fatalf("Failed to push message 1: %v", err)
		}

		err = multiQueue.Push("realtime", msg2, int(models.PriorityRealtime))
		if err != nil {
			t.Fatalf("Failed to push message 2: %v", err)
		}

		err = multiQueue.Push("normal", msg3, int(models.PriorityNormal))
		if err != nil {
			t.Fatalf("Failed to push message 3: %v", err)
		}

		// 检查队列大小
		size1, err := multiQueue.Size("realtime")
		if err != nil || size1 != 1 {
			t.Errorf("Expected realtime queue size to be 1, got %d, err: %v", size1, err)
		}

		size2, err := multiQueue.Size("high")
		if err != nil || size2 != 1 {
			t.Errorf("Expected high queue size to be 1, got %d, err: %v", size2, err)
		}

		size3, err := multiQueue.Size("normal")
		if err != nil || size3 != 1 {
			t.Errorf("Expected normal queue size to be 1, got %d, err: %v", size3, err)
		}

		// 弹出消息并验证
		poppedMsg1, err := multiQueue.Pop("realtime")
		if err != nil {
			t.Fatalf("Failed to pop message from realtime queue: %v", err)
		}
		if poppedMsg1.ID != msg2.ID {
			t.Errorf("Expected to pop msg2 from realtime queue, got %s", poppedMsg1.ID)
		}

		poppedMsg2, err := multiQueue.Pop("high")
		if err != nil {
			t.Fatalf("Failed to pop message from high queue: %v", err)
		}
		if poppedMsg2.ID != msg1.ID {
			t.Errorf("Expected to pop msg1 from high queue, got %s", poppedMsg2.ID)
		}

		poppedMsg3, err := multiQueue.Pop("normal")
		if err != nil {
			t.Fatalf("Failed to pop message from normal queue: %v", err)
		}
		if poppedMsg3.ID != msg3.ID {
			t.Errorf("Expected to pop msg3 from normal queue, got %s", poppedMsg3.ID)
		}

		// 完成所有弹出的消息
		multiQueue.CompleteMessage("realtime")
		multiQueue.CompleteMessage("high")
		multiQueue.CompleteMessage("normal")

		// 队列应该为空
		_, err = multiQueue.Pop("realtime")
		if err != priorityqueue.ErrQueueEmpty {
			t.Errorf("Expected ErrQueueEmpty for realtime queue, got %v", err)
		}
	})

	// 测试Peek
	t.Run("Peek", func(t *testing.T) {
		// 创建测试消息
		msg := models.NewMessage("conv3", "user3", "Test message 4", models.PriorityHigh)

		// 推送消息到队列
		err := multiQueue.Push("high", msg, int(models.PriorityHigh))
		if err != nil {
			t.Fatalf("Failed to push message: %v", err)
		}

		// 查看消息但不移除
		peekedMsg, err := multiQueue.Peek("high")
		if err != nil {
			t.Fatalf("Failed to peek message: %v", err)
		}
		if peekedMsg.ID != msg.ID {
			t.Errorf("Expected to peek msg4, got %s", peekedMsg.ID)
		}

		// 消息应该仍在队列中
		size, err := multiQueue.Size("high")
		if err != nil || size != 1 {
			t.Errorf("Expected high queue size to be 1 after peek, got %d, err: %v", size, err)
		}

		// 弹出消息以清理
		_, err = multiQueue.Pop("high")
		if err != nil {
			t.Fatalf("Failed to pop message: %v", err)
		}
		multiQueue.CompleteMessage("high")
	})

	// 测试队列统计
	t.Run("QueueStats", func(t *testing.T) {
		// 创建并推送测试消息
		for i := 0; i < 5; i++ {
			msg := models.NewMessage("conv-stats", "user-stats", "Stats test message", models.PriorityRealtime)
			multiQueue.Push("realtime", msg, int(models.PriorityRealtime))
		}

		for i := 0; i < 3; i++ {
			msg := models.NewMessage("conv-stats", "user-stats", "Stats test message", models.PriorityHigh)
			multiQueue.Push("high", msg, int(models.PriorityHigh))
		}

		// 获取队列统计
		statsRealtime, err := multiQueue.GetStats("realtime")
		if err != nil {
			t.Fatalf("Failed to get realtime queue stats: %v", err)
		}

		statsHigh, err := multiQueue.GetStats("high")
		if err != nil {
			t.Fatalf("Failed to get high queue stats: %v", err)
		}

		// 验证统计
		if statsRealtime.PendingCount != 5 {
			t.Errorf("Expected realtime pending count to be 5, got %d", statsRealtime.PendingCount)
		}

		if statsHigh.PendingCount != 3 {
			t.Errorf("Expected high pending count to be 3, got %d", statsHigh.PendingCount)
		}

		// 清理，弹出所有消息并标记为完成
		for i := 0; i < 5; i++ {
			_, err := multiQueue.Pop("realtime")
			if err != nil {
				t.Fatalf("Failed to pop message from realtime queue during cleanup: %v", err)
			}
			multiQueue.CompleteMessage("realtime")
		}

		for i := 0; i < 3; i++ {
			_, err := multiQueue.Pop("high")
			if err != nil {
				t.Fatalf("Failed to pop message from high queue during cleanup: %v", err)
			}
			multiQueue.CompleteMessage("high")
		}
	})

	// 测试完成/失败消息计数
	t.Run("CompleteFailMessages", func(t *testing.T) {
		// 创建一个新的队列实例以确保统计信息重置
		testQueue := priorityqueue.NewMultiLevelQueue(1000)
		testQueue.AddQueue("normal")

		// 创建并推送测试消息
		for i := 0; i < 5; i++ {
			msg := models.NewMessage("conv-complete", "user-complete", "Complete test message", models.PriorityNormal)
			testQueue.Push("normal", msg, int(models.PriorityNormal))
		}

		// 弹出消息
		for i := 0; i < 5; i++ {
			_, err := testQueue.Pop("normal")
			if err != nil {
				t.Fatalf("Failed to pop message: %v", err)
			}
		}

		// 标记3个消息为完成，2个为失败
		for i := 0; i < 3; i++ {
			testQueue.CompleteMessage("normal")
		}

		for i := 0; i < 2; i++ {
			testQueue.FailMessage("normal")
		}

		// 等待一小段时间确保统计更新
		time.Sleep(10 * time.Millisecond)

		// 验证统计
		stats, err := testQueue.GetStats("normal")
		if err != nil {
			t.Fatalf("Failed to get normal queue stats: %v", err)
		}

		// 调试信息
		t.Logf("Stats: Pending=%d, Processing=%d, Completed=%d, Failed=%d", 
			stats.PendingCount, stats.ProcessingCount, stats.CompletedCount, stats.FailedCount)

		if stats.CompletedCount != 3 {
			t.Errorf("Expected completed count to be 3, got %d", stats.CompletedCount)
		}

		if stats.FailedCount != 2 {
			t.Errorf("Expected failed count to be 2, got %d", stats.FailedCount)
		}

		if stats.ProcessingCount != 0 {
			t.Errorf("Expected processing count to be 0, got %d", stats.ProcessingCount)
		}
	})
}

func TestQueueManager(t *testing.T) {
	// 创建日志记录器
	logger, _ := zap.NewDevelopment()

	// 创建队列管理器配置
	config := &priorityqueue.QueueManagerConfig{
		DefaultMaxSize:     1000,
		MonitorInterval:    100 * time.Millisecond,
		CleanupInterval:    1 * time.Second,
		MaxRetentionPeriod: 1 * time.Hour,
		EnableMetrics:      false,
		EnableAutoScaling:  false,
		ScalingThresholds:  map[string]int{"test": 100},
	}

	// 创建队列管理器
	manager := priorityqueue.NewQueueManager(config, logger)

	// 启动队列管理器
	manager.Start()
	defer func() {
		manager.Stop()
		// 等待一小段时间确保goroutine完全停止
		time.Sleep(100 * time.Millisecond)
	}()

	// 创建队列
	manager.CreateQueue("test", 100)

	// 测试推送和弹出消息
	t.Run("PushPopMessage", func(t *testing.T) {
		// 创建测试消息
		msg := models.NewMessage("conv-manager", "user-manager", "Manager test message", models.PriorityHigh)

		// 推送消息
		err := manager.PushMessage("test", msg)
		if err != nil {
			t.Fatalf("Failed to push message: %v", err)
		}

		// 弹出消息
		poppedMsg, err := manager.PopMessage("test")
		if err != nil {
			t.Fatalf("Failed to pop message: %v", err)
		}

		// 验证消息
		if poppedMsg.ID != msg.ID {
			t.Errorf("Expected to pop message with ID %s, got %s", msg.ID, poppedMsg.ID)
		}
	})

	// 测试批量操作
	t.Run("BatchOperations", func(t *testing.T) {
		// 创建测试消息
		messages := make([]*models.Message, 5)
		for i := 0; i < 5; i++ {
			messages[i] = models.NewMessage("conv-batch", "user-batch", "Batch test message", models.PriorityNormal)
		}

		// 批量推送
		count, err := manager.BatchPushMessages("test", messages)
		if err != nil {
			t.Fatalf("Failed to batch push messages: %v", err)
		}
		if count != 5 {
			t.Errorf("Expected to push 5 messages, pushed %d", count)
		}

		// 批量弹出
		popped, err := manager.BatchPopMessages("test", 3)
		if err != nil {
			t.Fatalf("Failed to batch pop messages: %v", err)
		}
		if len(popped) != 3 {
			t.Errorf("Expected to pop 3 messages, popped %d", len(popped))
		}

		// 清理剩余消息
		_, err = manager.BatchPopMessages("test", 2)
		if err != nil {
			t.Fatalf("Failed to clean up remaining messages: %v", err)
		}
	})

	// 测试完成/失败消息
	t.Run("CompleteFailMessages", func(t *testing.T) {
		// 创建并推送测试消息
		for i := 0; i < 5; i++ {
			msg := models.NewMessage("conv-status", "user-status", "Status test message", models.PriorityNormal)
			manager.PushMessage("test", msg)
		}

		// 弹出消息
		messages, err := manager.BatchPopMessages("test", 5)
		if err != nil {
			t.Fatalf("Failed to pop messages: %v", err)
		}

		// 标记3个消息为完成，2个为失败
		for i := 0; i < 3; i++ {
			manager.CompleteMessage("test", messages[i].ID, 100*time.Millisecond)
		}

		for i := 3; i < 5; i++ {
			manager.FailMessage("test", messages[i].ID, nil)
		}

		// 验证统计
		stats, err := manager.GetQueueStats("test")
		if err != nil {
			t.Fatalf("Failed to get queue stats: %v", err)
		}

		if stats.CompletedCount != 3 {
			t.Errorf("Expected completed count to be 3, got %d", stats.CompletedCount)
		}

		if stats.FailedCount != 2 {
			t.Errorf("Expected failed count to be 2, got %d", stats.FailedCount)
		}
	})
}

func TestWorker(t *testing.T) {
	// 创建日志记录器
	logger, _ := zap.NewDevelopment()

	// 创建队列管理器
	config := &priorityqueue.QueueManagerConfig{
		DefaultMaxSize:     100,
		EnableMetrics:      false,
		MonitorInterval:    100 * time.Millisecond,
		CleanupInterval:    1 * time.Second,
		MaxRetentionPeriod: 1 * time.Hour,
	}
	manager := priorityqueue.NewQueueManager(config, logger)

	// 创建队列
	manager.CreateQueue("worker-test", 100)

	// 启动队列管理器
	manager.Start()
	defer func() {
		manager.Stop()
		// 等待一小段时间确保goroutine完全停止
		time.Sleep(100 * time.Millisecond)
	}()

	// 创建处理结果通道
	resultCh := make(chan string, 10)

	// 创建工作器配置
	workerConfig := &priorityqueue.WorkerConfig{
		ID:              "test-worker",
		QueueName:       "worker-test",
		MaxBatchSize:    5,
		ProcessInterval: 100 * time.Millisecond,
		MaxConcurrent:   2,
		BackoffStrategy: priorityqueue.NewExponentialBackoff(
			100*time.Millisecond,
			1*time.Second,
			2.0,
			3,
		),
	}

	// 创建处理函数
	processFunc := func(ctx context.Context, msg *models.Message) error {
		// 简单处理，将消息ID发送到结果通道
		resultCh <- msg.ID
		return nil
	}

	// 创建工作器
	worker := priorityqueue.NewWorker(workerConfig, manager, processFunc, logger)

	// 启动工作器
	worker.Start()
	defer func() {
		worker.Stop()
		// 等待一小段时间确保goroutine完全停止
		time.Sleep(100 * time.Millisecond)
	}()

	// 测试消息处理
	t.Run("ProcessMessages", func(t *testing.T) {
		// 创建测试消息
		messages := make([]*models.Message, 5)
		expectedIDs := make(map[string]bool)

		for i := 0; i < 5; i++ {
			messages[i] = models.NewMessage("conv-worker", "user-worker", "Worker test message", models.PriorityNormal)
			expectedIDs[messages[i].ID] = true
		}

		// 推送消息
		for _, msg := range messages {
			manager.PushMessage("worker-test", msg)
		}

		// 等待所有消息被处理
		processedIDs := make(map[string]bool)
		for i := 0; i < 5; i++ {
			select {
			case id := <-resultCh:
				processedIDs[id] = true
			case <-time.After(2 * time.Second):
				t.Fatalf("Timeout waiting for message processing")
			}
		}

		// 验证所有消息都被处理
		for id := range expectedIDs {
			if !processedIDs[id] {
				t.Errorf("Message %s was not processed", id)
			}
		}

		// 验证工作器指标
		metrics := worker.GetMetrics()
		if metrics.ProcessedCount != 5 {
			t.Errorf("Expected processed count to be 5, got %d", metrics.ProcessedCount)
		}
		if metrics.SuccessCount != 5 {
			t.Errorf("Expected success count to be 5, got %d", metrics.SuccessCount)
		}
	})
}

func TestDelayedQueue(t *testing.T) {
	// 创建日志记录器
	logger, _ := zap.NewDevelopment()

	// 创建处理结果通道
	resultCh := make(chan string, 10)

	// 创建处理函数
	processFunc := func(msg *models.Message) error {
		resultCh <- msg.ID
		return nil
	}

	// 创建延迟队列
	dq := priorityqueue.NewDelayedQueue(processFunc, logger)

	// 启动延迟队列
	dq.Start()
	defer func() {
		dq.Stop()
		// 等待一小段时间确保goroutine完全停止
		time.Sleep(100 * time.Millisecond)
	}()

	// 测试延迟调度
	t.Run("DelayedScheduling", func(t *testing.T) {
		// 创建测试消息
		msg1 := models.NewMessage("conv-delay", "user-delay", "Delay test message 1", models.PriorityNormal)
		msg2 := models.NewMessage("conv-delay", "user-delay", "Delay test message 2", models.PriorityHigh)

		// 调度消息，一个短延迟，一个长延迟
		start := time.Now()
		dq.ScheduleAfter(msg1, 200*time.Millisecond)
		dq.ScheduleAfter(msg2, 500*time.Millisecond)

		// 接收第一个消息
		select {
		case id := <-resultCh:
			if id != msg1.ID {
				t.Errorf("Expected first message to be %s, got %s", msg1.ID, id)
			}
			elapsed := time.Since(start)
			if elapsed < 200*time.Millisecond {
				t.Errorf("Message processed too early: %v", elapsed)
			}
		case <-time.After(1 * time.Second):
			t.Fatalf("Timeout waiting for first message")
		}

		// 接收第二个消息
		select {
		case id := <-resultCh:
			if id != msg2.ID {
				t.Errorf("Expected second message to be %s, got %s", msg2.ID, id)
			}
			elapsed := time.Since(start)
			if elapsed < 500*time.Millisecond {
				t.Errorf("Message processed too early: %v", elapsed)
			}
		case <-time.After(1 * time.Second):
			t.Fatalf("Timeout waiting for second message")
		}
	})

	// 测试查看下一个项目
	t.Run("PeekNextItem", func(t *testing.T) {
		// 创建测试消息
		msg := models.NewMessage("conv-peek", "user-peek", "Peek test message", models.PriorityNormal)

		// 调度消息，较长延迟以便有时间查看
		readyAt := time.Now().Add(1 * time.Second)
		dq.Schedule(msg, readyAt)

		// 等待消息被调度到队列中
		time.Sleep(50 * time.Millisecond)

		// 查看下一个项目
		peekMsg, peekTime, ok := dq.Peek()
		if !ok {
			t.Fatalf("Failed to peek next item")
		}
		if peekMsg.ID != msg.ID {
			t.Errorf("Expected to peek message %s, got %s", msg.ID, peekMsg.ID)
		}
		if peekTime.Sub(readyAt) > 10*time.Millisecond {
			t.Errorf("Expected ready time to be %v, got %v", readyAt, peekTime)
		}

		// 等待消息处理完成
		select {
		case <-resultCh:
			// 消息已处理
		case <-time.After(2 * time.Second):
			t.Fatalf("Timeout waiting for message processing")
		}
	})
}

func TestDeadLetterQueue(t *testing.T) {
	// 创建日志记录器
	logger, _ := zap.NewDevelopment()

	// 创建死信队列
	dlq := priorityqueue.NewDeadLetterQueue(100, logger)

	// 创建队列管理器用于重新入队测试
	config := &priorityqueue.QueueManagerConfig{
		DefaultMaxSize: 100,
		EnableMetrics:  false,
	}
	manager := priorityqueue.NewQueueManager(config, logger)
	manager.CreateQueue("requeue-test", 100)

	// 测试推送和获取
	t.Run("PushAndGet", func(t *testing.T) {
		// 创建测试消息
		msg := models.NewMessage("conv-dlq", "user-dlq", "DLQ test message", models.PriorityNormal)
		msg.RetryCount = 3

		// 推送到死信队列
		err := dlq.Push(msg, "处理超时", "test-queue")
		if err != nil {
			t.Fatalf("Failed to push message to dead letter queue: %v", err)
		}

		// 获取死信项
		item, err := dlq.Get(0)
		if err != nil {
			t.Fatalf("Failed to get item from dead letter queue: %v", err)
		}

		// 验证项目
		if item.Message.ID != msg.ID {
			t.Errorf("Expected message ID to be %s, got %s", msg.ID, item.Message.ID)
		}
		if item.FailReason != "处理超时" {
			t.Errorf("Expected fail reason to be '处理超时', got '%s'", item.FailReason)
		}
		if item.SourceQueue != "test-queue" {
			t.Errorf("Expected source queue to be 'test-queue', got '%s'", item.SourceQueue)
		}
		if item.RetryCount != 3 {
			t.Errorf("Expected retry count to be 3, got %d", item.RetryCount)
		}
	})

	// 测试重新入队
	t.Run("Requeue", func(t *testing.T) {
		// 创建测试消息
		msg := models.NewMessage("conv-requeue", "user-requeue", "Requeue test message", models.PriorityHigh)
		msg.RetryCount = 2

		// 推送到死信队列
		err := dlq.Push(msg, "处理失败", "requeue-test")
		if err != nil {
			t.Fatalf("Failed to push message to dead letter queue: %v", err)
		}

		// 获取队列大小
		size := dlq.Size()
		if size < 1 {
			t.Fatalf("Expected dead letter queue size to be at least 1, got %d", size)
		}

		// 重新入队
		err = dlq.Requeue(size-1, manager)
		if err != nil {
			t.Fatalf("Failed to requeue message: %v", err)
		}

		// 验证消息已重新入队
		poppedMsg, err := manager.PopMessage("requeue-test")
		if err != nil {
			t.Fatalf("Failed to pop requeued message: %v", err)
		}

		// 验证消息
		if poppedMsg.ID != msg.ID {
			t.Errorf("Expected requeued message ID to be %s, got %s", msg.ID, poppedMsg.ID)
		}
		if poppedMsg.RetryCount != 0 {
			t.Errorf("Expected retry count to be reset to 0, got %d", poppedMsg.RetryCount)
		}
	})

	// 测试批量重新入队
	t.Run("BatchRequeue", func(t *testing.T) {
		// 清空死信队列
		dlq.Clear()

		// 创建多个测试消息
		for i := 0; i < 5; i++ {
			msg := models.NewMessage("conv-batch-requeue", "user-batch", "Batch requeue test", models.PriorityNormal)
			msg.RetryCount = 1
			dlq.Push(msg, "批量测试", "requeue-test")
		}

		// 获取队列大小
		size := dlq.Size()
		if size != 5 {
			t.Fatalf("Expected dead letter queue size to be 5, got %d", size)
		}

		// 批量重新入队
		indices := []int{0, 2, 4}
		count, err := dlq.BatchRequeue(indices, manager)
		if err != nil {
			t.Fatalf("Failed to batch requeue messages: %v", err)
		}
		if count != 3 {
			t.Errorf("Expected to requeue 3 messages, requeued %d", count)
		}

		// 验证死信队列大小
		newSize := dlq.Size()
		if newSize != 2 {
			t.Errorf("Expected dead letter queue size to be 2 after requeue, got %d", newSize)
		}

		// 验证消息已重新入队
		for i := 0; i < 3; i++ {
			_, err := manager.PopMessage("requeue-test")
			if err != nil {
				t.Fatalf("Failed to pop requeued message %d: %v", i, err)
			}
		}
	})
}
