# LLM消息队列系统配置说明

## 1. 配置概述

LLM消息队列系统使用YAML格式的配置文件，支持环境变量覆盖和多环境配置。配置文件位于 `configs/config.yaml`，系统启动时会自动加载。

### 1.1 配置文件结构

```yaml
server:          # 服务器配置
database:        # 数据库配置
queue:           # 队列配置
scheduler:       # 调度器配置
loadbalancer:    # 负载均衡配置
logging:         # 日志配置
metrics:         # 监控指标配置
security:        # 安全配置
```

### 1.2 环境变量支持

配置支持通过环境变量覆盖，格式为 `SECTION_KEY`，例如：
- `SERVER_PORT=8080` 覆盖 `server.port`
- `DATABASE_POSTGRES_HOST=localhost` 覆盖 `database.postgres.host`
- `LOGGING_LEVEL=debug` 覆盖 `logging.level`

## 2. 服务器配置 (server)

### 2.1 基础配置

```yaml
server:
  # 服务器监听端口
  port: 8080

  # 服务器监听地址
  host: "0.0.0.0"

  # 运行模式: development, production, test
  mode: "production"

  # 请求超时时间 (秒)
  timeout: 30

  # 最大请求体大小 (MB)
  max_body_size: 10

  # 启用 CORS
  cors_enabled: true

  # 允许的源域名
  cors_origins:
    - "http://localhost:3000"
    - "https://yourdomain.com"

  # 启用 HTTPS
  tls_enabled: false

  # TLS 证书文件路径
  tls_cert_file: "/path/to/cert.pem"

  # TLS 私钥文件路径
  tls_key_file: "/path/to/key.pem"

  # 优雅关闭超时时间 (秒)
  shutdown_timeout: 10
```

### 2.2 性能配置

```yaml
server:
  # 最大并发连接数
  max_connections: 1000

  # 读取超时时间 (秒)
  read_timeout: 15

  # 写入超时时间 (秒)
  write_timeout: 15

  # 空闲超时时间 (秒)
  idle_timeout: 60

  # 启用 HTTP/2
  http2_enabled: true

  # 启用 gzip 压缩
  compression_enabled: true

  # 压缩级别 (1-9)
  compression_level: 6
```

### 2.3 环境变量

- `SERVER_PORT`: 服务器端口
- `SERVER_HOST`: 服务器地址
- `SERVER_MODE`: 运行模式
- `SERVER_TIMEOUT`: 请求超时时间
- `SERVER_TLS_ENABLED`: 是否启用 HTTPS

## 3. 数据库配置 (database)

### 3.1 PostgreSQL 配置

```yaml
database:
  postgres:
    # 数据库主机地址
    host: "localhost"

    # 数据库端口
    port: 5432

    # 数据库用户名
    user: "postgres"

    # 数据库密码
    password: "postgres"

    # 数据库名称
    dbname: "llm_queue"

    # SSL 模式: disable, require, verify-ca, verify-full
    sslmode: "disable"

    # 连接池配置
    pool:
      # 最大连接数
      max_open_conns: 25

      # 最大空闲连接数
      max_idle_conns: 5

      # 连接最大生存时间 (分钟)
      conn_max_lifetime: 60

      # 连接最大空闲时间 (分钟)
      conn_max_idle_time: 10

    # 迁移配置
    migration:
      # 是否自动运行迁移
      auto_migrate: true

      # 迁移文件路径
      path: "migrations"

    # 慢查询日志阈值 (毫秒)
    slow_query_threshold: 200

    # 启用查询日志
    log_queries: false
```

### 3.2 Redis 配置

