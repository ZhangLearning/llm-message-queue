# LLM消息队列系统 API 接口文档

## 1. API 概述

### 1.1 基本信息
- **Base URL**: `http://localhost:8080/api/v1`
- **协议**: HTTP/HTTPS
- **数据格式**: JSON
- **字符编码**: UTF-8
- **API版本**: v1

### 1.2 通用响应格式
```json
{
  "code": 200,
  "message": "success",
  "data": {},
  "timestamp": "2025-01-01T10:30:00Z"
}
```

### 1.3 HTTP状态码
- `200 OK`: 请求成功
- `201 Created`: 资源创建成功
- `400 Bad Request`: 请求参数错误
- `401 Unauthorized`: 未授权
- `404 Not Found`: 资源不存在
- `500 Internal Server Error`: 服务器内部错误

### 1.4 错误响应格式
```json
{
  "code": 400,
  "message": "Invalid request parameters",
  "error": "validation failed",
  "details": {
    "field": "content",
    "reason": "content cannot be empty"
  },
  "timestamp": "2025-01-01T10:30:00Z"
}
```

## 2. 消息管理 API

### 2.1 发送消息

**接口描述**: 向队列发送新消息

**请求信息**:
- **URL**: `POST /messages`
- **Content-Type**: `application/json`

**请求参数**:
```json
{
  "conversation_id": "conv_123456",
  "user_id": "user_789",
  "content": "Hello, this is a test message",
  "priority": "normal",
  "metadata": {
    "source": "web",
    "client_version": "1.0.0"
  }
}
```

**参数说明**:
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| conversation_id | string | 是 | 对话ID |
| user_id | string | 是 | 用户ID |
| content | string | 是 | 消息内容 |
| priority | string | 否 | 优先级: realtime/high/normal/low，默认normal |
| metadata | object | 否 | 元数据信息 |

**响应示例**:
```json
{
  "code": 201,
  "message": "Message created successfully",
  "data": {
    "id": "msg_abc123",
    "conversation_id": "conv_123456",
    "user_id": "user_789",
    "content": "Hello, this is a test message",
    "priority": "normal",
    "status": "pending",
    "queue_name": "normal_priority",
    "retry_count": 0,
    "max_retries": 3,
    "timeout": "30s",
    "created_at": "2025-01-01T10:30:00Z",
    "updated_at": "2025-01-01T10:30:00Z",
    "metadata": {
      "source": "web",
      "client_version": "1.0.0"
    }
  },
  "timestamp": "2025-01-01T10:30:00Z"
}
```

### 2.2 获取消息详情

**接口描述**: 根据消息ID获取消息详细信息

**请求信息**:
- **URL**: `GET /messages/{message_id}`
- **方法**: GET

**路径参数**:
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| message_id | string | 是 | 消息ID |

**响应示例**:
```json
{
  "code": 200,
  "message": "success",
  "data": {
    "id": "msg_abc123",
    "conversation_id": "conv_123456",
    "user_id": "user_789",
    "content": "Hello, this is a test message",
    "priority": "normal",
    "status": "completed",
    "queue_name": "normal_priority",
    "retry_count": 0,
    "max_retries": 3,
    "timeout": "30s",
    "created_at": "2025-01-01T10:30:00Z",
    "updated_at": "2025-01-01T10:30:15Z",
    "scheduled_at": "2025-01-01T10:30:01Z",
    "completed_at": "2025-01-01T10:30:15Z",
    "metadata": {
      "source": "web",
      "client_version": "1.0.0",
      "processing_time": "14s"
    }
  },
  "timestamp": "2025-01-01T10:35:00Z"
}
```

### 2.3 获取消息列表

**接口描述**: 获取消息列表，支持分页和过滤

**请求信息**:
- **URL**: `GET /messages`
- **方法**: GET

**查询参数**:
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| conversation_id | string | 否 | 对话ID过滤 |
| user_id | string | 否 | 用户ID过滤 |
| status | string | 否 | 状态过滤: pending/processing/completed/failed/timeout |
| priority | string | 否 | 优先级过滤: realtime/high/normal/low |
| page | int | 否 | 页码，默认1 |
| page_size | int | 否 | 每页大小，默认20，最大100 |
| sort | string | 否 | 排序字段: created_at/updated_at/priority，默认created_at |
| order | string | 否 | 排序方向: asc/desc，默认desc |

