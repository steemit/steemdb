# Project Rules

## Steem Blockchain Connectivity

- **Use `steemgosdk`** (`github.com/steemit/steemgosdk`) for any Steem
  blockchain connectivity from Go code — RPC calls, transaction broadcasting,
  key management, and the `steem://` URI protocol.
- Both `steemdb-web` and `steemdb-sync` depend on it (see their `go.mod`);
  do not introduce alternative Steem RPC clients.
