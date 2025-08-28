# LLM消息队列系统性能调优指南

## 1. 性能调优概述

### 1.1 性能目标

LLM消息队列系统的性能目标包括：

- **吞吐量**: 每秒处理消息数 > 10,000 条
- **延迟**: 平均响应时间 < 100ms，P99 < 500ms
- **并发**: 支持 > 1,000 并发连接
- **可用性**: 99.9% 系统可用性
- **资源利用率**: CPU < 80%, 内存 < 85%

### 1.2 性能瓶颈识别

常见的性能瓶颈包括：

1. **CPU 瓶颈**: 计算密集型操作、序列化/反序列化
2. **内存瓶颈**: 大量消息缓存、内存泄漏
3. **I/O 瓶颈**: 数据库查询、网络通信
4. **锁竞争**: 并发访问共享资源
5. **垃圾回收**: Go GC 压力过大

### 1.3 性能监控指标

关键性能指标 (KPI)：

```yaml
# 核心业务指标
business_metrics:
  - message_throughput_per_second
  - message_processing_latency
  - queue_depth
  - error_rate
  - success_rate

# 系统资源指标
system_metrics:
  - cpu_usage_percent
  - memory_usage_percent
  - disk_io_ops_per_second
  - network_bandwidth_utilization
  - goroutine_count

# 应用指标
application_metrics:
  - active_connections
  - database_connection_pool_usage
  - cache_hit_rate
  - gc_pause_time
```

## 2. 系统级性能优化

### 2.1 操作系统优化

#### 2.1.1 内核参数调优

```bash
# /etc/sysctl.conf

# 网络优化
net.core.somaxconn = 65535
net.core.netdev_max_backlog = 5000
net.ipv4.tcp_max_syn_backlog = 65535
net.ipv4.tcp_fin_timeout = 30
net.ipv4.tcp_keepalive_time = 1200
net.ipv4.tcp_rmem = 4096 65536 16777216
net.ipv4.tcp_wmem = 4096 65536 16777216

# 文件描述符限制
fs.file-max = 1000000
fs.nr_open = 1000000

# 虚拟内存优化
vm.swappiness = 10
vm.dirty_ratio = 15
vm.dirty_background_ratio = 5

# 应用生效
sudo sysctl -p
```

#### 2.1.2 文件描述符限制

```bash
# /etc/security/limits.conf
* soft nofile 65535
* hard nofile 65535
* soft nproc 65535
* hard nproc 65535

# 验证设置
ulimit -n
ulimit -u
```

#### 2.1.3 CPU 亲和性设置

```bash
# 绑定进程到特定 CPU 核心
taskset -c 0-7 ./bin/api-gateway

# 或在 systemd 服务中设置
# /etc/systemd/system/llm-queue.service
[Service]
CPUAffinity=0-7
```

### 2.2 Go 运行时优化

#### 2.2.1 GOMAXPROCS 设置

```go
// 在 main 函数中设置
func main() {
    // 自动设置为 CPU 核心数
    runtime.GOMAXPROCS(runtime.NumCPU())

    // 或手动设置
    // runtime.GOMAXPROCS(8)

    // 应用启动逻辑
    // ...
}
```

```bash
# 通过环境变量设置
export GOMAXPROCS=8
./bin/api-gateway
```

#### 2.2.2 垃圾回收优化

```bash
# GC 调优环境变量
export GOGC=100          # GC 触发阈值 (默认 100%)
export GOMEMLIMIT=2GiB   # 内存限制
export GODEBUG=gctrace=1 # 启用 GC 跟踪

# 运行应用
./bin/api-gateway
```

```go
// 代码中的 GC 优化
func init() {
    // 设置 GC 目标百分比
    debug.SetGCPercent(50)

    // 定期强制 GC (谨慎使用)
    go func() {
        ticker := time.NewTicker(5 * time.Minute)
        defer ticker.Stop()

        for range ticker.C {
            runtime.GC()
            debug.FreeOSMemory()
        }
    }()
}
```

#### 2.2.3 内存池优化

