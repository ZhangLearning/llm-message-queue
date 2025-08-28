package scheduler

import (
	"context"
	"errors"
	"sync"
	"time"

	"llm-message-queue/pkg/config"

	"go.uber.org/zap"
)

// ResourceType 表示资源类型
type ResourceType string

const (
	ResourceTypeCPU    ResourceType = "cpu"
	ResourceTypeGPU    ResourceType = "gpu"
	ResourceTypeMemory ResourceType = "memory"
	ResourceTypeTokens ResourceType = "tokens"
)

// ResourceStatus 表示资源状态
type ResourceStatus string

const (
	ResourceStatusAvailable ResourceStatus = "available"
	ResourceStatusBusy      ResourceStatus = "busy"
	ResourceStatusOffline   ResourceStatus = "offline"
	ResourceStatusError     ResourceStatus = "error"
)

// Resource 表示一个LLM资源
type Resource struct {
	ID            string                 `json:"id"`
	Name          string                 `json:"name"`
	Type          string                 `json:"type"` // 模型类型，如gpt-4, llama-2等
	Capabilities  []string               `json:"capabilities"`
	Status        ResourceStatus         `json:"status"`
	Load          float64                `json:"load"` // 0.0-1.0表示负载程度
	Capacity      map[ResourceType]int64 `json:"capacity"`
	Used          map[ResourceType]int64 `json:"used"`
	Endpoint      string                 `json:"endpoint"`
	LastHeartbeat time.Time              `json:"last_heartbeat"`
	Metadata      map[string]interface{} `json:"metadata"`
}

// ResourceRequest 表示资源请求
type ResourceRequest struct {
	RequestID    string                 `json:"request_id"`
	ModelType    string                 `json:"model_type"`
	Requirements map[ResourceType]int64 `json:"requirements"`
	Priority     int                    `json:"priority"`
	Timeout      time.Duration          `json:"timeout"`
	Capabilities []string               `json:"capabilities"` // 所需的特殊能力，如图像处理
	Metadata     map[string]interface{} `json:"metadata"`
}

// ResourceAllocation 表示资源分配结果
type ResourceAllocation struct {
	RequestID  string    `json:"request_id"`
	ResourceID string    `json:"resource_id"`
	Endpoint   string    `json:"endpoint"`
	Allocated  time.Time `json:"allocated"`
	Expires    time.Time `json:"expires"`
	Token      string    `json:"token"` // 用于验证资源使用权
}

// ResourceSchedulerConfig 资源调度器配置
type ResourceSchedulerConfig struct {
	HeartbeatTimeout    time.Duration // 心跳超时时间
	AllocationTimeout   time.Duration // 资源分配超时时间
	ResourceCheckPeriod time.Duration // 资源检查周期
	EnableAutoScaling   bool          // 是否启用自动扩缩容
	MinResources        int           // 最小资源数量
	MaxResources        int           // 最大资源数量
	ScaleUpThreshold    float64       // 扩容阈值
	ScaleDownThreshold  float64       // 缩容阈值
	ScaleCooldown       time.Duration // 扩缩容冷却时间
}

// ResourceScheduler 负责动态分配和管理LLM资源
type ResourceScheduler struct {
	config ResourceSchedulerConfig
	logger *zap.Logger

	resources     map[string]*Resource
	allocations   map[string]*ResourceAllocation
	pendingQueue  []*ResourceRequest
	resourcesLock sync.RWMutex

	lastScaleAction time.Time

	ctx        context.Context
	cancelFunc context.CancelFunc
}

// NewResourceScheduler 创建新的资源调度器
func NewResourceScheduler(cfg *config.SchedulerConfig, logger *zap.Logger) *ResourceScheduler {
	if logger == nil {
		logger, _ = zap.NewProduction()
	}

	ctx, cancel := context.WithCancel(context.Background())

	// 使用默认值填充缺失的配置字段
	schedulerConfig := ResourceSchedulerConfig{
		HeartbeatTimeout:    30 * time.Second, // 默认值
		AllocationTimeout:   cfg.Timeout, // 使用现有的Timeout字段
		ResourceCheckPeriod: cfg.CheckInterval, // 使用现有的CheckInterval字段
		EnableAutoScaling:   false, // 默认值
		MinResources:        1, // 默认值
		MaxResources:        10, // 默认值
		ScaleUpThreshold:    0.8, // 默认值
		ScaleDownThreshold:  0.2, // 默认值
		ScaleCooldown:       5 * time.Minute, // 默认值
	}

	scheduler := &ResourceScheduler{
		config:       schedulerConfig,
		logger:       logger,
		resources:    make(map[string]*Resource),
		allocations:  make(map[string]*ResourceAllocation),
		pendingQueue: make([]*ResourceRequest, 0),
		ctx:          ctx,
		cancelFunc:   cancel,
	}

	// 启动后台任务
	go scheduler.resourceMonitor()
	go scheduler.pendingRequestProcessor()

	return scheduler
}

