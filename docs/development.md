# LLM消息队列系统开发者指南

## 1. 开发环境搭建

### 1.1 环境要求

#### 1.1.1 基础环境
- **Go**: 1.21 或更高版本
- **Git**: 2.25 或更高版本
- **Docker**: 20.10 或更高版本
- **Docker Compose**: 2.0 或更高版本
- **Make**: 用于构建自动化

#### 1.1.2 开发工具推荐
- **IDE**: VS Code, GoLand, Vim/Neovim
- **Go插件**: Go extension for VS Code
- **数据库工具**: DBeaver, pgAdmin
- **API测试**: Postman, curl, httpie
- **版本控制**: Git

### 1.2 项目克隆和初始化

```bash
# 克隆项目
git clone <repository-url>
cd llm-message-queue

# 安装 Go 依赖
go mod download

# 安装开发工具
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
go install github.com/swaggo/swag/cmd/swag@latest
go install github.com/air-verse/air@latest

# 复制配置文件
cp configs/config.yaml.example configs/config.yaml
```

### 1.3 本地开发环境启动

```bash
# 启动依赖服务 (PostgreSQL, Redis)
docker-compose -f docker-compose.dev.yml up -d postgres redis

# 等待服务启动
sleep 10

# 运行数据库迁移
make migrate-up

# 启动开发服务器 (热重载)
make dev
```

### 1.4 开发配置文件

```yaml
# configs/config.yaml (开发环境)
server:
  port: 8080
  host: "localhost"
  mode: "development"

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
    pool_size: 10

logging:
  level: "debug"
  format: "text"
  output: "stdout"

metrics:
  enabled: true
  port: 9090
  path: "/metrics"
```

## 2. 项目结构详解

### 2.1 目录结构

```
llm-message-queue/
├── api/                    # API 处理器
│   ├── handlers/          # HTTP 处理器
│   ├── middleware/        # 中间件
│   └── routes/           # 路由定义
├── cmd/                   # 应用程序入口
│   ├── api-gateway/      # API 网关服务
│   ├── queue-manager/    # 队列管理服务
│   ├── scheduler/        # 调度服务
│   └── server/           # 主服务器
├── configs/              # 配置文件
├── deployments/          # 部署相关文件
│   ├── docker/          # Docker 文件
│   └── k8s/             # Kubernetes 配置
├── docs/                 # 文档
├── internal/             # 内部包
│   ├── conversation/    # 对话管理
│   ├── loadbalancer/    # 负载均衡
│   ├── preprocessor/    # 消息预处理
│   ├── priorityqueue/   # 优先级队列
│   ├── scheduler/       # 调度器
│   └── statemanager/    # 状态管理
├── pkg/                  # 公共包
│   ├── config/          # 配置管理
│   ├── database/        # 数据库连接
│   ├── logger/          # 日志管理
│   ├── metrics/         # 指标收集
│   └── models/          # 数据模型
├── scripts/              # 脚本文件
├── tests/                # 测试文件
│   ├── integration/     # 集成测试
│   ├── unit/            # 单元测试
│   └── fixtures/        # 测试数据
├── go.mod               # Go 模块文件
├── go.sum               # Go 依赖校验
├── Makefile             # 构建脚本
└── README.md            # 项目说明
```

### 2.2 核心包说明

#### 2.2.1 internal/priorityqueue
多级优先级队列的核心实现，包含：
- `queue.go`: 队列数据结构和操作
- `manager.go`: 队列管理器
- `worker.go`: 工作者池

#### 2.2.2 internal/scheduler
任务调度器实现，包含：
- `scheduler.go`: 调度器核心逻辑
- `resource_scheduler.go`: 资源调度
- `strategy.go`: 调度策略

#### 2.2.3 internal/loadbalancer
负载均衡器实现，包含：
- `load_balancer.go`: 负载均衡核心
- `algorithms.go`: 负载均衡算法
- `health_check.go`: 健康检查

#### 2.2.4 pkg/models
数据模型定义，包含：
- `message.go`: 消息模型
- `conversation.go`: 对话模型
- `user.go`: 用户模型

## 3. 开发规范

### 3.1 代码规范