```yaml
database:
  redis:
    # Redis 服务器地址
    addr: "localhost:6379"

    # Redis 密码
    password: ""

    # Redis 数据库编号
    db: 0

    # 连接池大小
    pool_size: 10

    # 最小空闲连接数
    min_idle_conns: 2

    # 连接超时时间 (秒)
    dial_timeout: 5

    # 读取超时时间 (秒)
    read_timeout: 3

    # 写入超时时间 (秒)
    write_timeout: 3

    # 连接最大空闲时间 (分钟)
    idle_timeout: 5

    # 连接检查间隔 (分钟)
    idle_check_frequency: 1

    # 最大重试次数
    max_retries: 3

    # 重试间隔 (毫秒)
    retry_delay: 100

    # 启用集群模式
    cluster_enabled: false

    # 集群节点地址 (集群模式)
    cluster_addrs:
      - "localhost:7000"
      - "localhost:7001"
      - "localhost:7002"
```

### 3.3 环境变量

- `DATABASE_POSTGRES_HOST`: PostgreSQL 主机
- `DATABASE_POSTGRES_PORT`: PostgreSQL 端口
- `DATABASE_POSTGRES_USER`: PostgreSQL 用户名
- `DATABASE_POSTGRES_PASSWORD`: PostgreSQL 密码
- `DATABASE_POSTGRES_DBNAME`: PostgreSQL 数据库名
- `DATABASE_REDIS_ADDR`: Redis 地址
- `DATABASE_REDIS_PASSWORD`: Redis 密码

## 4. 队列配置 (queue)

### 4.1 基础配置

```yaml
queue:
  # 默认队列容量
  default_capacity: 10000

  # 优先级级别数量
  priority_levels: 4

  # 批处理大小
  batch_size: 100

  # 批处理超时时间 (毫秒)
  batch_timeout: 1000

  # 工作者数量
  worker_count: 10

  # 工作者缓冲区大小
  worker_buffer_size: 1000

  # 消息过期时间 (小时)
  message_ttl: 24

  # 死信队列配置
  dead_letter:
    # 是否启用死信队列
    enabled: true

    # 最大重试次数
    max_retries: 3

    # 重试间隔 (秒)
    retry_interval: 60

    # 死信队列容量
    capacity: 1000
```

### 4.2 优先级配置

```yaml
queue:
  priorities:
    # 紧急优先级
    urgent:
      level: 0
      weight: 100
      timeout: 5  # 秒

    # 高优先级
    high:
      level: 1
      weight: 75
      timeout: 30

    # 普通优先级
    normal:
      level: 2
      weight: 50
      timeout: 300

    # 低优先级
    low:
      level: 3
      weight: 25
      timeout: 3600
```

### 4.3 持久化配置

```yaml
queue:
  persistence:
    # 是否启用持久化
    enabled: true

    # 持久化策略: memory, redis, postgres
    strategy: "redis"

    # 同步间隔 (秒)
    sync_interval: 10

    # 快照间隔 (分钟)
    snapshot_interval: 60

    # 快照保留数量
    snapshot_retention: 10
```

### 4.4 环境变量

- `QUEUE_DEFAULT_CAPACITY`: 默认队列容量
- `QUEUE_WORKER_COUNT`: 工作者数量
- `QUEUE_BATCH_SIZE`: 批处理大小
- `QUEUE_MESSAGE_TTL`: 消息过期时间

## 5. 调度器配置 (scheduler)

### 5.1 基础配置

```yaml
scheduler:
  # 调度策略: round_robin, weighted_round_robin, least_connections, priority_based
  strategy: "priority_based"

  # 调度间隔 (毫秒)
  interval: 100

  # 最大并发任务数
  max_concurrent_tasks: 1000

  # 任务超时时间 (秒)
  task_timeout: 300

  # 启用预测性调度
  predictive_enabled: true

  # 预测窗口大小 (分钟)
  prediction_window: 30

  # 资源监控间隔 (秒)
  resource_monitor_interval: 10
```

### 5.2 资源限制配置

