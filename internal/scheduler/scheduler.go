package scheduler

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/go-redis/redis/v8"

	"llm-message-queue/internal/loadbalancer"
	"llm-message-queue/internal/priorityqueue"
)

// Strategy defines the scheduling strategy
type Strategy string

const (
	// Static uses fixed resource allocation
	Static Strategy = "static"
	// Dynamic adjusts resources based on queue length
	Dynamic Strategy = "dynamic"
	// Adaptive uses machine learning to predict resource needs
	Adaptive Strategy = "adaptive"
	// Hybrid combines multiple strategies
	Hybrid Strategy = "hybrid"
)

// Config holds the scheduler configuration
type Config struct {
	Strategy           Strategy
	MonitorInterval    time.Duration
	ScalingThresholds  map[string]int
	ResourceLimits     map[string]int
	AdaptiveParameters map[string]float64
}

// Scheduler manages resource allocation and scheduling
type Scheduler struct {
	config      Config
	queue       *priorityqueue.MultiLevelQueue
	lb          *loadbalancer.LoadBalancer
	redisClient *redis.Client
	isRunning   bool
}

// NewScheduler creates a new scheduler
func NewScheduler(config Config, queue *priorityqueue.MultiLevelQueue, lb *loadbalancer.LoadBalancer, redisClient *redis.Client) *Scheduler {
	return &Scheduler{
		config:      config,
		queue:       queue,
		lb:          lb,
		redisClient: redisClient,
		isRunning:   false,
	}
}

// Start begins the scheduling process
func (s *Scheduler) Start(ctx context.Context) {
	if s.isRunning {
		log.Println("Scheduler is already running")
		return
	}

	s.isRunning = true
	log.Printf("Starting scheduler with strategy: %s", s.config.Strategy)

	ticker := time.NewTicker(s.config.MonitorInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("Scheduler stopping due to context cancellation")
			s.isRunning = false
			return
		case <-ticker.C:
			s.scheduleResources(ctx)
		}
	}
}

// scheduleResources allocates resources based on the current strategy
func (s *Scheduler) scheduleResources(ctx context.Context) {
	// Get current queue stats
	stats := s.queue.GetAllStats()

	// Log current queue state
	log.Printf("Queue stats: Realtime=%d, High=%d, Normal=%d, Low=%d",
		stats["realtime"].PendingCount,
		stats["high"].PendingCount,
		stats["normal"].PendingCount,
		stats["low"].PendingCount)

	// Apply scheduling strategy
	switch s.config.Strategy {
	case Static:
		s.applyStaticScheduling(stats)
	case Dynamic:
		s.applyDynamicScheduling(stats)
	case Adaptive:
		s.applyAdaptiveScheduling(ctx, stats)
	case Hybrid:
		s.applyHybridScheduling(ctx, stats)
	default:
		// Default to dynamic scheduling
		s.applyDynamicScheduling(stats)
	}
}

// applyStaticScheduling uses fixed resource allocation
func (s *Scheduler) applyStaticScheduling(stats map[string]*priorityqueue.QueueStats) {
	// Static scheduling maintains a fixed allocation of resources
	// This is the simplest strategy and doesn't adjust based on load
	log.Println("Applied static scheduling (no changes)")
}

// applyDynamicScheduling adjusts resources based on queue length
func (s *Scheduler) applyDynamicScheduling(stats map[string]*priorityqueue.QueueStats) {
	// Get current endpoints
	endpoints := s.lb.GetAllEndpoints()
	currentEndpointCount := len(endpoints)

	// Calculate total pending messages
	totalPending := 0
	for _, stat := range stats {
		totalPending += stat.PendingCount
	}

	// Check if we need to scale up or down
	minEndpoints := s.config.ResourceLimits["min_endpoints"]
	maxEndpoints := s.config.ResourceLimits["max_endpoints"]
	scaleUpThreshold := s.config.ScalingThresholds["scale_up_queue_length"]
	scaleDownThreshold := s.config.ScalingThresholds["scale_down_queue_length"]

	if totalPending > scaleUpThreshold && currentEndpointCount < maxEndpoints {
		// Scale up: Add a new endpoint
		newEndpointURL := generateEndpointURL(currentEndpointCount + 1)
		newEndpoint := &loadbalancer.Endpoint{
			ID:             fmt.Sprintf("endpoint-%d", currentEndpointCount+1),
			URL:            newEndpointURL,
			Name:           fmt.Sprintf("Endpoint %d", currentEndpointCount+1),
			Type:           "llm",
			Weight:         1,
			MaxConnections: 100,
		}
		s.lb.AddEndpoint(newEndpoint)
		log.Printf("Scaled up: Added new endpoint %s (total: %d)", newEndpointURL, currentEndpointCount+1)
	} else if totalPending < scaleDownThreshold && currentEndpointCount > minEndpoints {
		// Scale down: Remove the last endpoint
		endpointToRemove := endpoints[currentEndpointCount-1]
		s.lb.RemoveEndpoint(endpointToRemove.ID)
		log.Printf("Scaled down: Removed endpoint %s (total: %d)", endpointToRemove.URL, currentEndpointCount-1)
	} else {
		log.Printf("No scaling needed: Current endpoints=%d, Total pending=%d", currentEndpointCount, totalPending)
	}

	// Adjust load balancing strategy based on queue distribution
	realtimePending := 0
	highPending := 0
	if realtimeStats, exists := stats["realtime"]; exists {
		realtimePending = realtimeStats.PendingCount
	}
	if highStats, exists := stats["high"]; exists {
		highPending = highStats.PendingCount
	}

	if realtimePending > highPending*2 {
		// If realtime queue is much larger, use least connections to process faster
		// Note: LoadBalancer strategy is set during initialization
		log.Println("High realtime load detected - would switch to least connections strategy")
	} else if totalPending > scaleUpThreshold {
		// Under high load, use adaptive load balancing
		// Note: LoadBalancer strategy is set during initialization
		log.Println("High overall load detected - would switch to adaptive load strategy")
	} else {
		// Under normal conditions, use weighted random
		// Note: LoadBalancer strategy is set during initialization
		log.Println("Normal load detected - would use weighted random strategy")
	}
}

