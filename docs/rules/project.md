# Project Rules

## Steem Blockchain Connectivity

- **Use `steemgosdk`** (`github.com/steemit/steemgosdk`) for any Steem
  blockchain connectivity from Go code — RPC calls, transaction broadcasting,
  key management, and the `steem://` URI protocol.
- Both `steemdb-web` and `steemdb-sync` depend on it (see their `go.mod`);
  do not introduce alternative Steem RPC clients.
- **Known exception — steemdb-web `pkg/steem/rc.go`** (`rc_api.find_rc_accounts`):
  `rc_api` is an appbase plugin API that only accepts named JSON-RPC params
  (`{"accounts": [...]}`); positional arrays are rejected by nodes, and the
  SDK's `Call`/`CallWithResult` surface only emits array params. That one
  call therefore posts the JSON-RPC body directly while reusing the SDK
  client's node rotation/timeout/backoff semantics. Migrate back to the SDK
  and remove this exception if the SDK grows a named-params method.
