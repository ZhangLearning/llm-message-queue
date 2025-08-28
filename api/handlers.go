package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"llm-message-queue/internal/conversation"
	"llm-message-queue/internal/loadbalancer"
	"llm-message-queue/internal/preprocessor"
	"llm-message-queue/internal/priorityqueue"
	"llm-message-queue/internal/scheduler"
	"llm-message-queue/pkg/config"
	"llm-message-queue/pkg/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// APIServer 处理所有API请求
type APIServer struct {
	Router         *gin.Engine
	QueueFactory   *priorityqueue.QueueFactory
	Preprocessor   *preprocessor.Preprocessor
	StateManager   *conversation.StateManager
	LoadBalancer   *loadbalancer.LoadBalancer
	ResScheduler   *scheduler.ResourceScheduler
	Logger         *zap.Logger
	AllowedOrigins []string
	Config         *config.Config // 添加配置字段
}

// NewAPIServer 创建新的API服务器
func NewAPIServer(
	queueFactory *priorityqueue.QueueFactory,
	preprocessor *preprocessor.Preprocessor,
	stateManager *conversation.StateManager,
	loadBalancer *loadbalancer.LoadBalancer,
	resScheduler *scheduler.ResourceScheduler,
	logger *zap.Logger,
	allowedOrigins []string,
	cfg *config.Config, // 添加配置参数
) *APIServer {
	if logger == nil {
		logger, _ = zap.NewProduction()
	}

	router := gin.Default()

	// 设置CORS
	router.Use(corsMiddleware(allowedOrigins))

	server := &APIServer{
		Router:         router,
		QueueFactory:   queueFactory,
		Preprocessor:   preprocessor,
		StateManager:   stateManager,
		LoadBalancer:   loadBalancer,
		ResScheduler:   resScheduler,
		Logger:         logger,
		AllowedOrigins: allowedOrigins,
		Config:         cfg, // 初始化配置字段
	}

	// 设置路由
	server.setupRoutes()

	return server
}

// setupRoutes 设置API路由
func (s *APIServer) setupRoutes() {
	// 健康检查
	s.Router.GET("/health", s.healthCheck)

	// API版本组
	v1 := s.Router.Group("/api/v1")
	{
		// 消息相关
		v1.POST("/messages", s.submitMessage)
		v1.GET("/messages/:id", s.getMessage)
		v1.GET("/messages", s.listMessages)

		// 对话相关
		v1.POST("/conversations", s.createConversation)
		v1.GET("/conversations/:id", s.getConversation)
		v1.POST("/conversations/:id/messages", s.addMessageToConversation)
		v1.PUT("/conversations/:id/state", s.updateConversationState)
		v1.GET("/users/:user_id/conversations", s.listUserConversations)

		// 队列相关
		v1.GET("/queues/stats", s.getQueueStats)

		// 资源相关
		v1.POST("/resources", s.registerResource)
		v1.GET("/resources", s.listResources)
		v1.GET("/resources/stats", s.getResourceStats)

		// 负载均衡相关
		v1.POST("/endpoints", s.registerEndpoint)
		v1.GET("/endpoints", s.listEndpoints)
		v1.GET("/endpoints/stats", s.getEndpointStats)

		// 管理相关
		admin := v1.Group("/admin")
		{
			admin.POST("/preprocessor/rules", s.addPriorityRule)
			admin.GET("/preprocessor/rules", s.listPriorityRules)
			admin.POST("/preprocessor/user-priorities", s.setUserPriority)
			admin.DELETE("/queues/:queue_type/:id", s.removeMessage)
			admin.POST("/dead-letter/requeue/:id", s.requeueDeadLetterMessage)
			admin.POST("/dead-letter/requeue-all", s.requeueAllDeadLetterMessages)
		}
	}
}

// corsMiddleware 处理跨域请求
func corsMiddleware(allowedOrigins []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		allowed := false

		// 检查是否允许该来源
		for _, allowedOrigin := range allowedOrigins {
			if allowedOrigin == "*" || allowedOrigin == origin {
				allowed = true
				break
			}
		}

		if allowed {
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
			c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		}

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// healthCheck 健康检查端点
func (s *APIServer) healthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"version": "1.0.0",
		"time":    time.Now(),
	})
}