```yaml
scheduler:
  resources:
    # CPU 限制配置
    cpu:
      # 最大 CPU 使用率 (%)
      max_usage: 80

      # CPU 监控间隔 (秒)
      monitor_interval: 5

      # CPU 告警阈值 (%)
      alert_threshold: 90

    # 内存限制配置
    memory:
      # 最大内存使用量 (MB)
      max_usage: 2048

      # 内存监控间隔 (秒)
      monitor_interval: 5

      # 内存告警阈值 (MB)
      alert_threshold: 1800

    # 网络限制配置
    network:
      # 最大带宽 (Mbps)
      max_bandwidth: 100

      # 网络监控间隔 (秒)
      monitor_interval: 10
```

### 5.3 自动扩缩容配置

```yaml
scheduler:
  autoscaling:
    # 是否启用自动扩缩容
    enabled: true

    # 最小实例数
    min_instances: 1

    # 最大实例数
    max_instances: 10

    # 扩容阈值 (%)
    scale_up_threshold: 70

    # 缩容阈值 (%)
    scale_down_threshold: 30

    # 扩容冷却时间 (秒)
    scale_up_cooldown: 300

    # 缩容冷却时间 (秒)
    scale_down_cooldown: 600
```

### 5.4 环境变量

- `SCHEDULER_STRATEGY`: 调度策略
- `SCHEDULER_INTERVAL`: 调度间隔
- `SCHEDULER_MAX_CONCURRENT_TASKS`: 最大并发任务数
- `SCHEDULER_TASK_TIMEOUT`: 任务超时时间

## 6. 负载均衡配置 (loadbalancer)

### 6.1 基础配置

```yaml
loadbalancer:
  # 负载均衡算法: round_robin, weighted_round_robin, least_connections, ip_hash
  algorithm: "weighted_round_robin"

  # 健康检查配置
  health_check:
    # 是否启用健康检查
    enabled: true

    # 检查间隔 (秒)
    interval: 30

    # 检查超时时间 (秒)
    timeout: 5

    # 失败阈值
    failure_threshold: 3

    # 成功阈值
    success_threshold: 2

    # 检查路径
    path: "/health"

    # 期望状态码
    expected_status: 200

  # 会话亲和性
  session_affinity:
    # 是否启用会话亲和性
    enabled: true

    # 亲和性策略: client_ip, session_id, user_id
    strategy: "session_id"

    # 会话超时时间 (分钟)
    timeout: 30

    # 会话清理间隔 (分钟)
    cleanup_interval: 10
```

### 6.2 端点配置

```yaml
loadbalancer:
  endpoints:
    # 端点组配置
    groups:
      # 主要端点组
      primary:
        # 端点列表
        endpoints:
          - url: "http://localhost:8081"
            weight: 100
            max_connections: 1000
          - url: "http://localhost:8082"
            weight: 100
            max_connections: 1000

        # 组权重
        weight: 80

        # 故障转移配置
        failover:
          enabled: true
          max_failures: 3
          recovery_time: 300  # 秒

      # 备用端点组
      backup:
        endpoints:
          - url: "http://localhost:8083"
            weight: 50
            max_connections: 500

        weight: 20

        # 仅在主要组不可用时使用
        backup_only: true
```

### 6.3 限流配置

```yaml
loadbalancer:
  rate_limiting:
    # 是否启用限流
    enabled: true

    # 全局限流配置
    global:
      # 每秒请求数
      requests_per_second: 1000

      # 突发请求数
      burst_size: 100

    # 按 IP 限流配置
    per_ip:
      # 每秒请求数
      requests_per_second: 10

      # 突发请求数
      burst_size: 20

      # 限流窗口 (秒)
      window_size: 60

    # 按用户限流配置
    per_user:
      # 每秒请求数
      requests_per_second: 100

      # 突发请求数
      burst_size: 200
```

### 6.4 环境变量

- `LOADBALANCER_ALGORITHM`: 负载均衡算法
- `LOADBALANCER_HEALTH_CHECK_ENABLED`: 是否启用健康检查
- `LOADBALANCER_SESSION_AFFINITY_ENABLED`: 是否启用会话亲和性

