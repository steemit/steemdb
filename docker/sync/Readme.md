# Steem Blockchain Dual-Sync Service

A high-performance, resilient Python service to synchronize the **Steem Blockchain** into **MongoDB**, supporting **real-time streaming** and **full historical backfill**. Designed for developers and data engineers needing reliable, up-to-date access to blockchain data.

---

## 🚀 Overview

This service employs a **dual-sync architecture**:

* **Real-Time Streamer**: Continuously captures new blocks as they become irreversible.
* **Reverse Historical Worker**: Concurrently backfills historical blocks from the current point to genesis.

Key features include **parallelized fetching**, **node failover**, **automatic resume on restart**, and **data integrity with unique indexing**.

---

## 🧠 Core Features

### 1. Dual-Sync Architecture

* **Real-Time Streamer (Main Thread)**
  Watches for `last_irreversible_block_num` and streams live blocks into MongoDB.

* **Reverse Historical Worker (Background Thread)**
  Concurrently fetches past blocks down to block #1 without affecting the live stream.

### 2. Parallel Fetching & Failover

* **Multi-Node Support**: Rotates through nodes in `config.py`.
* **Thread Pooling**: Fetches blocks in batches using `ThreadPoolExecutor`.
* **Automatic Failover**: Re-attempts failed batches using alternate nodes.

### 3. Intelligent Resume on Restart

* Maintains `stream_status` and `reverse_sync_status` in MongoDB.
* Seamlessly resumes both real-time and historical sync from the last synced block.

### 4. Data Model & Integrity

* Stores full raw block data with virtual operations (`virtual_ops`).
* Enforces a unique index on `block_num` to prevent duplicates.
* Utilizes native MongoDB `_id` for efficient, non-blocking inserts.

---

## 🧰 Prerequisites

Install required Python libraries:

```bash
pip install -r requirements.txt
```

---

## ⚙️ Configuration

All settings are located in `config.py`:

| Key                   | Description                               |
| --------------------- | ----------------------------------------- |
| `steemd_nodes`        | List of public Steem nodes                |
| `mongodb_url`         | MongoDB connection string                 |
| `db_name`             | Target MongoDB database name              |
| `collection_name`     | MongoDB collection for storing blocks     |
| `parallel_batch_size` | Batch size per thread for historical sync |

---

## 💪 Running the Script

Run the script from the terminal:

```bash
python blocks.py
```

Logs will be output to both the console and configured log files.

---

## 🔄 How It Works: Walkthrough

### On First Run:

1. Connects to MongoDB and the Steem network.
2. Fetches `last_irreversible_block_num` (e.g., 97,000,000) and begins streaming.
3. Initializes `stream_status` and launches `ReverseHistoricalWorker`.
4. Historical worker starts syncing from 96,999,999 down to block #1.

### On Restart:

1. Loads `stream_status` and resumes real-time streaming.
2. Loads `reverse_sync_status` and resumes historical backfill.
3. Both processes continue from where they left off.

---

## 📁 Data Structure

* Each document in `Blocks` collection contains:

  * `block_num`: Block height
  * `raw_block`: Raw block data
  * `virtual_ops`: Array of virtual operations
  * `_id`: Auto-generated MongoDB ObjectId

---

## 📢 Notes

* Ensure MongoDB is running and accessible before starting the script.
* More Steem nodes in the config = better reliability and speed.
* Customize logging behavior in `config.py` if needed.

---

## ✅ License

MIT License. See [LICENSE](LICENSE) for details.

---

## 📖 References

* [Steem Developer Docs](https://developers.steem.io)
* [pymongo](https://pymongo.readthedocs.io)
* [Python ThreadPoolExecutor](https://docs.python.org/3/library/concurrent.futures.html#threadpoolexecutor)