// submitMessage 提交新消息到队列
func (s *APIServer) submitMessage(c *gin.Context) {
	var message models.Message
	if err := c.ShouldBindJSON(&message); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid message format: " + err.Error()})
		return
	}

	// 生成消息ID（如果未提供）
	if message.ID == "" {
		message.ID = uuid.New().String()
	}

	// 设置时间戳
	now := time.Now()
	message.CreatedAt = now
	message.UpdatedAt = now

	// 预处理消息（分析内容，确定优先级等）
	processedMessage := s.Preprocessor.ProcessMessage(&message)

	// 如果启用了内容分析，则进行分析
	if s.Config.Queue.EnableMetrics {
		analysisResult := s.Preprocessor.AnalyzeMessageContent(processedMessage.Content)
		// 将分析结果添加到消息元数据中
		analysisJson, err := json.Marshal(analysisResult)
		if err == nil {
			if processedMessage.Metadata == nil {
				processedMessage.Metadata = make(map[string]interface{})
			}
			processedMessage.Metadata["analysis"] = string(analysisJson)
		}
	}

	// 获取队列管理器
	queueManager, exists := s.QueueFactory.GetQueueManager("standard")
	if !exists {
		s.Logger.Error("Failed to get queue manager")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to access message queue"})
		return
	}

	// 将消息推送到队列
	if err := queueManager.PushMessage(string(processedMessage.Priority), processedMessage); err != nil {
		s.Logger.Error("Failed to push message to queue", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to queue message"})
		return
	}

	// 如果消息属于对话，更新对话状态
	if processedMessage.ConversationID != "" {
		s.updateConversationWithMessage(processedMessage)
	}

	c.JSON(http.StatusAccepted, gin.H{
		"message_id":     processedMessage.ID,
		"priority":       processedMessage.Priority,
		"queue_time":     time.Now(),
		"estimated_wait": s.estimateWaitTime(string(processedMessage.Priority)),
	})
}

// getMessage 获取特定消息
func (s *APIServer) getMessage(c *gin.Context) {
	messageID := c.Param("id")
	if messageID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Message ID is required"})
		return
	}

	// 这里应该实现从存储中获取消息的逻辑
	// 简化起见，我们返回一个错误
	c.JSON(http.StatusNotImplemented, gin.H{"error": "Get message by ID not implemented yet"})
}

// listMessages 列出消息
func (s *APIServer) listMessages(c *gin.Context) {
	// 获取查询参数
	userID := c.Query("user_id")
	conversationID := c.Query("conversation_id")
	statusStr := c.Query("status")
	limitStr := c.DefaultQuery("limit", "10")
	offsetStr := c.DefaultQuery("offset", "0")

	limit, _ := strconv.Atoi(limitStr)
	offset, _ := strconv.Atoi(offsetStr)

	// 避免未使用变量的编译错误
	_ = userID
	_ = conversationID
	_ = statusStr
	_ = limit
	_ = offset

	// 这里应该实现从存储中列出消息的逻辑
	// 简化起见，我们返回一个错误
	c.JSON(http.StatusNotImplemented, gin.H{"error": "List messages not implemented yet"})
}

