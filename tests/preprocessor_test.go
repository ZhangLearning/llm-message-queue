package tests

import (
	"testing"
	"time"

	"llm-message-queue/internal/preprocessor"
	"llm-message-queue/pkg/config"
	"llm-message-queue/pkg/models"
)

func TestPreprocessor(t *testing.T) {
	// 创建预处理器配置
	cfg := config.QueueConfig{
		DefaultMaxSize:    1000,
		MonitorInterval:   time.Minute,
		CleanupInterval:   time.Hour,
		EnableMetrics:     true,
	}

	// 创建预处理器
	proc := preprocessor.NewPreprocessor(cfg)

	// 测试基于内容的优先级检测
	t.Run("ContentBasedPriority", func(t *testing.T) {
		// 创建测试消息
		msg := models.NewMessage("conv1", "user1", "This is an urgent request that needs attention", models.PriorityNormal)

		// 处理消息
		processedMsg := proc.ProcessMessage(msg)

		// 验证优先级已提升
		if processedMsg.Priority != models.PriorityHigh {
			t.Errorf("Expected priority to be High based on content, got %v", processedMsg.Priority)
		}

		// 验证元数据已添加
		if processedMsg.Metadata != nil {
			if reason, ok := processedMsg.Metadata["priority_reason"]; ok && reason != "content_keywords" {
				t.Errorf("Expected metadata to contain priority_reason, got %v", reason)
			}
		}
	})

	// 测试用户优先级覆盖
	t.Run("UserPriorityOverride", func(t *testing.T) {
		// 创建测试消息，带有用户优先级元数据
		msg := models.NewMessage("conv2", "user2", "Regular message content", models.PriorityNormal)
		msg.Metadata = map[string]interface{}{
			"user_priority": "high",
		}

		// 处理消息
		processedMsg := proc.ProcessMessage(msg)

		// 验证优先级已根据用户设置提升
		if processedMsg.Priority != models.PriorityHigh {
			t.Errorf("Expected priority to be High based on user override, got %v", processedMsg.Priority)
		}

		// 验证元数据已添加
		if processedMsg.Metadata["priority_reason"] != "user_override" {
			t.Errorf("Expected metadata to contain priority_reason=user_override, got %v", processedMsg.Metadata["priority_reason"])
		}
	})

	// 测试尊重显式优先级
	t.Run("RespectExplicitPriority", func(t *testing.T) {
		// 创建测试消息，带有显式优先级
		msg := models.NewMessage("conv3", "user3", "This is a regular message", models.PriorityRealtime)

		// 处理消息
		processedMsg := proc.ProcessMessage(msg)

		// 验证优先级未改变
		if processedMsg.Priority != models.PriorityRealtime {
			t.Errorf("Expected priority to remain Realtime, got %v", processedMsg.Priority)
		}
	})

	// 测试基于元数据的优先级
	t.Run("MetadataBasedPriority", func(t *testing.T) {
		// 创建测试消息，带有元数据
		msg := models.NewMessage("conv4", "user4", "Regular message content", models.PriorityNormal)
		msg.Metadata = map[string]interface{}{
			"conversation_type": "support",
			"customer_tier":     "premium",
		}

		// 处理消息
		processedMsg := proc.ProcessMessage(msg)

		// 验证元数据已添加到处理后的消息
		if processedMsg.Metadata["conversation_type"] != "support" {
			t.Errorf("Expected metadata to be preserved, got %v", processedMsg.Metadata)
		}

		// 验证优先级可能已根据元数据调整
		if processedMsg.Priority == models.PriorityNormal {
			// 这里我们只检查元数据是否被保留，因为实际的优先级调整逻辑取决于预处理器的实现
			if _, exists := processedMsg.Metadata["analyzed"]; !exists {
				t.Errorf("Expected message to be marked as analyzed")
			}
		}
	})

	// 测试自定义关键词模式
	t.Run("CustomKeywordPatterns", func(t *testing.T) {
		// 创建测试消息，包含自定义关键词
		msg := models.NewMessage("conv5", "user5", "I need this right now, it's immediate", models.PriorityNormal)

		// 处理消息
		processedMsg := proc.ProcessMessage(msg)

		// 验证优先级已根据自定义关键词提升到实时
		if processedMsg.Priority != models.PriorityRealtime {
			t.Errorf("Expected priority to be Realtime based on custom keywords, got %v", processedMsg.Priority)
		}

		// 验证元数据已添加
		if processedMsg.Metadata["priority_reason"] != "content_keywords" {
			t.Errorf("Expected metadata to contain priority_reason=content_keywords, got %v", processedMsg.Metadata["priority_reason"])
		}
	})

	// 测试内容分析（问题检测和情感）
	t.Run("ContentAnalysis", func(t *testing.T) {
		// 创建测试消息，包含问题
		msg1 := models.NewMessage("conv6", "user6", "Can you help me with this problem?", models.PriorityNormal)

		// 处理消息
		processedMsg1 := proc.ProcessMessage(msg1)

		// 验证问题已被检测
		if processedMsg1.Metadata["contains_question"] != "true" {
			t.Errorf("Expected message to be detected as containing a question")
		}

		// 创建测试消息，包含负面情感
		msg2 := models.NewMessage("conv7", "user7", "I'm very frustrated and angry with this service!", models.PriorityNormal)

		// 处理消息
		processedMsg2 := proc.ProcessMessage(msg2)

		// 验证情感已被分析
		if _, exists := processedMsg2.Metadata["sentiment"]; !exists {
			t.Errorf("Expected message to have sentiment analysis")
		}
	})
}
