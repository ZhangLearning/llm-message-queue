package tests

import (
	"testing"
	"time"

	"llm-message-queue/internal/loadbalancer"
	"llm-message-queue/pkg/config"

	"go.uber.org/zap"
)

func TestLoadBalancer(t *testing.T) {
	// 创建日志记录器
	logger, _ := zap.NewDevelopment()

	// 测试轮询策略
	t.Run("RoundRobinStrategy", func(t *testing.T) {
		// 创建轮询负载均衡器
		cfg := &config.LoadBalancerConfig{
			Algorithm:           "round_robin",
			HealthCheckInterval: 0, // 禁用健康检查以避免goroutine泄漏
			MaxFailures:         3,
		}
		lb := loadbalancer.NewLoadBalancer(cfg, logger)
		defer lb.Stop()

		// 添加端点
		endpoints := []*loadbalancer.Endpoint{
			{ID: "ep1", URL: "http://endpoint1:8080", Name: "endpoint1", Type: "llm", Weight: 1, Status: loadbalancer.EndpointStatusHealthy, MaxConnections: 100},
			{ID: "ep2", URL: "http://endpoint2:8080", Name: "endpoint2", Type: "llm", Weight: 1, Status: loadbalancer.EndpointStatusHealthy, MaxConnections: 100},
			{ID: "ep3", URL: "http://endpoint3:8080", Name: "endpoint3", Type: "llm", Weight: 1, Status: loadbalancer.EndpointStatusHealthy, MaxConnections: 100},
		}

		for _, ep := range endpoints {
			err := lb.AddEndpoint(ep)
			if err != nil {
				t.Fatalf("Failed to add endpoint: %v", err)
			}
		}

		// 验证端点数量
		allEndpoints := lb.GetAllEndpoints()
		if len(allEndpoints) != 3 {
			t.Errorf("Expected 3 endpoints, got %d", len(allEndpoints))
		}

		// 测试轮询分配
		selectedEndpoints := make(map[string]int)
		for i := 0; i < 9; i++ { // 3个端点，轮询3轮
			endpoint, err := lb.GetEndpoint(nil, "")
			if err != nil {
				t.Fatalf("Failed to get endpoint: %v", err)
			}
			selectedEndpoints[endpoint.URL]++
		}

		// 验证每个端点被选择了相同次数
		for _, ep := range endpoints {
			if count := selectedEndpoints[ep.URL]; count != 3 {
				t.Errorf("Expected endpoint %s to be selected 3 times, got %d", ep.URL, count)
			}
		}
	})

	// 测试最少连接策略
	t.Run("LeastConnectionsStrategy", func(t *testing.T) {
		// 创建最少连接负载均衡器
		cfg := &config.LoadBalancerConfig{
			Algorithm:           "least_connections",
			HealthCheckInterval: 30 * time.Second,
			MaxFailures:         3,
		}
		lb := loadbalancer.NewLoadBalancer(cfg, logger)
		defer lb.Stop()

		// 添加端点
		endpoints := []*loadbalancer.Endpoint{
			{ID: "ep1", URL: "http://endpoint1:8080", Name: "endpoint1", Type: "llm", Weight: 1, Status: loadbalancer.EndpointStatusHealthy, MaxConnections: 100, Connections: 2},
			{ID: "ep2", URL: "http://endpoint2:8080", Name: "endpoint2", Type: "llm", Weight: 1, Status: loadbalancer.EndpointStatusHealthy, MaxConnections: 100, Connections: 1},
			{ID: "ep3", URL: "http://endpoint3:8080", Name: "endpoint3", Type: "llm", Weight: 1, Status: loadbalancer.EndpointStatusHealthy, MaxConnections: 100, Connections: 0},
		}

		for _, ep := range endpoints {
			err := lb.AddEndpoint(ep)
			if err != nil {
				t.Fatalf("Failed to add endpoint: %v", err)
			}
		}

		// 获取端点，应该选择连接数最少的endpoint3
		endpoint, err := lb.GetEndpoint(nil, "")
		if err != nil {
			t.Fatalf("Failed to get endpoint: %v", err)
		}
		if endpoint.URL != "http://endpoint3:8080" {
			t.Errorf("Expected endpoint3 with 0 connections, got %s", endpoint.URL)
		}

		// 验证端点统计信息
		stats := lb.GetEndpointStats()
		if stats == nil {
			t.Errorf("Expected endpoint stats to be available")
		}
	})

	// 测试加权随机策略
	t.Run("WeightedRandomStrategy", func(t *testing.T) {
		// 创建加权随机负载均衡器
		cfg := &config.LoadBalancerConfig{
			Algorithm:           "weighted_random",
			HealthCheckInterval: 30 * time.Second,
			MaxFailures:         3,
		}
		lb := loadbalancer.NewLoadBalancer(cfg, logger)
		defer lb.Stop()

		// 添加端点并设置权重
		endpoints := []*loadbalancer.Endpoint{
			{ID: "ep1", URL: "http://endpoint1:8080", Name: "endpoint1", Type: "llm", Weight: 1, Status: loadbalancer.EndpointStatusHealthy, MaxConnections: 100},
			{ID: "ep2", URL: "http://endpoint2:8080", Name: "endpoint2", Type: "llm", Weight: 5, Status: loadbalancer.EndpointStatusHealthy, MaxConnections: 100},
			{ID: "ep3", URL: "http://endpoint3:8080", Name: "endpoint3", Type: "llm", Weight: 10, Status: loadbalancer.EndpointStatusHealthy, MaxConnections: 100},
		}

		for _, ep := range endpoints {
			err := lb.AddEndpoint(ep)
			if err != nil {
				t.Fatalf("Failed to add endpoint: %v", err)
			}
		}

		// 进行多次选择，统计结果
		selectedEndpoints := make(map[string]int)
		for i := 0; i < 1000; i++ {
			endpoint, err := lb.GetEndpoint(nil, "")
			if err != nil {
				t.Fatalf("Failed to get endpoint: %v", err)
			}
			selectedEndpoints[endpoint.URL]++
		}

		// 验证选择分布与权重大致成比例
		// 由于随机性，我们只检查高权重的端点是否被选择的次数明显多于低权重的
		if selectedEndpoints["http://endpoint1:8080"] >= selectedEndpoints["http://endpoint3:8080"] {
			t.Errorf("Expected endpoint3 (weight 10) to be selected more than endpoint1 (weight 1)")
		}
		if selectedEndpoints["http://endpoint2:8080"] >= selectedEndpoints["http://endpoint3:8080"] {
			t.Errorf("Expected endpoint3 (weight 10) to be selected more than endpoint2 (weight 5)")
		}
	})

	// 测试自适应负载策略
	t.Run("AdaptiveLoadStrategy", func(t *testing.T) {
		// 创建自适应负载均衡器
		cfg := &config.LoadBalancerConfig{
			Algorithm:           "adaptive_load",
			HealthCheckInterval: 30 * time.Second,
			MaxFailures:         3,
		}
		lb := loadbalancer.NewLoadBalancer(cfg, logger)
		defer lb.Stop()

		// 添加端点
		endpoints := []*loadbalancer.Endpoint{
			{ID: "ep1", URL: "http://endpoint1:8080", Name: "endpoint1", Type: "llm", Weight: 1, Status: loadbalancer.EndpointStatusHealthy, MaxConnections: 100, ResponseTime: 150 * time.Millisecond, ErrorRate: 0.1},
			{ID: "ep2", URL: "http://endpoint2:8080", Name: "endpoint2", Type: "llm", Weight: 1, Status: loadbalancer.EndpointStatusHealthy, MaxConnections: 100, ResponseTime: 50 * time.Millisecond, ErrorRate: 0.02},
			{ID: "ep3", URL: "http://endpoint3:8080", Name: "endpoint3", Type: "llm", Weight: 1, Status: loadbalancer.EndpointStatusHealthy, MaxConnections: 100, ResponseTime: 100 * time.Millisecond, ErrorRate: 0.05},
		}

		for _, ep := range endpoints {
			err := lb.AddEndpoint(ep)
			if err != nil {
				t.Fatalf("Failed to add endpoint: %v", err)
			}
		}

		// 多次获取端点，验证endpoint2被选择的次数最多
		selectedEndpoints := make(map[string]int)
		for i := 0; i < 20; i++ {
			endpoint, err := lb.GetEndpoint(nil, "")
			if err != nil {
				t.Fatalf("Failed to get endpoint: %v", err)
			}
			selectedEndpoints[endpoint.URL]++
			// 释放连接以重置状态
			lb.ReleaseEndpoint(endpoint.ID, 50*time.Millisecond, false)
		}

		// 验证endpoint2（性能最好）被选择的次数最多
		if selectedEndpoints["http://endpoint2:8080"] < selectedEndpoints["http://endpoint1:8080"] ||
			selectedEndpoints["http://endpoint2:8080"] < selectedEndpoints["http://endpoint3:8080"] {
			t.Errorf("Expected endpoint2 to be selected most frequently, got counts: ep1=%d, ep2=%d, ep3=%d",
				selectedEndpoints["http://endpoint1:8080"],
				selectedEndpoints["http://endpoint2:8080"],
				selectedEndpoints["http://endpoint3:8080"])
		}
	})

	// 测试端点健康管理
	t.Run("EndpointHealthManagement", func(t *testing.T) {
		// 创建负载均衡器
		cfg := &config.LoadBalancerConfig{
			Algorithm:           "round_robin",
			HealthCheckInterval: 0, // 禁用健康检查以避免goroutine泄漏
			MaxFailures:         3,
		}
		lb := loadbalancer.NewLoadBalancer(cfg, logger)
		defer lb.Stop()

		// 添加端点
		endpoints := []*loadbalancer.Endpoint{
			{ID: "ep1", URL: "http://endpoint1:8080", Name: "endpoint1", Type: "llm", Weight: 1, Status: loadbalancer.EndpointStatusHealthy, MaxConnections: 100},
			{ID: "ep2", URL: "http://endpoint2:8080", Name: "endpoint2", Type: "llm", Weight: 1, Status: loadbalancer.EndpointStatusUnhealthy, MaxConnections: 100},
			{ID: "ep3", URL: "http://endpoint3:8080", Name: "endpoint3", Type: "llm", Weight: 1, Status: loadbalancer.EndpointStatusHealthy, MaxConnections: 100},
		}

		for _, ep := range endpoints {
			err := lb.AddEndpoint(ep)
			if err != nil {
				t.Fatalf("Failed to add endpoint: %v", err)
			}
		}

		// 获取端点，不应该返回不健康的端点
		for i := 0; i < 6; i++ { // 测试多次以确保不会选择不健康的端点
			endpoint, err := lb.GetEndpoint(nil, "")
			if err != nil {
				t.Fatalf("Failed to get endpoint: %v", err)
			}
			if endpoint.URL == "http://endpoint2:8080" {
				t.Errorf("Selected unhealthy endpoint: %s", endpoint.URL)
			}
		}

		// 更新端点状态为健康
		err := lb.UpdateEndpointStatus("ep2", loadbalancer.EndpointStatusHealthy)
		if err != nil {
			t.Fatalf("Failed to update endpoint status: %v", err)
		}

		// 现在应该可以选择该端点
		found := false
		for i := 0; i < 9; i++ { // 3个端点，轮询3轮
			endpoint, _ := lb.GetEndpoint(nil, "")
			if endpoint.URL == "http://endpoint2:8080" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Endpoint2 was not selected after being marked healthy")
		}
	})

	// 测试添加和移除端点
	t.Run("AddRemoveEndpoints", func(t *testing.T) {
		// 创建负载均衡器
		cfg := &config.LoadBalancerConfig{
			Algorithm:           "round_robin",
			HealthCheckInterval: 30 * time.Second,
			MaxFailures:         3,
		}
		lb := loadbalancer.NewLoadBalancer(cfg, logger)
		defer lb.Stop()

		// 添加端点
		ep1 := &loadbalancer.Endpoint{ID: "ep1", URL: "http://endpoint1:8080", Name: "endpoint1", Type: "llm", Weight: 1, Status: loadbalancer.EndpointStatusHealthy, MaxConnections: 100}
		ep2 := &loadbalancer.Endpoint{ID: "ep2", URL: "http://endpoint2:8080", Name: "endpoint2", Type: "llm", Weight: 1, Status: loadbalancer.EndpointStatusHealthy, MaxConnections: 100}
		lb.AddEndpoint(ep1)
		lb.AddEndpoint(ep2)

		// 验证端点数量
		allEndpoints := lb.GetAllEndpoints()
		if len(allEndpoints) != 2 {
			t.Errorf("Expected 2 endpoints, got %d", len(allEndpoints))
		}

		// 移除端点
		err := lb.RemoveEndpoint("ep1")
		if err != nil {
			t.Fatalf("Failed to remove endpoint: %v", err)
		}

		// 验证端点数量
		allEndpoints = lb.GetAllEndpoints()
		if len(allEndpoints) != 1 {
			t.Errorf("Expected 1 endpoint after removal, got %d", len(allEndpoints))
		}

		// 验证剩余的端点
		if allEndpoints[0].URL != "http://endpoint2:8080" {
			t.Errorf("Expected endpoint2 to remain, got %s", allEndpoints[0].URL)
		}

		// 添加更多端点
		ep3 := &loadbalancer.Endpoint{ID: "ep3", URL: "http://endpoint3:8080", Name: "endpoint3", Type: "llm", Weight: 1, Status: loadbalancer.EndpointStatusHealthy, MaxConnections: 100}
		ep4 := &loadbalancer.Endpoint{ID: "ep4", URL: "http://endpoint4:8080", Name: "endpoint4", Type: "llm", Weight: 1, Status: loadbalancer.EndpointStatusHealthy, MaxConnections: 100}
		lb.AddEndpoint(ep3)
		lb.AddEndpoint(ep4)

		// 验证端点数量
		allEndpoints = lb.GetAllEndpoints()
		if len(allEndpoints) != 3 {
			t.Errorf("Expected 3 endpoints after additions, got %d", len(allEndpoints))
		}
	})

	// 测试会话亲和性
	t.Run("SessionAffinity", func(t *testing.T) {
		// 创建负载均衡器
		cfg := &config.LoadBalancerConfig{
			Algorithm:             "round_robin",
			HealthCheckInterval:   0,
			MaxFailures:           3,
			EnableSessionAffinity: true,
			SessionTimeout:        30 * time.Minute,
		}
		lb := loadbalancer.NewLoadBalancer(cfg, logger)
		defer lb.Stop()

		// 添加端点
		endpoints := []*loadbalancer.Endpoint{
			{ID: "ep1", URL: "http://endpoint1:8080", Name: "endpoint1", Type: "llm", Weight: 1, Status: loadbalancer.EndpointStatusHealthy, MaxConnections: 100},
			{ID: "ep2", URL: "http://endpoint2:8080", Name: "endpoint2", Type: "llm", Weight: 1, Status: loadbalancer.EndpointStatusHealthy, MaxConnections: 100},
			{ID: "ep3", URL: "http://endpoint3:8080", Name: "endpoint3", Type: "llm", Weight: 1, Status: loadbalancer.EndpointStatusHealthy, MaxConnections: 100},
		}

		for _, ep := range endpoints {
			err := lb.AddEndpoint(ep)
			if err != nil {
				t.Fatalf("Failed to add endpoint: %v", err)
			}
		}

		// 获取第一个会话的端点
		endpoint1, err := lb.GetEndpoint(nil, "session1")
		if err != nil {
			t.Fatalf("Failed to get endpoint for session1: %v", err)
		}

		// 获取第二个会话的端点
		endpoint2, err := lb.GetEndpoint(nil, "session2")
		if err != nil {
			t.Fatalf("Failed to get endpoint for session2: %v", err)
		}

		// 再次获取第一个会话的端点，应该与之前相同
		endpoint1Again, err := lb.GetEndpoint(nil, "session1")
		if err != nil {
			t.Fatalf("Failed to get endpoint for session1 again: %v", err)
		}

		if endpoint1.URL != endpoint1Again.URL {
			t.Errorf("Expected same endpoint for session1, got %s and %s", endpoint1.URL, endpoint1Again.URL)
		}

		// 再次获取第二个会话的端点，应该与之前相同
		endpoint2Again, err := lb.GetEndpoint(nil, "session2")
		if err != nil {
			t.Fatalf("Failed to get endpoint for session2 again: %v", err)
		}

		if endpoint2.URL != endpoint2Again.URL {
			t.Errorf("Expected same endpoint for session2, got %s and %s", endpoint2.URL, endpoint2Again.URL)
		}
	})
}