// RegisterResource 注册新的资源
func (s *ResourceScheduler) RegisterResource(resource *Resource) error {
	s.resourcesLock.Lock()
	defer s.resourcesLock.Unlock()

	// 检查资源ID是否已存在
	if _, exists := s.resources[resource.ID]; exists {
		return errors.New("resource with this ID already exists")
	}

	// 设置初始状态
	resource.Status = ResourceStatusAvailable
	resource.Load = 0.0
	resource.LastHeartbeat = time.Now()

	// 添加到资源池
	s.resources[resource.ID] = resource
	s.logger.Info("Resource registered",
		zap.String("resourceID", resource.ID),
		zap.String("type", resource.Type))

	// 处理等待中的请求
	go s.processPendingRequests()

	return nil
}

// UpdateResourceStatus 更新资源状态
func (s *ResourceScheduler) UpdateResourceStatus(resourceID string, status ResourceStatus, load float64) error {
	s.resourcesLock.Lock()
	defer s.resourcesLock.Unlock()

	resource, exists := s.resources[resourceID]
	if !exists {
		return errors.New("resource not found")
	}

	resource.Status = status
	resource.Load = load
	resource.LastHeartbeat = time.Now()

	return nil
}

// Heartbeat 资源心跳更新
func (s *ResourceScheduler) Heartbeat(resourceID string, load float64, usedResources map[ResourceType]int64) error {
	s.resourcesLock.Lock()
	defer s.resourcesLock.Unlock()

	resource, exists := s.resources[resourceID]
	if !exists {
		return errors.New("resource not found")
	}

	resource.LastHeartbeat = time.Now()
	resource.Load = load

	if usedResources != nil {
		resource.Used = usedResources
	}

	return nil
}

// RequestResource 请求分配资源
func (s *ResourceScheduler) RequestResource(req *ResourceRequest) (*ResourceAllocation, error) {
	// 尝试立即分配资源
	allocation, err := s.tryAllocateResource(req)
	if err == nil {
		return allocation, nil
	}

	// 如果无法立即分配，将请求加入等待队列
	s.resourcesLock.Lock()
	defer s.resourcesLock.Unlock()

	// 按优先级插入队列
	inserted := false
	for i, pendingReq := range s.pendingQueue {
		if req.Priority > pendingReq.Priority {
			// 在此位置插入新请求
			s.pendingQueue = append(s.pendingQueue[:i], append([]*ResourceRequest{req}, s.pendingQueue[i:]...)...)
			inserted = true
			break
		}
	}

	if !inserted {
		// 添加到队列末尾
		s.pendingQueue = append(s.pendingQueue, req)
	}

	s.logger.Info("Resource request queued",
		zap.String("requestID", req.RequestID),
		zap.Int("priority", req.Priority),
		zap.Int("queueLength", len(s.pendingQueue)))

	return nil, errors.New("no resources available, request queued")
}

// ReleaseResource 释放已分配的资源
func (s *ResourceScheduler) ReleaseResource(requestID string) error {
	s.resourcesLock.Lock()
	defer s.resourcesLock.Unlock()

	allocation, exists := s.allocations[requestID]
	if !exists {
		return errors.New("allocation not found")
	}

	// 从分配表中删除
	delete(s.allocations, requestID)

	// 更新资源状态
	resource, exists := s.resources[allocation.ResourceID]
	if exists {
		// 如果资源仍然存在，更新其负载
		resource.Load = calculateNewLoad(resource)
		if resource.Load < 0.01 {
			resource.Status = ResourceStatusAvailable
		}
	}

	s.logger.Info("Resource released",
		zap.String("requestID", requestID),
		zap.String("resourceID", allocation.ResourceID))

	// 处理等待中的请求
	go s.processPendingRequests()

	return nil
}

