# LLM消息队列系统部署和运维文档

## 1. 部署概述

### 1.1 系统要求

#### 1.1.1 硬件要求
**最低配置**:
- CPU: 2核心
- 内存: 4GB RAM
- 存储: 20GB SSD
- 网络: 100Mbps

**推荐配置**:
- CPU: 8核心
- 内存: 16GB RAM
- 存储: 100GB SSD
- 网络: 1Gbps

**生产环境配置**:
- CPU: 16核心
- 内存: 32GB RAM
- 存储: 500GB SSD (系统) + 1TB SSD (数据)
- 网络: 10Gbps

#### 1.1.2 软件要求
- **操作系统**: Linux (Ubuntu 20.04+ / CentOS 8+ / RHEL 8+)
- **Docker**: 20.10+
- **Docker Compose**: 2.0+
- **Go**: 1.21+ (开发环境)
- **Git**: 2.25+

### 1.2 依赖服务
- **PostgreSQL**: 13+
- **Redis**: 6.0+
- **Prometheus**: 2.30+ (可选，用于监控)
- **Grafana**: 8.0+ (可选，用于可视化)

## 2. 快速部署

### 2.1 使用 Docker Compose 部署

#### 2.1.1 克隆项目
```bash
git clone <repository-url>
cd llm-message-queue
```

#### 2.1.2 环境配置
```bash
# 复制配置文件
cp configs/config.yaml.example configs/config.yaml

# 编辑配置文件
vim configs/config.yaml
```

#### 2.1.3 启动服务
```bash
# 启动所有服务
docker-compose up -d

# 查看服务状态
docker-compose ps

# 查看日志
docker-compose logs -f
```

#### 2.1.4 验证部署
```bash
# 健康检查
curl http://localhost:8080/api/v1/health

# 查看队列状态
curl http://localhost:8080/api/v1/queues/status
```

### 2.2 服务端口说明
| 服务 | 端口 | 说明 |
|------|------|------|
| API Gateway | 8080 | 主要API入口 |
| Queue Manager | 8081 | 队列管理服务 |
| Scheduler | 8082 | 调度服务 |
| Server | 8083 | 主服务器 |
| PostgreSQL | 5432 | 数据库 |
| Redis | 6379 | 缓存服务 |
| Prometheus | 9090 | 监控指标收集 |
| Grafana | 3000 | 监控面板 |

## 3. 生产环境部署

### 3.1 环境准备

#### 3.1.1 创建用户和目录
```bash
# 创建应用用户
sudo useradd -m -s /bin/bash llmqueue
sudo usermod -aG docker llmqueue

# 创建应用目录
sudo mkdir -p /opt/llm-message-queue
sudo chown llmqueue:llmqueue /opt/llm-message-queue

# 创建数据目录
sudo mkdir -p /data/llm-queue/{postgres,redis,logs}
sudo chown -R llmqueue:llmqueue /data/llm-queue
```

#### 3.1.2 配置防火墙
```bash
# Ubuntu/Debian
sudo ufw allow 8080/tcp
sudo ufw allow 22/tcp
sudo ufw enable

# CentOS/RHEL
sudo firewall-cmd --permanent --add-port=8080/tcp
sudo firewall-cmd --permanent --add-port=22/tcp
sudo firewall-cmd --reload
```

### 3.2 数据库配置

#### 3.2.1 PostgreSQL 配置
```bash
# 创建数据库和用户
sudo -u postgres psql << EOF
CREATE DATABASE llm_queue;
CREATE USER llmqueue WITH PASSWORD 'your_secure_password';
GRANT ALL PRIVILEGES ON DATABASE llm_queue TO llmqueue;
\q
EOF
```

#### 3.2.2 Redis 配置
```bash
# 创建 Redis 配置文件
sudo mkdir -p /etc/redis
sudo tee /etc/redis/redis.conf << EOF
bind 127.0.0.1
port 6379
requireauth your_redis_password
maxmemory 2gb
maxmemory-policy allkeys-lru
save 900 1
save 300 10
save 60 10000
EOF
```

