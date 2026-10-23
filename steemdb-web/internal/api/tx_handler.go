package api

import (
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/steemit/steemdb/web/internal/models"
	"github.com/steemit/steemdb/web/internal/services"
	"github.com/steemit/steemdb/web/pkg/utils"
)

// TxHandler handles transaction-related HTTP requests
type TxHandler struct {
	txService *services.TxService
	logger    utils.Logger
}

// NewTxHandler creates a new transaction handler
func NewTxHandler(txService *services.TxService, logger utils.Logger) *TxHandler {
	return &TxHandler{
		txService: txService,
		logger:    logger,
	}
}

// txIDPattern matches a Steem transaction ID: 40 hex characters.
var txIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)

// GetTx handles GET /api/v1/tx/:id
// Serves the transaction local-first from the tx_cache collection, falling
// back to the public node (condenser_api.get_transaction — the same upstream
// call the legacy /tx/{txid} page used) with the result written back to the
// cache. See services.TxService for the read-through contract.
func (h *TxHandler) GetTx(c *gin.Context) {
	txID := c.Param("id")
	if !txIDPattern.MatchString(txID) {
		h.respondWithError(c, http.StatusBadRequest, "Invalid transaction ID")
		return
	}

	tx, err := h.txService.GetTransaction(c.Request.Context(), txID)
	if err != nil {
		// steemd reports an unknown transaction ID as a JSON-RPC error; tell
		// that apart from a genuine upstream failure so exchanges polling for
		// a not-yet-seen deposit get a 404 instead of a 502.
		if isTxNotFoundError(err) {
			h.respondWithError(c, http.StatusNotFound, "Transaction not found")
			return
		}
		h.logger.Error("Failed to get transaction", utils.String("tx_id", txID), utils.Error(err))
		h.respondWithError(c, http.StatusBadGateway, "Failed to retrieve transaction")
		return
	}
	if len(tx) == 0 {
		h.respondWithError(c, http.StatusNotFound, "Transaction not found")
		return
	}

	h.respondWithSuccess(c, tx)
}

// isTxNotFoundError reports whether an RPC error means the transaction does
// not exist, as opposed to a connectivity/timeout failure. steemd signals a
// missing transaction two ways: "Unknown Transaction <id>" for ids its index
// never saw, and the "blk->transactions.size() > txInBlock" assert for
// degenerate ids (e.g. all zeros) that resolve to a block but no transaction.
func isTxNotFoundError(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unknown transaction") ||
		strings.Contains(msg, "transactions.size()") ||
		strings.Contains(msg, "not found")
}

// Helper methods

func (h *TxHandler) respondWithSuccess(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, models.APIResponse{
		Success:   true,
		Data:      data,
		Timestamp: time.Now(),
	})
}

func (h *TxHandler) respondWithError(c *gin.Context, statusCode int, message string) {
	c.JSON(statusCode, models.APIResponse{
		Success: false,
		Error: &models.APIError{
			Code:    statusCode,
			Message: message,
		},
		Timestamp: time.Now(),
	})
}