// GetResourceStats 获取资源统计信息
func (s *ResourceScheduler) GetResourceStats() map[string]interface{} {
	s.resourcesLock.RLock()
	defer s.resourcesLock.RUnlock()

	stats := make(map[string]interface{})

	// 资源统计
	resourceStats := make(map[string]int)
	resourceStats["total"] = len(s.resources)
	resourceStats["available"] = 0
	resourceStats["busy"] = 0
	resourceStats["offline"] = 0
	resourceStats["error"] = 0

	// 按类型统计资源
	resourceByType := make(map[string]int)

	// 计算平均负载
	totalLoad := 0.0
	activeResources := 0

	for _, resource := range s.resources {
		// 按状态统计
		switch resource.Status {
		case ResourceStatusAvailable:
			resourceStats["available"]++
		case ResourceStatusBusy:
			resourceStats["busy"]++
		case ResourceStatusOffline:
			resourceStats["offline"]++
		case ResourceStatusError:
			resourceStats["error"]++
		}

		// 按类型统计
		resourceByType[resource.Type]++

		// 计算负载
		if resource.Status != ResourceStatusOffline && resource.Status != ResourceStatusError {
			totalLoad += resource.Load
			activeResources++
		}
	}

	// 计算平均负载
	averageLoad := 0.0
	if activeResources > 0 {
		averageLoad = totalLoad / float64(activeResources)
	}

	stats["resources"] = resourceStats
	stats["resource_by_type"] = resourceByType
	stats["average_load"] = averageLoad
	stats["allocations"] = len(s.allocations)
	stats["pending_requests"] = len(s.pendingQueue)

	return stats
}

// Stop 停止调度器
func (s *ResourceScheduler) Stop() {
	s.cancelFunc()
}

// 内部方法：尝试分配资源
func (s *ResourceScheduler) tryAllocateResource(req *ResourceRequest) (*ResourceAllocation, error) {
	s.resourcesLock.Lock()
	defer s.resourcesLock.Unlock()

	// 查找最佳匹配的资源
	var bestResource *Resource
	lowestLoad := 1.1 // 初始值设置为大于1，确保任何负载都会更新它

	for _, resource := range s.resources {
		// 检查资源是否可用
		if resource.Status != ResourceStatusAvailable && resource.Status != ResourceStatusBusy {
			continue
		}

		// 检查模型类型是否匹配
		if resource.Type != req.ModelType {
			continue
		}

		// 检查资源能力是否满足要求
		if !hasRequiredCapabilities(resource, req.Capabilities) {
			continue
		}

		// 检查资源容量是否足够
		if !hasEnoughCapacity(resource, req.Requirements) {
			continue
		}

		// 选择负载最低的资源
		if resource.Load < lowestLoad {
			bestResource = resource
			lowestLoad = resource.Load
		}
	}

	if bestResource == nil {
		return nil, errors.New("no suitable resource found")
	}

	// 创建资源分配
	allocation := &ResourceAllocation{
		RequestID:  req.RequestID,
		ResourceID: bestResource.ID,
		Endpoint:   bestResource.Endpoint,
		Allocated:  time.Now(),
		Expires:    time.Now().Add(req.Timeout),
		Token:      generateAllocationToken(req.RequestID, bestResource.ID),
	}

	// 更新资源状态
	updateResourceAfterAllocation(bestResource, req.Requirements)

	// 保存分配信息
	s.allocations[req.RequestID] = allocation

	s.logger.Info("Resource allocated",
		zap.String("requestID", req.RequestID),
		zap.String("resourceID", bestResource.ID),
		zap.Float64("resourceLoad", bestResource.Load))

	return allocation, nil
}

// 后台任务：监控资源状态
func (s *ResourceScheduler) resourceMonitor() {
	ticker := time.NewTicker(s.config.ResourceCheckPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.checkResourceStatus()
			s.checkAllocations()
			s.checkAutoScaling()
		case <-s.ctx.Done():
			return
		}
	}
}

// 后台任务：处理等待中的请求
func (s *ResourceScheduler) pendingRequestProcessor() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.processPendingRequests()
		case <-s.ctx.Done():
			return
		}
	}
}