### 3.3 应用配置

#### 3.3.1 生产环境配置文件
```yaml
# configs/config.yaml
server:
  port: 8080
  host: "0.0.0.0"
  mode: "production"

database:
  postgres:
    host: "localhost"
    port: 5432
    user: "llmqueue"
    password: "your_secure_password"
    dbname: "llm_queue"
    sslmode: "require"
  redis:
    addr: "localhost:6379"
    password: "your_redis_password"
    db: 0
    pool_size: 100

queue:
  levels:
    - name: "realtime"
      priority: 1
      max_wait_time: "100ms"
      max_concurrent: 10
    - name: "high"
      priority: 2
      max_wait_time: "5s"
      max_concurrent: 20
    - name: "normal"
      priority: 3
      max_wait_time: "30s"
      max_concurrent: 30
    - name: "low"
      priority: 4
      max_wait_time: "120s"
      max_concurrent: 20
  default_max_size: 10000
  monitor_interval: "30s"
  cleanup_interval: "5m"
  enable_metrics: true
  enable_auto_scaling: true
  worker:
    max_batch_size: 100
    process_interval: "1s"
    max_concurrent: 50
  retry:
    initial_backoff: "1s"
    max_backoff: "60s"
    factor: 2.0
    max_retries: 3

scheduler:
  strategy: "priority_based"
  check_interval: "1s"
  max_retries: 3
  timeout: "30s"

loadbalancer:
  algorithm: "round_robin"
  health_check_interval: "10s"
  max_failures: 3
  enable_session_affinity: true
  session_timeout: "30m"

logging:
  level: "info"
  format: "json"
  output: "/data/llm-queue/logs/app.log"

metrics:
  enabled: true
  port: 9090
  path: "/metrics"
```

### 3.4 Docker Compose 生产配置

```yaml
# docker-compose.prod.yml
version: '3.8'

services:
  postgres:
    image: postgres:15
    container_name: llm-queue-postgres
    environment:
      POSTGRES_DB: llm_queue
      POSTGRES_USER: llmqueue
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}
    volumes:
      - /data/llm-queue/postgres:/var/lib/postgresql/data
    ports:
      - "5432:5432"
    restart: unless-stopped
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U llmqueue"]
      interval: 30s
      timeout: 10s
      retries: 3

  redis:
    image: redis:7-alpine
    container_name: llm-queue-redis
    command: redis-server --requirepass ${REDIS_PASSWORD}
    volumes:
      - /data/llm-queue/redis:/data
    ports:
      - "6379:6379"
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 30s
      timeout: 10s
      retries: 3

  api-gateway:
    build:
      context: .
      dockerfile: deployments/Dockerfile.api-gateway
    container_name: llm-queue-api-gateway
    ports:
      - "8080:8080"
    volumes:
      - ./configs:/app/configs
      - /data/llm-queue/logs:/app/logs
    environment:
      - CONFIG_PATH=/app/configs/config.yaml
    depends_on:
      - postgres
      - redis
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8080/api/v1/health"]
      interval: 30s
      timeout: 10s
      retries: 3

  queue-manager:
    build:
      context: .
      dockerfile: deployments/Dockerfile.queue-manager
    container_name: llm-queue-manager
    ports:
      - "8081:8081"
    volumes:
      - ./configs:/app/configs
      - /data/llm-queue/logs:/app/logs
    environment:
      - CONFIG_PATH=/app/configs/config.yaml
    depends_on:
      - postgres
      - redis
    restart: unless-stopped

  scheduler:
    build:
      context: .
      dockerfile: deployments/Dockerfile.scheduler
    container_name: llm-queue-scheduler
    ports:
      - "8082:8082"
    volumes:
      - ./configs:/app/configs
      - /data/llm-queue/logs:/app/logs
    environment:
      - CONFIG_PATH=/app/configs/config.yaml
    depends_on:
      - postgres
      - redis
    restart: unless-stopped

  server:
    build:
      context: .
      dockerfile: deployments/Dockerfile.server
    container_name: llm-queue-server
    ports:
      - "8083:8083"
    volumes:
      - ./configs:/app/configs
      - /data/llm-queue/logs:/app/logs
    environment:
      - CONFIG_PATH=/app/configs/config.yaml
    depends_on:
      - postgres
      - redis
    restart: unless-stopped

  prometheus:
    image: prom/prometheus:latest
    container_name: llm-queue-prometheus
    ports:
      - "9090:9090"
    volumes:
      - ./deployments/prometheus.yml:/etc/prometheus/prometheus.yml
      - /data/llm-queue/prometheus:/prometheus
    command:
      - '--config.file=/etc/prometheus/prometheus.yml'
      - '--storage.tsdb.path=/prometheus'
      - '--web.console.libraries=/etc/prometheus/console_libraries'
      - '--web.console.templates=/etc/prometheus/consoles'
    restart: unless-stopped

  grafana:
    image: grafana/grafana:latest
    container_name: llm-queue-grafana
    ports:
      - "3000:3000"
    volumes:
      - /data/llm-queue/grafana:/var/lib/grafana
    environment:
      - GF_SECURITY_ADMIN_PASSWORD=${GRAFANA_PASSWORD}
    restart: unless-stopped

networks:
  default:
    name: llm-queue-network
```