```go
// 使用 sync.Pool 减少内存分配
var (
    messagePool = sync.Pool{
        New: func() interface{} {
            return &Message{}
        },
    }

    bufferPool = sync.Pool{
        New: func() interface{} {
            return make([]byte, 0, 1024)
        },
    }
)

// 获取和归还对象
func processMessage(data []byte) error {
    // 从池中获取消息对象
    msg := messagePool.Get().(*Message)
    defer func() {
        msg.Reset()
        messagePool.Put(msg)
    }()

    // 从池中获取缓冲区
    buf := bufferPool.Get().([]byte)
    defer func() {
        buf = buf[:0]
        bufferPool.Put(buf)
    }()

    // 处理逻辑
    return nil
}
```

## 3. 应用层性能优化

### 3.1 消息队列优化

#### 3.1.1 队列容量调优

```yaml
# configs/config.yaml
queue:
  # 根据内存大小调整队列容量
  default_capacity: 100000  # 4GB 内存建议值

  # 批处理优化
  batch_size: 1000         # 增大批处理大小
  batch_timeout: 100       # 减少批处理超时

  # 工作者数量优化
  worker_count: 16         # CPU 核心数的 2 倍
  worker_buffer_size: 2000 # 工作者缓冲区大小
```

#### 3.1.2 优先级队列优化

```go
// internal/priorityqueue/optimized_queue.go
type OptimizedQueue struct {
    queues    []*lockfree.Queue // 使用无锁队列
    priorities []int32           // 原子操作的优先级计数
    scheduler  *Scheduler        // 优化的调度器
}

// 无锁入队操作
func (q *OptimizedQueue) Enqueue(msg *Message) error {
    queueIndex := msg.Priority

    // 原子递增优先级计数
    atomic.AddInt32(&q.priorities[queueIndex], 1)

    // 无锁入队
    return q.queues[queueIndex].Enqueue(msg)
}

// 批量出队操作
func (q *OptimizedQueue) DequeueBatch(maxSize int) ([]*Message, error) {
    messages := make([]*Message, 0, maxSize)

    // 按优先级顺序处理
    for i := 0; i < len(q.queues) && len(messages) < maxSize; i++ {
        if atomic.LoadInt32(&q.priorities[i]) > 0 {
            if msg, ok := q.queues[i].Dequeue(); ok {
                messages = append(messages, msg.(*Message))
                atomic.AddInt32(&q.priorities[i], -1)
            }
        }
    }

    return messages, nil
}
```

#### 3.1.3 消息预处理优化

```go
// internal/preprocessor/optimized_preprocessor.go
type OptimizedPreprocessor struct {
    workers    int
    inputChan  chan *Message
    outputChan chan *Message
    wg         sync.WaitGroup
}

func NewOptimizedPreprocessor(workers int) *OptimizedPreprocessor {
    p := &OptimizedPreprocessor{
        workers:    workers,
        inputChan:  make(chan *Message, 10000),
        outputChan: make(chan *Message, 10000),
    }

    // 启动工作者协程
    for i := 0; i < workers; i++ {
        p.wg.Add(1)
        go p.worker()
    }

    return p
}

func (p *OptimizedPreprocessor) worker() {
    defer p.wg.Done()

    // 批量处理消息
    batch := make([]*Message, 0, 100)
    ticker := time.NewTicker(10 * time.Millisecond)
    defer ticker.Stop()

    for {
        select {
        case msg := <-p.inputChan:
            batch = append(batch, msg)

            // 批量大小达到阈值时处理
            if len(batch) >= 100 {
                p.processBatch(batch)
                batch = batch[:0]
            }

        case <-ticker.C:
            // 定期处理剩余消息
            if len(batch) > 0 {
                p.processBatch(batch)
                batch = batch[:0]
            }
        }
    }
}

func (p *OptimizedPreprocessor) processBatch(messages []*Message) {
    // 并行处理批量消息
    var wg sync.WaitGroup
    semaphore := make(chan struct{}, 10) // 限制并发数

    for _, msg := range messages {
        wg.Add(1)
        go func(m *Message) {
            defer wg.Done()

            semaphore <- struct{}{}
            defer func() { <-semaphore }()

            // 处理单个消息
            p.processMessage(m)
            p.outputChan <- m
        }(msg)
    }

    wg.Wait()
}
```

