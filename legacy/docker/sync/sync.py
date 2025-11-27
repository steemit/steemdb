import os
import time
import json
import sys
from datetime import datetime, timedelta
from steem import Steem
from pymongo import MongoClient
import requests
import logging
from concurrent.futures import ThreadPoolExecutor, as_completed
from functools import lru_cache
import threading

log_tag = '[Sync] '

# Configure logging
logging.basicConfig(filename='error.log', level=logging.ERROR)

# Load configuration from config.json
config_path = 'config.json'
with open(config_path, 'r') as config_file:
    config = json.load(config_file)

steemd_url = config.get('steemd_url', 'https://api.steemit.com')
last_block_env = config.get('last_block_env')
mongodb_url = config.get('mongodb_url')
batch_size = config.get('batch_size', 50)
block_interval = config.get('block_interval', 60)

if not mongodb_url:
    print(f"{log_tag}NEED MONGODB")
    exit()

print(f"{log_tag}mongo url: {mongodb_url}")

# Initialize Steem and MongoDB connections
fullnodes = [steemd_url]
rpc = Steem(fullnodes)
mongo = MongoClient(mongodb_url)
db = mongo.steemdb

# Retrieve last processed block
init = db.status.find_one({'_id': 'height'})
last_block = init['value'] if init else (last_block_env or 1)

def process_op(op_obj, block, blockid):
    op_type = op_obj[0]
    op = op_obj[1]
    try:
        if op_type == "comment":
            update_comment(op['author'], op['permlink'], op, block, blockid)
        elif op_type == "comment_options":
            update_comment_options(op, block, blockid)
        elif op_type == "vote":
            save_vote(op, block, blockid)
        elif op_type == "convert":
            save_convert(op, block, blockid)
        elif op_type == "comment_benefactor_reward":
            save_benefactor_reward(op, block, blockid)
        elif op_type == "custom_json":
            save_custom_json(op, block, blockid)
        elif op_type == "feed_publish":
            save_feed_publish(op, block, blockid)
        elif op_type == "account_witness_vote":
            save_witness_vote(op, block, blockid)
        elif op_type in ["pow", "pow2"]:
            save_pow(op, block, blockid)
        elif op_type == "transfer":
            save_transfer(op, block, blockid)
        elif op_type == "curation_reward":
            save_curation_reward(op, block, blockid)
        elif op_type == "author_reward":
            save_author_reward(op, block, blockid)
        elif op_type == "transfer_to_vesting":
            save_vesting_deposit(op, block, blockid)
        elif op_type == "fill_vesting_withdraw":
            save_vesting_withdraw(op, block, blockid)
    except Exception as e:
        error_message = f"{log_tag}Error processing operation {op_type}: {e}"
        print(error_message)
        logging.error(error_message)
        # Don't exit, let the script continue processing other operations

def process_block(block, blockid):
    try:
        save_block(block, blockid)
        ops = rpc.get_ops_in_block(blockid, True)

        for tx in block['transactions']:
            for op_obj in tx['operations']:
                process_op(op_obj, block, blockid)
        for op_obj in ops:
            process_op(op_obj['op'], block, blockid)
    except Exception as e:
        print(f"{log_tag}Error processing block {blockid}: {e}")

def save_convert(op, block, blockid):
    convert = op.copy()
    _id = f"{blockid}/{op['requestid']}"
    convert.update({
        '_id': _id,
        '_ts': datetime.strptime(block['timestamp'], "%Y-%m-%dT%H:%M:%S"),
        'amount': float(convert['amount'].split()[0]),
        'type': convert['amount'].split()[1]
    })
    queue_update_account(op['owner'])
    db.convert.update_one({'_id': _id}, {"$set": convert}, upsert=True)

def save_transfer(op, block, blockid):
    transfer = op.copy()
    _id = f"{blockid}/{op['from']}/{op['to']}"
    transfer.update({
        '_id': _id,
        '_ts': datetime.strptime(block['timestamp'], "%Y-%m-%dT%H:%M:%S"),
        'amount': float(transfer['amount'].split()[0]),
        'type': transfer['amount'].split()[1]
    })
    db.transfer.update_one({'_id': _id}, {"$set": transfer}, upsert=True)
    queue_update_account(op['from'])
    if op['from'] != op['to']:
        queue_update_account(op['to'])

