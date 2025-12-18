package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/steemit/steemdb/web/internal/models"
	"github.com/steemit/steemdb/web/internal/services"
	"github.com/steemit/steemdb/web/pkg/utils"
)

// CommentHandler handles comment/post-related HTTP requests
type CommentHandler struct {
	commentService *services.CommentService
	logger         utils.Logger
}

// NewCommentHandler creates a new comment handler
func NewCommentHandler(commentService *services.CommentService, logger utils.Logger) *CommentHandler {
	return &CommentHandler{
		commentService: commentService,
		logger:         logger,
	}
}

// GetPosts handles GET /api/v1/posts
func (h *CommentHandler) GetPosts(c *gin.Context) {
	// Parse pagination parameters
	params := models.PaginationParams{
		Page:     1,
		PageSize: 20,
	}

	if page := c.Query("page"); page != "" {
		if p, err := strconv.Atoi(page); err == nil && p > 0 {
			params.Page = p
		}
	}

	if pageSize := c.Query("page_size"); pageSize != "" {
		if ps, err := strconv.Atoi(pageSize); err == nil && ps > 0 && ps <= 100 {
			params.PageSize = ps
		}
	}

	// Parse sort parameters
	sortParams := models.SortParams{
		SortBy:    c.DefaultQuery("sort_by", "created"),
		SortOrder: c.DefaultQuery("sort_order", "desc"),
	}

	result, err := h.commentService.GetPosts(c.Request.Context(), params, sortParams)
	if err != nil {
		h.logger.Error("Failed to get posts", utils.Error(err))
		h.respondWithError(c, http.StatusInternalServerError, "Failed to retrieve posts")
		return
	}

	h.respondWithSuccessAndMeta(c, result.Data, &models.Meta{
		Page:       result.Page,
		PageSize:   result.PageSize,
		Total:      result.Total,
		TotalPages: result.TotalPages,
	})
}

// GetPost handles GET /api/v1/posts/:author/:permlink
func (h *CommentHandler) GetPost(c *gin.Context) {
	author := c.Param("author")
	permlink := c.Param("permlink")

	if author == "" || permlink == "" {
		h.respondWithError(c, http.StatusBadRequest, "Author and permlink are required")
		return
	}

	post, err := h.commentService.GetPost(c.Request.Context(), author, permlink)
	if err != nil {
		h.logger.Error("Failed to get post", utils.String("author", author), utils.String("permlink", permlink), utils.Error(err))
		h.respondWithError(c, http.StatusNotFound, "Post not found")
		return
	}

	h.respondWithSuccess(c, post)
}

// GetPostsByDate handles GET /api/v1/posts/daily
func (h *CommentHandler) GetPostsByDate(c *gin.Context) {
	// Parse date parameter (format: YYYY-MM-DD)
	dateStr := c.DefaultQuery("date", time.Now().Format("2006-01-02"))
	tag := c.DefaultQuery("tag", "all")
	sortBy := c.DefaultQuery("sort", "combined_payout")

	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		h.respondWithError(c, http.StatusBadRequest, "Invalid date format. Use YYYY-MM-DD")
		return
	}

	posts, err := h.commentService.GetPostsByDate(c.Request.Context(), date, tag, sortBy)
	if err != nil {
		h.logger.Error("Failed to get posts by date", utils.Error(err))
		h.respondWithError(c, http.StatusInternalServerError, "Failed to retrieve posts")
		return
	}

	h.respondWithSuccess(c, posts)
}

// GetPostReplies handles GET /api/v1/posts/:author/:permlink/replies
func (h *CommentHandler) GetPostReplies(c *gin.Context) {
	author := c.Param("author")
	permlink := c.Param("permlink")

	if author == "" || permlink == "" {
		h.respondWithError(c, http.StatusBadRequest, "Author and permlink are required")
		return
	}

	replies, err := h.commentService.GetPostReplies(c.Request.Context(), author, permlink)
	if err != nil {
		h.logger.Error("Failed to get replies", utils.String("author", author), utils.String("permlink", permlink), utils.Error(err))
		h.respondWithError(c, http.StatusInternalServerError, "Failed to retrieve replies")
		return
	}

	h.respondWithSuccess(c, replies)
}

// GetPostVotes handles GET /api/v1/posts/:author/:permlink/votes
func (h *CommentHandler) GetPostVotes(c *gin.Context) {
	author := c.Param("author")
	permlink := c.Param("permlink")

	if author == "" || permlink == "" {
		h.respondWithError(c, http.StatusBadRequest, "Author and permlink are required")
		return
	}

	votes, err := h.commentService.GetPostVotes(c.Request.Context(), author, permlink)
	if err != nil {
		h.logger.Error("Failed to get votes", utils.String("author", author), utils.String("permlink", permlink), utils.Error(err))
		h.respondWithError(c, http.StatusInternalServerError, "Failed to retrieve votes")
		return
	}

	h.respondWithSuccess(c, votes)
}

// GetPostReblogs handles GET /api/v1/posts/:author/:permlink/reblogs
func (h *CommentHandler) GetPostReblogs(c *gin.Context) {
	author := c.Param("author")
	permlink := c.Param("permlink")

	if author == "" || permlink == "" {
		h.respondWithError(c, http.StatusBadRequest, "Author and permlink are required")
		return
	}

	reblogs, err := h.commentService.GetPostReblogs(c.Request.Context(), author, permlink)
	if err != nil {
		h.logger.Error("Failed to get reblogs", utils.String("author", author), utils.String("permlink", permlink), utils.Error(err))
		h.respondWithError(c, http.StatusInternalServerError, "Failed to retrieve reblogs")
		return
	}

	h.respondWithSuccess(c, reblogs)
}

// Helper methods (reuse from account_handler)
func (h *CommentHandler) respondWithSuccess(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, models.APIResponse{
		Success:   true,
		Data:      data,
		Timestamp: time.Now(),
	})
}

func (h *CommentHandler) respondWithSuccessAndMeta(c *gin.Context, data interface{}, meta *models.Meta) {
	c.JSON(http.StatusOK, models.APIResponse{
		Success:   true,
		Data:      data,
		Meta:      meta,
		Timestamp: time.Now(),
	})
}

func (h *CommentHandler) respondWithError(c *gin.Context, statusCode int, message string) {
	c.JSON(statusCode, models.APIResponse{
		Success: false,
		Error: &models.APIError{
			Code:    statusCode,
			Message: message,
		},
		Timestamp: time.Now(),
	})
}