### 3.2 数据库性能优化

#### 3.2.1 连接池优化

```yaml
# configs/config.yaml
database:
  postgres:
    pool:
      # 连接池大小 = CPU 核心数 * 2-4
      max_open_conns: 32

      # 空闲连接数 = max_open_conns / 4
      max_idle_conns: 8

      # 连接生存时间 (分钟)
      conn_max_lifetime: 30

      # 连接空闲时间 (分钟)
      conn_max_idle_time: 5

    # 慢查询阈值 (毫秒)
    slow_query_threshold: 100
```

#### 3.2.2 查询优化

```go
// pkg/database/optimized_queries.go

// 使用预编译语句
var (
    insertMessageStmt *sql.Stmt
    selectMessageStmt *sql.Stmt
    updateMessageStmt *sql.Stmt
)

func init() {
    // 预编译常用查询
    insertMessageStmt, _ = db.Prepare(`
        INSERT INTO messages (id, conversation_id, user_id, content, priority, created_at)
        VALUES ($1, $2, $3, $4, $5, $6)
    `)

    selectMessageStmt, _ = db.Prepare(`
        SELECT id, conversation_id, user_id, content, priority, status, created_at
        FROM messages
        WHERE conversation_id = $1
        ORDER BY created_at DESC
        LIMIT $2
    `)
}

// 批量插入优化
func BatchInsertMessages(messages []*Message) error {
    // 使用事务批量插入
    tx, err := db.Begin()
    if err != nil {
        return err
    }
    defer tx.Rollback()

    // 准备批量插入语句
    stmt, err := tx.Prepare(pq.CopyIn("messages",
        "id", "conversation_id", "user_id", "content", "priority", "created_at"))
    if err != nil {
        return err
    }
    defer stmt.Close()

    // 批量执行
    for _, msg := range messages {
        _, err = stmt.Exec(msg.ID, msg.ConversationID, msg.UserID,
            msg.Content, msg.Priority, msg.CreatedAt)
        if err != nil {
            return err
        }
    }

    // 执行批量插入
    _, err = stmt.Exec()
    if err != nil {
        return err
    }

    return tx.Commit()
}

// 分页查询优化
func GetMessagesPaginated(conversationID string, offset, limit int) ([]*Message, error) {
    // 使用游标分页而不是 OFFSET
    query := `
        SELECT id, conversation_id, user_id, content, priority, status, created_at
        FROM messages
        WHERE conversation_id = $1 AND id > $2
        ORDER BY id
        LIMIT $3
    `

    rows, err := db.Query(query, conversationID, offset, limit)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var messages []*Message
    for rows.Next() {
        msg := &Message{}
        err := rows.Scan(&msg.ID, &msg.ConversationID, &msg.UserID,
            &msg.Content, &msg.Priority, &msg.Status, &msg.CreatedAt)
        if err != nil {
            return nil, err
        }
        messages = append(messages, msg)
    }

    return messages, nil
}
```

#### 3.2.3 索引优化

```sql
-- 创建复合索引
CREATE INDEX CONCURRENTLY idx_messages_conversation_created
    ON messages (conversation_id, created_at DESC);

-- 创建部分索引
CREATE INDEX CONCURRENTLY idx_messages_pending
    ON messages (priority, created_at)
    WHERE status = 'pending';

-- 创建表达式索引
CREATE INDEX CONCURRENTLY idx_messages_content_gin
    ON messages USING gin (to_tsvector('english', content));

-- 分析表统计信息
ANALYZE messages;

-- 查看索引使用情况
SELECT
    schemaname,
    tablename,
    indexname,
    idx_scan,
    idx_tup_read,
    idx_tup_fetch
FROM pg_stat_user_indexes
WHERE tablename = 'messages'
ORDER BY idx_scan DESC;
```

### 3.3 缓存优化

#### 3.3.1 Redis 缓存优化

```yaml
# configs/config.yaml
database:
  redis:
    # 连接池优化
    pool_size: 50
    min_idle_conns: 10

    # 超时设置
    dial_timeout: 5
    read_timeout: 2
    write_timeout: 2

    # 重试配置
    max_retries: 3
    retry_delay: 100

    # 管道优化
    pipeline_enabled: true
    pipeline_size: 1000
```

