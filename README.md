# LLM Message Queue

<p align="center">
  <img src="./docs/LOGO.png" width="200" alt="Project Logo">
</p>

<p align="center">
  <a href="https://github.com/ZhangLearning/llm-message-queue/actions"><img src="https://github.com/ZhangLearning/llm-message-queue/workflows/Go/badge.svg" alt="Build Status"></a>
  <a href="https://codecov.io/gh/ZhangLearning/llm-message-queue"><img src="https://codecov.io/gh/ZhangLearning/llm-message-queue/branch/main/graph/badge.svg" alt="Code Coverage"></a>
  <a href="https://goreportcard.com/report/github.com/ZhangLearning/llm-message-queue"><img src="https://goreportcard.com/badge/github.com/ZhangLearning/llm-message-queue" alt="Go Report Card"></a>
  <a href="https://github.com/ZhangLearning/llm-message-queue/blob/main/LICENSE"><img src="https://img.shields.io/github/license/ZhangLearning/llm-message-queue.svg" alt="License"></a>
</p>

## 简介

LLM Message Queue 是一个专为大型语言模型（LLM）设计的高性能消息队列服务。它旨在优化 LLM 应用中的消息处理流程，提供多级优先级、动态资源调度和对话状态管理等功能，以解决消息处理延迟、资源分配不均和上下文管理复杂等问题。

## 核心功能

- **多级优先级队列**：支持实时、高、普通和低四个优先级，确保关键消息得到优先处理。
- **动态资源调度**：根据队列负载自动调整处理资源，最大化系统效率。
- **对话状态管理**：维护对话上下文，支持长对话历史和状态恢复。
- **负载均衡**：提供多种负载均衡策略（如轮询、最少连接、加权随机），高效分配请求。
- **消息预处理**：自动分析消息内容，智能确定优先级和处理策略。
- **可扩展架构**：采用模块化设计，易于扩展和定制，以满足不同业务需求。

## 系统架构

系统由以下主要组件构成：

1.  **API 网关** (`cmd/api-gateway`)：处理外部 RESTful API 请求，是系统的统一入口。
2.  **队列管理器** (`cmd/queue-manager`)：管理多级优先级队列，负责消息的入队和出队。
3.  **调度器** (`cmd/scheduler`)：动态分配和管理资源，监控系统负载，确保服务稳定。
4.  **状态管理器** (`internal/statemanager`)：维护和管理对话的上下文及状态。
5.  **预处理器** (`internal/preprocessor`)：在消息入队前进行分析，确定其优先级。
6.  **负载均衡器** (`internal/loadbalancer`)：在多个处理端点之间智能分配请求。

## 目录结构

```
/
├── api/
│   └── handlers.go
├── cmd/
│   ├── api-gateway/
│   │   └── main.go
│   ├── queue-manager/
│   │   └── main.go
│   ├── scheduler/
│   │   └── main.go
│   └── server/
│       └── main.go
├── configs/
│   └── config.yaml
├── deployments/
│   ├── Dockerfile
│   ├── docker-compose.yml
│   └── prometheus.yml
├── docs/
│   ├── api.md
│   ├── architecture.md
│   ├── configuration.md
│   ├── deployment.md
│   ├── development.md
│   └── performance.md
├── go.mod
├── go.sum
├── internal/
│   ├── conversation/
│   │   ├── persistence.go
│   │   └── state_manager.go
│   ├── loadbalancer/
│   │   └── load_balancer.go
│   ├── preprocessor/
│   │   └── preprocessor.go
│   ├── priorityqueue/
│   │   ├── dead_letter_queue.go
│   │   ├── delayed_queue.go
│   │   ├── queue.go
│   │   ├── queue_factory.go
│   │   ├── queue_manager.go
│   │   └── worker.go
│   ├── scheduler/
│   │   ├── resource_scheduler.go
│   │   └── scheduler.go
│   └── statemanager/
│       └── manager.go
├── pkg/
│   ├── config/
│   │   └── config.go
│   ├── models/
│   │   └── message.go
│   └── utils/
└── tests/
    ├── loadbalancer_test.go
    ├── preprocessor_test.go
    ├── priorityqueue_test.go
    └── queue_factory_test.go
```

