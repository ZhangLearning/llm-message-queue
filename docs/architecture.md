# LLM消息队列系统架构设计文档

## 1. 系统概述

### 1.1 项目简介
LLM消息队列系统是一个专为大语言模型(LLM)设计的高性能消息队列服务，提供多级优先级处理、动态资源调度、对话状态管理和负载均衡等核心功能。

### 1.2 设计目标
- **高性能**: 支持高并发消息处理，优化LLM推理任务的吞吐量
- **智能调度**: 基于消息内容和系统负载的动态优先级调度
- **可扩展性**: 支持水平扩展和微服务架构
- **可靠性**: 提供消息持久化、故障恢复和重试机制
- **易用性**: 提供RESTful API和完善的监控指标

## 2. 系统架构

### 2.1 整体架构图
```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   API Gateway   │    │   Queue Manager │    │    Scheduler    │
│                 │    │                 │    │                 │
│ - 请求路由      │    │ - 多级队列      │    │ - 资源调度      │
│ - 负载均衡      │◄──►│ - 消息处理      │◄──►│ - 优先级管理    │
│ - 认证授权      │    │ - 状态管理      │    │ - 任务分发      │
└─────────────────┘    └─────────────────┘    └─────────────────┘
         │                       │                       │
         ▼                       ▼                       ▼
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│  Load Balancer  │    │   Preprocessor  │    │ State Manager   │
│                 │    │                 │    │                 │
│ - 端点管理      │    │ - 内容分析      │    │ - 对话状态      │
│ - 健康检查      │    │ - 优先级计算    │    │ - 上下文管理    │
│ - 会话亲和性    │    │ - 消息预处理    │    │ - 状态持久化    │
└─────────────────┘    └─────────────────┘    └─────────────────┘
         │                       │                       │
         ▼                       ▼                       ▼
┌─────────────────────────────────────────────────────────────────┐
│                        数据存储层                                │
│  ┌─────────────┐    ┌─────────────┐    ┌─────────────┐          │
│  │ PostgreSQL  │    │    Redis    │    │ Prometheus  │          │
│  │             │    │             │    │             │          │
│  │ - 消息持久化│    │ - 缓存      │    │ - 监控指标  │          │
│  │ - 对话记录  │    │ - 会话状态  │    │ - 性能统计  │          │
│  │ - 用户数据  │    │ - 队列状态  │    │ - 告警管理  │          │
│  └─────────────┘    └─────────────┘    └─────────────┘          │
└─────────────────────────────────────────────────────────────────┘
```

### 2.2 核心组件

#### 2.2.1 API Gateway (api-gateway)
- **功能**: 统一入口，处理所有外部请求
- **职责**:
  - HTTP请求路由和转发
  - 请求验证和参数校验
  - 响应格式化和错误处理
  - API版本管理

#### 2.2.2 Queue Manager (queue-manager)
- **功能**: 核心消息队列管理器
- **职责**:
  - 多级优先级队列管理
  - 消息入队和出队操作
  - 队列状态监控
  - 消息持久化

#### 2.2.3 Scheduler (scheduler)
- **功能**: 智能任务调度器
- **职责**:
  - 资源调度和分配
  - 优先级动态调整
  - 负载均衡策略
  - 任务超时管理

#### 2.2.4 Server (server)
- **功能**: 主服务器，协调各组件
- **职责**:
  - 服务启动和配置管理
  - 组件间通信协调
  - 健康检查和监控
  - 优雅关闭处理

## 3. 核心模块设计

### 3.1 多级优先级队列 (MultiLevelQueue)

#### 3.1.1 设计理念
- 基于消息重要性和紧急程度的多级分类
- 支持动态优先级调整
- 防止低优先级消息饥饿

#### 3.1.2 优先级定义
```go
type Priority int

const (
    PriorityRealtime Priority = iota + 1  // 实时处理
    PriorityHigh                          // 高优先级
    PriorityNormal                        // 普通优先级
    PriorityLow                           // 低优先级
)
```

#### 3.1.3 队列结构
- **Realtime Queue**: 实时消息，立即处理
- **High Priority Queue**: 重要消息，优先处理
- **Normal Priority Queue**: 普通消息，正常处理
- **Low Priority Queue**: 低优先级消息，延迟处理

### 3.2 消息预处理器 (Preprocessor)

#### 3.2.1 功能特性
- 消息内容分析和分类
- 自动优先级计算
- 消息格式验证
- 元数据提取

#### 3.2.2 优先级计算算法
```go
func (p *Preprocessor) CalculatePriority(content string) Priority {
    // 基于关键词、消息长度、用户等级等因素计算优先级
    score := p.analyzeContent(content)

    switch {
    case score >= 90:
        return PriorityRealtime
    case score >= 70:
        return PriorityHigh
    case score >= 40:
        return PriorityNormal
    default:
        return PriorityLow
    }
}
```

### 3.3 负载均衡器 (LoadBalancer)

#### 3.3.1 负载均衡算法
- **Round Robin**: 轮询分配
- **Weighted Round Robin**: 加权轮询
- **Least Connections**: 最少连接数
- **Session Affinity**: 会话亲和性