## 7. 日志配置 (logging)

### 7.1 基础配置

```yaml
logging:
  # 日志级别: debug, info, warn, error, fatal
  level: "info"

  # 日志格式: text, json
  format: "json"

  # 输出目标: stdout, stderr, file
  output: "stdout"

  # 日志文件路径 (当 output 为 file 时)
  file_path: "/var/log/llm-queue/app.log"

  # 是否启用颜色输出
  color_enabled: true

  # 是否显示调用者信息
  caller_enabled: true

  # 是否显示堆栈跟踪
  stacktrace_enabled: false
```

### 7.2 日志轮转配置

```yaml
logging:
  rotation:
    # 是否启用日志轮转
    enabled: true

    # 最大文件大小 (MB)
    max_size: 100

    # 最大文件数量
    max_files: 10

    # 最大保留天数
    max_age: 30

    # 是否压缩旧文件
    compress: true

    # 轮转时间间隔: daily, weekly, monthly
    interval: "daily"
```

### 7.3 结构化日志配置

```yaml
logging:
  structured:
    # 是否启用结构化日志
    enabled: true

    # 时间戳格式
    timestamp_format: "2006-01-02T15:04:05.000Z07:00"

    # 自定义字段
    fields:
      service: "llm-message-queue"
      version: "1.0.0"
      environment: "production"

    # 敏感字段过滤
    sensitive_fields:
      - "password"
      - "token"
      - "secret"
      - "key"
```

### 7.4 环境变量

- `LOGGING_LEVEL`: 日志级别
- `LOGGING_FORMAT`: 日志格式
- `LOGGING_OUTPUT`: 输出目标
- `LOGGING_FILE_PATH`: 日志文件路径

## 8. 监控指标配置 (metrics)

### 8.1 基础配置

```yaml
metrics:
  # 是否启用指标收集
  enabled: true

  # 指标服务器端口
  port: 9090

  # 指标路径
  path: "/metrics"

  # 指标收集间隔 (秒)
  interval: 15

  # 指标保留时间 (小时)
  retention: 168  # 7 天
```

### 8.2 Prometheus 配置

```yaml
metrics:
  prometheus:
    # 是否启用 Prometheus 指标
    enabled: true

    # 指标前缀
    namespace: "llm_queue"

    # 子系统名称
    subsystem: "api"

    # 自定义标签
    labels:
      service: "llm-message-queue"
      version: "1.0.0"

    # 直方图桶配置
    histogram_buckets:
      - 0.005
      - 0.01
      - 0.025
      - 0.05
      - 0.1
      - 0.25
      - 0.5
      - 1
      - 2.5
      - 5
      - 10
```

### 8.3 自定义指标配置

```yaml
metrics:
  custom:
    # 业务指标
    business:
      # 消息处理指标
      message_processing:
        enabled: true
        labels: ["priority", "status"]

      # 队列状态指标
      queue_status:
        enabled: true
        labels: ["queue_name", "priority"]

      # 用户活动指标
      user_activity:
        enabled: true
        labels: ["user_id", "action"]

    # 系统指标
    system:
      # CPU 使用率
      cpu_usage:
        enabled: true
        interval: 10  # 秒

      # 内存使用率
      memory_usage:
        enabled: true
        interval: 10

      # 磁盘使用率
      disk_usage:
        enabled: true
        interval: 60
```

### 8.4 环境变量

- `METRICS_ENABLED`: 是否启用指标收集
- `METRICS_PORT`: 指标服务器端口
- `METRICS_PATH`: 指标路径
- `METRICS_INTERVAL`: 指标收集间隔

## 9. 安全配置 (security)

### 9.1 认证配置

