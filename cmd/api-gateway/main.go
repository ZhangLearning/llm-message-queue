package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
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

	// Auto-migrate database schema
	err = db.AutoMigrate(&models.Message{}, &models.Conversation{})
	if err != nil {
		log.Fatalf("Failed to auto-migrate database: %v", err)
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
	for _, q := range cfg.Queue.Levels {
		priority := models.Priority(q.Priority)
		multiQueue.AddQueue(string(priority))
	}

	// Initialize StateManager
	stateManager := statemanager.NewStateManager(db, rdb)

	// Setup Gin router
	r := gin.Default()

	// Health check endpoint
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// API group for messages
	api := r.Group("/api/v1")
	{
		api.POST("/messages", func(c *gin.Context) {
			var msg models.Message
			if err := c.ShouldBindJSON(&msg); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}

			// Set default values
			if msg.ID == "" {
				msg.ID = uuid.New().String()
			}
			if msg.Status == "" {
				msg.Status = models.StatusPending
			}
			if msg.CreatedAt.IsZero() {
				msg.CreatedAt = time.Now()
			}
			msg.UpdatedAt = time.Now()

			// Add message to queue
			queueName := string(msg.Priority) // Use priority as queue name
			err := multiQueue.Push(queueName, &msg, int(msg.Priority))
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to add message to queue: %v", err)})
				return
			}

			c.JSON(http.StatusAccepted, gin.H{"message": "Message accepted", "id": msg.ID})
		})

		api.GET("/messages/:id", func(c *gin.Context) {
			msgID := c.Param("id")
			// In a real scenario, you would retrieve the message from DB/cache
			// For now, we'll just acknowledge the request
			c.JSON(http.StatusOK, gin.H{"message": "Message status endpoint (not implemented)", "id": msgID})
		})

		api.GET("/queues/stats", func(c *gin.Context) {
			// Get stats for all queues
			stats := make(map[string]interface{})
			queues := []string{"realtime", "high", "normal", "low"}
			for _, queueName := range queues {
				queueStats, err := multiQueue.GetStats(queueName)
				if err == nil {
					stats[queueName] = queueStats
				}
			}
			c.JSON(http.StatusOK, stats)
		})

		// Conversation endpoints
		api.POST("/conversations", func(c *gin.Context) {
			var conv models.Conversation
			if err := c.ShouldBindJSON(&conv); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}

			createdConv, err := stateManager.CreateConversation(c.Request.Context(), conv.UserID, conv.Title, conv.Priority)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to create conversation: %v", err)})
				return
			}
			c.JSON(http.StatusCreated, createdConv)
		})

		api.GET("/conversations/:id", func(c *gin.Context) {
			convID := c.Param("id")
			conv, err := stateManager.GetConversation(c.Request.Context(), convID)
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("Conversation not found: %v", err)})
				return
			}
			c.JSON(http.StatusOK, conv)
		})

		api.POST("/conversations/:id/messages", func(c *gin.Context) {
			convID := c.Param("id")
			var msg models.Message
			// Assuming message content is part of the request body
			if err := c.ShouldBindJSON(&msg); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			msg.ConversationID = convID

			err = stateManager.AddMessage(c.Request.Context(), convID, &msg)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to add message to conversation: %v", err)})
				return
			}
			c.JSON(http.StatusOK, gin.H{"message": "Message added successfully"})
		})
	}

	// Start HTTP server
	go func() {
		if err := r.Run(fmt.Sprintf(":%s", cfg.Server.Port)); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %s\n", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	// Perform cleanup operations here
	// For example, close database connections, Redis connections
	// db.Close() // GORM doesn't have a direct Close method, it manages connection pool
	rdb.Close()
	log.Println("Server exited")
}
