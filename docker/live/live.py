from autobahn.twisted.websocket import WebSocketServerFactory, \
    WebSocketServerProtocol, \
    listenWS
from datetime import datetime, timedelta
from steem import Steem
from pprint import pprint
from twisted.internet import reactor
from twisted.python import log
from collections import Counter

import json
import math
import sys
import os
import re
import time
import urllib.request

log_tag = '[Live] '
env_dist = os.environ
steemd_url = env_dist.get('STEEMD_URL')
if steemd_url == None or steemd_url == "":
    steemd_url = 'https://api.steemit.com'

live_port = env_dist.get('LIVE_PORT')
if live_port == None or live_port == "":
    live_port = 8888

fullnodes = [
    #'http://10.40.103.102:8090',
    #'https://api.steemit.com',
    steemd_url,
]
rpc = Steem(fullnodes)

# The legacy steem-python client can fail on get_dynamic_global_properties when a
# node answers with the newer asset format (bad_cast_exception). Talk to the node
# over plain JSON-RPC instead and keep steem-python only for get_block.
PROPS_METHODS = [
    'database_api.get_dynamic_global_properties',
    'condenser_api.get_dynamic_global_properties',
]


def fetch_props():
    last_error = None
    for node in fullnodes:
        for method in PROPS_METHODS:
            try:
                body = json.dumps(
                    {'jsonrpc': '2.0', 'id': 1, 'method': method, 'params': []}
                ).encode('utf8')
                with urllib.request.urlopen(node, data=body, timeout=15) as response:
                    payload = json.loads(response.read())
                if 'result' in payload:
                    return payload['result']
                last_error = payload.get('error', payload)
            except Exception as e:
                last_error = e
    # last resort: the steem-python client
    try:
        return rpc.get_dynamic_global_properties()
    except Exception as e:
        last_error = e
    raise RuntimeError('cannot fetch dynamic global properties: %s' % (last_error,))


def asset_amount(value):
    """Accept legacy "123.456 STEEM" strings, NAI objects and plain numbers."""
    if isinstance(value, dict):
        amount = float(value.get('amount', 0))
        return amount / (10 ** int(value.get('precision', 3)))
    if isinstance(value, (int, float)):
        return float(value)
    return float(str(value).split(" ")[0])


class BroadcastServerProtocol(WebSocketServerProtocol):

    def onOpen(self):
        self.factory.register(self)

    def onMessage(self, payload, isBinary):
        if not isBinary:
            self.factory.subscribe(self, payload.decode('utf8'))

    def connectionLost(self, reason):
        WebSocketServerProtocol.connectionLost(self, reason)
        self.factory.unregister(self)