```go
// pkg/cache/optimized_cache.go
type OptimizedCache struct {
    client   redis.Cmdable
    pipeline redis.Pipeliner
    mutex    sync.Mutex
}

// 批量操作
func (c *OptimizedCache) MSet(pairs map[string]interface{}) error {
    pipe := c.client.Pipeline()

    for key, value := range pairs {
        pipe.Set(context.Background(), key, value, time.Hour)
    }

    _, err := pipe.Exec(context.Background())
    return err
}

// 批量获取
func (c *OptimizedCache) MGet(keys []string) (map[string]string, error) {
    pipe := c.client.Pipeline()

    cmds := make([]*redis.StringCmd, len(keys))
    for i, key := range keys {
        cmds[i] = pipe.Get(context.Background(), key)
    }

    _, err := pipe.Exec(context.Background())
    if err != nil {
        return nil, err
    }

    result := make(map[string]string)
    for i, cmd := range cmds {
        if val, err := cmd.Result(); err == nil {
            result[keys[i]] = val
        }
    }

    return result, nil
}

// 本地缓存 + Redis 二级缓存
type TieredCache struct {
    local  *ristretto.Cache
    redis  *OptimizedCache
    stats  *CacheStats
}

func (c *TieredCache) Get(key string) (interface{}, error) {
    // 先查本地缓存
    if value, found := c.local.Get(key); found {
        c.stats.LocalHits.Inc()
        return value, nil
    }

    // 再查 Redis
    value, err := c.redis.Get(key)
    if err == nil {
        c.stats.RedisHits.Inc()
        // 回写本地缓存
        c.local.Set(key, value, 1)
        return value, nil
    }

    c.stats.Misses.Inc()
    return nil, err
}
```

#### 3.3.2 应用级缓存

```go
// internal/cache/application_cache.go
type ApplicationCache struct {
    conversations *ristretto.Cache
    users        *ristretto.Cache
    config       *ristretto.Cache
}

func NewApplicationCache() *ApplicationCache {
    // 配置本地缓存
    conversationCache, _ := ristretto.NewCache(&ristretto.Config{
        NumCounters: 1e7,     // 计数器数量
        MaxCost:     1 << 30, // 最大内存 (1GB)
        BufferItems: 64,      // 缓冲区大小
        Metrics:     true,    // 启用指标
    })

    userCache, _ := ristretto.NewCache(&ristretto.Config{
        NumCounters: 1e6,
        MaxCost:     1 << 28, // 256MB
        BufferItems: 64,
        Metrics:     true,
    })

    configCache, _ := ristretto.NewCache(&ristretto.Config{
        NumCounters: 1e4,
        MaxCost:     1 << 20, // 1MB
        BufferItems: 64,
        Metrics:     true,
    })

    return &ApplicationCache{
        conversations: conversationCache,
        users:        userCache,
        config:       configCache,
    }
}

// 缓存预热
func (c *ApplicationCache) Warmup() error {
    // 预加载热点数据
    hotConversations, err := getHotConversations()
    if err != nil {
        return err
    }

    for _, conv := range hotConversations {
        c.conversations.Set(conv.ID, conv, int64(len(conv.Messages)))
    }

    // 预加载活跃用户
    activeUsers, err := getActiveUsers()
    if err != nil {
        return err
    }

    for _, user := range activeUsers {
        c.users.Set(user.ID, user, 1)
    }

    return nil
}
```

### 3.4 网络优化

#### 3.4.1 HTTP 服务器优化