## 技术栈

- **Go**: 主要开发语言
- **PostgreSQL**: 持久化存储
- **Redis**: 缓存和消息队列
- **Gin**: Web 框架
- **GORM**: ORM 库
- **Prometheus**: 监控和指标收集
- **Zap**: 日志记录
- **Viper**: 配置管理
- **Docker**: 容器化

## 快速开始

### 环境准备

- Go 1.21+
- Docker
- Docker Compose

### 本地开发

1.  **克隆仓库**

    ```bash
    git clone https://github.com/ZhangLearning/llm-message-queue.git
    cd llm-message-queue
    ```

2.  **配置**

    复制 `configs/config.yaml.example` 并重命名为 `configs/config.yaml`，然后根据本地环境修改配置。

3.  **启动服务 (使用 Docker Compose)**

    ```bash
    docker-compose up --build
    ```

    此命令将启动所有服务，包括 API 网关、队列管理器、调度器以及所需的数据库和缓存。

4.  **手动构建和运行**

    项目同时支持微服务和单体两种部署模式。

    **单体模式 (推荐)**

    构建并运行集成的 `server` 服务：

    ```bash
    # 构建服务
    go build -o bin/server cmd/server/main.go

    # 启动服务
    ./bin/server --config=configs/config.yaml
    ```

    **微服务模式**

    如果您希望独立运行每个服务：

    ```bash
    # 构建服务
    go build -o bin/api-gateway cmd/api-gateway/main.go
    go build -o bin/queue-manager cmd/queue-manager/main.go
    go build -o bin/scheduler cmd/scheduler/main.go

    # 启动服务 (需要分别在不同终端中运行)
    ./bin/api-gateway --config=configs/config.yaml
    ./bin/queue-manager --config=configs/config.yaml
    ./bin/scheduler --config=configs/config.yaml
    ```

## API 端点

### 发送消息

- **POST** `/api/v1/messages`

  ```json
  {
    "user_id": "user123",
    "content": "这是一条测试消息",
    "priority": "high"
  }
  ```

### 创建对话

- **POST** `/api/v1/conversations`

  ```json
  {
    "user_id": "user123",
    "title": "测试对话"
  }
  ```

### 向对话添加消息

- **POST** `/api/v1/conversations/{conv_id}/messages`

  ```json
  {
    "content": "这是对话中的一条消息"
  }
  ```

## 配置

应用配置通过 `configs/config.yaml` 文件管理。您可以配置数据库连接、Redis 地址、服务器端口等。

```yaml
server:
  port: 8080
database:
  host: localhost
  port: 5432
  user: user
  password: password
  dbname: llm_queue
redis:
  addr: localhost:6379
```

## 文档

本项目提供了详细的文档，以帮助您更好地理解和使用系统：

- **[系统架构](docs/architecture.md)**: 深入了解系统设计和组件交互。
- **[API 参考](docs/api.md)**: 完整的 API 接口文档。
- **[部署指南](docs/deployment.md)**: 生产环境部署和运维说明。
- **[开发指南](docs/development.md)**: 开发环境搭建、代码规范和贡献流程。
- **[配置说明](docs/configuration.md)**: 所有配置参数的详细解释。
- **[性能调优](docs/performance.md)**: 关于如何优化系统性能的建议。

## 贡献

我们欢迎任何形式的贡献！如果您希望为项目做出贡献，请遵循以下步骤：

1.  Fork 本仓库。
2.  创建一个新的分支 (`git checkout -b feature/your-feature`)。
3.  提交您的更改 (`git commit -m 'Add some feature'`)。
4.  将您的分支推送到远程仓库 (`git push origin feature/your-feature`)。
5.  创建一个新的 Pull Request。

请确保您的代码符合项目规范，并通过所有测试。

## 许可证

本项目基于 [MIT License](LICENSE) 开源。
