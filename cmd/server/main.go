package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"llm-message-queue/api"
	"llm-message-queue/internal/conversation"
	"llm-message-queue/internal/loadbalancer"
	"llm-message-queue/internal/preprocessor"
	"llm-message-queue/internal/priorityqueue"
	"llm-message-queue/internal/scheduler"
	"llm-message-queue/pkg/config"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	// 解析命令行参数
	configPath := flag.String("config", "./configs", "配置文件路径")
	flag.Parse()

	// 加载配置
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		fmt.Printf("加载配置失败: %v\n", err)
		os.Exit(1)
	}

	// 初始化日志
	logger, err := initLogger(&cfg.Logging)
	if err != nil {
		fmt.Printf("初始化日志失败: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	logger.Info("启动LLM消息队列服务", zap.String("version", "1.0.0"))

	// 初始化数据库连接
	db, err := initDatabase(cfg.Database.Postgres)
	if err != nil {
		logger.Fatal("初始化数据库连接失败", zap.Error(err))
	}

	// 初始化Redis连接
	redisClient, err := initRedis(cfg.Database.Redis)
	if err != nil {
		logger.Fatal("初始化Redis连接失败", zap.Error(err))
	}

	// 初始化组件
	preprocessor := preprocessor.NewPreprocessor(cfg.Queue)
	queueFactory := priorityqueue.NewQueueFactory(&cfg.Queue, logger)
	resScheduler := scheduler.NewResourceScheduler(&cfg.Scheduler, logger)
	loadBalancer := loadbalancer.NewLoadBalancer(&cfg.LoadBalancer, logger)

	// 初始化对话状态管理器
	persistenceStore, err := initPersistenceStore(cfg, db, redisClient, logger)
	if err != nil {
		logger.Fatal("初始化持久化存储失败", zap.Error(err))
	}

	stateManagerConfig := conversation.StateManagerConfig{
		ConversationTTL:    cfg.Queue.MaxRetentionPeriod,
		CleanupInterval:    cfg.Queue.CleanupInterval,
		MaxConversations:   1000,
		MaxContextLength:   4096,
		MaxIdleTime:        30 * time.Minute,
		PersistenceEnabled: true,
		PersistenceStore:   persistenceStore,
	}

	stateManager := conversation.NewStateManager(stateManagerConfig, logger)

	// 初始化API服务器
	apiServer := api.NewAPIServer(
		queueFactory,
		preprocessor,
		stateManager,
		loadBalancer,
		resScheduler,
		logger,
		[]string{"*"}, // 默认允许所有来源
		cfg,
	)

	// 启动工作者
	startWorkers(queueFactory, *cfg, logger)

	// 启动API服务器
	go func() {
		address := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
		logger.Info("API服务器启动", zap.String("address", address))
		if err := apiServer.Start(address); err != nil {
			logger.Fatal("API服务器启动失败", zap.Error(err))
		}
	}()

	// 等待信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	// 优雅关闭
	logger.Info("正在关闭服务...")
	apiServer.Stop()
	queueFactory.StopAll()
	resScheduler.Stop()
	logger.Info("服务已关闭")
}

// 初始化日志记录器
func initLogger(cfg *config.LoggingConfig) (*zap.Logger, error) {
	var logger *zap.Logger
	var err error

	if cfg.Level == "debug" {
		logger, err = zap.NewDevelopment()
	} else {
		logger, err = zap.NewProduction()
	}

	return logger, err
}

// 初始化数据库连接
func initDatabase(cfg config.PostgresConfig) (*gorm.DB, error) {
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.DBName, cfg.SSLMode,
	)

	return gorm.Open(postgres.Open(dsn), &gorm.Config{})
}

// 初始化Redis连接
func initRedis(cfg config.RedisConfig) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
		PoolSize: cfg.PoolSize,
	})

	// 测试连接
	ctx := context.Background()
	_, err := client.Ping(ctx).Result()
	return client, err
}

// 初始化持久化存储
func initPersistenceStore(cfg *config.Config, db *gorm.DB, redisClient *redis.Client, logger *zap.Logger) (conversation.PersistenceStore, error) {
	// 默认使用Redis
	return conversation.NewRedisPersistenceStore(
		redisClient,
		"conversation:",
		cfg.Queue.MaxRetentionPeriod,
		logger,
	), nil
}

// 启动工作者
func startWorkers(queueFactory *priorityqueue.QueueFactory, cfg config.Config, logger *zap.Logger) {
	// 标准队列工作者
	standardManager := queueFactory.CreateQueueManager("standard", priorityqueue.StandardQueueType)
	if standardManager != nil {
		logger.Info("标准队列管理器已创建")
		// TODO: 创建和启动工作者
	}

	// 延迟队列工作者
	delayedManager := queueFactory.CreateQueueManager("delayed", priorityqueue.DelayedQueueType)
	if delayedManager != nil {
		logger.Info("延迟队列管理器已创建")
		// TODO: 创建和启动工作者
	}

	// 优先级队列工作者
	priorityManager := queueFactory.CreateQueueManager("priority", priorityqueue.PriorityQueueType)
	if priorityManager != nil {
		logger.Info("优先级队列管理器已创建")
		// TODO: 创建和启动工作者
	}
}