// 处理等待中的资源请求
func (s *ResourceScheduler) processPendingRequests() {
	s.resourcesLock.Lock()

	// 如果没有等待的请求，直接返回
	if len(s.pendingQueue) == 0 {
		s.resourcesLock.Unlock()
		return
	}

	// 获取队列的副本并清空原队列
	pendingRequests := make([]*ResourceRequest, len(s.pendingQueue))
	copy(pendingRequests, s.pendingQueue)
	s.pendingQueue = s.pendingQueue[:0]

	s.resourcesLock.Unlock()

	// 尝试为每个等待的请求分配资源
	var remainingRequests []*ResourceRequest

	for _, req := range pendingRequests {
		// 检查请求是否已超时
		if req.Timeout > 0 && time.Since(req.Metadata["queuedAt"].(time.Time)) > req.Timeout {
			s.logger.Warn("Request timeout while in queue",
				zap.String("requestID", req.RequestID))
			continue
		}

		// 尝试分配资源
		_, err := s.tryAllocateResource(req)
		if err != nil {
			// 如果分配失败，将请求放回队列
			remainingRequests = append(remainingRequests, req)
		}
	}

	// 将未处理的请求放回队列
	if len(remainingRequests) > 0 {
		s.resourcesLock.Lock()
		s.pendingQueue = append(s.pendingQueue, remainingRequests...)
		s.resourcesLock.Unlock()
	}
}

// 检查资源状态
func (s *ResourceScheduler) checkResourceStatus() {
	s.resourcesLock.Lock()
	defer s.resourcesLock.Unlock()

	now := time.Now()

	for id, resource := range s.resources {
		// 检查心跳超时
		if now.Sub(resource.LastHeartbeat) > s.config.HeartbeatTimeout {
			// 将资源标记为离线
			resource.Status = ResourceStatusOffline
			s.logger.Warn("Resource marked as offline due to heartbeat timeout",
				zap.String("resourceID", id))
		}
	}
}

// 检查资源分配状态
func (s *ResourceScheduler) checkAllocations() {
	s.resourcesLock.Lock()
	defer s.resourcesLock.Unlock()

	now := time.Now()

	for id, allocation := range s.allocations {
		// 检查分配是否已过期
		if now.After(allocation.Expires) {
			// 从分配表中删除
			delete(s.allocations, id)

			// 更新资源状态
			resource, exists := s.resources[allocation.ResourceID]
			if exists {
				// 如果资源仍然存在，更新其负载
				resource.Load = calculateNewLoad(resource)
				if resource.Load < 0.01 {
					resource.Status = ResourceStatusAvailable
				}
			}

			s.logger.Info("Allocation expired and released",
				zap.String("requestID", id),
				zap.String("resourceID", allocation.ResourceID))
		}
	}
}

// 检查是否需要自动扩缩容
func (s *ResourceScheduler) checkAutoScaling() {
	if !s.config.EnableAutoScaling {
		return
	}

	// 检查冷却期
	if time.Since(s.lastScaleAction) < s.config.ScaleCooldown {
		return
	}

	s.resourcesLock.RLock()
	defer s.resourcesLock.RUnlock()

	// 计算平均负载
	totalLoad := 0.0
	activeResources := 0

	for _, resource := range s.resources {
		if resource.Status != ResourceStatusOffline && resource.Status != ResourceStatusError {
			totalLoad += resource.Load
			activeResources++
		}
	}

	averageLoad := 0.0
	if activeResources > 0 {
		averageLoad = totalLoad / float64(activeResources)
	}

	// 检查是否需要扩容
	if (averageLoad > s.config.ScaleUpThreshold || len(s.pendingQueue) > 0) &&
		activeResources < s.config.MaxResources {
		// 触发扩容
		s.triggerScaleUp()
		s.lastScaleAction = time.Now()
		return
	}

	// 检查是否需要缩容
	if averageLoad < s.config.ScaleDownThreshold &&
		len(s.pendingQueue) == 0 &&
		activeResources > s.config.MinResources {
		// 触发缩容
		s.triggerScaleDown()
		s.lastScaleAction = time.Now()
	}
}

// 触发扩容
func (s *ResourceScheduler) triggerScaleUp() {
	// 这里应该实现实际的扩容逻辑，例如：
	// 1. 调用云服务API创建新实例
	// 2. 启动新的容器
	// 3. 通知外部系统增加资源

	s.logger.Info("Triggered scale up action")

	// 实际实现会根据具体的部署环境和基础设施而定
}

// 触发缩容
func (s *ResourceScheduler) triggerScaleDown() {
	// 这里应该实现实际的缩容逻辑，例如：
	// 1. 选择负载最低的资源
	// 2. 确保没有正在处理的请求
	// 3. 安全地关闭资源

	s.logger.Info("Triggered scale down action")

	// 实际实现会根据具体的部署环境和基础设施而定
}

