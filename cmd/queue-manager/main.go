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

	"llm-message-queue/internal/priorityqueue"
	"llm-message-queue/internal/statemanager"
	"llm-message-queue/pkg/config"
	"llm-message-queue/pkg/models"
)

func main() {
	// Load configuration
	cfg, err := config.LoadConfig("configs/config.yaml")
	if err != nil {
		log.Fatalf("Error loading configuration: %v", err)
	}

	// Initialize PostgreSQL
	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%d sslmode=disable TimeZone=Asia/Shanghai",
		cfg.Database.Postgres.Host,
		cfg.Database.Postgres.User,
		cfg.Database.Postgres.Password,
		cfg.Database.Postgres.DBName,
		cfg.Database.Postgres.Port)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
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

	// Initialize MultiLevelQueue
	multiQueue := priorityqueue.NewMultiLevelQueue(cfg.Queue.DefaultMaxSize)
	
	// Add queues for each level
	for _, q := range cfg.Queue.Levels {
		multiQueue.AddQueue(q.Name)
	}

	// Initialize StateManager
	stateManager := statemanager.NewStateManager(db, rdb)

	// Create context for graceful shutdown
	rootCtx, rootCancel := context.WithCancel(context.Background())
	defer rootCancel()

	// Start queue workers
	var wg sync.WaitGroup
	workerCount := cfg.Queue.Worker.MaxConcurrent
	log.Printf("Starting %d queue workers", workerCount)

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			processMessages(rootCtx, workerID, multiQueue, stateManager)
		}(i)
	}

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down queue manager...")

	// Signal all workers to stop
	rootCancel()

	// Wait for all workers to finish
	wg.Wait()

	// Perform cleanup operations
	rdb.Close()
	log.Println("Queue manager exited")
}

func processMessages(ctx context.Context, workerID int, queue *priorityqueue.MultiLevelQueue, stateManager *statemanager.StateManager) {
	log.Printf("Worker %d started", workerID)

	for {
		select {
		case <-ctx.Done():
			log.Printf("Worker %d stopping due to context cancellation", workerID)
			return
		default:
			// Try to get a message from queues in priority order
			queueNames := []string{"realtime", "high", "normal", "low"}
			var msg *models.Message
			var err error
			
			for _, queueName := range queueNames {
				msg, err = queue.Pop(queueName)
				if err == nil {
					break // Successfully got a message
				}
				if err != priorityqueue.ErrQueueEmpty {
					log.Printf("Worker %d error popping from queue %s: %v", workerID, queueName, err)
				}
			}
			
			if msg == nil {
				// All queues are empty, wait a bit before trying again
				time.Sleep(500 * time.Millisecond)
				continue
			}

			// Process the message
			log.Printf("Worker %d processing message %s (priority: %s)", workerID, msg.ID, msg.Priority)

			// Update message status to processing
			msg.Status = models.StatusProcessing
			msg.UpdatedAt = time.Now()

			// In a real implementation, you would send the message to an LLM service
			// For this example, we'll simulate processing time based on priority
			var processingTime time.Duration
			switch msg.Priority {
			case models.PriorityRealtime:
				processingTime = 500 * time.Millisecond
			case models.PriorityHigh:
				processingTime = 1 * time.Second
			case models.PriorityNormal:
				processingTime = 2 * time.Second
			case models.PriorityLow:
				processingTime = 3 * time.Second
			default:
				processingTime = 2 * time.Second
			}

			// Simulate processing
			processingCtx, cancel := context.WithTimeout(ctx, processingTime)
			select {
			case <-processingCtx.Done():
				if processingCtx.Err() == context.Canceled {
					// The main context was canceled, stop processing
					cancel()
					return
				}
				// Processing completed (timeout reached)
				cancel()
			}

			// Update message status to completed
			msg.Status = models.StatusCompleted
			msg.UpdatedAt = time.Now()
			msg.CompletedAt = &msg.UpdatedAt

			// In a real implementation, you would update the message in the database
			// and notify any waiting clients
			log.Printf("Worker %d completed message %s", workerID, msg.ID)

			// Update conversation if this message is part of one
			if msg.ConversationID != "" {
				err := stateManager.UpdateMessage(context.Background(), msg)
				if err != nil {
					log.Printf("Worker %d error updating message in conversation: %v", workerID, err)
				}
			}
		}
	}
}