```go
// api/server/optimized_server.go
func NewOptimizedServer() *http.Server {
    return &http.Server{
        Addr:         ":8080",
        Handler:      setupRoutes(),
        ReadTimeout:  15 * time.Second,
        WriteTimeout: 15 * time.Second,
        IdleTimeout:  60 * time.Second,

        // 优化 TCP 设置
        ConnState: func(conn net.Conn, state http.ConnState) {
            switch state {
            case http.StateNew:
                // 设置 TCP_NODELAY
                if tcpConn, ok := conn.(*net.TCPConn); ok {
                    tcpConn.SetNoDelay(true)
                    tcpConn.SetKeepAlive(true)
                    tcpConn.SetKeepAlivePeriod(30 * time.Second)
                }
            }
        },

        // 限制最大头部大小
        MaxHeaderBytes: 1 << 20, // 1MB
    }
}

// 连接池复用
var httpClient = &http.Client{
    Transport: &http.Transport{
        MaxIdleConns:        100,
        MaxIdleConnsPerHost: 10,
        IdleConnTimeout:     90 * time.Second,
        DisableCompression:  false,

        // 启用 HTTP/2
        ForceAttemptHTTP2: true,

        // TCP 优化
        DialContext: (&net.Dialer{
            Timeout:   30 * time.Second,
            KeepAlive: 30 * time.Second,
        }).DialContext,

        // TLS 优化
        TLSHandshakeTimeout: 10 * time.Second,
    },
    Timeout: 30 * time.Second,
}
```

#### 3.4.2 gRPC 优化

```go
// api/grpc/optimized_server.go
func NewOptimizedGRPCServer() *grpc.Server {
    return grpc.NewServer(
        // 连接参数优化
        grpc.KeepaliveParams(keepalive.ServerParameters{
            MaxConnectionIdle:     15 * time.Second,
            MaxConnectionAge:      30 * time.Second,
            MaxConnectionAgeGrace: 5 * time.Second,
            Time:                  5 * time.Second,
            Timeout:               1 * time.Second,
        }),

        // 强制策略
        grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
            MinTime:             5 * time.Second,
            PermitWithoutStream: true,
        }),

        // 消息大小限制
        grpc.MaxRecvMsgSize(4*1024*1024), // 4MB
        grpc.MaxSendMsgSize(4*1024*1024), // 4MB

        // 并发流限制
        grpc.MaxConcurrentStreams(1000),

        // 压缩
        grpc.RPCCompressor(grpc.NewGZIPCompressor()),
        grpc.RPCDecompressor(grpc.NewGZIPDecompressor()),
    )
}
```

## 4. 监控和诊断

### 4.1 性能监控

#### 4.1.1 Prometheus 指标

```go
// pkg/metrics/performance_metrics.go
var (
    // 请求指标
    requestDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "http_request_duration_seconds",
            Help:    "HTTP request duration in seconds",
            Buckets: prometheus.DefBuckets,
        },
        []string{"method", "endpoint", "status"},
    )

    // 队列指标
    queueDepth = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "queue_depth",
            Help: "Current queue depth",
        },
        []string{"queue_name", "priority"},
    )

    // 数据库指标
    dbConnections = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "database_connections",
            Help: "Database connection pool status",
        },
        []string{"database", "status"},
    )

    // 缓存指标
    cacheOperations = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "cache_operations_total",
            Help: "Total cache operations",
        },
        []string{"operation", "result"},
    )
)

// 中间件收集指标
func MetricsMiddleware() gin.HandlerFunc {
    return gin.HandlerFunc(func(c *gin.Context) {
        start := time.Now()

        c.Next()

        duration := time.Since(start)
        requestDuration.WithLabelValues(
            c.Request.Method,
            c.FullPath(),
            strconv.Itoa(c.Writer.Status()),
        ).Observe(duration.Seconds())
    })
}
```

#### 4.1.2 自定义性能监控