// 辅助函数：检查资源是否具有所需的能力
func hasRequiredCapabilities(resource *Resource, requiredCapabilities []string) bool {
	if len(requiredCapabilities) == 0 {
		return true
	}

	for _, required := range requiredCapabilities {
		found := false
		for _, capability := range resource.Capabilities {
			if capability == required {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

// 辅助函数：检查资源是否有足够的容量
func hasEnoughCapacity(resource *Resource, requirements map[ResourceType]int64) bool {
	for resType, required := range requirements {
		capacity, exists := resource.Capacity[resType]
		if !exists {
			return false
		}

		used, exists := resource.Used[resType]
		if !exists {
			used = 0
		}

		if capacity-used < required {
			return false
		}
	}

	return true
}

// 辅助函数：更新资源分配后的状态
func updateResourceAfterAllocation(resource *Resource, requirements map[ResourceType]int64) {
	// 更新已使用的资源
	for resType, required := range requirements {
		if resource.Used == nil {
			resource.Used = make(map[ResourceType]int64)
		}
		resource.Used[resType] += required
	}

	// 计算新的负载
	resource.Load = calculateResourceLoad(resource)

	// 更新状态
	if resource.Load > 0.9 {
		resource.Status = ResourceStatusBusy
	}
}

// 辅助函数：计算资源负载
func calculateResourceLoad(resource *Resource) float64 {
	if len(resource.Capacity) == 0 {
		return 0.0
	}

	totalLoadPercentage := 0.0
	resourceTypes := 0

	for resType, capacity := range resource.Capacity {
		if capacity <= 0 {
			continue
		}

		used, exists := resource.Used[resType]
		if !exists {
			used = 0
		}

		loadPercentage := float64(used) / float64(capacity)
		totalLoadPercentage += loadPercentage
		resourceTypes++
	}

	if resourceTypes == 0 {
		return 0.0
	}

	return totalLoadPercentage / float64(resourceTypes)
}

// 辅助函数：计算释放资源后的新负载
func calculateNewLoad(resource *Resource) float64 {
	// 这里可以实现更复杂的负载计算逻辑
	// 简单起见，我们直接返回当前负载的一半
	return resource.Load * 0.5
}

// 辅助函数：生成分配令牌
func generateAllocationToken(requestID, resourceID string) string {
	// 实际实现应该使用安全的令牌生成方法
	// 这里简化处理
	return requestID + "-" + resourceID + "-" + time.Now().Format(time.RFC3339)
}

// 辅助函数：从等待队列中移除请求
func (s *ResourceScheduler) removeFromPendingQueue(requestID string) {
	s.resourcesLock.Lock()
	defer s.resourcesLock.Unlock()

	for i, req := range s.pendingQueue {
		if req.RequestID == requestID {
			// 从队列中移除
			s.pendingQueue = append(s.pendingQueue[:i], s.pendingQueue[i+1:]...)
			break
		}
	}
}

// 辅助函数：获取资源分配信息
func (s *ResourceScheduler) GetAllocation(requestID string) (*ResourceAllocation, error) {
	s.resourcesLock.RLock()
	defer s.resourcesLock.RUnlock()

	allocation, exists := s.allocations[requestID]
	if !exists {
		return nil, errors.New("allocation not found")
	}

	return allocation, nil
}

// 辅助函数：获取资源信息
func (s *ResourceScheduler) GetResource(resourceID string) (*Resource, error) {
	s.resourcesLock.RLock()
	defer s.resourcesLock.RUnlock()

	resource, exists := s.resources[resourceID]
	if !exists {
		return nil, errors.New("resource not found")
	}

	return resource, nil
}

// 辅助函数：获取所有资源
func (s *ResourceScheduler) GetAllResources() []*Resource {
	s.resourcesLock.RLock()
	defer s.resourcesLock.RUnlock()

	resources := make([]*Resource, 0, len(s.resources))
	for _, resource := range s.resources {
		resources = append(resources, resource)
	}

	return resources
}

// 辅助函数：获取所有分配
func (s *ResourceScheduler) GetAllAllocations() []*ResourceAllocation {
	s.resourcesLock.RLock()
	defer s.resourcesLock.RUnlock()

	allocations := make([]*ResourceAllocation, 0, len(s.allocations))
	for _, allocation := range s.allocations {
		allocations = append(allocations, allocation)
	}

	return allocations
}

// 辅助函数：获取等待队列长度
func (s *ResourceScheduler) GetPendingQueueLength() int {
	s.resourcesLock.RLock()
	defer s.resourcesLock.RUnlock()

	return len(s.pendingQueue)
}