### 3.5 环境变量配置

```bash
# .env
POSTGRES_PASSWORD=your_secure_postgres_password
REDIS_PASSWORD=your_secure_redis_password
GRAFANA_PASSWORD=your_secure_grafana_password
```

## 4. 监控和日志

### 4.1 Prometheus 配置

```yaml
# deployments/prometheus.yml
global:
  scrape_interval: 15s
  evaluation_interval: 15s

rule_files:
  - "alert_rules.yml"

scrape_configs:
  - job_name: 'llm-queue-api-gateway'
    static_configs:
      - targets: ['api-gateway:8080']
    metrics_path: '/metrics'
    scrape_interval: 30s

  - job_name: 'llm-queue-manager'
    static_configs:
      - targets: ['queue-manager:8081']
    metrics_path: '/metrics'
    scrape_interval: 30s

  - job_name: 'llm-queue-scheduler'
    static_configs:
      - targets: ['scheduler:8082']
    metrics_path: '/metrics'
    scrape_interval: 30s

  - job_name: 'llm-queue-server'
    static_configs:
      - targets: ['server:8083']
    metrics_path: '/metrics'
    scrape_interval: 30s

  - job_name: 'postgres-exporter'
    static_configs:
      - targets: ['postgres-exporter:9187']

  - job_name: 'redis-exporter'
    static_configs:
      - targets: ['redis-exporter:9121']

alerting:
  alertmanagers:
    - static_configs:
        - targets:
          - alertmanager:9093
```

### 4.2 告警规则配置

```yaml
# deployments/alert_rules.yml
groups:
  - name: llm-queue-alerts
    rules:
      - alert: HighErrorRate
        expr: rate(http_requests_total{status=~"5.."}[5m]) > 0.1
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "High error rate detected"
          description: "Error rate is {{ $value }} errors per second"

      - alert: HighQueueDepth
        expr: queue_depth > 1000
        for: 2m
        labels:
          severity: warning
        annotations:
          summary: "Queue depth is high"
          description: "Queue depth is {{ $value }} messages"

      - alert: DatabaseDown
        expr: up{job="postgres-exporter"} == 0
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "Database is down"
          description: "PostgreSQL database is not responding"

      - alert: RedisDown
        expr: up{job="redis-exporter"} == 0
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "Redis is down"
          description: "Redis cache is not responding"

      - alert: HighMemoryUsage
        expr: (node_memory_MemTotal_bytes - node_memory_MemAvailable_bytes) / node_memory_MemTotal_bytes > 0.9
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High memory usage"
          description: "Memory usage is {{ $value | humanizePercentage }}"

      - alert: HighCPUUsage
        expr: 100 - (avg by(instance) (irate(node_cpu_seconds_total{mode="idle"}[5m])) * 100) > 80
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High CPU usage"
          description: "CPU usage is {{ $value }}%"
```

