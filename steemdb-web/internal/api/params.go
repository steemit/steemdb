package api

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/steemit/steemdb/web/internal/models"
)

// Shared query-parameter parsing for the collection listing endpoints
// (blocks, accounts, posts). Every listing accepts one canonical parameter
// set plus the short aliases the frontend historically used:
//
//	page       (canonical only)
//	page_size  ← limit
//	sort_by    ← sort
//	sort_order ← order
//
// The canonical name wins when both are supplied. Unparsable or
// out-of-range values keep their defaults; page_size is clamped to
// 1..maxPageSize. This is the alias convention the blocks endpoint
// established first; accounts and posts follow it (fix for the silent
// param break where the frontend's limit/sort/order were dropped).
const (
	defaultPage     = 1
	defaultPageSize = 20
	maxPageSize     = 100
)

// parsePaginationParams reads page and page_size (alias: limit).
func parsePaginationParams(c *gin.Context) models.PaginationParams {
	params := models.PaginationParams{
		Page:     defaultPage,
		PageSize: defaultPageSize,
	}

	if page := c.Query("page"); page != "" {
		if p, err := strconv.Atoi(page); err == nil && p > 0 {
			params.Page = p
		}
	}

	pageSizeStr := c.Query("page_size")
	if pageSizeStr == "" {
		pageSizeStr = c.Query("limit")
	}
	if pageSizeStr != "" {
		if ps, err := strconv.Atoi(pageSizeStr); err == nil && ps > 0 && ps <= maxPageSize {
			params.PageSize = ps
		}
	}

	return params
}

// parseSortParams reads sort_by/sort_order (aliases: sort/order) with the
// endpoint-specific defaults. A sort_order that is neither "asc" nor "desc"
// falls back to the default instead of being passed through.
func parseSortParams(c *gin.Context, defaultSortBy, defaultSortOrder string) models.SortParams {
	sortBy := c.Query("sort_by")
	if sortBy == "" {
		sortBy = c.Query("sort")
	}
	if sortBy == "" {
		sortBy = defaultSortBy
	}

	sortOrder := c.Query("sort_order")
	if sortOrder == "" {
		sortOrder = c.Query("order")
	}
	if sortOrder != "asc" && sortOrder != "desc" {
		sortOrder = defaultSortOrder
	}

	return models.SortParams{
		SortBy:    sortBy,
		SortOrder: sortOrder,
	}
}
