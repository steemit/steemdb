import logging
import json
import requests
from datetime import datetime, timedelta
from steem import Steem
from pymongo import MongoClient, UpdateOne
from pprint import pprint
from tenacity import retry, stop_after_attempt, wait_exponential, retry_if_exception_type
import re  # Importing re for regular expressions
from multiprocessing import Pool
from apscheduler.schedulers.background import BackgroundScheduler

# Configure logging
logging.basicConfig(level=logging.INFO, format='%(asctime)s - %(name)s - %(levelname)s - %(message)s')
logger = logging.getLogger(__name__)

log_tag = '[History] '

# Load configuration from config.json
with open('config.json') as config_file:
    config = json.load(config_file)

steemd_url = config.get('STEEMD_URL', 'https://api.steemit.com')
mongodb_url = config.get('MONGODB')
if not mongodb_url:
    logger.error(log_tag + 'NEED MONGODB')
    exit()

fullnodes = [steemd_url]
rpc = Steem(fullnodes)
mongo = MongoClient(mongodb_url)
db = mongo.steemdb

mvest_per_account = {}

def steem_batch_request(url, batch_data):
    headers = {
        'Content-Type': 'application/json',
    }
    logger.info("Requesting: %s", json.dumps(batch_data))
    response = requests.post(url, headers=headers, data=json.dumps(batch_data))
    logger.info("Response: %s", response.text)
    return response.json()

def load_accounts():
    logger.info(log_tag + "[STEEM] - Loading mvest per account")
    for account in db.account.find():
        if "name" in account.keys():
            mvest_per_account.update({account['name']: account['vesting_shares']})

def update_fund_history():
    logger.info(log_tag + "[STEEM] - Update Fund History")

    fund = rpc.get_reward_fund('post')
    for key in ['recent_claims', 'content_constant']:
        fund[key] = float(fund[key])
    for key in ['reward_balance']:
        fund[key] = float(fund[key].split()[0])
    for key in ['last_update']:
        fund[key] = datetime.strptime(fund[key], "%Y-%m-%dT%H:%M:%S")

    db.funds_history.insert_one(fund)

def update_props_history():
    logger.info(log_tag + "[STEEM] - Update Global Properties")

    props = rpc.get_dynamic_global_properties()

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

def update_tx_history():
    logger.info(log_tag + "[STEEM] - Update Transaction History")
    now = datetime.now().date()

    today = datetime.combine(now, datetime.min.time())
    yesterday = today - timedelta(1)

    query = {
        '_ts': {
            '$gte': today,
            '$lte': today + timedelta(1)
        }
    }
    count = db.block_30d.count_documents(query)

    logger.info(log_tag + str(count))
    logger.info(log_tag + str(now))
    logger.info(log_tag + str(today))
    logger.info(log_tag + str(yesterday))

@retry(wait=wait_exponential(multiplier=1, min=1, max=10), stop=stop_after_attempt(5), retry=retry_if_exception_type(Exception))
def get_batch_account_details(accounts):
    batch_data = [{"jsonrpc": "2.0", "method": "condenser_api.get_accounts", "params": [accounts], "id": i+1} for i, account in enumerate(accounts)]
    response = steem_batch_request(steemd_url, batch_data)
    return [res['result'][0] for res in response]

def process_account_details(account):
    account_data = collections.OrderedDict(sorted(account.items()))
    account_data['proxy_witness'] = sum(float(i) for i in account_data['proxied_vsf_votes']) / 1000000
    for key in ['reputation', 'to_withdraw']:
        account_data[key] = float(account_data[key])
    for key in ['balance', 'sbd_balance', 'sbd_seconds', 'savings_balance', 'savings_sbd_balance', 'vesting_balance', 'vesting_shares', 'vesting_withdraw_rate']:
        account_data[key] = float(account_data[key].split()[0])
    for key in ['created','last_account_recovery','last_account_update','last_owner_update','last_post','last_root_post','last_vote_time','next_vesting_withdrawal','savings_sbd_last_interest_payment','savings_sbd_seconds_last_update','sbd_last_interest_payment','sbd_seconds_last_update']:
        account_data[key] = datetime.strptime(account_data[key], "%Y-%m-%dT%H:%M:%S")
    account_data['total_balance'] = account_data['balance'] + account_data['savings_balance']
    account_data['total_sbd_balance'] = account_data['sbd_balance'] + account_data['savings_sbd_balance']
    account_data['scanned'] = datetime.now()
    return account_data