**响应示例**:
```json
{
  "code": 200,
  "message": "success",
  "data": {
    "messages": [
      {
        "id": "msg_abc123",
        "conversation_id": "conv_123456",
        "user_id": "user_789",
        "content": "Hello, this is a test message",
        "priority": "normal",
        "status": "completed",
        "created_at": "2025-01-01T10:30:00Z",
        "updated_at": "2025-01-01T10:30:15Z"
      }
    ],
    "pagination": {
      "page": 1,
      "page_size": 20,
      "total": 1,
      "total_pages": 1
    }
  },
  "timestamp": "2025-01-01T10:35:00Z"
}
```

### 2.4 更新消息状态

**接口描述**: 更新消息的处理状态

**请求信息**:
- **URL**: `PUT /messages/{message_id}/status`
- **Content-Type**: `application/json`

**路径参数**:
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| message_id | string | 是 | 消息ID |

**请求参数**:
```json
{
  "status": "processing",
  "metadata": {
    "processor_id": "worker_001",
    "start_time": "2025-01-01T10:30:01Z"
  }
}
```

**参数说明**:
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| status | string | 是 | 新状态: pending/processing/completed/failed/timeout |
| metadata | object | 否 | 更新的元数据 |

**响应示例**:
```json
{
  "code": 200,
  "message": "Message status updated successfully",
  "data": {
    "id": "msg_abc123",
    "status": "processing",
    "updated_at": "2025-01-01T10:30:01Z"
  },
  "timestamp": "2025-01-01T10:30:01Z"
}
```

### 2.5 删除消息

**接口描述**: 删除指定消息

**请求信息**:
- **URL**: `DELETE /messages/{message_id}`
- **方法**: DELETE

**路径参数**:
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| message_id | string | 是 | 消息ID |

**响应示例**:
```json
{
  "code": 200,
  "message": "Message deleted successfully",
  "data": null,
  "timestamp": "2025-01-01T10:35:00Z"
}
```

## 3. 对话管理 API

### 3.1 创建对话

**接口描述**: 创建新的对话会话

**请求信息**:
- **URL**: `POST /conversations`
- **Content-Type**: `application/json`

**请求参数**:
```json
{
  "user_id": "user_789",
  "title": "技术咨询对话",
  "context": "用户询问关于API使用的问题",
  "priority": "normal",
  "metadata": {
    "source": "web",
    "category": "technical_support"
  }
}
```

**参数说明**:
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| user_id | string | 是 | 用户ID |
| title | string | 否 | 对话标题 |
| context | string | 否 | 对话上下文 |
| priority | string | 否 | 优先级，默认normal |
| metadata | object | 否 | 元数据信息 |

**响应示例**:
```json
{
  "code": 201,
  "message": "Conversation created successfully",
  "data": {
    "id": "conv_123456",
    "user_id": "user_789",
    "title": "技术咨询对话",
    "context": "用户询问关于API使用的问题",
    "status": "active",
    "state": "active",
    "priority": "normal",
    "created_at": "2025-01-01T10:30:00Z",
    "updated_at": "2025-01-01T10:30:00Z",
    "last_active_at": "2025-01-01T10:30:00Z",
    "expires_at": "2024-01-16T10:30:00Z",
    "metadata": {
      "source": "web",
      "category": "technical_support"
    }
  },
  "timestamp": "2025-01-01T10:30:00Z"
}
```

### 3.2 获取对话详情

**接口描述**: 根据对话ID获取对话详细信息

**请求信息**:
- **URL**: `GET /conversations/{conversation_id}`
- **方法**: GET

**路径参数**:
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| conversation_id | string | 是 | 对话ID |

**响应示例**:
```json
{
  "code": 200,
  "message": "success",
  "data": {
    "id": "conv_123456",
    "user_id": "user_789",
    "title": "技术咨询对话",
    "context": "用户询问关于API使用的问题",
    "status": "active",
    "state": "active",
    "priority": "normal",
    "created_at": "2025-01-01T10:30:00Z",
    "updated_at": "2025-01-01T10:35:00Z",
    "last_active_at": "2025-01-01T10:35:00Z",
    "expires_at": "2024-01-16T10:30:00Z",
    "message_count": 5,
    "metadata": {
      "source": "web",
      "category": "technical_support",
      "total_processing_time": "45s"
    }
  },
  "timestamp": "2025-01-01T10:40:00Z"
}
```

