# Steem Blockchain Sync Script

## 📝 Overview

This Python script is designed to synchronize a MongoDB database with the Steem blockchain by fetching and processing blocks. The script has been optimized to fetch historical blocks in batches of 50, and once reaching the head block, it switches to fetching one block every 3 seconds with retry logic on RPC failure. This approach significantly speeds up the synchronization process, achieving up to 60% faster sync times in test scenarios.

## ✨ Features

- ⚡ Fetches historical blocks in batches for faster synchronization.
- 🔄 Switches to single block fetching mode at the head block for real-time updates.
- 🛠️ Robust retry mechanism on RPC failures to ensure continuous operation.
- 📦 Processes various operations within blocks and updates MongoDB collections accordingly.

## 🔍 Differences Between Old and New Code

### Old Code

- 🚶 Processed blocks one at a time.
- 🔧 Relied on environment variables for configuration.
- 🐢 Less efficient in handling historical block synchronization.

### New Code

- 🗂️ **Batch Processing**: Fetches historical blocks in batches of 50, significantly reducing the time required to synchronize the blockchain.
- 🔄 **Retry Logic**: Implements retry logic for RPC failures to ensure continuous and reliable operation.
- 📁 **Configuration Management**: Moved from environment variable-based configuration to a `config.json` file for easier management and flexibility.
- 🚀 **Performance Improvement**: Achieved up to 60% faster sync times in test scenarios due to batch processing and improved error handling.

## 🚀 Running the Script

### Using Docker

1. **Build the Docker Image**:
    ```sh
    docker build -t steemdb_sync .
    ```

2. **Run the Docker Container**:
    ```sh
    docker run -d --name steem-sync-container steemdb_sync
    ```

### 🛠️ Configuration

Modify the `config.json` file with the appropriate settings before running the Docker container. Example `config.json`:

```json
{
    "mongodb_url": "mongodb://10.10.100.30:27017/",
    "steemd_url": "http://10.10.100.12:8080",
    "last_block_env": 78090042,
    "batch_size": 50
}

```

`mongodb_url`: The connection string to your MongoDB instance.
`steemd_url`: The URL of the Steem node you are connecting to.
`last_block_env`: The block number to start synchronization from.
`batch_size`: Number of blocks to fetch in one batch (default is 50).



🤝 Contributing
Contributions are welcome! Please feel free to submit a pull request or open an issue on GitHub.

📜 License
This project is licensed under the MIT License. See the LICENSE file for details.