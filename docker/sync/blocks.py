import os
import time
import sys
import logging
import itertools
import threading
from datetime import datetime
from concurrent.futures import ThreadPoolExecutor, as_completed
from steem import Steem
from pymongo import MongoClient, errors

# Import configuration from the separate config.py file
from config import CONFIG

# --- Logging Setup ---
log_tag = '[BlockStream] '
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s %(threadName)s %(levelname)s: %(message)s',
    handlers=[
        logging.FileHandler(CONFIG['log_file']),
        logging.StreamHandler(sys.stdout)
    ]
)
error_logger = logging.getLogger('error_logger')
error_handler = logging.FileHandler(CONFIG['error_log_file'])
error_handler.setFormatter(logging.Formatter('%(asctime)s: %(message)s'))
error_logger.addHandler(error_handler)
error_logger.setLevel(logging.ERROR)


def get_last_synced_block(db_collection, rpc_instance):
    """
    Retrieves the last block number successfully stored in the database.
    If the DB is empty, it starts from the current last irreversible block.
    """
    try:
        last_entry = db_collection.find_one(sort=[("block_num", -1)])
        if last_entry:
            start_block = last_entry['block_num']
            logging.info(f"{log_tag}Resuming stream from block #{start_block + 1}")
            return start_block
        else:
            live_head_block = rpc_instance.get_dynamic_global_properties()['last_irreversible_block_num']
            logging.info(f"{log_tag}No previous blocks found. Starting live stream from block #{live_head_block}")
            return live_head_block
    except errors.PyMongoError as e:
        error_message = f"MongoDB error while getting last block: {e}"
        logging.critical(error_message)
        error_logger.error(error_message)
        sys.exit(f"CRITICAL: {error_message}")


def fetch_block_range(node_url, start, end):
    """
    Worker function executed by threads to fetch a range of blocks from a single node.
    """
    thread_log_tag = f"[{threading.current_thread().name}-{start}-{end}] "
    try:
        rpc = Steem([node_url], timeout=15)
        blocks_in_range = []
        for block_num in range(start, end, 1 if start < end else -1):
            try:
                block_data = rpc.get_block(block_num)
                if not block_data:
                    logging.warning(f"{thread_log_tag}Block #{block_num} not found on node {node_url}. Skipping.")
                    continue
                
                virtual_ops = rpc.get_ops_in_block(block_num, True)
                
                block_data["block_num"] = block_num
                block_data["virtual_ops"] = [op['op'] for op in virtual_ops]
                blocks_in_range.append(block_data)

            except Exception as e:
                if 'account_history_api_plugin not enabled' in str(e):
                    logging.error(f"{thread_log_tag}CRITICAL NODE ERROR on {node_url}: History plugin not enabled. Failing this worker.")
                else:
                    logging.error(f"{thread_log_tag}Error fetching block #{block_num} from {node_url}: {e}")
                return None
        
        logging.info(f"{thread_log_tag}Successfully fetched {len(blocks_in_range)} blocks from {node_url}.")
        return blocks_in_range
    except Exception as e:
        logging.error(f"{thread_log_tag}Failed to initialize Steem instance for node {node_url}: {e}")
        return None


class ReverseHistoricalWorker(threading.Thread):
    """A dedicated thread to sync historical blocks in reverse from a start point."""
    def __init__(self, start_block, db_collection, status_collection):
        super().__init__()
        self.name = "ReverseSyncWorker"
        self.current_block = start_block
        self.collection = db_collection
        self.status_collection = status_collection
        self.node_cycler = itertools.cycle(CONFIG['steemd_nodes'])
        self.max_workers = len(CONFIG['steemd_nodes'])
        logging.info(f"[{self.name}] Initialized to sync backwards from block #{start_block}")

    def run(self):
        logging.info(f"[{self.name}] Starting reverse historical sync process.")
        self.status_collection.update_one(
            {'_id': 'reverse_sync_status'},
            {'$set': {'start_time': datetime.utcnow(), 'current_block': self.current_block, 'status': 'running'}},
            upsert=True
        )

        while self.current_block > 0:
            # Define a larger chunk of blocks to process in this pass
            chunk_size = CONFIG['parallel_batch_size'] * self.max_workers * 2
            start_of_chunk = self.current_block
            end_of_chunk = max(0, self.current_block - chunk_size)
            
            logging.info(f"[{self.name}] Creating jobs to sync from #{start_of_chunk} down to #{end_of_chunk + 1}")
            
            # Create all the small jobs for this chunk
            jobs_for_chunk = []
            for i in range(start_of_chunk, end_of_chunk, -CONFIG['parallel_batch_size']):
                jobs_for_chunk.append({'start': i, 'end': max(0, i - CONFIG['parallel_batch_size'])})

            # First pass
            failed_jobs = self._run_pass(jobs_for_chunk)

            # Retry pass for any failed jobs
            if failed_jobs:
                 logging.warning(f"[{self.name}] A pass completed with {len(failed_jobs)} failures. Pausing before retry...")
                 time.sleep(5)
                 final_failures = self._run_pass(failed_jobs)
                 if final_failures:
                     logging.error(f"[{self.name}] Permanently failed to sync {len(final_failures)} jobs. Skipping these ranges.")

            # Update the current block to the end of the processed chunk for the next loop
            self.current_block = end_of_chunk
            self.status_collection.update_one(
                {'_id': 'reverse_sync_status'},
                {'$set': {'current_block': self.current_block, 'last_update_ts': datetime.utcnow()}}
            )

        logging.info(f"[{self.name}] Reverse historical sync complete. Reached block #1.")
        self.status_collection.update_one(
            {'_id': 'reverse_sync_status'},
            {'$set': {'status': 'complete', 'completion_time': datetime.utcnow()}}
        )

    def _run_pass(self, jobs):
        """Runs a pass with a list of jobs, returns any that failed."""
        failed_jobs = []
        with ThreadPoolExecutor(max_workers=self.max_workers) as executor:
            future_to_job = {executor.submit(fetch_block_range, next(self.node_cycler), j['start'], j['end']): j for j in jobs}
            for future in as_completed(future_to_job):
                job = future_to_job[future]
                try:
                    block_batch = future.result()
                    if block_batch:
                        self._save_batch(block_batch)
                    else:
                        failed_jobs.append(job)
                except Exception as exc:
                    logging.error(f"[{self.name}] Job for {job} generated an exception: {exc}")
                    failed_jobs.append(job)
        return failed_jobs
        
    def _save_batch(self, batch):
        try:
            self.collection.insert_many(batch, ordered=False)
        except errors.BulkWriteError:
            logging.warning(f"[{self.name}] Bulk insert completed with some duplicates (expected).")
        except Exception as e:
            logging.error(f"[{self.name}] Failed to save batch to DB: {e}")