### 3.3 获取对话列表

**接口描述**: 获取对话列表，支持分页和过滤

**请求信息**:
- **URL**: `GET /conversations`
- **方法**: GET

**查询参数**:
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| user_id | string | 否 | 用户ID过滤 |
| state | string | 否 | 状态过滤: active/inactive/completed/archived |
| priority | string | 否 | 优先级过滤 |
| page | int | 否 | 页码，默认1 |
| page_size | int | 否 | 每页大小，默认20 |
| sort | string | 否 | 排序字段，默认created_at |
| order | string | 否 | 排序方向，默认desc |

**响应示例**:
```json
{
  "code": 200,
  "message": "success",
  "data": {
    "conversations": [
      {
        "id": "conv_123456",
        "user_id": "user_789",
        "title": "技术咨询对话",
        "state": "active",
        "priority": "normal",
        "message_count": 5,
        "created_at": "2025-01-01T10:30:00Z",
        "last_active_at": "2025-01-01T10:35:00Z"
      }
    ],
    "pagination": {
      "page": 1,
      "page_size": 20,
      "total": 1,
      "total_pages": 1
    }
  },
  "timestamp": "2025-01-01T10:40:00Z"
}
```

### 3.4 更新对话状态

**接口描述**: 更新对话的状态信息

**请求信息**:
- **URL**: `PUT /conversations/{conversation_id}`
- **Content-Type**: `application/json`

**路径参数**:
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| conversation_id | string | 是 | 对话ID |

**请求参数**:
```json
{
  "state": "completed",
  "title": "已完成的技术咨询",
  "context": "问题已解决，用户满意",
  "metadata": {
    "resolution": "问题已解决",
    "satisfaction_score": 5
  }
}
```

**参数说明**:
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| state | string | 否 | 对话状态 |
| title | string | 否 | 对话标题 |
| context | string | 否 | 对话上下文 |
| metadata | object | 否 | 元数据更新 |

**响应示例**:
```json
{
  "code": 200,
  "message": "Conversation updated successfully",
  "data": {
    "id": "conv_123456",
    "state": "completed",
    "title": "已完成的技术咨询",
    "updated_at": "2025-01-01T11:00:00Z"
  },
  "timestamp": "2025-01-01T11:00:00Z"
}
```

## 4. 队列管理 API

### 4.1 获取队列状态

**接口描述**: 获取所有队列的当前状态信息

**请求信息**:
- **URL**: `GET /queues/status`
- **方法**: GET

**响应示例**:
```json
{
  "code": 200,
  "message": "success",
  "data": {
    "queues": [
      {
        "name": "realtime",
        "priority": 1,
        "size": 5,
        "max_size": 1000,
        "processing_count": 2,
        "max_concurrent": 10,
        "avg_wait_time": "50ms",
        "max_wait_time": "100ms",
        "total_processed": 1250,
        "total_failed": 3,
        "last_updated": "2025-01-01T10:40:00Z"
      },
      {
        "name": "high",
        "priority": 2,
        "size": 15,
        "max_size": 5000,
        "processing_count": 8,
        "max_concurrent": 20,
        "avg_wait_time": "2s",
        "max_wait_time": "5s",
        "total_processed": 5680,
        "total_failed": 12,
        "last_updated": "2025-01-01T10:40:00Z"
      },
      {
        "name": "normal",
        "priority": 3,
        "size": 45,
        "max_size": 10000,
        "processing_count": 15,
        "max_concurrent": 30,
        "avg_wait_time": "8s",
        "max_wait_time": "30s",
        "total_processed": 15420,
        "total_failed": 45,
        "last_updated": "2025-01-01T10:40:00Z"
      },
      {
        "name": "low",
        "priority": 4,
        "size": 120,
        "max_size": 20000,
        "processing_count": 10,
        "max_concurrent": 20,
        "avg_wait_time": "25s",
        "max_wait_time": "120s",
        "total_processed": 8950,
        "total_failed": 28,
        "last_updated": "2025-01-01T10:40:00Z"
      }
    ],
    "total_size": 185,
    "total_processing": 35,
    "total_processed": 31300,
    "total_failed": 88,
    "system_load": 0.65
  },
  "timestamp": "2025-01-01T10:40:00Z"
}
```

### 4.2 获取队列统计信息

**接口描述**: 获取队列的详细统计信息

