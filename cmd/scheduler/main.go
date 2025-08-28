package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/go-redis/redis/v8"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"llm-message-queue/internal/loadbalancer"
	"llm-message-queue/internal/priorityqueue"
	"llm-message-queue/internal/scheduler"
	"llm-message-queue/pkg/config"

	"go.uber.org/zap"
)

func main() {
	// Load configuration
	cfg, err := config.LoadConfig("configs/config.yaml")
	if err != nil {
		log.Fatalf("Error loading configuration: %v", err)
	}

	// Initialize PostgreSQL
	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%d sslmode=%s",
		cfg.Database.Postgres.Host,
		cfg.Database.Postgres.User,
		cfg.Database.Postgres.Password,
		cfg.Database.Postgres.DBName,
		cfg.Database.Postgres.Port,
		cfg.Database.Postgres.SSLMode)
	_, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to PostgreSQL: %v", err)
	}

	// Initialize Redis
	rdb := redis.NewClient(&redis.Options{
		Addr: cfg.Database.Redis.Addr,
		Password: cfg.Database.Redis.Password,
		DB: cfg.Database.Redis.DB,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = rdb.Ping(ctx).Result()
	if err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}

	// Initialize MultiLevelQueue for monitoring
	multiQueue := priorityqueue.NewMultiLevelQueue(cfg.Queue.DefaultMaxSize)

	// Initialize LoadBalancer
	logger, _ := zap.NewProduction()
	lb := loadbalancer.NewLoadBalancer(&cfg.LoadBalancer, logger)

	// Initialize Scheduler
	schedulerConfig := scheduler.Config{
		Strategy:           scheduler.Strategy(cfg.Scheduler.Strategy),
		MonitorInterval:    cfg.Scheduler.CheckInterval,
		ScalingThresholds:  map[string]int{"scale_up_queue_length": 100, "scale_down_queue_length": 10},
		ResourceLimits:     map[string]int{"min_endpoints": 1, "max_endpoints": 10},
		AdaptiveParameters: map[string]float64{"response_time_weight": 0.5, "error_rate_weight": 0.3},
	}

	scheduler := scheduler.NewScheduler(schedulerConfig, multiQueue, lb, rdb)

	// Create context for graceful shutdown
	rootCtx, rootCancel := context.WithCancel(context.Background())
	defer rootCancel()

	// Start scheduler in a goroutine
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		scheduler.Start(rootCtx)
	}()

	log.Println("Scheduler started successfully")

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down scheduler...")

	// Signal scheduler to stop
	rootCancel()

	// Wait for scheduler to finish
	wg.Wait()

	// Perform cleanup operations
	rdb.Close()
	log.Println("Scheduler exited")
}