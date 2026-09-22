package api

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// queryContext builds a gin.Context carrying a GET request with the given
// raw query string, so the parameter parsing helpers can be exercised
// without a router or database.
func queryContext(query string) *gin.Context {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("GET", "/?"+query, nil)
	return c
}

// TestParsePaginationParamsAliases covers the alias contract shared by the
// blocks/accounts/posts listings: both parameter vocabularies must produce
// identical PaginationParams, the canonical name must win when both are
// supplied, and invalid or out-of-range values must keep the defaults.
// Before the fix, accounts/posts only read page_size, so the frontend's
// limit was silently dropped and page size was stuck at the default 20.
func TestParsePaginationParamsAliases(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  struct{ page, pageSize int }
	}{
		{
			name:  "canonical names",
			query: "page=3&page_size=50",
			want:  struct{ page, pageSize int }{3, 50},
		},
		{
			name:  "short aliases are equivalent to canonical names",
			query: "limit=50",
			want:  struct{ page, pageSize int }{1, 50},
		},
		{
			name:  "canonical page_size wins over limit",
			query: "page_size=30&limit=70",
			want:  struct{ page, pageSize int }{1, 30},
		},
		{
			name:  "defaults when nothing is supplied",
			query: "",
			want:  struct{ page, pageSize int }{1, 20},
		},
		{
			name:  "unparsable page_size keeps default",
			query: "page_size=abc",
			want:  struct{ page, pageSize int }{1, 20},
		},
		{
			name:  "zero page is ignored",
			query: "page=0",
			want:  struct{ page, pageSize int }{1, 20},
		},
		{
			name:  "negative page is ignored",
			query: "page=-2",
			want:  struct{ page, pageSize int }{1, 20},
		},
		{
			name:  "page_size above the cap is ignored",
			query: "page_size=101",
			want:  struct{ page, pageSize int }{1, 20},
		},
		{
			name:  "page_size at the cap is accepted",
			query: "page_size=100",
			want:  struct{ page, pageSize int }{1, 100},
		},
		{
			name:  "alias limit at the cap is accepted",
			query: "limit=100",
			want:  struct{ page, pageSize int }{1, 100},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parsePaginationParams(queryContext(tt.query))
			if got.Page != tt.want.page {
				t.Errorf("Page = %d, want %d", got.Page, tt.want.page)
			}
			if got.PageSize != tt.want.pageSize {
				t.Errorf("PageSize = %d, want %d", got.PageSize, tt.want.pageSize)
			}
		})
	}
}

// TestParseSortParamsAliases covers the sort alias contract: sort/sort_order
// behave exactly like sort_by/sort_order, the canonical names win, defaults
// apply per endpoint, and a sort_order outside asc/desc falls back to the
// default instead of being passed through to the query layer.
func TestParseSortParamsAliases(t *testing.T) {
	tests := []struct {
		name             string
		query            string
		defaultSortBy    string
		defaultSortOrder string
		wantSortBy       string
		wantSortOrder    string
	}{
		{
			name:             "canonical names",
			query:            "sort_by=created&sort_order=asc",
			defaultSortBy:    "block_num",
			defaultSortOrder: "desc",
			wantSortBy:       "created",
			wantSortOrder:    "asc",
		},
		{
			name:             "short aliases are equivalent to canonical names",
			query:            "sort=created&order=asc",
			defaultSortBy:    "block_num",
			defaultSortOrder: "desc",
			wantSortBy:       "created",
			wantSortOrder:    "asc",
		},
		{
			name:             "canonical sort_by wins over sort",
			query:            "sort_by=reputation&sort=name",
			defaultSortBy:    "block_num",
			defaultSortOrder: "desc",
			wantSortBy:       "reputation",
			wantSortOrder:    "desc",
		},
		{
			name:             "defaults when nothing is supplied",
			query:            "",
			defaultSortBy:    "reputation",
			defaultSortOrder: "desc",
			wantSortBy:       "reputation",
			wantSortOrder:    "desc",
		},
		{
			name:             "alias sort fills an absent canonical sort_by",
			query:            "sort=net_votes",
			defaultSortBy:    "created",
			defaultSortOrder: "desc",
			wantSortBy:       "net_votes",
			wantSortOrder:    "desc",
		},
		{
			name:             "unknown sort_order falls back to default",
			query:            "sort_order=sideways",
			defaultSortBy:    "created",
			defaultSortOrder: "desc",
			wantSortBy:       "created",
			wantSortOrder:    "desc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseSortParams(queryContext(tt.query), tt.defaultSortBy, tt.defaultSortOrder)
			if got.SortBy != tt.wantSortBy {
				t.Errorf("SortBy = %q, want %q", got.SortBy, tt.wantSortBy)
			}
			if got.SortOrder != tt.wantSortOrder {
				t.Errorf("SortOrder = %q, want %q", got.SortOrder, tt.wantSortOrder)
			}
		})
	}
}