#### 3.1.1 Go 代码规范
- 遵循 [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
- 使用 `gofmt` 格式化代码
- 使用 `golangci-lint` 进行代码检查
- 函数和方法必须有注释
- 导出的类型和函数必须有文档注释

#### 3.1.2 命名规范
```go
// 包名：小写，简短，有意义
package priorityqueue

// 常量：大写，使用下划线分隔
const (
    DEFAULT_QUEUE_SIZE = 1000
    MAX_RETRY_COUNT    = 3
)

// 变量：驼峰命名
var (
    defaultConfig Config
    queueManager  *QueueManager
)

// 函数：驼峰命名，动词开头
func CreateMessage(content string) *Message {
    // ...
}

// 结构体：驼峰命名，名词
type MessageQueue struct {
    messages []Message
    capacity int
}

// 接口：驼峰命名，通常以 -er 结尾
type QueueProcessor interface {
    Process(msg *Message) error
}
```

#### 3.1.3 错误处理
```go
// 定义错误类型
var (
    ErrQueueFull     = errors.New("queue is full")
    ErrInvalidMessage = errors.New("invalid message")
)

// 错误包装
func ProcessMessage(msg *Message) error {
    if msg == nil {
        return fmt.Errorf("process message: %w", ErrInvalidMessage)
    }

    if err := validateMessage(msg); err != nil {
        return fmt.Errorf("validate message: %w", err)
    }

    return nil
}

// 错误检查
if err := ProcessMessage(msg); err != nil {
    log.Error("failed to process message", "error", err)
    return err
}
```

### 3.2 Git 工作流

#### 3.2.1 分支策略
- `main`: 主分支，稳定版本
- `develop`: 开发分支，集成最新功能
- `feature/*`: 功能分支，开发新功能
- `bugfix/*`: 修复分支，修复 bug
- `hotfix/*`: 热修复分支，紧急修复

#### 3.2.2 提交规范
```bash
# 提交格式
<type>(<scope>): <subject>

<body>

<footer>

# 类型说明
feat:     新功能
fix:      修复 bug
docs:     文档更新
style:    代码格式调整
refactor: 代码重构
test:     测试相关
chore:    构建过程或辅助工具的变动

# 示例
feat(queue): add priority-based message scheduling

Implement a new scheduling algorithm that prioritizes messages
based on their importance and urgency levels.

- Add Priority enum with four levels
- Implement PriorityQueue data structure
- Add scheduling logic in QueueManager

Closes #123
```

#### 3.2.3 Pull Request 流程
1. 从 `develop` 分支创建功能分支
2. 开发功能并提交代码
3. 编写测试并确保通过
4. 更新相关文档
5. 创建 Pull Request
6. 代码审查和讨论
7. 合并到 `develop` 分支

### 3.3 测试规范

#### 3.3.1 单元测试
```go
// internal/priorityqueue/queue_test.go
package priorityqueue

import (
    "testing"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestQueueEnqueue(t *testing.T) {
    tests := []struct {
        name     string
        capacity int
        messages []string
        wantErr  bool
    }{
        {
            name:     "successful enqueue",
            capacity: 10,
            messages: []string{"msg1", "msg2"},
            wantErr:  false,
        },
        {
            name:     "queue full",
            capacity: 1,
            messages: []string{"msg1", "msg2"},
            wantErr:  true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            q := NewQueue(tt.capacity)

            var err error
            for _, msg := range tt.messages {
                err = q.Enqueue(NewMessage(msg))
                if err != nil {
                    break
                }
            }

            if tt.wantErr {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
                assert.Equal(t, len(tt.messages), q.Size())
            }
        })
    }
}

func TestQueueDequeue(t *testing.T) {
    q := NewQueue(10)
    msg := NewMessage("test")

    err := q.Enqueue(msg)
    require.NoError(t, err)

    dequeued, err := q.Dequeue()
    require.NoError(t, err)
    assert.Equal(t, msg.ID, dequeued.ID)
    assert.Equal(t, 0, q.Size())
}
```

#### 3.3.2 集成测试
```go
// tests/integration/api_test.go
package integration

import (
    "bytes"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/gin-gonic/gin"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/suite"
)

type APITestSuite struct {
    suite.Suite
    router *gin.Engine
    db     *sql.DB
}

func (suite *APITestSuite) SetupSuite() {
    // 设置测试数据库
    suite.db = setupTestDB()

    // 设置路由
    suite.router = setupRouter(suite.db)
}

func (suite *APITestSuite) TearDownSuite() {
    suite.db.Close()
}

func (suite *APITestSuite) TestSendMessage() {
    payload := map[string]interface{}{
        "conversation_id": "conv_123",
        "user_id":         "user_456",
        "content":         "Hello, World!",
        "priority":        "normal",
    }

    body, _ := json.Marshal(payload)
    req := httptest.NewRequest("POST", "/api/v1/messages", bytes.NewBuffer(body))
    req.Header.Set("Content-Type", "application/json")

    w := httptest.NewRecorder()
    suite.router.ServeHTTP(w, req)

    assert.Equal(suite.T(), http.StatusCreated, w.Code)

    var response map[string]interface{}
    err := json.Unmarshal(w.Body.Bytes(), &response)
    assert.NoError(suite.T(), err)
    assert.Equal(suite.T(), "success", response["message"])
}

func TestAPITestSuite(t *testing.T) {
    suite.Run(t, new(APITestSuite))
}
```

#### 3.3.3 基准测试
```go
// internal/priorityqueue/queue_bench_test.go
package priorityqueue

import (
    "testing"
)

func BenchmarkQueueEnqueue(b *testing.B) {
    q := NewQueue(10000)

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        msg := NewMessage(fmt.Sprintf("message_%d", i))
        q.Enqueue(msg)
    }
}

func BenchmarkQueueDequeue(b *testing.B) {
    q := NewQueue(10000)

    // 预填充队列
    for i := 0; i < 1000; i++ {
        msg := NewMessage(fmt.Sprintf("message_%d", i))
        q.Enqueue(msg)
    }

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        q.Dequeue()
    }
}
```

## 4. 构建和部署

### 4.1 Makefile

```makefile
# Makefile
.PHONY: help build test lint clean dev docker

# 默认目标
help:
	@echo "Available targets:"
	@echo "  build      - Build all binaries"
	@echo "  test       - Run all tests"
	@echo "  lint       - Run linters"
	@echo "  clean      - Clean build artifacts"
	@echo "  dev        - Start development server"
	@echo "  docker     - Build Docker images"
	@echo "  migrate-up - Run database migrations"

# 构建变量
GO_VERSION := $(shell go version | cut -d ' ' -f 3)
GIT_COMMIT := $(shell git rev-parse --short HEAD)
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS := -X main.version=$(GIT_COMMIT) -X main.buildTime=$(BUILD_TIME)

# 构建所有二进制文件
build:
	@echo "Building binaries..."
	go build -ldflags "$(LDFLAGS)" -o bin/api-gateway cmd/api-gateway/main.go
	go build -ldflags "$(LDFLAGS)" -o bin/queue-manager cmd/queue-manager/main.go
	go build -ldflags "$(LDFLAGS)" -o bin/scheduler cmd/scheduler/main.go
	go build -ldflags "$(LDFLAGS)" -o bin/server cmd/server/main.go

# 运行测试
test:
	@echo "Running tests..."
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# 运行单元测试
test-unit:
	go test -v -race ./internal/... ./pkg/...

# 运行集成测试
test-integration:
	go test -v -race ./tests/integration/...

# 运行基准测试
test-bench:
	go test -bench=. -benchmem ./...

# 代码检查
lint:
	@echo "Running linters..."
	golangci-lint run
	go vet ./...
	go fmt ./...

# 清理构建产物
clean:
	@echo "Cleaning..."
	rm -rf bin/
	rm -f coverage.out coverage.html
	docker system prune -f

# 开发模式
dev:
	@echo "Starting development server..."
	air -c .air.toml

# 构建 Docker 镜像
docker:
	@echo "Building Docker images..."
	docker build -f deployments/Dockerfile.api-gateway -t llm-queue/api-gateway:$(GIT_COMMIT) .
	docker build -f deployments/Dockerfile.queue-manager -t llm-queue/queue-manager:$(GIT_COMMIT) .
	docker build -f deployments/Dockerfile.scheduler -t llm-queue/scheduler:$(GIT_COMMIT) .
	docker build -f deployments/Dockerfile.server -t llm-queue/server:$(GIT_COMMIT) .

# 数据库迁移
migrate-up:
	@echo "Running database migrations..."
	migrate -path migrations -database "postgres://postgres:postgres@localhost:5432/llm_queue_dev?sslmode=disable" up

migrate-down:
	@echo "Rolling back database migrations..."
	migrate -path migrations -database "postgres://postgres:postgres@localhost:5432/llm_queue_dev?sslmode=disable" down

# 生成 API 文档
docs:
	@echo "Generating API documentation..."
	swag init -g cmd/api-gateway/main.go -o docs/swagger

# 安装开发依赖
install-tools:
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install github.com/swaggo/swag/cmd/swag@latest
	go install github.com/air-verse/air@latest
	go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
```

### 4.2 热重载配置

```toml
# .air.toml
root = "."
testdata_dir = "testdata"
tmp_dir = "tmp"

[build]
  args_bin = ["-config", "configs/config.yaml"]
  bin = "./tmp/main"
  cmd = "go build -o ./tmp/main cmd/api-gateway/main.go"
  delay = 1000
  exclude_dir = ["assets", "tmp", "vendor", "testdata", "docs"]
  exclude_file = []
  exclude_regex = ["_test.go"]
  exclude_unchanged = false
  follow_symlink = false
  full_bin = ""
  include_dir = []
  include_ext = ["go", "tpl", "tmpl", "html", "yaml", "yml"]
  kill_delay = "0s"
  log = "build-errors.log"
  send_interrupt = false
  stop_on_root = false

[color]
  app = ""
  build = "yellow"
  main = "magenta"
  runner = "green"
  watcher = "cyan"

[log]
  time = false

[misc]
  clean_on_exit = false

[screen]
  clear_on_rebuild = false
```

## 5. 调试和性能分析

### 5.1 调试技巧

#### 5.1.1 使用 Delve 调试器
```bash
# 安装 Delve
go install github.com/go-delve/delve/cmd/dlv@latest

# 调试应用
dlv debug cmd/api-gateway/main.go -- -config configs/config.yaml

# 在代码中设置断点
(dlv) break main.main
(dlv) break internal/priorityqueue.(*Queue).Enqueue

# 运行程序
(dlv) continue

# 查看变量
(dlv) print msg
(dlv) locals
(dlv) args
```

#### 5.1.2 日志调试
```go
// 使用结构化日志
log := logger.New()
log.Info("processing message",
    "message_id", msg.ID,
    "priority", msg.Priority,
    "queue_size", queue.Size(),
)

// 调试级别日志
log.Debug("queue state",
    "pending", queue.PendingCount(),
    "processing", queue.ProcessingCount(),
    "completed", queue.CompletedCount(),
)

// 错误日志
if err != nil {
    log.Error("failed to process message",
        "error", err,
        "message_id", msg.ID,
        "retry_count", msg.RetryCount,
    )
}
```

### 5.2 性能分析

#### 5.2.1 CPU 性能分析
```go
// 在代码中添加性能分析
import _ "net/http/pprof"

func main() {
    // 启动 pprof 服务器
    go func() {
        log.Println(http.ListenAndServe("localhost:6060", nil))
    }()

    // 应用程序逻辑
    // ...
}
```

```bash
# 收集 CPU 性能数据
go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30

# 分析性能数据
(pprof) top
(pprof) list functionName
(pprof) web
```

#### 5.2.2 内存性能分析
```bash
# 收集内存性能数据
go tool pprof http://localhost:6060/debug/pprof/heap

# 分析内存使用
(pprof) top
(pprof) list functionName
(pprof) svg > heap.svg
```

#### 5.2.3 并发分析
```bash
# 分析 goroutine
go tool pprof http://localhost:6060/debug/pprof/goroutine

# 分析锁竞争
go tool pprof http://localhost:6060/debug/pprof/mutex
```

## 6. 贡献指南

### 6.1 如何贡献

1. **Fork 项目**: 在 GitHub 上 fork 项目到你的账户
2. **创建分支**: 从 `develop` 分支创建功能分支
3. **开发功能**: 编写代码并添加测试
4. **提交代码**: 遵循提交规范提交代码
5. **创建 PR**: 创建 Pull Request 并描述变更
6. **代码审查**: 参与代码审查讨论
7. **合并代码**: 审查通过后合并到主分支

### 6.2 代码审查清单

#### 6.2.1 功能性
- [ ] 功能是否按预期工作
- [ ] 是否处理了边界情况
- [ ] 错误处理是否恰当
- [ ] 是否有足够的测试覆盖

#### 6.2.2 代码质量
- [ ] 代码是否易读易懂
- [ ] 是否遵循项目编码规范
- [ ] 是否有适当的注释
- [ ] 是否有性能问题

#### 6.2.3 安全性
- [ ] 是否有安全漏洞
- [ ] 输入验证是否充分
- [ ] 是否泄露敏感信息
- [ ] 权限控制是否正确

### 6.3 发布流程

#### 6.3.1 版本号规范
遵循 [Semantic Versioning](https://semver.org/):
- `MAJOR.MINOR.PATCH`
- `MAJOR`: 不兼容的 API 变更
- `MINOR`: 向后兼容的功能新增
- `PATCH`: 向后兼容的问题修正

#### 6.3.2 发布步骤
```bash
# 1. 更新版本号
git checkout develop
git pull origin develop

# 2. 创建发布分支
git checkout -b release/v1.2.0

# 3. 更新 CHANGELOG.md
vim CHANGELOG.md

# 4. 提交版本更新
git add .
git commit -m "chore: bump version to v1.2.0"

# 5. 合并到 main 分支
git checkout main
git merge release/v1.2.0

# 6. 创建标签
git tag -a v1.2.0 -m "Release v1.2.0"

# 7. 推送到远程
git push origin main
git push origin v1.2.0

# 8. 合并回 develop 分支
git checkout develop
git merge main
git push origin develop
```

## 7. 常见问题

### 7.1 开发环境问题

**Q: Go 模块下载失败**
```bash
# 设置 Go 代理
go env -w GOPROXY=https://goproxy.cn,direct
go env -w GOSUMDB=sum.golang.google.cn
```

**Q: Docker 构建失败**
```bash
# 清理 Docker 缓存
docker system prune -a

# 重新构建
docker-compose build --no-cache
```

**Q: 数据库连接失败**
```bash
# 检查数据库服务状态
docker-compose ps postgres

# 查看数据库日志
docker-compose logs postgres

# 测试连接
psql -h localhost -U postgres -d llm_queue_dev
```

### 7.2 测试问题

**Q: 测试数据库冲突**
```go
// 使用测试数据库
func setupTestDB() *sql.DB {
    dbName := fmt.Sprintf("test_db_%d", time.Now().UnixNano())
    // 创建临时数据库
    // ...
    return db
}
```

**Q: 并发测试不稳定**
```go
// 使用同步机制
func TestConcurrentAccess(t *testing.T) {
    var wg sync.WaitGroup
    var mu sync.Mutex
    results := make([]int, 0)

    for i := 0; i < 100; i++ {
        wg.Add(1)
        go func(id int) {
            defer wg.Done()
            result := processData(id)

            mu.Lock()
            results = append(results, result)
            mu.Unlock()
        }(i)
    }

    wg.Wait()
    assert.Equal(t, 100, len(results))
}
```

### 7.3 性能问题

**Q: 内存泄漏**
```go
// 正确关闭资源
func processMessages() error {
    db, err := sql.Open("postgres", dsn)
    if err != nil {
        return err
    }
    defer db.Close() // 确保关闭

    // 处理逻辑
    return nil
}

// 使用对象池
var messagePool = sync.Pool{
    New: func() interface{} {
        return &Message{}
    },
}

func getMessage() *Message {
    return messagePool.Get().(*Message)
}

func putMessage(msg *Message) {
    msg.Reset()
    messagePool.Put(msg)
}
```

**Q: CPU 使用率过高**
```go
// 避免忙等待
for {
    select {
    case msg := <-msgChan:
        processMessage(msg)
    case <-time.After(100 * time.Millisecond):
        // 定期检查
        checkStatus()
    }
}

// 使用缓冲通道
msgChan := make(chan *Message, 1000)
```

## 8. 工具和资源

### 8.1 开发工具

- **golangci-lint**: 代码检查工具
- **air**: 热重载工具
- **swag**: API 文档生成
- **migrate**: 数据库迁移工具
- **delve**: Go 调试器
- **pprof**: 性能分析工具

### 8.2 有用的资源

- [Go 官方文档](https://golang.org/doc/)
- [Effective Go](https://golang.org/doc/effective_go.html)
- [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
- [Gin 框架文档](https://gin-gonic.com/docs/)
- [GORM 文档](https://gorm.io/docs/)
- [Redis Go 客户端](https://github.com/go-redis/redis)

### 8.3 社区和支持

- **GitHub Issues**: 报告 bug 和功能请求
- **GitHub Discussions**: 技术讨论和问答
- **Slack/Discord**: 实时交流 (如果有的话)
- **邮件列表**: 开发者邮件列表 (如果有的话)

---

*本文档版本: v1.0*
*最后更新: 2025年1月*
*维护者: 开发团队*
