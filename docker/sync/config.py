# config.py


CONFIG = {
    # --- Node Configuration ---
    # List of Steem API nodes to use for fetching blocks.
    # The script will distribute load across these nodes when syncing in parallel.
    "steemd_nodes": [
        "https://api.justyy.com",
        "https://api.moecki.online",
        "https://api.pennsif.net",
        "https://api.botsteem.com",
        "https://api2.justyy.com",
        "https://api.steemitdev.com",
       	"https://api.steememory.com",
#        "https://api.cotina.org/",
    ],

    # --- Database Configuration ---
    "mongodb_url": "mongodb://host.docker.internal:27017/",
    "db_name": "BlazeDB",
    "collection_name": "Blocks",

    # The number of blocks each worker thread will fetch in a single batch during parallel sync.
    "parallel_batch_size": 50,

    # The threshold for activating parallel sync. If the number of blocks to sync
    # is greater than this value, the script will use all available nodes.
    # Otherwise, it will sync one block at a time.
    "parallel_sync_threshold": 100,

    # --- Logging Configuration ---
    "log_file": "blocks_sync.log",
    "error_log_file": "blocks_error.log"
}