#### 3.3.2 健康检查机制
- 定期检查端点健康状态
- 自动故障转移
- 端点恢复检测
- 动态权重调整

### 3.4 状态管理器 (StateManager)

#### 3.4.1 对话状态管理
- 对话生命周期管理
- 上下文信息维护
- 状态持久化
- 过期对话清理

#### 3.4.2 状态类型
```go
type ConversationState string

const (
    ConversationStateActive    ConversationState = "active"    // 活跃状态
    ConversationStateInactive  ConversationState = "inactive"  // 非活跃状态
    ConversationStateCompleted ConversationState = "completed" // 已完成
    ConversationStateArchived  ConversationState = "archived"  // 已归档
)
```

## 4. 数据模型设计

### 4.1 消息模型 (Message)
```go
type Message struct {
    ID             string                 `json:"id"`
    ConversationID string                 `json:"conversation_id"`
    UserID         string                 `json:"user_id"`
    Content        string                 `json:"content"`
    Priority       Priority               `json:"priority"`
    Status         MessageStatus          `json:"status"`
    QueueName      string                 `json:"queue_name"`
    RetryCount     int                    `json:"retry_count"`
    MaxRetries     int                    `json:"max_retries"`
    Timeout        time.Duration          `json:"timeout"`
    CreatedAt      time.Time              `json:"created_at"`
    UpdatedAt      time.Time              `json:"updated_at"`
    ScheduledAt    *time.Time             `json:"scheduled_at"`
    CompletedAt    *time.Time             `json:"completed_at"`
    Metadata       map[string]interface{} `json:"metadata"`
}
```

### 4.2 对话模型 (Conversation)
```go
type Conversation struct {
    ID             string            `json:"id"`
    UserID         string            `json:"user_id"`
    Title          string            `json:"title"`
    Context        string            `json:"context"`
    Status         string            `json:"status"`
    State          ConversationState `json:"state"`
    Priority       Priority          `json:"priority"`
    CreatedAt      time.Time         `json:"created_at"`
    UpdatedAt      time.Time         `json:"updated_at"`
    LastActiveAt   *time.Time        `json:"last_active_at"`
    ExpiresAt      *time.Time        `json:"expires_at"`
    Metadata       map[string]interface{} `json:"metadata"`
}
```

## 5. 技术栈

### 5.1 后端技术
- **编程语言**: Go 1.21+
- **Web框架**: Gin
- **数据库**: PostgreSQL (主数据库)
- **缓存**: Redis (缓存和会话存储)
- **消息队列**: 自研多级优先级队列
- **监控**: Prometheus + Grafana
- **日志**: 结构化日志 (JSON格式)

### 5.2 部署技术
- **容器化**: Docker
- **编排**: Docker Compose
- **配置管理**: Viper (YAML配置)
- **进程管理**: 优雅关闭和重启

## 6. 性能特性

### 6.1 性能指标
- **吞吐量**: 支持每秒处理数千条消息
- **延迟**: 实时消息处理延迟 < 100ms
- **并发**: 支持数千并发连接
- **可用性**: 99.9% 服务可用性

### 6.2 优化策略
- **连接池**: 数据库和Redis连接池优化
- **批处理**: 消息批量处理减少I/O开销
- **缓存**: 多层缓存策略
- **异步处理**: 非阻塞消息处理

## 7. 安全设计

### 7.1 安全特性
- **输入验证**: 严格的参数校验和过滤
- **错误处理**: 安全的错误信息返回
- **日志安全**: 敏感信息脱敏
- **配置安全**: 敏感配置加密存储

### 7.2 最佳实践
- 最小权限原则
- 定期安全审计
- 依赖库安全更新
- 安全编码规范

## 8. 监控和运维

### 8.1 监控指标
- **系统指标**: CPU、内存、磁盘、网络
- **应用指标**: 请求量、响应时间、错误率
- **业务指标**: 消息处理量、队列长度、优先级分布

### 8.2 告警策略
- **阈值告警**: 基于指标阈值的自动告警
- **趋势告警**: 基于指标趋势的预警
- **异常检测**: 基于机器学习的异常检测

## 9. 扩展性设计

### 9.1 水平扩展
- **无状态设计**: 服务组件无状态化
- **负载均衡**: 多实例负载分担
- **数据分片**: 支持数据库分片

### 9.2 垂直扩展
- **资源调优**: CPU和内存优化
- **算法优化**: 队列和调度算法优化
- **缓存优化**: 多级缓存策略

## 10. 未来规划

### 10.1 功能增强
- **智能调度**: 基于机器学习的智能调度算法
- **多租户**: 支持多租户隔离
- **流式处理**: 支持流式消息处理
- **插件系统**: 可扩展的插件架构

### 10.2 性能优化
- **分布式部署**: 支持分布式集群部署
- **存储优化**: 支持多种存储后端
- **网络优化**: 支持更高效的网络协议
- **资源管理**: 智能资源分配和回收

---

*本文档版本: v1.0*
*最后更新: 2025年1月*
*维护者: 开发团队*