// applyAdaptiveScheduling uses machine learning to predict resource needs
func (s *Scheduler) applyAdaptiveScheduling(ctx context.Context, stats map[string]*priorityqueue.QueueStats) {
	// In a real implementation, this would use historical data and ML models
	// to predict future load and allocate resources accordingly
	
	// For this example, we'll use a simple time-based prediction
	// Assume higher load during business hours on weekdays
	now := time.Now()
	hour := now.Hour()
	weekday := now.Weekday()

	// Define "business hours" as 9 AM to 5 PM, Monday through Friday
	isBusinessHours := hour >= 9 && hour < 17 && weekday >= time.Monday && weekday <= time.Friday

	// Get current endpoints
	endpoints := s.lb.GetAllEndpoints()
	currentEndpointCount := len(endpoints)

	// Calculate target endpoint count based on time and current load
	targetEndpointCount := currentEndpointCount
	minEndpoints := s.config.ResourceLimits["min_endpoints"]
	maxEndpoints := s.config.ResourceLimits["max_endpoints"]

	// Calculate total pending messages
	totalPending := 0
	for _, stat := range stats {
		totalPending += stat.PendingCount
	}

	// Adjust target based on time of day and current load
	if isBusinessHours {
		// During business hours, maintain higher capacity
		targetEndpointCount = maxEndpoints - 1
		if totalPending > s.config.ScalingThresholds["scale_up_queue_length"] {
			targetEndpointCount = maxEndpoints
		}
	} else {
		// Outside business hours, scale down unless there's high load
		targetEndpointCount = minEndpoints + 1
		if totalPending < s.config.ScalingThresholds["scale_down_queue_length"] {
			targetEndpointCount = minEndpoints
		}
	}

	// Apply the target endpoint count
	if targetEndpointCount > currentEndpointCount {
		// Scale up
		for i := currentEndpointCount; i < targetEndpointCount; i++ {
			newEndpointURL := generateEndpointURL(i + 1)
			newEndpoint := &loadbalancer.Endpoint{
				ID:             fmt.Sprintf("adaptive-endpoint-%d", i+1),
				URL:            newEndpointURL,
				Name:           fmt.Sprintf("Adaptive Endpoint %d", i+1),
				Type:           "llm",
				Weight:         1,
				MaxConnections: 100,
			}
			s.lb.AddEndpoint(newEndpoint)
			log.Printf("Adaptive scaling: Added new endpoint %s", newEndpointURL)
		}
	} else if targetEndpointCount < currentEndpointCount {
		// Scale down
		for i := currentEndpointCount - 1; i >= targetEndpointCount; i-- {
			endpointToRemove := endpoints[i]
			s.lb.RemoveEndpoint(endpointToRemove.ID)
			log.Printf("Adaptive scaling: Removed endpoint %s", endpointToRemove.URL)
		}
	}

	log.Printf("Applied adaptive scheduling: Endpoints=%d, Business hours=%v, Total pending=%d",
		targetEndpointCount, isBusinessHours, totalPending)
}

// applyHybridScheduling combines multiple strategies
func (s *Scheduler) applyHybridScheduling(ctx context.Context, stats map[string]*priorityqueue.QueueStats) {
	// Hybrid scheduling combines aspects of dynamic and adaptive scheduling
	
	// First, apply dynamic scaling based on current load
	// Convert stats to the format expected by applyDynamicScheduling
	dynamicStats := make(map[string]*priorityqueue.QueueStats)
	for key, stat := range stats {
		dynamicStats[key] = stat
	}
	s.applyDynamicScheduling(dynamicStats)
	
	// Then, adjust weights based on historical performance
	endpoints := s.lb.GetAllEndpoints()
	
	// In a real implementation, we would analyze historical performance data
	// For this example, we'll just adjust weights based on response times
	for _, endpoint := range endpoints {
		// Calculate average response time for this endpoint
		avgResponseTime := endpoint.ResponseTime
		
		// Adjust weight based on response time
		// Faster endpoints get higher weights
		if avgResponseTime > 0 {
			// Base weight calculation - lower response times get higher weights
			// We use 100ms as a baseline good response time
			baseWeight := int(100 * time.Millisecond / avgResponseTime)
			if baseWeight < 1 {
				baseWeight = 1
			} else if baseWeight > 10 {
				baseWeight = 10
			}
			
			// Log weight adjustment (actual weight update would require LoadBalancer API)
			log.Printf("Would adjust weight for endpoint %s to %d based on avg response time %v",
				endpoint.URL, baseWeight, avgResponseTime)
		}
	}
	
	log.Println("Applied hybrid scheduling")
}

// generateEndpointURL creates a URL for a new endpoint
func generateEndpointURL(index int) string {
	return fmt.Sprintf("http://llm-processor-%d:8080", index)
}

// Stop stops the scheduler
func (s *Scheduler) Stop() {
	s.isRunning = false
	log.Println("Scheduler stopped")
}

// IsRunning returns whether the scheduler is running
func (s *Scheduler) IsRunning() bool {
	return s.isRunning
}