def update_history():
    update_fund_history()
    update_props_history()

    users = rpc.lookup_accounts(-1, 1000)
    more = True
    while more:
        newUsers = rpc.lookup_accounts(users[-1], 1000)
        if len(newUsers) < 1000:
            more = False
        users = users + newUsers

    batch_size = 50
    for i in range(0, len(users), batch_size):
        batch_users = users[i:i+batch_size]
        account_details = get_batch_account_details(batch_users)

        operations = []
        for account in account_details:
            account_data = process_account_details(account)
            operations.append(UpdateOne({'_id': account_data['name']}, {'$set': account_data}, upsert=True))
            
            wanted_keys = ['name', 'proxy_witness', 'activity_shares', 'average_bandwidth', 'average_market_bandwidth', 'savings_balance', 'balance', 'comment_count', 'curation_rewards', 'lifetime_bandwidth', 'lifetime_vote_count', 'next_vesting_withdrawal', 'reputation', 'post_bandwidth', 'post_count', 'posting_rewards', 'sbd_balance', 'savings_sbd_balance', 'sbd_last_interest_payment', 'sbd_seconds', 'sbd_seconds_last_update', 'to_withdraw', 'vesting_balance', 'vesting_shares', 'vesting_withdraw_rate', 'voting_power', 'withdraw_routes', 'withdrawn', 'witnesses_voted_for']
            snapshot = dict((k, account_data[k]) for k in wanted_keys if k in account_data)
            snapshot.update({
                'account': account_data['name'],
                'date': datetime.combine(datetime.now().date(), datetime.min.time()),
            })
            operations.append(UpdateOne({'account': account_data['name'], 'date': datetime.combine(datetime.now().date(), datetime.min.time())}, {'$set': snapshot}, upsert=True))

        db.account.bulk_write(operations)
        db.account_history.bulk_write(operations)

    logger.info(log_tag + "history update finish")

def update_stats():
    logger.info(log_tag + "updating stats")
    results = db.block_30d.aggregate([
        {
            '$sort': {
                '_id': -1
            }
        },
        {
            '$limit': 28800 * 1
        },
        {
            '$unwind': '$transactions'
        },
        {
            '$group': {
                '_id': '24h',
                'tx': {
                    '$sum': 1
                }
            }
        }
    ])
    data = list(results)[0]['tx']
    db.status.update_one({'_id': 'transactions-24h'}, {'$set': {'data': data}}, upsert=True)
    now = datetime.now().date()
    today = datetime.combine(now, datetime.min.time())
    db.tx_history.update_one({
        'timeframe': '24h',
        'date': today
    }, {'$set': {'data': data}}, upsert=True)

    results = db.block_30d.aggregate([
        {
            '$sort': {
                '_id': -1
            }
        },
        {
            '$limit': 1200 * 1
        },
        {
            '$unwind': '$transactions'
        },
        {
            '$group': {
                '_id': '1h',
                'tx': {
                    '$sum': 1
                }
            }
        }
    ])
    db.status.update_one({'_id': 'transactions-1h'}, {'$set': {'data': list(results)[0]['tx']}}, upsert=True)

    results = db.block_30d.aggregate([
        {
            '$sort': {
                '_id': -1
            }
        },
        {
            '$limit': 28800 * 1
        },
        {
            '$unwind': '$transactions'
        },
        {
            '$group': {
                '_id': '24h',
                'tx': {
                    '$sum': {
                        '$size': '$transactions.operations'
                    }
                }
            }
        }
    ])
    data = list(results)[0]['tx']
    db.status.update_one({'_id': 'operations-24h'}, {'$set': {'data': data}}, upsert=True)
    db.op_history.update_one({
        'timeframe': '24h',
        'date': today
    }, {'$set': {'data': data}}, upsert=True)

    results = db.block_30d.aggregate([
        {
            '$sort': {
                '_id': -1
            }
        },
        {
            '$limit': 1200 * 1
        },
        {
            '$unwind': '$transactions'
        },
        {
            '$group': {
                '_id': '1h',
                'tx': {
                    '$sum': {
                        '$size': '$transactions.operations'
                    }
                }
            }
        }
    ])
    db.status.update_one({'_id': 'operations-1h'}, {'$set': {'data': list(results)[0]['tx']}}, upsert=True)

