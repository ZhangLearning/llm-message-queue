package config

import (
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	Queue    QueueConfig    `mapstructure:"queue"`
	Scheduler SchedulerConfig `mapstructure:"scheduler"`
	LoadBalancer LoadBalancerConfig `mapstructure:"loadbalancer"`
	Logging  LoggingConfig  `mapstructure:"logging"`
	Metrics  MetricsConfig  `mapstructure:"metrics"`
}

type ServerConfig struct {
	Port int    `mapstructure:"port"`
	Host string `mapstructure:"host"`
	Mode string `mapstructure:"mode"`
}

type DatabaseConfig struct {
	Postgres PostgresConfig `mapstructure:"postgres"`
	Redis    RedisConfig    `mapstructure:"redis"`
}

type PostgresConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	DBName   string `mapstructure:"dbname"`
	SSLMode  string `mapstructure:"sslmode"`
}

type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
	PoolSize int    `mapstructure:"pool_size"`
}

type QueueLevel struct {
	Name         string        `mapstructure:"name"`
	Priority     int           `mapstructure:"priority"`
	MaxWaitTime  time.Duration `mapstructure:"max_wait_time"`
	MaxConcurrent int          `mapstructure:"max_concurrent"`
}

type QueueConfig struct {
	Levels            []QueueLevel           `mapstructure:"levels"`
	DefaultMaxSize    int                    `mapstructure:"default_max_size"`
	MonitorInterval   time.Duration          `mapstructure:"monitor_interval"`
	CleanupInterval   time.Duration          `mapstructure:"cleanup_interval"`
	MaxRetentionPeriod time.Duration         `mapstructure:"max_retention_period"`
	EnableMetrics     bool                   `mapstructure:"enable_metrics"`
	EnableAutoScaling bool                   `mapstructure:"enable_auto_scaling"`
	ScalingThresholds map[string]int         `mapstructure:"scaling_thresholds"`
	Worker            WorkerConfig           `mapstructure:"worker"`
	Retry             RetryConfig            `mapstructure:"retry"`
}

type WorkerConfig struct {
	MaxBatchSize    int           `mapstructure:"max_batch_size"`
	ProcessInterval time.Duration `mapstructure:"process_interval"`
	MaxConcurrent   int           `mapstructure:"max_concurrent"`
}

type RetryConfig struct {
	InitialBackoff time.Duration `mapstructure:"initial_backoff"`
	MaxBackoff     time.Duration `mapstructure:"max_backoff"`
	Factor         float64       `mapstructure:"factor"`
	MaxRetries     int           `mapstructure:"max_retries"`
}

type SchedulerConfig struct {
	Strategy      string        `mapstructure:"strategy"`
	CheckInterval time.Duration `mapstructure:"check_interval"`
	MaxRetries    int           `mapstructure:"max_retries"`
	Timeout       time.Duration `mapstructure:"timeout"`
}

type LoadBalancerConfig struct {
	Algorithm             string        `mapstructure:"algorithm"`
	HealthCheckInterval   time.Duration `mapstructure:"health_check_interval"`
	MaxFailures           int           `mapstructure:"max_failures"`
	EnableSessionAffinity bool          `mapstructure:"enable_session_affinity"`
	SessionTimeout        time.Duration `mapstructure:"session_timeout"`
}

type LoggingConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
	Output string `mapstructure:"output"`
}

type MetricsConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Port    int    `mapstructure:"port"`
	Path    string `mapstructure:"path"`
}

func LoadConfig(configPath string) (*Config, error) {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(configPath)
	viper.AddConfigPath(".")
	viper.AddConfigPath("./configs")

	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		return nil, err
	}

	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		return nil, err
	}

	return &config, nil
}

func GetDefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Port: 8080,
			Host: "0.0.0.0",
			Mode: "debug",
		},
		Database: DatabaseConfig{
			Postgres: PostgresConfig{
				Host:     "localhost",
				Port:     5432,
				User:     "postgres",
				Password: "password",
				DBName:   "llm_queue",
				SSLMode:  "disable",
			},
			Redis: RedisConfig{
				Addr:     "localhost:6379",
				Password: "",
				DB:       0,
				PoolSize: 100,
			},
		},
		Queue: QueueConfig{
			Levels: []QueueLevel{
				{Name: "realtime", Priority: 1, MaxWaitTime: time.Second, MaxConcurrent: 100},
				{Name: "high", Priority: 2, MaxWaitTime: 5 * time.Second, MaxConcurrent: 200},
				{Name: "normal", Priority: 3, MaxWaitTime: 30 * time.Second, MaxConcurrent: 500},
				{Name: "low", Priority: 4, MaxWaitTime: 5 * time.Minute, MaxConcurrent: 1000},
			},
			DefaultMaxSize:    10000,
			MonitorInterval:   5 * time.Second,
			CleanupInterval:   1 * time.Minute,
			MaxRetentionPeriod: 24 * time.Hour,
			EnableMetrics:     true,
			EnableAutoScaling: true,
			ScalingThresholds: map[string]int{
				"realtime": 100,
				"high":     500,
				"normal":   1000,
				"low":      5000,
			},
			Worker: WorkerConfig{
				MaxBatchSize:    10,
				ProcessInterval: 100 * time.Millisecond,
				MaxConcurrent:   50,
			},
			Retry: RetryConfig{
				InitialBackoff: 1 * time.Second,
				MaxBackoff:     1 * time.Minute,
				Factor:         2.0,
				MaxRetries:     3,
			},
		},
		Scheduler: SchedulerConfig{
			Strategy:      "priority_weighted",
			CheckInterval: 100 * time.Millisecond,
			MaxRetries:    3,
			Timeout:       30 * time.Second,
		},
		LoadBalancer: LoadBalancerConfig{
			Algorithm:           "weighted_round_robin",
			HealthCheckInterval: 30 * time.Second,
			MaxFailures:         3,
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
			Output: "stdout",
		},
		Metrics: MetricsConfig{
			Enabled: true,
			Port:    9090,
			Path:    "/metrics",
		},
	}
}