### 4.3 日志管理

#### 4.3.1 日志轮转配置
```bash
# /etc/logrotate.d/llm-queue
/data/llm-queue/logs/*.log {
    daily
    rotate 30
    compress
    delaycompress
    missingok
    notifempty
    create 644 llmqueue llmqueue
    postrotate
        docker kill -s USR1 $(docker ps -q --filter name=llm-queue)
    endscript
}
```

#### 4.3.2 日志收集配置 (可选)
```yaml
# filebeat.yml
filebeat.inputs:
- type: log
  enabled: true
  paths:
    - /data/llm-queue/logs/*.log
  fields:
    service: llm-queue
  fields_under_root: true
  json.keys_under_root: true
  json.add_error_key: true

output.elasticsearch:
  hosts: ["elasticsearch:9200"]
  index: "llm-queue-%{+yyyy.MM.dd}"

logging.level: info
logging.to_files: true
logging.files:
  path: /var/log/filebeat
  name: filebeat
  keepfiles: 7
  permissions: 0644
```

## 5. 备份和恢复

### 5.1 数据库备份

#### 5.1.1 自动备份脚本
```bash
#!/bin/bash
# /opt/llm-message-queue/scripts/backup.sh

BACKUP_DIR="/data/llm-queue/backups"
DATE=$(date +%Y%m%d_%H%M%S)
DB_NAME="llm_queue"
DB_USER="llmqueue"

# 创建备份目录
mkdir -p $BACKUP_DIR

# PostgreSQL 备份
echo "Starting PostgreSQL backup..."
pg_dump -h localhost -U $DB_USER -d $DB_NAME | gzip > $BACKUP_DIR/postgres_$DATE.sql.gz

# Redis 备份
echo "Starting Redis backup..."
redis-cli --rdb $BACKUP_DIR/redis_$DATE.rdb

# 清理旧备份 (保留7天)
find $BACKUP_DIR -name "*.gz" -mtime +7 -delete
find $BACKUP_DIR -name "*.rdb" -mtime +7 -delete

echo "Backup completed: $DATE"
```

#### 5.1.2 定时备份
```bash
# 添加到 crontab
crontab -e

# 每天凌晨2点执行备份
0 2 * * * /opt/llm-message-queue/scripts/backup.sh >> /data/llm-queue/logs/backup.log 2>&1
```

### 5.2 数据恢复

#### 5.2.1 PostgreSQL 恢复
```bash
# 恢复数据库
gunzip -c /data/llm-queue/backups/postgres_20250101_020000.sql.gz | psql -h localhost -U llmqueue -d llm_queue
```

#### 5.2.2 Redis 恢复
```bash
# 停止 Redis 服务
docker stop llm-queue-redis

# 复制备份文件
cp /data/llm-queue/backups/redis_20250101_020000.rdb /data/llm-queue/redis/dump.rdb

# 启动 Redis 服务
docker start llm-queue-redis
```

## 6. 安全配置

### 6.1 SSL/TLS 配置

#### 6.1.1 生成证书
```bash
# 使用 Let's Encrypt
sudo apt install certbot
sudo certbot certonly --standalone -d your-domain.com

# 或者生成自签名证书
openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
  -keyout /etc/ssl/private/llm-queue.key \
  -out /etc/ssl/certs/llm-queue.crt
```