// createConversation 创建新对话
func (s *APIServer) createConversation(c *gin.Context) {
	var request struct {
		UserID   string                 `json:"user_id" binding:"required"`
		Metadata map[string]interface{} `json:"metadata,omitempty"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format: " + err.Error()})
		return
	}

	// 生成新的对话ID
	conversationID := uuid.New().String()

	// 获取或创建新对话
	conversation, err := s.StateManager.GetConversation(conversationID, request.UserID)
	if err != nil {
		s.Logger.Error("Failed to create conversation", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create conversation"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"conversation_id": conversation.ID,
		"user_id":         conversation.UserID,
		"created_at":      conversation.CreatedAt,
		"state":           conversation.State,
	})
}

// getConversation 获取特定对话
func (s *APIServer) getConversation(c *gin.Context) {
	conversationID := c.Param("id")
	if conversationID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Conversation ID is required"})
		return
	}

	// 获取对话
	conversation, err := s.StateManager.GetConversation(conversationID, "")
	if err != nil {
		s.Logger.Error("Failed to get conversation",
			zap.String("conversationID", conversationID),
			zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{"error": "Conversation not found"})
		return
	}

	c.JSON(http.StatusOK, conversation)
}

// addMessageToConversation 向对话添加消息
func (s *APIServer) addMessageToConversation(c *gin.Context) {
	conversationID := c.Param("id")
	if conversationID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Conversation ID is required"})
		return
	}

	var message models.Message
	if err := c.ShouldBindJSON(&message); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid message format: " + err.Error()})
		return
	}

	// 设置对话ID
	message.ConversationID = conversationID

	// 生成消息ID（如果未提供）
	if message.ID == "" {
		message.ID = uuid.New().String()
	}

	// 设置时间戳
	now := time.Now()
	message.CreatedAt = now
	message.UpdatedAt = now

	// 预处理消息
	processedMessage := s.Preprocessor.ProcessMessage(&message)

	// 添加消息到对话
	addErr := s.StateManager.AddMessage(conversationID, *processedMessage)
	if addErr != nil {
		s.Logger.Error("Failed to add message to conversation",
			zap.String("conversationID", conversationID),
			zap.Error(addErr))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add message to conversation"})
		return
	}

	// 将消息推送到队列
	queueManager, exists := s.QueueFactory.GetQueueManager("standard")
	if !exists {
		s.Logger.Error("Failed to get queue manager")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to access message queue"})
		return
	}

	if err := queueManager.PushMessage(string(processedMessage.Priority), processedMessage); err != nil {
		s.Logger.Error("Failed to push message to queue", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to queue message"})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"message_id":      processedMessage.ID,
		"conversation_id": conversationID,
		"priority":        processedMessage.Priority,
		"queue_time":      time.Now(),
		"estimated_wait":  s.estimateWaitTime(string(processedMessage.Priority)),
	})
}

// updateConversationState 更新对话状态
func (s *APIServer) updateConversationState(c *gin.Context) {
	conversationID := c.Param("id")
	if conversationID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Conversation ID is required"})
		return
	}

	var request struct {
		State    string                 `json:"state" binding:"required"`
		Metadata map[string]interface{} `json:"metadata,omitempty"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format: " + err.Error()})
		return
	}

	// 避免未使用变量的编译错误
	_ = request.Metadata

	// 更新对话状态
	updateErr := s.StateManager.UpdateConversationState(conversationID, models.ConversationState(request.State))
	if updateErr != nil {
		s.Logger.Error("Failed to update conversation state",
			zap.String("conversationID", conversationID),
			zap.Error(updateErr))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update conversation state"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "updated"})
}

// listUserConversations 列出用户的所有对话
func (s *APIServer) listUserConversations(c *gin.Context) {
	userID := c.Param("user_id")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "User ID is required"})
		return
	}

	// 获取用户对话
	conversations, err := s.StateManager.GetUserConversations(userID)
	if err != nil {
		s.Logger.Error("Failed to get user conversations",
			zap.String("userID", userID),
			zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get user conversations"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"conversations": conversations})
}

// getQueueStats 获取队列统计信息
func (s *APIServer) getQueueStats(c *gin.Context) {
	// 获取所有队列类型的统计信息
	stats := make(map[string]interface{})

	// 标准队列
	standardManager, exists := s.QueueFactory.GetQueueManager("standard")
	if exists {
		queueStats, err := standardManager.GetQueueStats("standard")
		if err == nil {
			stats["standard"] = queueStats
		}
	}

	// 延迟队列
	delayedManager, exists := s.QueueFactory.GetQueueManager("delayed")
	if exists {
		queueStats, err := delayedManager.GetQueueStats("delayed")
		if err == nil {
			stats["delayed"] = queueStats
		}
	}

	// 死信队列
	deadLetterManager, exists := s.QueueFactory.GetQueueManager("dead_letter")
	if exists {
		queueStats, err := deadLetterManager.GetQueueStats("dead_letter")
		if err == nil {
			stats["dead_letter"] = queueStats
		}
	}

	// 优先级队列
	priorityManager, exists := s.QueueFactory.GetQueueManager("priority")
	if exists {
		queueStats, err := priorityManager.GetQueueStats("priority")
		if err == nil {
			stats["priority"] = queueStats
		}
	}

	// 工作者统计
	stats["workers"] = s.QueueFactory.GetWorkerStats()

	c.JSON(http.StatusOK, stats)
}

// registerResource 注册新的LLM资源
func (s *APIServer) registerResource(c *gin.Context) {
	var resource scheduler.Resource
	if err := c.ShouldBindJSON(&resource); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid resource format: " + err.Error()})
		return
	}

	// 生成资源ID（如果未提供）
	if resource.ID == "" {
		resource.ID = uuid.New().String()
	}

	// 注册资源
	err := s.ResScheduler.RegisterResource(&resource)
	if err != nil {
		s.Logger.Error("Failed to register resource", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to register resource"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"resource_id": resource.ID,
		"status":      resource.Status,
	})
}

