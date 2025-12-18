package api

import (
	"github.com/gin-gonic/gin"

	"github.com/steemit/steemdb/web/internal/database"
	"github.com/steemit/steemdb/web/internal/services"
	"github.com/steemit/steemdb/web/pkg/steem"
	"github.com/steemit/steemdb/web/pkg/utils"
)

// SetupRoutes configures all API routes
func SetupRoutes(router *gin.Engine, db *database.MongoDB, redis *database.Redis, steemClient *steem.Client, logger utils.Logger) {
	// Initialize services
	accountService := services.NewAccountService(db, redis, logger)
	blockService := services.NewBlockService(db, redis, logger)
	dashboardService := services.NewDashboardService(db, steemClient, logger)
	commentService := services.NewCommentService(db, redis, logger)
	labsService := services.NewLabsService(db, steemClient, logger)

	// Initialize handlers
	accountHandler := NewAccountHandler(accountService, logger)
	blockHandler := NewBlockHandler(blockService, logger)
	dashboardHandler := NewDashboardHandler(dashboardService, logger)
	commentHandler := NewCommentHandler(commentService, logger)
	labsHandler := NewLabsHandler(labsService, logger)
	legacyHandler := NewLegacyHandler(steemClient, logger)

	// API v1 routes - register first to avoid conflicts with legacy routes
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

		// Dashboard route
		v1.GET("/dashboard", dashboardHandler.GetDashboard)

		// Post/Comment routes
		posts := v1.Group("/posts")
		{
			posts.GET("", commentHandler.GetPosts)
			posts.GET("/daily", commentHandler.GetPostsByDate)
			posts.GET("/:author/:permlink", commentHandler.GetPost)
			posts.GET("/:author/:permlink/replies", commentHandler.GetPostReplies)
			posts.GET("/:author/:permlink/votes", commentHandler.GetPostVotes)
			posts.GET("/:author/:permlink/reblogs", commentHandler.GetPostReblogs)
		}

		// Labs routes
		labs := v1.Group("/labs")
		{
			labs.GET("", labsHandler.GetLabsIndex)
			labs.GET("/powerup", labsHandler.GetPowerUps)
			labs.GET("/powerdown", labsHandler.GetPowerDowns)
			labs.GET("/rshares", labsHandler.GetRshares)
			labs.GET("/curation", labsHandler.GetCuration)
			labs.GET("/author", labsHandler.GetAuthor)
			labs.GET("/flags", labsHandler.GetFlags)
			labs.GET("/clients", labsHandler.GetClients)
			labs.GET("/benefactors", labsHandler.GetBenefactors)
			labs.GET("/pending", labsHandler.GetPending)
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

	// Legacy API endpoints - register AFTER v1 routes, match specific legacy endpoints only
	legacyEndpoints := []string{
		"supply", "props", "percentage", "rshares", "downvotes",
		"topwitnesses", "rewards", "curation", "powerup", "steem",
	}
	for _, endpoint := range legacyEndpoints {
		router.GET("/api/"+endpoint, legacyHandler.RedirectToV1)
	}

	// Legacy token endpoint - returns plain text, not JSON
	router.GET("/api/token", legacyHandler.GetToken)
}