```go
// pkg/monitor/performance_monitor.go
type PerformanceMonitor struct {
    metrics map[string]*Metric
    mutex   sync.RWMutex
}

type Metric struct {
    Name      string
    Value     float64
    Timestamp time.Time
    Labels    map[string]string
}

func (pm *PerformanceMonitor) RecordLatency(operation string, duration time.Duration) {
    pm.mutex.Lock()
    defer pm.mutex.Unlock()

    key := fmt.Sprintf("latency_%s", operation)
    pm.metrics[key] = &Metric{
        Name:      key,
        Value:     duration.Seconds(),
        Timestamp: time.Now(),
        Labels:    map[string]string{"operation": operation},
    }
}

func (pm *PerformanceMonitor) RecordThroughput(operation string, count int64) {
    pm.mutex.Lock()
    defer pm.mutex.Unlock()

    key := fmt.Sprintf("throughput_%s", operation)
    if existing, exists := pm.metrics[key]; exists {
        existing.Value += float64(count)
    } else {
        pm.metrics[key] = &Metric{
            Name:      key,
            Value:     float64(count),
            Timestamp: time.Now(),
            Labels:    map[string]string{"operation": operation},
        }
    }
}

// 定期报告性能指标
func (pm *PerformanceMonitor) StartReporting(interval time.Duration) {
    ticker := time.NewTicker(interval)
    defer ticker.Stop()

    for range ticker.C {
        pm.reportMetrics()
    }
}

func (pm *PerformanceMonitor) reportMetrics() {
    pm.mutex.RLock()
    defer pm.mutex.RUnlock()

    for _, metric := range pm.metrics {
        log.Info("Performance metric",
            "name", metric.Name,
            "value", metric.Value,
            "timestamp", metric.Timestamp,
            "labels", metric.Labels,
        )
    }
}
```

### 4.2 性能分析工具

#### 4.2.1 pprof 集成

```go
// cmd/server/main.go
import (
    _ "net/http/pprof"
    "net/http"
)

func main() {
    // 启动 pprof 服务器
    go func() {
        log.Println("Starting pprof server on :6060")
        log.Println(http.ListenAndServe("localhost:6060", nil))
    }()

    // 应用程序逻辑
    // ...
}
```

```bash
# 性能分析命令

# CPU 分析
go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30

# 内存分析
go tool pprof http://localhost:6060/debug/pprof/heap

# Goroutine 分析
go tool pprof http://localhost:6060/debug/pprof/goroutine

# 阻塞分析
go tool pprof http://localhost:6060/debug/pprof/block

# 互斥锁分析
go tool pprof http://localhost:6060/debug/pprof/mutex
```

#### 4.2.2 火焰图生成

```bash
# 安装 go-torch
go install github.com/uber/go-torch@latest

# 生成 CPU 火焰图
go-torch -u http://localhost:6060 -t 30

# 生成内存火焰图
go-torch -u http://localhost:6060/debug/pprof/heap --colors mem

# 使用 pprof 生成火焰图
go tool pprof -http=:8081 http://localhost:6060/debug/pprof/profile?seconds=30
```

#### 4.2.3 分布式追踪

```go
// pkg/tracing/jaeger.go
import (
    "github.com/opentracing/opentracing-go"
    "github.com/uber/jaeger-client-go"
    "github.com/uber/jaeger-client-go/config"
)

func InitJaeger(serviceName string) (opentracing.Tracer, io.Closer, error) {
    cfg := config.Configuration{
        ServiceName: serviceName,
        Sampler: &config.SamplerConfig{
            Type:  jaeger.SamplerTypeConst,
            Param: 1,
        },
        Reporter: &config.ReporterConfig{
            LogSpans:            true,
            BufferFlushInterval: 1 * time.Second,
            LocalAgentHostPort:  "localhost:6831",
        },
    }

    tracer, closer, err := cfg.NewTracer()
    if err != nil {
        return nil, nil, err
    }

    opentracing.SetGlobalTracer(tracer)
    return tracer, closer, nil
}

// 在处理函数中使用追踪
func processMessageWithTracing(ctx context.Context, msg *Message) error {
    span, ctx := opentracing.StartSpanFromContext(ctx, "process_message")
    defer span.Finish()

    span.SetTag("message.id", msg.ID)
    span.SetTag("message.priority", msg.Priority)

    // 处理逻辑
    if err := validateMessage(ctx, msg); err != nil {
        span.SetTag("error", true)
        span.LogFields(
            log.String("event", "validation_failed"),
            log.Error(err),
        )
        return err
    }

    return nil
}
```

## 5. 负载测试

### 5.1 压力测试工具

#### 5.1.1 使用 wrk 进行 HTTP 压测

