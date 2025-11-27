package api

import (
	"github.com/gin-gonic/gin"

	"github.com/steemdb/web/internal/database"
	"github.com/steemdb/web/internal/services"
	"github.com/steemdb/web/pkg/utils"
)

// SetupRoutes configures all API routes
func SetupRoutes(router *gin.Engine, db *database.MongoDB, redis *database.Redis, logger utils.Logger) {
	// Initialize services
	accountService := services.NewAccountService(db, redis, logger)
	blockService := services.NewBlockService(db, redis, logger)

	// Initialize handlers
	accountHandler := NewAccountHandler(accountService, logger)
	blockHandler := NewBlockHandler(blockService, logger)

	// API v1 routes
	v1 := router.Group("/api/v1")
	{
		// Account routes
		accounts := v1.Group("/accounts")
		{
			accounts.GET("", accountHandler.GetAccounts)
			accounts.GET("/search", accountHandler.SearchAccounts)
			accounts.GET("/stats", accountHandler.GetAccountStats)
			accounts.GET("/top", accountHandler.GetTopAccounts)
			accounts.GET("/:name", accountHandler.GetAccount)
		}

		// Block routes
		blocks := v1.Group("/blocks")
		{
			blocks.GET("", blockHandler.GetBlocks)
			blocks.GET("/latest", blockHandler.GetLatestBlocks)
			blocks.GET("/stats", blockHandler.GetBlockStats)
			blocks.GET("/:number", blockHandler.GetBlock)
		}

		// Operation routes
		operations := v1.Group("/operations")
		{
			operations.GET("/stats", blockHandler.GetOperationStats)
		}

		// Status endpoint
		v1.GET("/status", func(c *gin.Context) {
			c.JSON(200, gin.H{
				"status":  "ok",
				"version": "1.0.0",
				"service": "steemdb-web",
			})
		})
	}
}