def save_curation_reward(op, block, blockid):
    reward = op.copy()
    _id = f"{blockid}/{op['curator']}/{op['comment_author']}/{op['comment_permlink']}"
    reward.update({
        '_id': _id,
        '_ts': datetime.strptime(block['timestamp'], "%Y-%m-%dT%H:%M:%S"),
        'reward': float(reward['reward'].split()[0])
    })
    db.curation_reward.update_one({'_id': _id}, {"$set": reward}, upsert=True)
    queue_update_account(op['curator'])

def save_author_reward(op, block, blockid):
    reward = op.copy()
    comment_id = f"{op['author']}/{op['permlink']}"
    update_comment(op['author'], op['permlink'])
    comment = db.comment.find_one({'_id': comment_id})
    if comment and isinstance(comment, dict) and 'json_metadata' in comment and isinstance(comment['json_metadata'], dict) and 'app' in comment['json_metadata']:
        if not isinstance(comment['json_metadata']['app'], str):
            comment['json_metadata']['app'] = str(comment['json_metadata']['app'])
        parts = comment['json_metadata']['app'].split('/')
        if len(parts) > 1:
            reward.update({
                'app_name': parts[0],
                'app_version': parts[1]
            })
    _id = f"{blockid}/{op['author']}/{op['permlink']}"
    reward.update({
        '_id': _id,
        '_ts': datetime.strptime(block['timestamp'], "%Y-%m-%dT%H:%M:%S")
    })
    for key in ['sbd_payout', 'steem_payout', 'vesting_payout']:
        reward[key] = float(reward[key].split()[0])
    db.author_reward.update_one({'_id': _id}, {"$set": reward}, upsert=True)
    db.comment.update_one({'_id': comment_id}, {"$set": {'reward': reward}})
    queue_update_account(op['author'])

def save_vesting_deposit(op, block, blockid):
    vesting = op.copy()
    _id = f"{blockid}/{op['from']}/{op['to']}"
    vesting.update({
        '_id': _id,
        '_ts': datetime.strptime(block['timestamp'], "%Y-%m-%dT%H:%M:%S"),
        'amount': float(vesting['amount'].split()[0])
    })
    db.vesting_deposit.update_one({'_id': _id}, {"$set": vesting}, upsert=True)
    queue_update_account(op['from'])
    if op['from'] != op['to']:
        queue_update_account(op['to'])

def save_vesting_withdraw(op, block, blockid):
    vesting = op.copy()
    _id = f"{blockid}/{op['from_account']}/{op['to_account']}"
    vesting.update({
        '_id': _id,
        '_ts': datetime.strptime(block['timestamp'], "%Y-%m-%dT%H:%M:%S")
    })
    for key in ['deposited', 'withdrawn']:
        vesting[key] = float(vesting[key].split()[0])
    db.vesting_withdraw.update_one({'_id': _id}, {"$set": vesting}, upsert=True)
    queue_update_account(op['from_account'])
    if op['from_account'] != op['to_account']:
        queue_update_account(op['to_account'])

def save_custom_json(op, block, blockid):
    try:
        data = json.loads(op['json'])
        if isinstance(data, list) and data:
            if data[0] == 'reblog':
                save_reblog(data, op, block, blockid)
            elif data[0] == 'follow':
                save_follow(data, op, block, blockid)
    except Exception as e:
        print(f"{log_tag}Error processing custom_json: {e}")

def save_feed_publish(op, block, blockid):
    doc = op.copy()
    _id = f"{blockid}|{doc['publisher']}"
    query = {'_id': _id}
    doc.update({
        '_id': _id,
        '_block': blockid,
        '_ts': datetime.strptime(block['timestamp'], "%Y-%m-%dT%H:%M:%S"),
    })
    for key in ['base', 'quote']:
        doc['exchange_rate'][key] = float(doc['exchange_rate'][key].split()[0])
    db.feed_publish.update_one(query, {"$set": doc}, upsert=True)