```bash
# 安装 wrk
sudo apt-get install wrk

# 基础压测
wrk -t12 -c400 -d30s http://localhost:8080/api/v1/messages

# 使用 Lua 脚本进行复杂测试
wrk -t12 -c400 -d30s -s post_message.lua http://localhost:8080
```

```lua
-- post_message.lua
wrk.method = "POST"
wrk.body   = '{"conversation_id":"conv_123","user_id":"user_456","content":"Hello, World!","priority":"normal"}'
wrk.headers["Content-Type"] = "application/json"

function response(status, headers, body)
    if status ~= 201 then
        print("Error: " .. status .. " " .. body)
    end
end
```

#### 5.1.2 使用 k6 进行负载测试

```javascript
// load_test.js
import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate } from 'k6/metrics';

export let errorRate = new Rate('errors');

export let options = {
    stages: [
        { duration: '2m', target: 100 },   // 2分钟内增加到100用户
        { duration: '5m', target: 100 },   // 保持100用户5分钟
        { duration: '2m', target: 200 },   // 2分钟内增加到200用户
        { duration: '5m', target: 200 },   // 保持200用户5分钟
        { duration: '2m', target: 0 },     // 2分钟内减少到0用户
    ],
    thresholds: {
        http_req_duration: ['p(99)<500'],  // 99%的请求在500ms内
        errors: ['rate<0.1'],              // 错误率小于10%
    },
};

export default function() {
    let payload = JSON.stringify({
        conversation_id: `conv_${Math.floor(Math.random() * 1000)}`,
        user_id: `user_${Math.floor(Math.random() * 100)}`,
        content: `Test message ${Math.random()}`,
        priority: ['urgent', 'high', 'normal', 'low'][Math.floor(Math.random() * 4)]
    });

    let params = {
        headers: {
            'Content-Type': 'application/json',
        },
    };

    let response = http.post('http://localhost:8080/api/v1/messages', payload, params);

    let result = check(response, {
        'status is 201': (r) => r.status === 201,
        'response time < 500ms': (r) => r.timings.duration < 500,
    });

    errorRate.add(!result);

    sleep(1);
}
```

#### 5.1.3 自定义负载测试

```go
// tests/load/load_test.go
package load

import (
    "context"
    "fmt"
    "sync"
    "testing"
    "time"
)

func TestMessageQueueLoad(t *testing.T) {
    const (
        numWorkers = 100
        duration   = 30 * time.Second
        targetRPS  = 1000
    )

    client := NewTestClient("http://localhost:8080")

    var (
        totalRequests int64
        totalErrors   int64
        wg           sync.WaitGroup
        ctx, cancel  = context.WithTimeout(context.Background(), duration)
    )
    defer cancel()

    // 启动工作者
    for i := 0; i < numWorkers; i++ {
        wg.Add(1)
        go func(workerID int) {
            defer wg.Done()

            ticker := time.NewTicker(time.Second / time.Duration(targetRPS/numWorkers))
            defer ticker.Stop()

            for {
                select {
                case <-ctx.Done():
                    return
                case <-ticker.C:
                    if err := client.SendMessage(generateTestMessage()); err != nil {
                        atomic.AddInt64(&totalErrors, 1)
                    }
                    atomic.AddInt64(&totalRequests, 1)
                }
            }
        }(i)
    }

    // 等待测试完成
    wg.Wait()

    // 计算结果
    actualRPS := float64(totalRequests) / duration.Seconds()
    errorRate := float64(totalErrors) / float64(totalRequests) * 100

    t.Logf("Load test results:")
    t.Logf("  Duration: %v", duration)
    t.Logf("  Total requests: %d", totalRequests)
    t.Logf("  Total errors: %d", totalErrors)
    t.Logf("  Actual RPS: %.2f", actualRPS)
    t.Logf("  Error rate: %.2f%%", errorRate)

    // 断言
    if actualRPS < float64(targetRPS)*0.9 {
        t.Errorf("RPS too low: got %.2f, want >= %.2f", actualRPS, float64(targetRPS)*0.9)
    }

    if errorRate > 5.0 {
        t.Errorf("Error rate too high: got %.2f%%, want <= 5.0%%", errorRate)
    }
}
```

### 5.2 性能基准测试

#### 5.2.1 基准测试套件