```yaml
security:
  authentication:
    # 认证方式: jwt, oauth2, api_key
    method: "jwt"

    # JWT 配置
    jwt:
      # JWT 密钥
      secret: "your-secret-key"

      # 令牌过期时间 (小时)
      expiration: 24

      # 刷新令牌过期时间 (天)
      refresh_expiration: 7

      # 签名算法
      algorithm: "HS256"

      # 发行者
      issuer: "llm-message-queue"

    # API Key 配置
    api_key:
      # API Key 头名称
      header_name: "X-API-Key"

      # 有效的 API Keys
      valid_keys:
        - "key1"
        - "key2"

      # API Key 过期时间 (天)
      expiration: 30
```

### 9.2 授权配置

```yaml
security:
  authorization:
    # 是否启用授权
    enabled: true

    # 授权策略: rbac, abac
    policy: "rbac"

    # RBAC 配置
    rbac:
      # 角色定义
      roles:
        admin:
          permissions:
            - "*"
        user:
          permissions:
            - "message:read"
            - "message:write"
            - "conversation:read"
        readonly:
          permissions:
            - "message:read"
            - "conversation:read"

      # 默认角色
      default_role: "user"
```

### 9.3 加密配置

```yaml
security:
  encryption:
    # 是否启用数据加密
    enabled: true

    # 加密算法: aes-256-gcm, chacha20-poly1305
    algorithm: "aes-256-gcm"

    # 加密密钥 (base64 编码)
    key: "your-encryption-key"

    # 是否加密敏感字段
    encrypt_sensitive_fields: true

    # 敏感字段列表
    sensitive_fields:
      - "password"
      - "token"
      - "secret"
      - "private_key"
```

### 9.4 环境变量

- `SECURITY_JWT_SECRET`: JWT 密钥
- `SECURITY_ENCRYPTION_KEY`: 加密密钥
- `SECURITY_API_KEYS`: API Keys (逗号分隔)

## 10. 配置示例

### 10.1 开发环境配置

```yaml
# configs/config.dev.yaml
server:
  port: 8080
  host: "localhost"
  mode: "development"
  cors_enabled: true
  cors_origins:
    - "http://localhost:3000"

database:
  postgres:
    host: "localhost"
    port: 5432
    user: "postgres"
    password: "postgres"
    dbname: "llm_queue_dev"
    sslmode: "disable"
  redis:
    addr: "localhost:6379"
    password: ""
    db: 0

queue:
  default_capacity: 1000
  worker_count: 5
  batch_size: 50

scheduler:
  strategy: "round_robin"
  interval: 100
  max_concurrent_tasks: 100

logging:
  level: "debug"
  format: "text"
  output: "stdout"
  color_enabled: true

metrics:
  enabled: true
  port: 9090

security:
  authentication:
    method: "jwt"
    jwt:
      secret: "dev-secret-key"
      expiration: 24
```

### 10.2 生产环境配置

```yaml
# configs/config.prod.yaml
server:
  port: 8080
  host: "0.0.0.0"
  mode: "production"
  tls_enabled: true
  tls_cert_file: "/etc/ssl/certs/app.crt"
  tls_key_file: "/etc/ssl/private/app.key"
  max_connections: 10000
  timeout: 30

database:
  postgres:
    host: "postgres.example.com"
    port: 5432
    user: "llm_queue_user"
    password: "${DATABASE_PASSWORD}"
    dbname: "llm_queue_prod"
    sslmode: "require"
    pool:
      max_open_conns: 100
      max_idle_conns: 20
  redis:
    addr: "redis.example.com:6379"
    password: "${REDIS_PASSWORD}"
    db: 0
    pool_size: 50

queue:
  default_capacity: 100000
  worker_count: 50
  batch_size: 500
  persistence:
    enabled: true
    strategy: "redis"

scheduler:
  strategy: "priority_based"
  interval: 50
  max_concurrent_tasks: 10000
  autoscaling:
    enabled: true
    min_instances: 3
    max_instances: 20

loadbalancer:
  algorithm: "weighted_round_robin"
  health_check:
    enabled: true
    interval: 30
  rate_limiting:
    enabled: true
    global:
      requests_per_second: 10000

logging:
  level: "info"
  format: "json"
  output: "file"
  file_path: "/var/log/llm-queue/app.log"
  rotation:
    enabled: true
    max_size: 100
    max_files: 30

metrics:
  enabled: true
  port: 9090
  prometheus:
    enabled: true
    namespace: "llm_queue"

security:
  authentication:
    method: "jwt"
    jwt:
      secret: "${JWT_SECRET}"
      expiration: 8
  authorization:
    enabled: true
    policy: "rbac"
  encryption:
    enabled: true
    key: "${ENCRYPTION_KEY}"
```