#### 6.1.2 Nginx 反向代理配置
```nginx
# /etc/nginx/sites-available/llm-queue
server {
    listen 80;
    server_name your-domain.com;
    return 301 https://$server_name$request_uri;
}

server {
    listen 443 ssl http2;
    server_name your-domain.com;

    ssl_certificate /etc/ssl/certs/llm-queue.crt;
    ssl_certificate_key /etc/ssl/private/llm-queue.key;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers ECDHE-RSA-AES256-GCM-SHA512:DHE-RSA-AES256-GCM-SHA512:ECDHE-RSA-AES256-GCM-SHA384:DHE-RSA-AES256-GCM-SHA384;
    ssl_prefer_server_ciphers off;
    ssl_session_cache shared:SSL:10m;
    ssl_session_timeout 10m;

    location / {
        proxy_pass http://localhost:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_connect_timeout 30s;
        proxy_send_timeout 30s;
        proxy_read_timeout 30s;
    }

    location /metrics {
        deny all;
        return 403;
    }
}
```

### 6.2 防火墙配置

```bash
# UFW 配置
sudo ufw default deny incoming
sudo ufw default allow outgoing
sudo ufw allow ssh
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp
sudo ufw enable

# 限制内部服务端口访问
sudo ufw deny 5432
sudo ufw deny 6379
sudo ufw deny 9090
```

## 7. 性能优化

### 7.1 系统优化

#### 7.1.1 内核参数优化
```bash
# /etc/sysctl.conf
net.core.somaxconn = 65535
net.core.netdev_max_backlog = 5000
net.ipv4.tcp_max_syn_backlog = 65535
net.ipv4.tcp_fin_timeout = 30
net.ipv4.tcp_keepalive_time = 1200
net.ipv4.tcp_max_tw_buckets = 5000
vm.swappiness = 10
vm.dirty_ratio = 15
vm.dirty_background_ratio = 5

# 应用配置
sudo sysctl -p
```

#### 7.1.2 文件描述符限制
```bash
# /etc/security/limits.conf
llmqueue soft nofile 65535
llmqueue hard nofile 65535
llmqueue soft nproc 32768
llmqueue hard nproc 32768
```

### 7.2 数据库优化

#### 7.2.1 PostgreSQL 优化
```sql
-- postgresql.conf 关键配置
shared_buffers = 4GB
effective_cache_size = 12GB
work_mem = 256MB
maintenance_work_mem = 1GB
max_connections = 200
checkpoint_completion_target = 0.9
wal_buffers = 64MB
default_statistics_target = 100
```

#### 7.2.2 Redis 优化
```bash
# redis.conf 关键配置
maxmemory 8gb
maxmemory-policy allkeys-lru
tcp-keepalive 300
timeout 0
tcp-backlog 511
```

## 8. 故障排除

### 8.1 常见问题

#### 8.1.1 服务无法启动
```bash
# 检查日志
docker-compose logs api-gateway

# 检查配置文件
go run cmd/api-gateway/main.go -config configs/config.yaml -validate

# 检查端口占用
netstat -tlnp | grep 8080
```

#### 8.1.2 数据库连接问题
```bash
# 测试数据库连接
psql -h localhost -U llmqueue -d llm_queue -c "SELECT 1;"

# 检查 Redis 连接
redis-cli -h localhost -p 6379 ping
```

#### 8.1.3 性能问题
```bash
# 检查系统资源
top
htop
iostat -x 1

# 检查应用指标
curl http://localhost:8080/api/v1/metrics
```

### 8.2 日志分析

#### 8.2.1 错误日志查看
```bash
# 查看应用错误日志
tail -f /data/llm-queue/logs/app.log | grep ERROR

# 查看特定时间段的日志
journalctl -u docker --since "2025-01-01 10:00:00" --until "2025-01-01 11:00:00"
```