```go
// tests/benchmark/benchmark_test.go
package benchmark

import (
    "testing"
    "time"
)

func BenchmarkMessageEnqueue(b *testing.B) {
    queue := NewOptimizedQueue(10000)

    b.ResetTimer()
    b.RunParallel(func(pb *testing.PB) {
        for pb.Next() {
            msg := &Message{
                ID:      generateID(),
                Content: "Benchmark message",
                Priority: PriorityNormal,
            }
            queue.Enqueue(msg)
        }
    })
}

func BenchmarkMessageDequeue(b *testing.B) {
    queue := NewOptimizedQueue(10000)

    // 预填充队列
    for i := 0; i < 1000; i++ {
        msg := &Message{
            ID:      generateID(),
            Content: fmt.Sprintf("Message %d", i),
            Priority: PriorityNormal,
        }
        queue.Enqueue(msg)
    }

    b.ResetTimer()
    b.RunParallel(func(pb *testing.PB) {
        for pb.Next() {
            queue.Dequeue()
        }
    })
}

func BenchmarkDatabaseInsert(b *testing.B) {
    db := setupTestDB()
    defer db.Close()

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        msg := &Message{
            ID:             generateID(),
            ConversationID: "conv_123",
            UserID:         "user_456",
            Content:        fmt.Sprintf("Benchmark message %d", i),
            Priority:       PriorityNormal,
            CreatedAt:      time.Now(),
        }

        if err := insertMessage(db, msg); err != nil {
            b.Fatal(err)
        }
    }
}

func BenchmarkCacheOperations(b *testing.B) {
    cache := NewOptimizedCache()

    b.Run("Set", func(b *testing.B) {
        b.RunParallel(func(pb *testing.PB) {
            for pb.Next() {
                key := fmt.Sprintf("key_%d", rand.Int())
                value := fmt.Sprintf("value_%d", rand.Int())
                cache.Set(key, value)
            }
        })
    })

    b.Run("Get", func(b *testing.B) {
        // 预填充缓存
        for i := 0; i < 1000; i++ {
            cache.Set(fmt.Sprintf("key_%d", i), fmt.Sprintf("value_%d", i))
        }

        b.ResetTimer()
        b.RunParallel(func(pb *testing.PB) {
            for pb.Next() {
                key := fmt.Sprintf("key_%d", rand.Intn(1000))
                cache.Get(key)
            }
        })
    })
}
```

## 6. 性能调优检查清单

### 6.1 系统级检查

- [ ] **操作系统参数**: 文件描述符限制、网络参数、内存参数
- [ ] **CPU 配置**: GOMAXPROCS 设置、CPU 亲和性
- [ ] **内存配置**: 堆大小、GC 参数、内存限制
- [ ] **磁盘 I/O**: SSD 使用、文件系统优化
- [ ] **网络配置**: TCP 参数、带宽限制

### 6.2 应用级检查

- [ ] **连接池**: 数据库连接池、Redis 连接池、HTTP 连接池
- [ ] **缓存策略**: 本地缓存、分布式缓存、缓存预热
- [ ] **批处理**: 消息批处理、数据库批操作
- [ ] **异步处理**: 异步 I/O、协程池、消息队列
- [ ] **资源复用**: 对象池、内存池、连接复用

### 6.3 数据库检查

- [ ] **索引优化**: 查询索引、复合索引、部分索引
- [ ] **查询优化**: SQL 优化、预编译语句、分页查询
- [ ] **连接管理**: 连接池配置、连接生命周期
- [ ] **事务优化**: 事务大小、隔离级别、锁粒度
- [ ] **分区表**: 水平分区、垂直分区

### 6.4 监控检查

- [ ] **性能指标**: 吞吐量、延迟、错误率、资源使用率
- [ ] **告警配置**: 阈值设置、告警通知、自动恢复
- [ ] **日志分析**: 慢查询日志、错误日志、访问日志
- [ ] **分布式追踪**: 请求链路、性能瓶颈识别
- [ ] **容量规划**: 资源预测、扩容策略

---

*本文档版本: v1.0*
*最后更新: 2025年1月*
*维护者: 开发团队*