def save_follow(data, op, block, blockid):
    doc = data[1].copy()
    if 'following' in doc and 'follower' in doc:
        query = {
            '_block': blockid,
            'follower': doc['follower'],
            'following': doc['following']
        }
        doc.update({
            '_block': blockid,
            '_ts': datetime.strptime(block['timestamp'], "%Y-%m-%dT%H:%M:%S"),
        })
        db.follow.update_one(query, {"$set": doc}, upsert=True)
        queue_update_account(doc['follower'])
        if doc['follower'] != doc['following']:
            queue_update_account(doc['following'])

def save_benefactor_reward(op, block, blockid):
    doc = op.copy()
    query = {
        '_block': blockid,
        'benefactor': doc['benefactor'],
        'permlink': doc['permlink'],
        'author': doc['author']
    }
    doc.update({
        '_block': blockid,
        '_ts': datetime.strptime(block['timestamp'], "%Y-%m-%dT%H:%M:%S"),
        'reward': float(doc['vesting_payout'].split()[0])
    })
    db.benefactor_reward.update_one(query, {"$set": doc}, upsert=True)

def save_reblog(data, op, block, blockid):
    if len(data) > 1:
        doc = data[1].copy()
        if 'permlink' in doc and 'account' in doc:
            query = {
                '_block': blockid,
                'permlink': doc['permlink'],
                'account': doc['account']
            }
            doc.update({
                '_block': blockid,
                '_ts': datetime.strptime(block['timestamp'], "%Y-%m-%dT%H:%M:%S"),
            })
            db.reblog.update_one(query, {"$set": doc}, upsert=True)

def save_block(block, blockid):
    doc = block.copy()
    doc.update({
        '_id': blockid,
        '_ts': datetime.strptime(doc['timestamp'], "%Y-%m-%dT%H:%M:%S"),
    })
    db.block_30d.update_one({'_id': blockid}, {"$set": doc}, upsert=True)

def save_pow(op, block, blockid):
    _id = str(blockid)
    if isinstance(op['work'], list):
        _id += '-' + op['work'][1]['input']['worker_account']
    else:
        _id += '-' + op['worker_account']
    doc = op.copy()
    doc.update({
        '_id': _id,
        '_ts': datetime.strptime(block['timestamp'], "%Y-%m-%dT%H:%M:%S"),
        'block': blockid,
    })
    db.pow.update_one({'_id': _id}, {"$set": doc}, upsert=True)

def save_vote(op, block, blockid):
    vote = op.copy()
    _id = f"{blockid}/{op['voter']}/{op['author']}/{op['permlink']}"
    vote.update({
        '_id': _id,
        '_ts': datetime.strptime(block['timestamp'], "%Y-%m-%dT%H:%M:%S")
    })
    db.vote.update_one({'_id': _id}, {"$set": vote}, upsert=True)

def save_witness_vote(op, block, blockid):
    witness_vote = op.copy()
    query = {
        '_ts': datetime.strptime(block['timestamp'], "%Y-%m-%dT%H:%M:%S"),
        'account': witness_vote['account'],
        'witness': witness_vote['witness']
    }
    witness_vote.update({
        '_ts': datetime.strptime(block['timestamp'], "%Y-%m-%dT%H:%M:%S")
    })
    db.witness_vote.update_one(query, {"$set": witness_vote}, upsert=True)
    queue_update_account(witness_vote['account'])
    if witness_vote['account'] != witness_vote['witness']:
        queue_update_account(witness_vote['witness'])