**请求信息**:
- **URL**: `GET /queues/stats`
- **方法**: GET

**查询参数**:
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| queue_name | string | 否 | 队列名称过滤 |
| time_range | string | 否 | 时间范围: 1h/6h/24h/7d，默认1h |

**响应示例**:
```json
{
  "code": 200,
  "message": "success",
  "data": {
    "time_range": "1h",
    "stats": {
      "total_messages": 1250,
      "processed_messages": 1200,
      "failed_messages": 15,
      "pending_messages": 35,
      "avg_processing_time": "12.5s",
      "max_processing_time": "45s",
      "min_processing_time": "0.1s",
      "throughput_per_minute": 20.8,
      "error_rate": 0.012,
      "queue_utilization": 0.45
    },
    "priority_breakdown": {
      "realtime": {
        "count": 125,
        "percentage": 10.0,
        "avg_wait_time": "50ms"
      },
      "high": {
        "count": 375,
        "percentage": 30.0,
        "avg_wait_time": "2s"
      },
      "normal": {
        "count": 500,
        "percentage": 40.0,
        "avg_wait_time": "8s"
      },
      "low": {
        "count": 250,
        "percentage": 20.0,
        "avg_wait_time": "25s"
      }
    }
  },
  "timestamp": "2025-01-01T10:40:00Z"
}
```

## 5. 系统监控 API

### 5.1 健康检查

**接口描述**: 检查系统各组件的健康状态

**请求信息**:
- **URL**: `GET /health`
- **方法**: GET

**响应示例**:
```json
{
  "code": 200,
  "message": "System is healthy",
  "data": {
    "status": "healthy",
    "timestamp": "2025-01-01T10:40:00Z",
    "uptime": "72h35m20s",
    "version": "1.0.0",
    "components": {
      "database": {
        "status": "healthy",
        "response_time": "2ms",
        "last_check": "2025-01-01T10:39:58Z"
      },
      "redis": {
        "status": "healthy",
        "response_time": "1ms",
        "last_check": "2025-01-01T10:39:58Z"
      },
      "queue_manager": {
        "status": "healthy",
        "active_workers": 25,
        "last_check": "2025-01-01T10:39:59Z"
      },
      "scheduler": {
        "status": "healthy",
        "scheduled_tasks": 150,
        "last_check": "2025-01-01T10:39:59Z"
      }
    }
  },
  "timestamp": "2025-01-01T10:40:00Z"
}
```

### 5.2 系统指标

**接口描述**: 获取系统性能指标

**请求信息**:
- **URL**: `GET /metrics`
- **方法**: GET

**查询参数**:
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| format | string | 否 | 输出格式: json/prometheus，默认json |

**响应示例** (JSON格式):
```json
{
  "code": 200,
  "message": "success",
  "data": {
    "system": {
      "cpu_usage": 45.2,
      "memory_usage": 68.5,
      "disk_usage": 32.1,
      "network_in": "125.6 MB/s",
      "network_out": "89.3 MB/s",
      "goroutines": 1250,
      "gc_pause": "2.5ms"
    },
    "application": {
      "requests_per_second": 156.8,
      "avg_response_time": "125ms",
      "error_rate": 0.008,
      "active_connections": 450,
      "queue_depth": 185,
      "processing_messages": 35
    },
    "database": {
      "connections_active": 25,
      "connections_idle": 15,
      "query_duration_avg": "8ms",
      "slow_queries": 2
    },
    "redis": {
      "connected_clients": 45,
      "used_memory": "256MB",
      "hit_rate": 0.95,
      "ops_per_second": 2500
    }
  },
  "timestamp": "2025-01-01T10:40:00Z"
}
```

## 6. 配置管理 API

### 6.1 获取系统配置

**接口描述**: 获取当前系统配置信息

**请求信息**:
- **URL**: `GET /config`
- **方法**: GET

**响应示例**:
```json
{
  "code": 200,
  "message": "success",
  "data": {
    "server": {
      "port": 8080,
      "host": "0.0.0.0",
      "mode": "production"
    },
    "queue": {
      "default_max_size": 10000,
      "monitor_interval": "30s",
      "cleanup_interval": "5m",
      "enable_metrics": true,
      "enable_auto_scaling": true
    },
    "scheduler": {
      "strategy": "priority_based",
      "check_interval": "1s",
      "max_retries": 3,
      "timeout": "30s"
    },
    "loadbalancer": {
      "algorithm": "round_robin",
      "health_check_interval": "10s",
      "max_failures": 3,
      "enable_session_affinity": true
    }
  },
  "timestamp": "2025-01-01T10:40:00Z"
}
```