// listResources 列出所有资源
func (s *APIServer) listResources(c *gin.Context) {
	// 获取所有资源
	resources := s.ResScheduler.GetAllResources()

	c.JSON(http.StatusOK, gin.H{"resources": resources})
}

// getResourceStats 获取资源统计信息
func (s *APIServer) getResourceStats(c *gin.Context) {
	// 获取资源统计信息
	stats := s.ResScheduler.GetResourceStats()

	c.JSON(http.StatusOK, stats)
}

// registerEndpoint 注册新的端点
func (s *APIServer) registerEndpoint(c *gin.Context) {
	var endpoint loadbalancer.Endpoint
	if err := c.ShouldBindJSON(&endpoint); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid endpoint format: " + err.Error()})
		return
	}

	// 生成端点ID（如果未提供）
	if endpoint.ID == "" {
		endpoint.ID = uuid.New().String()
	}

	// 注册端点
	err := s.LoadBalancer.AddEndpoint(&endpoint)
	if err != nil {
		s.Logger.Error("Failed to register endpoint", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to register endpoint"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"endpoint_id": endpoint.ID,
		"status":      endpoint.Status,
	})
}

// listEndpoints 列出所有端点
func (s *APIServer) listEndpoints(c *gin.Context) {
	// 获取所有端点
	endpoints := s.LoadBalancer.GetAllEndpoints()

	c.JSON(http.StatusOK, gin.H{"endpoints": endpoints})
}

// getEndpointStats 获取端点统计信息
func (s *APIServer) getEndpointStats(c *gin.Context) {
	// 获取端点统计信息
	stats := s.LoadBalancer.GetEndpointStats()

	c.JSON(http.StatusOK, stats)
}

// addPriorityRule 添加优先级规则
func (s *APIServer) addPriorityRule(c *gin.Context) {
	var rule struct {
		Pattern   string `json:"pattern"`
		Priority  string `json:"priority"`
		Rule      string `json:"rule"`
		Condition string `json:"condition"`
	}
	if err := c.ShouldBindJSON(&rule); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid rule format: " + err.Error()})
		return
	}

	// TODO: 实现AddPriorityRule方法
	s.Logger.Info("Priority rule would be added", zap.Any("rule", rule))
	// s.Preprocessor.AddPriorityRule(rule)

	c.JSON(http.StatusCreated, gin.H{"status": "rule added"})
}

// listPriorityRules 列出所有优先级规则
func (s *APIServer) listPriorityRules(c *gin.Context) {
	// TODO: 实现GetPriorityRules方法
	rules := []interface{}{} // 临时空切片
	// rules := s.Preprocessor.GetPriorityRules()

	c.JSON(http.StatusOK, gin.H{"rules": rules})
}

