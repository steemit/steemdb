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

	"github.com/steemdb/web/internal/api"
	"github.com/steemdb/web/internal/database"
	"github.com/steemdb/web/internal/services"
	"github.com/steemdb/web/pkg/steem"
	"github.com/steemdb/web/pkg/utils"
)

func main() {
	// Load configuration
	configPath := "configs/config.yaml"
	if len(os.Args) > 1 {
		configPath = os.Args[1]
	}

	config, err := utils.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize logger
	logger, err := utils.NewLogger(config.Log)
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	defer logger.Sync()

	logger.Info("Starting SteemDB Web Service",
		utils.String("version", "1.0.0"),
		utils.String("config", configPath))

	// Initialize MongoDB
	mongodb, err := database.NewMongoDB(config.Database.MongoDB, logger)
	if err != nil {
		logger.Fatal("Failed to connect to MongoDB", utils.Error(err))
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := mongodb.Close(ctx); err != nil {
			logger.Error("Failed to close MongoDB connection", utils.Error(err))
		}
	}()

	// Initialize Redis
	redis, err := database.NewRedis(config.Database.Redis, logger)
	if err != nil {
		logger.Fatal("Failed to connect to Redis", utils.Error(err))
	}
	defer func() {
		if err := redis.Close(); err != nil {
			logger.Error("Failed to close Redis connection", utils.Error(err))
		}
	}()

	// Create indexes
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := mongodb.CreateIndexes(ctx); err != nil {
		logger.Error("Failed to create indexes", utils.Error(err))
	}

	// Initialize Steem client
	steemClient := steem.NewClient(config.Steem.Nodes, logger)

	// Initialize WebSocket service
	var wsService *services.WebSocketService
	if config.WebSocket.Enabled {
		wsService = services.NewWebSocketService(steemClient, mongodb, logger)
		wsService.Start()
		defer wsService.Stop()
		
		logger.Info("WebSocket service started", utils.String("path", config.WebSocket.Path))
	}

	// Set Gin mode
	if config.Server.Mode == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	// Initialize Gin router
	router := gin.New()
	
	// Add middleware
	router.Use(gin.Logger())
	router.Use(gin.Recovery())

	// Health check endpoint
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":    "healthy",
			"timestamp": time.Now().Unix(),
			"version":   "1.0.0",
		})
	})

	// Readiness check endpoint
	router.GET("/ready", func(c *gin.Context) {
		// Check database connections
		if err := mongodb.Ping(c.Request.Context()); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "not_ready",
				"error":  "database_unavailable",
			})
			return
		}

		if err := redis.Ping(c.Request.Context()); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "not_ready",
				"error":  "redis_unavailable",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status":    "ready",
			"timestamp": time.Now().Unix(),
		})
	})

	// Setup WebSocket endpoint
	if config.WebSocket.Enabled && wsService != nil {
		router.GET(config.WebSocket.Path, func(c *gin.Context) {
			wsService.HandleWebSocket(c.Writer, c.Request)
		})
	}

	// Setup API routes
	api.SetupRoutes(router, mongodb, redis, logger)

	// Create HTTP server
	server := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", config.Server.Host, config.Server.Port),
		Handler:      router,
		ReadTimeout:  config.Server.ReadTimeout,
		WriteTimeout: config.Server.WriteTimeout,
		IdleTimeout:  config.Server.IdleTimeout,
	}

	// Start server in goroutine
	go func() {
		logger.Info("Starting HTTP server",
			utils.String("addr", server.Addr),
			utils.String("mode", config.Server.Mode))
		
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("Failed to start server", utils.Error(err))
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down server...")

	// Graceful shutdown with timeout
	ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Fatal("Server forced to shutdown", utils.Error(err))
	}

	logger.Info("Server exited")
}