## 7. 错误码说明

### 7.1 业务错误码
| 错误码 | 说明 | HTTP状态码 |
|--------|------|------------|
| 1001 | 消息内容为空 | 400 |
| 1002 | 消息内容过长 | 400 |
| 1003 | 无效的优先级 | 400 |
| 1004 | 消息不存在 | 404 |
| 1005 | 对话不存在 | 404 |
| 1006 | 用户ID无效 | 400 |
| 1007 | 队列已满 | 503 |
| 1008 | 系统维护中 | 503 |
| 1009 | 请求频率过高 | 429 |
| 1010 | 消息处理超时 | 408 |

### 7.2 系统错误码
| 错误码 | 说明 | HTTP状态码 |
|--------|------|------------|
| 2001 | 数据库连接失败 | 500 |
| 2002 | Redis连接失败 | 500 |
| 2003 | 队列服务不可用 | 503 |
| 2004 | 调度器异常 | 500 |
| 2005 | 配置加载失败 | 500 |
| 2006 | 内存不足 | 500 |
| 2007 | 磁盘空间不足 | 500 |
| 2008 | 网络异常 | 500 |

## 8. SDK 和示例

### 8.1 cURL 示例

**发送消息**:
```bash
curl -X POST http://localhost:8080/api/v1/messages \
  -H "Content-Type: application/json" \
  -d '{
    "conversation_id": "conv_123456",
    "user_id": "user_789",
    "content": "Hello, this is a test message",
    "priority": "normal"
  }'
```

**获取队列状态**:
```bash
curl -X GET http://localhost:8080/api/v1/queues/status
```

**健康检查**:
```bash
curl -X GET http://localhost:8080/api/v1/health
```

### 8.2 JavaScript 示例

```javascript
// 发送消息
async function sendMessage(conversationId, userId, content, priority = 'normal') {
  const response = await fetch('http://localhost:8080/api/v1/messages', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({
      conversation_id: conversationId,
      user_id: userId,
      content: content,
      priority: priority
    })
  });

  const result = await response.json();
  return result;
}

// 获取消息状态
async function getMessageStatus(messageId) {
  const response = await fetch(`http://localhost:8080/api/v1/messages/${messageId}`);
  const result = await response.json();
  return result;
}

// 使用示例
sendMessage('conv_123', 'user_456', 'Hello World', 'high')
  .then(result => console.log('Message sent:', result))
  .catch(error => console.error('Error:', error));
```

### 8.3 Python 示例

```python
import requests
import json

class LLMQueueClient:
    def __init__(self, base_url="http://localhost:8080/api/v1"):
        self.base_url = base_url
        self.session = requests.Session()
        self.session.headers.update({'Content-Type': 'application/json'})

    def send_message(self, conversation_id, user_id, content, priority="normal"):
        """发送消息到队列"""
        data = {
            "conversation_id": conversation_id,
            "user_id": user_id,
            "content": content,
            "priority": priority
        }
        response = self.session.post(f"{self.base_url}/messages", json=data)
        return response.json()

    def get_message(self, message_id):
        """获取消息详情"""
        response = self.session.get(f"{self.base_url}/messages/{message_id}")
        return response.json()

    def get_queue_status(self):
        """获取队列状态"""
        response = self.session.get(f"{self.base_url}/queues/status")
        return response.json()

    def health_check(self):
        """健康检查"""
        response = self.session.get(f"{self.base_url}/health")
        return response.json()

# 使用示例
client = LLMQueueClient()

# 发送消息
result = client.send_message(
    conversation_id="conv_123",
    user_id="user_456",
    content="Hello from Python!",
    priority="high"
)
print("Message sent:", result)

# 检查队列状态
status = client.get_queue_status()
print("Queue status:", status)
```

## 9. 版本历史

### v1.0.0 (2025-01-01)
- 初始版本发布
- 支持基本的消息队列功能
- 多级优先级队列
- 对话管理
- 系统监控和健康检查
- RESTful API接口

---

*本文档版本: v1.0*
*最后更新: 2025年1月*
*维护者: 开发团队*
