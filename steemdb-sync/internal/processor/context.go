package processor

import (
	"github.com/steemit/steemdb-sync/internal/config"
	"github.com/steemit/steemdb-sync/internal/mongo"
	"github.com/steemit/steemdb-sync/internal/rpc"
)

// Context holds shared dependencies for all processor components.
// Handlers and workers receive this to access MongoDB, RPC, and config.
type Context struct {
	Cfg         *config.Config
	MongoClient *mongo.Client
	RPCClient   *rpc.Client
}