def update_comment(author, permlink, op=None, block=None, blockid=None):
    _id = f"{author}/{permlink}"
    try:
        if _id == "xeroc/re-piston-20160818t080811":
            return

        if op and 'body' in op and op['body'].startswith("@@ "):
            diffid = f"{blockid}/{op['author']}/{op['permlink']}"
            diff = op.copy()
            query = {'_id': diffid}
            diff.update({
                '_id': diffid,
                '_ts': datetime.strptime(block['timestamp'], "%Y-%m-%dT%H:%M:%S"),
                'block': int(blockid),
            })
            db.comment_diff.update_one(query, {"$set": diff}, upsert=True)

        comment = rpc.get_content(author, permlink).copy()
        comment.update({'_id': _id})

        # Optimization: Pre-define date format string to reduce repeated parsing
        date_format = "%Y-%m-%dT%H:%M:%S"
        active_votes = []
        for vote in comment['active_votes']:
            vote['rshares'] = float(vote['rshares'])
            vote['weight'] = float(vote['weight'])
            vote['time'] = datetime.strptime(vote['time'], date_format)
            active_votes.append(vote)
        comment['active_votes'] = active_votes

        # Optimization: Batch type conversion for float fields
        float_keys = ['author_reputation', 'net_rshares', 'children_abs_rshares', 'abs_rshares', 'vote_rshares']
        for key in float_keys:
            if key in comment:
                comment[key] = float(comment[key])
        
        # Optimization: Batch process fields requiring split operation
        split_float_keys = ['total_pending_payout_value', 'pending_payout_value', 
                           'max_accepted_payout', 'total_payout_value', 'curator_payout_value']
        for key in split_float_keys:
            if key in comment and isinstance(comment[key], str):
                comment[key] = float(comment[key].split()[0])
        
        # Optimization: Batch date parsing
        date_keys = ['active', 'created', 'cashout_time', 'last_payout', 
                    'last_update', 'max_cashout_time']
        for key in date_keys:
            if key in comment and isinstance(comment[key], str):
                comment[key] = datetime.strptime(comment[key], date_format)
        for key in ['json_metadata']:
            try:
                comment[key] = json.loads(comment[key])
            except ValueError:
                comment[key] = comment[key]

        comment['scanned'] = datetime.now()
        results = db.comment.update_one({'_id': _id}, {"$set": comment}, upsert=True)

        if comment['depth'] > 0 and not results.matched_count and comment['url']:
            url = comment['url'].split('#')[0]
            parts = url.split('/')
            original_id = parts[2].replace('@', '') + '/' + parts[3]
            db.comment.update_one(
                {'_id': original_id},
                {'$set': {
                    'last_reply': comment['created'],
                    'last_reply_by': comment['author']
                }}
            )
    except Exception as e:
        print(f"{log_tag}Error updating comment {_id}: {e}")

def update_comment_options(op, block, blockid):
    _id = f"{op['author']}/{op['permlink']}"
    data = {'options': op.copy()}
    db.comment.update_one({'_id': _id}, {"$set": data}, upsert=True)

def queue_update_account(account_name):
    db.account.update_one({'_id': account_name}, {"$set": {'_dirty': True}}, upsert=True)

def update_account(account_name):
    state = rpc.get_accounts([account_name])
    if not state:
        return

    account = state[0]
    account['proxy_witness'] = float(account['proxied_vsf_votes'][0]) / 1000000
    for key in ['reputation', 'to_withdraw']:
        account[key] = float(account[key])
    for key in ['balance', 'sbd_balance', 'sbd_seconds', 'savings_balance', 'savings_sbd_balance', 'vesting_balance', 'vesting_shares', 'vesting_withdraw_rate']:
        account[key] = float(account[key].split()[0])
    for key in ['created','last_account_recovery','last_owner_update','last_post','last_root_post','last_vote_time','next_vesting_withdrawal','savings_sbd_last_interest_payment','savings_sbd_seconds_last_update','sbd_last_interest_payment','sbd_seconds_last_update']:
        account[key] = datetime.strptime(account[key], "%Y-%m-%dT%H:%M:%S")

    account['total_balance'] = account['balance'] + account['savings_balance']
    account['total_sbd_balance'] = account['sbd_balance'] + account['savings_sbd_balance']

    account['scanned'] = datetime.now()
    if '_dirty' in account:
        del account['_dirty']
    db.account.update_one({'_id': account_name}, {"$set": account}, upsert=True)