def main():
    """Main function to connect to services and start the streaming loop."""
    logging.info(f"{log_tag}Starting Steem Blockchain Live Streamer")

    try:
        mongo = MongoClient(CONFIG['mongodb_url'])
        mongo.admin.command('ismaster')
        db = mongo[CONFIG['db_name']]
        blocks_collection = db[CONFIG['collection_name']]
        status_collection = db['Status']
        blocks_collection.create_index("block_num", unique=True)
        logging.info(f"{log_tag}Successfully connected to MongoDB at {CONFIG['mongodb_url']}")
        rpc = Steem(CONFIG['steemd_nodes'], timeout=10)
    except Exception as e:
        error_message = f"Failed to initialize connections: {e}"
        logging.critical(error_message)
        error_logger.error(error_message)
        sys.exit(f"CRITICAL: {error_message}")

    last_synced = get_last_synced_block(blocks_collection, rpc)
    
    status_collection.update_one(
        {'_id': 'stream_status'},
        {'$set': {'script_start_time': datetime.utcnow(), 'last_synced_block': last_synced}},
        upsert=True
    )
    
    # --- Start or Resume Reverse Historical Sync (if needed) ---
    reverse_status = status_collection.find_one({'_id': 'reverse_sync_status'})
    
    # Start worker if it has never run OR if it started but has not completed.
    if not reverse_status or reverse_status.get('status') != 'complete':
        start_block_for_reverse = 0
        if not reverse_status:
            # First run: start from the last block synced by the real-time streamer.
            start_block_for_reverse = last_synced - 1
            logging.info(f"{log_tag}No reverse sync found. Starting historical backfill from block #{start_block_for_reverse}")
            # Update the main status document with the initial block number
            status_collection.update_one(
                {'_id': 'stream_status'}, {'$set': {'initial_start_block': last_synced}}
            )
        else:
            # Resume run: start from the last block the worker was processing.
            start_block_for_reverse = reverse_status['current_block']
            logging.info(f"{log_tag}Resuming historical backfill from block #{start_block_for_reverse}")

        if start_block_for_reverse > 0:
            reverse_worker = ReverseHistoricalWorker(start_block_for_reverse, blocks_collection, status_collection)
            reverse_worker.start()
        else:
            logging.info(f"{log_tag}Reverse sync already at block 1 or lower. Nothing to do.")


    # --- REAL-TIME STREAMING LOOP ---
    node_cycler = itertools.cycle(CONFIG['steemd_nodes'])
    
    while True:
        try:
            current_rpc = Steem([next(node_cycler)], timeout=5)
            current_live_head = current_rpc.get_dynamic_global_properties()['last_irreversible_block_num']
            
            if current_live_head > last_synced:
                block_to_fetch = last_synced + 1
                logging.info(f"{log_tag}New irreversible block detected: #{block_to_fetch}")
                
                if fetch_and_save_block(block_to_fetch, blocks_collection, current_rpc):
                    last_synced = block_to_fetch
                    status_collection.update_one(
                        {'_id': 'stream_status'},
                        {'$set': {'last_synced_block': last_synced, 'last_update_ts': datetime.utcnow()}}
                    )
                else:
                    logging.warning(f"{log_tag}Retrying block #{block_to_fetch} in a moment...")
                    time.sleep(1)
            else:
                time.sleep(3)

        except Exception as e:
            error_message = f"An unexpected error occurred in the main loop: {e}"
            logging.error(error_message, exc_info=True)
            error_logger.error(error_message)
            time.sleep(10)

def fetch_and_save_block(block_num, collection, rpc_instance):
    """
    Fetches a single block, its virtual ops, and saves it to the database.
    """
    try:
        block_data = rpc_instance.get_block(block_num)
        if not block_data:
            logging.warning(f"{log_tag}Block #{block_num} not found. It might be a skipped block.")
            return True

        virtual_ops = rpc_instance.get_ops_in_block(block_num, True)
        
        block_data["block_num"] = block_num
        block_data["virtual_ops"] = [op['op'] for op in virtual_ops]

        collection.insert_one(block_data)
        logging.info(f"{log_tag}Successfully streamed and saved block #{block_num}")
        return True
    except errors.DuplicateKeyError:
        logging.warning(f"{log_tag}Block #{block_num} already exists in database. Skipping.")
        return True
    except Exception as e:
        logging.error(f"{log_tag}Failed to fetch or save block #{block_num}: {e}")
        error_logger.error(f"Error on block {block_num}: {e}")
        return False

if __name__ == '__main__':
    main()