#### 8.2.2 性能分析
```bash
# 分析慢查询
grep "slow query" /data/llm-queue/logs/app.log

# 分析请求响应时间
awk '/response_time/ {sum+=$NF; count++} END {print "Average:", sum/count}' /data/llm-queue/logs/app.log
```

## 9. 升级和维护

### 9.1 版本升级

#### 9.1.1 升级流程
```bash
# 1. 备份数据
/opt/llm-message-queue/scripts/backup.sh

# 2. 拉取新版本
git pull origin main

# 3. 构建新镜像
docker-compose build

# 4. 滚动更新
docker-compose up -d --no-deps api-gateway
docker-compose up -d --no-deps queue-manager
docker-compose up -d --no-deps scheduler
docker-compose up -d --no-deps server

# 5. 验证服务
curl http://localhost:8080/api/v1/health
```

#### 9.1.2 回滚流程
```bash
# 回滚到上一个版本
git checkout <previous-commit>
docker-compose build
docker-compose up -d
```

### 9.2 定期维护

#### 9.2.1 维护检查清单
- [ ] 检查磁盘空间使用情况
- [ ] 检查日志文件大小
- [ ] 检查数据库性能指标
- [ ] 检查系统资源使用情况
- [ ] 更新安全补丁
- [ ] 检查备份完整性
- [ ] 检查监控告警

#### 9.2.2 维护脚本
```bash
#!/bin/bash
# /opt/llm-message-queue/scripts/maintenance.sh

echo "Starting maintenance tasks..."

# 清理旧日志
find /data/llm-queue/logs -name "*.log" -mtime +30 -delete

# 清理 Docker 镜像
docker system prune -f

# 检查磁盘空间
df -h /data

# 检查服务状态
docker-compose ps

# 检查数据库连接
psql -h localhost -U llmqueue -d llm_queue -c "SELECT COUNT(*) FROM messages;"

echo "Maintenance tasks completed."
```

## 10. 扩展部署

### 10.1 多实例部署

#### 10.1.1 负载均衡配置
```yaml
# docker-compose.scale.yml
version: '3.8'

services:
  nginx:
    image: nginx:alpine
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./nginx.conf:/etc/nginx/nginx.conf
    depends_on:
      - api-gateway

  api-gateway:
    build: .
    deploy:
      replicas: 3
    environment:
      - CONFIG_PATH=/app/configs/config.yaml
```

#### 10.1.2 数据库集群
```yaml
# PostgreSQL 主从配置
postgres-master:
  image: postgres:15
  environment:
    POSTGRES_REPLICATION_MODE: master
    POSTGRES_REPLICATION_USER: replicator
    POSTGRES_REPLICATION_PASSWORD: replicator_password

postgres-slave:
  image: postgres:15
  environment:
    POSTGRES_REPLICATION_MODE: slave
    POSTGRES_MASTER_HOST: postgres-master
    POSTGRES_REPLICATION_USER: replicator
    POSTGRES_REPLICATION_PASSWORD: replicator_password
```

### 10.2 容器编排 (Kubernetes)

#### 10.2.1 部署配置
```yaml
# k8s/deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: llm-queue-api-gateway
spec:
  replicas: 3
  selector:
    matchLabels:
      app: llm-queue-api-gateway
  template:
    metadata:
      labels:
        app: llm-queue-api-gateway
    spec:
      containers:
      - name: api-gateway
        image: llm-queue/api-gateway:latest
        ports:
        - containerPort: 8080
        env:
        - name: CONFIG_PATH
          value: "/app/configs/config.yaml"
        resources:
          requests:
            memory: "512Mi"
            cpu: "250m"
          limits:
            memory: "1Gi"
            cpu: "500m"
        livenessProbe:
          httpGet:
            path: /api/v1/health
            port: 8080
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /api/v1/health
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5
```

---

*本文档版本: v1.0*
*最后更新: 2025年1月*
*维护者: 运维团队*