def update_queue():
    queue_length = 100
    max_date = datetime.now() + timedelta(-3)
    scan_ignore = datetime.now() - timedelta(hours=6)
    
    # Use thread pool for concurrent processing to reduce CPU wait time
    max_workers = min(10, queue_length)  # Limit concurrency to avoid too many connections

    # Process comment queue
    queue = list(db.comment.find({
        'created': {'$gt': max_date},
        'scanned': {'$lt': scan_ignore},
    }).sort('scanned', 1).limit(queue_length))
    
    # Remove count_documents query to reduce database load
    total_comments = len(queue)
    print(f"{log_tag}[Queue] Comments - {total_comments} items")
    
    # Concurrently process comments
    if queue:
        with ThreadPoolExecutor(max_workers=max_workers) as executor:
            futures = [executor.submit(update_comment, item['author'], item['permlink']) 
                      for item in queue]
            completed = 0
            for future in as_completed(futures):
                try:
                    future.result()  # Get result, exceptions will be raised if any
                    completed += 1
                except Exception as e:
                    print(f"{log_tag}Error in update_comment: {e}")
        print(f"{log_tag}[Queue] Comments - Completed {completed}/{total_comments}")

    # Process past payout queue
    queue = list(db.comment.find({
        'cashout_time': {'$lt': datetime.now()},
        'mode': {'$in': ['first_payout', 'second_payout']},
        'depth': 0,
        'pending_payout_value': {'$gt': 0}
    }).limit(queue_length))
    
    total_past_payouts = len(queue)
    print(f"{log_tag}[Queue] Past Payouts - {total_past_payouts} items")
    
    # Concurrently process past payouts
    if queue:
        with ThreadPoolExecutor(max_workers=max_workers) as executor:
            futures = [executor.submit(update_comment, item['author'], item['permlink']) 
                      for item in queue]
            completed = 0
            for future in as_completed(futures):
                try:
                    future.result()
                    completed += 1
                except Exception as e:
                    print(f"{log_tag}Error in update_comment (past payouts): {e}")
        print(f"{log_tag}[Queue] Past Payouts - Completed {completed}/{total_past_payouts}")

    # Process account queue
    queue_length = 20
    queue = list(db.account.find({'_dirty': True}).limit(queue_length))
    total_accounts = len(queue)
    print(f"{log_tag}[Queue] Updating Accounts - {total_accounts} items")
    
    # Concurrently process accounts
    if queue:
        with ThreadPoolExecutor(max_workers=min(5, queue_length)) as executor:
            futures = [executor.submit(update_account, item['_id']) 
                      for item in queue]
            completed = 0
            for future in as_completed(futures):
                try:
                    future.result()
                    completed += 1
                except Exception as e:
                    print(f"{log_tag}Error in update_account: {e}")
        print(f"{log_tag}[Queue] Accounts - Completed {completed}/{total_accounts}")
    
    print(f"{log_tag}[Queue] Done")

def fetch_blocks_in_batch(start_block, end_block):
    requests_data = [
        {
            "jsonrpc": "2.0",
            "method": "condenser_api.get_block",
            "params": [block_num],
            "id": block_num
        }
        for block_num in range(start_block, min(end_block + 1, start_block + 50))
    ]
    try:
        response = requests.post(steemd_url, json=requests_data)
        response.raise_for_status()
        return response.json()
    except requests.exceptions.RequestException as e:
        print(f"{log_tag}Error fetching blocks: {e}")
        return []

def fetch_block(block_num):
    request_data = {
        "jsonrpc": "2.0",
        "method": "condenser_api.get_block",
        "params": [block_num],
        "id": block_num
    }
    try:
        response = requests.post(steemd_url, json=request_data)
        response.raise_for_status()
        return response.json()
    except requests.exceptions.RequestException as e:
        print(f"{log_tag}Error fetching block {block_num}: {e}")
        return None