### 10.3 测试环境配置

```yaml
# configs/config.test.yaml
server:
  port: 8080
  host: "localhost"
  mode: "test"

database:
  postgres:
    host: "localhost"
    port: 5432
    user: "test_user"
    password: "test_password"
    dbname: "llm_queue_test"
    sslmode: "disable"
  redis:
    addr: "localhost:6379"
    password: ""
    db: 1  # 使用不同的数据库

queue:
  default_capacity: 100
  worker_count: 2
  batch_size: 10

scheduler:
  strategy: "round_robin"
  interval: 1000  # 较长间隔用于测试
  max_concurrent_tasks: 10

logging:
  level: "debug"
  format: "text"
  output: "stdout"

metrics:
  enabled: false  # 测试时禁用指标

security:
  authentication:
    method: "jwt"
    jwt:
      secret: "test-secret-key"
      expiration: 1  # 短过期时间用于测试
```

## 11. 配置验证

### 11.1 配置验证规则

系统启动时会自动验证配置文件的有效性，包括：

1. **必填字段检查**: 确保所有必需的配置项都已设置
2. **数据类型验证**: 验证配置值的数据类型是否正确
3. **取值范围检查**: 验证数值配置是否在有效范围内
4. **依赖关系验证**: 检查配置项之间的依赖关系
5. **连接性测试**: 测试数据库和外部服务的连接

### 11.2 配置验证命令

```bash
# 验证配置文件
./bin/server -config configs/config.yaml -validate

# 验证并显示详细信息
./bin/server -config configs/config.yaml -validate -verbose

# 验证特定环境配置
./bin/server -config configs/config.prod.yaml -validate
```

### 11.3 常见配置错误

1. **端口冲突**: 多个服务使用相同端口
2. **数据库连接失败**: 数据库配置错误或服务不可用
3. **内存配置过大**: 超出系统可用内存
4. **SSL 证书路径错误**: 证书文件不存在或权限不足
5. **日志目录权限**: 日志目录没有写入权限

## 12. 配置最佳实践

### 12.1 安全最佳实践

1. **敏感信息**: 使用环境变量存储密码、密钥等敏感信息
2. **权限控制**: 限制配置文件的读取权限
3. **加密存储**: 对敏感配置进行加密存储
4. **定期轮换**: 定期更换密钥和密码
5. **审计日志**: 记录配置变更的审计日志

### 12.2 性能最佳实践

1. **连接池**: 合理配置数据库连接池大小
2. **缓存设置**: 根据内存大小调整缓存配置
3. **工作者数量**: 根据 CPU 核数设置工作者数量
4. **批处理**: 启用批处理以提高吞吐量
5. **监控指标**: 启用性能监控指标

### 12.3 运维最佳实践

1. **配置版本控制**: 使用 Git 管理配置文件版本
2. **环境隔离**: 为不同环境使用独立的配置
3. **配置模板**: 使用配置模板减少重复
4. **自动化部署**: 集成配置到 CI/CD 流程
5. **配置备份**: 定期备份重要配置文件

---

*本文档版本: v1.0*
*最后更新: 2025年1月*
*维护者: 开发团队*