def update_clients():
    try:
        logger.info(log_tag + "updating clients")
        start = datetime.today() - timedelta(days=90)
        end = datetime.today()
        regx = re.compile("([\w-]+\/[\w.]+)", re.IGNORECASE)
        results = db.comment.aggregate([
            {
                '$match': {
                    'created': {
                        '$gte': start,
                        '$lte': end,
                    },
                    'json_metadata.app': {
                        '$type': 'string',
                        '$regex': regx,
                    }
                }
            },
            {
                '$project': {
                    'created': '$created',
                    'parts': {
                        '$split': ['$json_metadata.app', '/']
                    },
                    'reward': {
                        '$add': ['$total_payout_value', '$pending_payout_value', '$total_pending_payout_value']
                    }
                }
            },
            {
                '$group': {
                    '_id': {
                        'client': {'$arrayElemAt': ['$parts', 0]},
                        'doy': {'$dayOfYear': '$created'},
                        'year': {'$year': '$created'},
                        'month': {'$month': '$created'},
                        'day': {'$dayOfMonth': '$created'},
                        'dow': {'$dayOfWeek': '$created'},
                    },
                    'reward': {'$sum': '$reward'},
                    'value': {'$sum': 1}
                }
            },
            {
                '$sort': {
                    '_id.year': 1,
                    '_id.doy': 1,
                    'value': -1,
                }
            },
            {
                '$group': {
                    '_id': {
                        'doy': '$_id.doy',
                        'year': '$_id.year',
                        'month': '$_id.month',
                        'day': '$_id.day',
                        'dow': '$_id.dow',
                    },
                    'clients': {
                        '$push': {
                            'client': '$_id.client',
                            'count': '$value',
                            'reward': '$reward'
                        }
                    },
                    'reward': {'$sum': '$reward'},
                    'total': {'$sum': '$value'}
                }
            },
            {
                '$sort': {
                    '_id.year': -1,
                    '_id.doy': -1
                }
            },
        ])
        logger.info(log_tag + "complete")
        sys.stdout.flush()
        data = list(results)
        db.status.update_one({'_id': 'clients-snapshot'}, {'$set': {'data': data}}, upsert=True)
        now = datetime.now().date()
        today = datetime.combine(now, datetime.min.time())
        db.clients_history.update_one({
            'date': today
        }, {'$set': {'data': data}}, upsert=True)
    except Exception as e:
        logger.error(log_tag + "Error updating clients: %s", str(e))

if __name__ == '__main__':
    logger.info(log_tag + "starting")
    update_clients()
    update_props_history()
    load_accounts()
    update_stats()
    update_history()
    sys.stdout.flush()

    scheduler = BackgroundScheduler()
    scheduler.add_job(update_history, 'interval', hours=24, id='update_history')
    scheduler.add_job(update_clients, 'interval', hours=1, id='update_clients')
    scheduler.add_job(update_stats, 'interval', minutes=5, id='update_stats')
    scheduler.start()
    try:
        while True:
            time.sleep(2)
    except (KeyboardInterrupt, SystemExit):
        scheduler.shutdown()