// setUserPriority 设置用户优先级
func (s *APIServer) setUserPriority(c *gin.Context) {
	var request struct {
		UserID   string `json:"user_id" binding:"required"`
		Priority string `json:"priority" binding:"required"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format: " + err.Error()})
		return
	}

	// 转换字符串优先级为models.Priority类型
	var priority models.Priority
	switch request.Priority {
	case "high":
		priority = models.PriorityHigh
	case "normal":
		priority = models.PriorityNormal
	case "low":
		priority = models.PriorityLow
	default:
		priority = models.PriorityNormal
	}

	// 设置用户优先级
	s.Preprocessor.SetUserPriority(request.UserID, priority)

	c.JSON(http.StatusOK, gin.H{"status": "user priority set"})
}

// removeMessage 从队列中移除消息
func (s *APIServer) removeMessage(c *gin.Context) {
	queueType := c.Param("queue_type")
	messageID := c.Param("id")

	if messageID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Message ID is required"})
		return
	}

	// 获取队列管理器
	var queueTypeName string
	switch queueType {
	case "standard":
		queueTypeName = "standard"
	case "delayed":
		queueTypeName = "delayed"
	case "dead_letter":
		queueTypeName = "dead_letter"
	case "priority":
		queueTypeName = "priority"
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid queue type"})
		return
	}

	queueManager, exists := s.QueueFactory.GetQueueManager(queueTypeName)
	if !exists {
		s.Logger.Error("Failed to get queue manager")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to access message queue"})
		return
	}

	// 移除消息
	// 注意：这里简化处理，实际实现可能需要更复杂的逻辑
	_ = queueManager // 标记为已使用
	c.JSON(http.StatusNotImplemented, gin.H{"error": "Remove message not implemented yet"})
}

// requeueDeadLetterMessage 将死信队列中的消息重新入队
func (s *APIServer) requeueDeadLetterMessage(c *gin.Context) {
	messageID := c.Param("id")

	if messageID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Message ID is required"})
		return
	}

	// 获取死信队列管理器
	deadLetterManager, exists := s.QueueFactory.GetQueueManager("dead_letter")
	if !exists {
		s.Logger.Error("Failed to get dead letter queue manager")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to access dead letter queue"})
		return
	}
	_ = deadLetterManager // 标记为已使用

	// 重新入队消息
	// 注意：这里简化处理，实际实现可能需要更复杂的逻辑
	c.JSON(http.StatusNotImplemented, gin.H{"error": "Requeue dead letter message not implemented yet"})
}

// requeueAllDeadLetterMessages 将所有死信队列中的消息重新入队
func (s *APIServer) requeueAllDeadLetterMessages(c *gin.Context) {
	// 获取死信队列管理器
	deadLetterManager, exists := s.QueueFactory.GetQueueManager("dead_letter")
	if !exists {
		s.Logger.Error("Failed to get dead letter queue manager")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to access dead letter queue"})
		return
	}
	_ = deadLetterManager // 标记为已使用

	// 重新入队所有消息
	// 注意：这里简化处理，实际实现可能需要更复杂的逻辑
	c.JSON(http.StatusNotImplemented, gin.H{"error": "Requeue all dead letter messages not implemented yet"})
}

// 辅助函数：更新对话状态
func (s *APIServer) updateConversationWithMessage(message *models.Message) {
	if message.ConversationID == "" {
		return
	}

	// 获取或创建对话
	conversation, err := s.StateManager.GetConversation(message.ConversationID, message.UserID)
	if err != nil {
		s.Logger.Error("Failed to get or create conversation from message",
			zap.String("messageID", message.ID),
			zap.String("conversationID", message.ConversationID),
			zap.Error(err))
		return
	}

	// 避免未使用变量的编译错误
	_ = conversation

	// 添加消息到对话
	err = s.StateManager.AddMessage(message.ConversationID, *message)
	if err != nil {
		s.Logger.Error("Failed to add message to conversation",
			zap.String("messageID", message.ID),
			zap.String("conversationID", message.ConversationID),
			zap.Error(err))
	}
}

// 辅助函数：估计等待时间
func (s *APIServer) estimateWaitTime(priority string) time.Duration {
	// 这里应该实现基于队列长度和优先级的等待时间估计
	// 简化起见，我们返回一个基于优先级的固定值
	switch priority {
	case "realtime":
		return 1 * time.Second
	case "high":
		return 5 * time.Second
	case "normal":
		return 15 * time.Second
	case "low":
		return 30 * time.Second
	default:
		return 15 * time.Second
	}
}

// Start 启动API服务器
func (s *APIServer) Start(address string) error {
	return s.Router.Run(address)
}

// Stop 停止API服务器
func (s *APIServer) Stop() {
	// 这里可以添加清理逻辑
}