class BroadcastServerFactory(WebSocketServerFactory):

    """
    Simple broadcast server broadcasting any message it receives to all
    currently connected clients.
    """

    def __init__(self, url):
        WebSocketServerFactory.__init__(self, url)
        self.clients = []
        self.channels = {}
        self.tickcount = 0
        props = None
        for attempt in range(60):
            try:
                props = fetch_props()
                break
            except Exception as e:
                print(log_tag + 'waiting for node: %s' % (e,))
                sys.stdout.flush()
                time.sleep(3)
        if props is None:
            # never start with last_block_processed=0: tick() would try to publish
            # the whole chain history in one go
            raise RuntimeError('could not read dynamic global properties at startup')
        self.last_block = props['head_block_number']
        self.last_block_processed = props['last_irreversible_block_num']
        self.mentions = re.compile(r"([@])(\w+)\b")
        print(log_tag + 'starting at head block %s' % (self.last_block,))
        sys.stdout.flush()
        self.tick()

    def tick(self):
        # any unhandled error here used to kill the reactor.callLater chain for
        # good (last seen 2026-06-16); the feed must survive node hiccups
        try:
            props = fetch_props()
            irreversible = props['last_irreversible_block_num']

            if props['head_block_number'] != self.last_block:
                self.last_block = props['head_block_number']
                # print("new block {}\n".format(self.last_block))
                self.publishProps(props)
                #self.publishState(state)

            # after a long outage do not replay the whole backlog to the clients
            backlog = irreversible - self.last_block_processed
            if backlog > 120:
                print(log_tag + 'skipping backlog of %s blocks' % (backlog,))
                sys.stdout.flush()
                self.last_block_processed = irreversible - 20

            while (irreversible - self.last_block_processed) > 0:
                self.last_block_processed += 1
                # publish operation events to subscribers
                # print("processing block {} [{}/{}/{}]".format(self.last_block_processed, len(self.clients), len(self.channels), sum(len(v) for v in self.channels.values())))
                self.publishBlock(self.last_block_processed)
                # self.publishOps(self.last_block_processed)
        except Exception as e:
            print(log_tag + 'tick error: %s' % (e,))
            sys.stdout.flush()

        reactor.callLater(1, self.tick)

    def publishProps(self, props):
        total_vesting_fund_steem = asset_amount(props['total_vesting_fund_steem'])
        total_vesting_shares = asset_amount(props['total_vesting_shares'])
        props['steem_per_mvests'] = math.floor(total_vesting_fund_steem / total_vesting_shares * 1000000 * 1000) / 1000
        props['reversible_blocks'] = props['head_block_number'] - props['last_irreversible_block_num']
        self.publish("props", "props", props)

    def publishState(self, state):
        print(log_tag + str(state))
        print(log_tag + 'state')
        partial = {
          #'witness_schedule': state['witness_schedule'],
          'feed_price': state['feed_price']
        }
        self.publish("state", "state", partial)

    def publishBlock(self, height):
        block = rpc.get_block(height)
        data = {
            'height': height,
            'accounts': set([]),
            'opCount': 0,
            'opCount': 0,
            'opTypes': [],
            'ts': block['timestamp'],
        }
        if block['transactions']:
            for tx in block['transactions']:
                for op in tx['operations']:
                    data['opCount'] += 1
                    data['opTypes'].append(op[0])
                    for account in self.getRelatedAccounts(op[0], op[1]):
                        data['accounts'].add(account)

        data['opCounts'] = Counter(data['opTypes'])
        data['accounts'] = list(data['accounts'])
        self.publish("blocks", "block", data)

    def publishOps(self, block):
        ops = rpc.get_ops_in_block(block, False)
        for op in ops:
            opType = op['op'][0]
            opData = op['op'][1]
            # notify anyone subscribed to an account channel (e.g. @username)
            for account in self.getRelatedAccounts(opType, opData):
                channel = "@{}".format(account)
                self.publish(channel, opType, op)
            # NYI - notify anyone subscribed to a related event channel (e.g. OnVote, OnComment, etc)
            # for channel in self.getRelatedEvents(opType):
            #     self.publish(event, opType, json.dumps(op))

    # NYI
    # def getRelatedEvents(self, opType):
    #     opTypeEvents = {
    #         'vote': 'OnVote',
    #         'comment': 'OnComment',
    #     }

    # retrieves list of related accounts based on op type
    def getRelatedAccounts(self, opType, opData):
        accounts = set([])
        fieldMap = {
            'account_create':           [],
            'account_update':           [],
            'account_witness_vote':     ['account', 'witness'],
            'author_reward':            ['author'],
            'comment':                  ['author', 'parent_author'],
            'convert':                  [],
            'curation_reward':          ['curator'],
            'custom_json':              [],
            'feed_publish':             [],
            'fill_order':               [],
            'fill_vesting_withdraw':    [],
            'limit_order_cancel':       [],
            'limit_order_create':       [],
            'pow2':                     [],
            'transfer':                 [],
            'transfer_to_vesting':      [],
            'vote':                     ['author', 'voter']
        }
        if opType in fieldMap.keys():
            for field in fieldMap[opType]:
                accounts.add(opData[field])

        # Find mentions of usernames (may return false positives)
        if opType == 'comment':
            matches = self.mentions.findall(opData['body'])
            for match in matches:
                accounts.add(match[1])

        return accounts

    def register(self, client):
#        if client not in self.clients:
#            # print("registered client [{}]".format(client.peer))
#            self.subscribe(client, "blocks")
#            self.subscribe(client, "props")
#            self.subscribe(client, "state")
#            for x in range(1, 11):
#              previous = self.last_block_processed - 10 + x
#              self.publishBlock(previous)
#            self.clients.append(client)
        try:
            self.subscribe(client, "blocks")
            self.subscribe(client, "props")
            self.subscribe(client, "state")
            for x in range(1, 11):
              previous = self.last_block_processed - 10 + x
              self.publishBlock(previous)
            self.clients.append(client)
        except Exception as e:
            print(log_tag + 'error', e)
            pass

    def unregister(self, client):
        if client in self.clients:
            # print("unregistered client [{}]".format(client.peer))
            self.clients.remove(client)

    def broadcast(self, msg):
        # print("broadcasting message '[{}]' ..".format(msg))
        for c in self.clients:
            c.sendMessage(msg.encode('utf8'))

    def subscribe(self, client, channel):
        # print("subscribed client [{}] to channel [{}]".format(client.peer, channel))
        # Create channel if it doesn't exist
        if channel not in self.channels:
            self.channels[channel] = set([])
        # Add client to channel if it isn't already subscribed
        if client not in self.channels[channel]:
            self.channels[channel].add(client)

    def publish(self, channel, opType, opData):
        if channel in self.channels:
#            for c in self.channels[channel]:
#                data = json.dumps({opType: opData})
#                # print("publishing op '{}' [{}] to subscriber [{}] based on channel subscription [{}]".format(opType, data, c.peer, channel))
#                c.sendMessage(data.encode('utf8'))
#
            clients = self.channels[channel].copy()
            for c in clients:
                try:
                    data = json.dumps({opType: opData})
                    # print("publishing op '{}' [{}] to subscriber [{}] based on channel subscription [{}]".format(opType, data, c.peer, channel))
                    c.sendMessage(data.encode('utf8'))
                except Exception as e:
                    print(log_tag + 'error:', e)
                    self.channels[channel].remove(c)




if __name__ == '__main__':

    log.startLogging(sys.stdout)

    ServerFactory = BroadcastServerFactory

    factory = ServerFactory(u"ws://0.0.0.0:%s" % live_port)
    factory.protocol = BroadcastServerProtocol
    listenWS(factory)

    reactor.run()