def update_props_history(props):
    try:
        print(f"{log_tag}[STEEM] - Update Global Properties")

        for key in ['recent_slots_filled', 'total_reward_shares2']:
            props[key] = float(props[key])
        for key in ['confidential_sbd_supply', 'confidential_supply', 'current_sbd_supply', 'current_supply', 'total_reward_fund_steem', 'total_vesting_fund_steem', 'total_vesting_shares', 'virtual_supply']:
            props[key] = float(props[key].split()[0])
        for key in ['time']:
            props[key] = datetime.strptime(props[key], "%Y-%m-%dT%H:%M:%S")

        props['steem_per_mvests'] = props['total_vesting_fund_steem'] / props['total_vesting_shares'] * 1000000

        db.status.update_one({
            '_id': 'steem_per_mvests'
        }, {
            '$set': {
                '_id': 'steem_per_mvests',
                'value': props['steem_per_mvests']
            }
        }, upsert=True)

        db.status.update_one({
            '_id': 'props'
        }, {
            '$set': {
                '_id': 'props',
                'props': props
            }
        }, upsert=True)

        db.props_history.insert_one(props)
        print(f"{log_tag}[STEEM] - Successfully updated props history")
    except Exception as e:
        error_message = f"{log_tag}Error updating props history: {e}"
        print(error_message)
        logging.error(error_message)
        # Don't exit, let the script continue

def props_history_updater(rpc, block_interval):
    """
    Background thread that updates props history every block_interval seconds
    This ensures props history is updated regularly regardless of how long block processing takes
    """
    while True:
        try:
            props = rpc.get_dynamic_global_properties()
            update_props_history(props)
        except Exception as e:
            error_message = f"{log_tag}Error in props history updater: {e}"
            print(error_message)
            logging.error(error_message)
        time.sleep(block_interval)

if __name__ == '__main__':
    print(f"{log_tag}[STEEM] - Starting SteemDB Sync Service")
    sys.stdout.flush()
    # block_interval is loaded from config.json (default: 60 seconds)
    
    # Start background thread for props history updates
    # This ensures props history is updated every block_interval regardless of block processing time
    props_thread = threading.Thread(target=props_history_updater, args=(rpc, block_interval), daemon=True)
    props_thread.start()
    print(f"{log_tag}[STEEM] - Started props history updater thread (interval: {block_interval}s)")

    while True:
        try:
            global_process_start_time = time.perf_counter()
            update_queue()
            
            try:
                props = rpc.get_dynamic_global_properties()
                block_number = props['last_irreversible_block_num']
            except Exception as e:
                error_message = f"{log_tag}Error fetching props: {e}"
                print(error_message)
                logging.error(error_message)
                # Continue to next iteration instead of crashing
                # Skip block processing if props fetch fails
                block_number = last_block

            while (block_number - last_block) > 0:
                #total_start_time = time.perf_counter()
                end_block = min(last_block + batch_size, block_number)
                if end_block <= last_block:
                    break
                try:
                    blocks = rpc.get_blocks_range(last_block + 1, end_block)
                except Exception as e:
                    error_message = f"{log_tag}Error fetching blocks range: {e}"
                    print(error_message)
                    logging.error(error_message)
                    break  # Break out of block processing loop, but continue main loop
                
                for block in blocks:
                    try:
                        last_block = block['block_num']
                        print(f"{log_tag}[STEEM] - Starting Block #{last_block}")
                        sys.stdout.flush()

                        process_block(block, last_block)
                        db.status.update_one({'_id': 'height'}, {"$set": {'value': last_block}}, upsert=True)
                        print(f"{log_tag}[STEEM] - Processed up to Block #{last_block}")
                        sys.stdout.flush()
                    except Exception as e:
                        error_message = f"{log_tag}Error processing block {block.get('block_num', 'unknown')}: {e}"
                        print(error_message)
                        logging.error(error_message)
                        # Continue processing next block instead of crashing
                        continue

                #total_time = time.perf_counter() - total_start_time
                #print(f"{log_tag}[TEST Time] Batch Process Time: [{total_time}]")

            sys.stdout.flush()
            print(f"{log_tag}[TEST Time] Global Process Time [{time.perf_counter() - global_process_start_time}]")
        except Exception as e:
            error_message = f"{log_tag}Error in main loop: {e}"
            print(error_message)
            logging.error(error_message)
            # Continue the main loop instead of crashing

        time.sleep(block_interval)
