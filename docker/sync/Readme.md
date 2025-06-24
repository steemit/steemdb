Steem Blockchain Dual-Sync Service
Overview
This Python script is a high-performance, resilient service designed to synchronize data from the Steem blockchain into a MongoDB database. It employs a robust dual-sync architecture to provide both real-time data streaming and a complete historical backfill, ensuring a comprehensive and always up-to-date copy of the blockchain.

The core principle is to capture live blocks immediately while simultaneously working in the background to fetch all past blocks, making it ideal for applications that need both current and historical blockchain data.

Core Features
1. Dual-Sync Architecture
The script operates using two concurrent threads to maximize efficiency:

Real-Time Streamer (Main Thread): This is the primary process. It constantly watches the Steem network for new last_irreversible_block_num. As soon as a new block is confirmed, it is fetched, processed, and saved to the database. This ensures your data is always current to within a few seconds of the live blockchain.

Reverse Historical Worker (Background Thread): This worker's job is to fill in the gaps. On the very first run, it identifies the block number where the Real-Time Streamer began and starts fetching all blocks backwards from that point down to block #1. This backfill process runs in the background and does not interfere with the live streaming.

2. Parallel Fetching & Failover
To make the historical sync as fast as possible, the service leverages parallelism and redundancy:

Multi-Node Connection: The script connects to a list of public Steem nodes defined in config.py.

Thread Pooling: The ReverseHistoricalWorker uses a ThreadPoolExecutor to create multiple worker threads, sending out simultaneous requests for different batches of blocks to different nodes.

Automatic Failover: If a Steem node is slow, unresponsive, or returns an error (e.g., "database lock"), the script automatically retries the failed batch of blocks, cycling to the next available node in the list. This makes the sync process highly resilient to public node instability.

3. Intelligent Resume on Restart
The service is designed for continuous operation and can be stopped and restarted without losing its place.

Status Tracking: It maintains a Status collection in MongoDB with two key documents:

stream_status: Tracks the progress of the Real-Time Streamer.

reverse_sync_status: Tracks the progress of the Reverse Historical Worker, including the current_block it's working on.

Seamless Resumption: When the script restarts, it reads these status documents to determine exactly where to resume both the real-time and historical syncs, preventing data loss and redundant work.

4. Data Model & Integrity
Raw Blocks with Virtual Ops: Each document in the Blocks collection contains the complete, raw block data, along with an added virtual_ops array that includes all virtual operations for that block.

MongoDB ObjectId: The primary _id of each document is the standard MongoDB ObjectId, allowing for fast, non-blocking inserts.

Unique Index on block_num: To ensure data integrity and prevent duplicate entries, the script creates a unique index on the block_num field.

How to Use
1. Prerequisites
Install the required Python libraries:

pip install -r requirements.txt

2. Configuration
All settings are managed in the config.py file. Key options include:

steemd_nodes: A list of public Steem API nodes to use. More nodes improve parallel performance.

mongodb_url: The connection string for your MongoDB instance.

db_name & collection_name: The names for your database and blocks collection.

parallel_batch_size: The number of blocks each historical worker thread fetches at a time.

3. Running the Script
Simply execute the script from your terminal:

python blocks.py

The script will start logging its progress to both the console and the configured log files.

How It Works: A Walkthrough
On First Run:
The script connects to MongoDB and the Steem network.

It sees the Blocks collection is empty and gets the current last_irreversible_block_num (e.g., 97,000,000). This becomes its starting point.

It creates the stream_status document, noting that it will start streaming from block 97,000,000.

It launches the ReverseHistoricalWorker in the background, instructing it to start syncing backwards from block 96,999,999.

The main thread immediately enters the Real-Time loop, waiting for block 97,000,001 to appear.

On Restart:
The script connects to MongoDB and Steem.

It reads the stream_status and sees the last synced block was, for example, 97,000,150. It prepares to wait for block 97,000,151.

It reads the reverse_sync_status and sees the historical worker left off at block 85,450,000.

It re-launches the ReverseHistoricalWorker, instructing it to resume syncing backwards from 85,450,000.

The main thread enters the Real-Time loop to continue streaming live blocks.
