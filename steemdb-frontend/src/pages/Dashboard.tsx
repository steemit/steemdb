import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Activity, Blocks, Users, Shield, TrendingUp, Clock } from 'lucide-react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../components/ui/Card';
import { Badge } from '../components/ui/Badge';
import { NetworkPerformance } from '../components/dashboard/NetworkPerformance';
import { RewardPool } from '../components/dashboard/RewardPool';
import { GlobalProperties } from '../components/dashboard/GlobalProperties';
import { useBlockchainStore, useWebSocketStore } from '../store';
import type { BlockData, BlockchainProps, OperationData, StateData } from '../types';
import { formatNumber, formatTimeAgo, formatCurrency } from '../lib/utils';
import { wsClient } from '../lib/websocket';
import { getDashboard } from '../lib/api';

interface StatCardProps {
  title: string;
  value: string | number;
  description?: string;
  icon: React.ReactNode;
  trend?: {
    value: number;
    isPositive: boolean;
  };
}

function StatCard({ title, value, description, icon, trend }: StatCardProps) {
  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
        <CardTitle className="text-sm font-medium">{title}</CardTitle>
        <div className="text-muted-foreground">{icon}</div>
      </CardHeader>
      <CardContent>
        <div className="text-2xl font-bold">{value}</div>
        {description && (
          <p className="text-xs text-muted-foreground">{description}</p>
        )}
        {trend && (
          <div className="flex items-center pt-1">
            <TrendingUp className={`h-3 w-3 mr-1 ${trend.isPositive ? 'text-green-600' : 'text-red-600'}`} />
            <span className={`text-xs ${trend.isPositive ? 'text-green-600' : 'text-red-600'}`}>
              {trend.isPositive ? '+' : ''}{trend.value}%
            </span>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

interface FeedItem {
  id: string;
  type: 'block' | 'operation';
  timestamp: Date | string;
  data: BlockData | OperationData;
}

function FeedItemRow({ item }: { item: FeedItem }) {
  if (item.type === 'block') {
    const block = item.data as BlockData;
    return (
      <div className="flex items-start space-x-4 p-4 border rounded-lg hover:bg-accent/50">
        <div className="flex-shrink-0">
          <div className="w-10 h-10 rounded-full bg-primary/10 flex items-center justify-center">
            <Blocks className="h-5 w-5 text-primary" />
          </div>
        </div>
        <div className="flex-1 min-w-0">
          <div className="flex items-center justify-between">
            <div>
              <div className="font-medium">Block #{formatNumber(block.number)}</div>
              <div className="text-sm text-muted-foreground">
                by @{block.witness}
              </div>
            </div>
            <div className="text-xs text-muted-foreground">
              {formatTimeAgo(item.timestamp)}
            </div>
          </div>
          <div className="mt-2 flex items-center space-x-4 text-sm text-muted-foreground">
            <span>{block.transactions || 0} transactions</span>
            <span>{block.operations || 0} operations</span>
          </div>
        </div>
      </div>
    );
  }

  const op = item.data as OperationData;
  return (
    <div className="flex items-start space-x-4 p-4 border rounded-lg hover:bg-accent/50">
      <div className="flex-shrink-0">
        <div className="w-10 h-10 rounded-full bg-blue-500/10 flex items-center justify-center">
          <Activity className="h-5 w-5 text-blue-500" />
        </div>
      </div>
      <div className="flex-1 min-w-0">
        <div className="flex items-center justify-between">
          <div>
            <div className="font-medium capitalize">{op.type || 'Operation'}</div>
            {op.accounts && op.accounts.length > 0 && (
              <div className="text-sm text-muted-foreground">
                {op.accounts.map((acc: string) => `@${acc}`).join(', ')}
              </div>
            )}
          </div>
          <div className="text-xs text-muted-foreground">
            {formatTimeAgo(item.timestamp)}
          </div>
        </div>
        {op.block && (
          <div className="mt-2 text-sm text-muted-foreground">
            Block #{formatNumber(op.block)}
          </div>
        )}
      </div>
    </div>
  );
}

export function Dashboard() {
  const {
    props,
    stats,
    latestBlocks,
    networkPerformance,
    rewardPool,
    setProps,
    setStats,
    setLatestBlocks,
    setNetworkPerformance,
    setRewardPool
  } = useBlockchainStore();
  const { state: wsState } = useWebSocketStore();
  // Items received live over the WebSocket while this page is mounted.
  const [liveItems, setLiveItems] = useState<FeedItem[]>([]);

  // The activity stream is the live items followed by the blocks already in
  // the store (REST preload / WS replay), deduplicated by id — derived at
  // render time so the stream is populated even before the first live event.
  const feedItems = useMemo(() => {
    const items = [...liveItems];
    const existing = new Set(liveItems.map((item) => item.id));
    for (const block of latestBlocks) {
      const id = `block-${block.number}`;
      if (existing.has(id)) continue;
      items.push({
        id,
        type: 'block',
        timestamp: block.timestamp,
        data: {
          number: block.number,
          timestamp: block.timestamp,
          witness: block.witness,
          transactions: block.transactions ?? block.transaction_count ?? 0,
          operations: block.operations ?? block.operation_count ?? 0,
        },
      });
    }
    return items.slice(0, 100);
  }, [liveItems, latestBlocks]);

  // Fetch dashboard data from REST API as fallback
  const fetchDashboardData = useCallback(async () => {
    try {
      const response = await getDashboard();
      if (response.success && response.data) {
        setProps(response.data.props);
        setStats(response.data.stats);
        setLatestBlocks(response.data.latest_blocks);
        if (response.data.network_performance) {
          setNetworkPerformance(response.data.network_performance);
        }
        if (response.data.reward_pool) {
          setRewardPool(response.data.reward_pool);
        }
      }
    } catch (error) {
      console.error('Failed to fetch dashboard data:', error);
    }
  }, [setProps, setStats, setLatestBlocks, setNetworkPerformance, setRewardPool]);

  useEffect(() => {
    // Initial fetch from REST API
    fetchDashboardData();

    // Subscribe to real-time data via WebSocket
    const unsubscribeProps = wsClient.on('props', (message) => {
      setProps(message.data as BlockchainProps);
    });

    const unsubscribeBlocks = wsClient.on('blocks', (message) => {
      const block = message.data as BlockData;
      useBlockchainStore.getState().addBlock(block);
      setLiveItems((prev) => {
        // The server replays recent blocks on (re)connect; skip items the
        // stream already shows.
        if (prev.some((item) => item.id === `block-${block.number}`)) {
          return prev;
        }
        return [
          {
            id: `block-${block.number}`,
            type: 'block' as const,
            timestamp: new Date(),
            data: block,
          },
          ...prev,
        ].slice(0, 100);
      });
    });

    const unsubscribeOps = wsClient.on('operation', (message) => {
      const op = message.data as OperationData;
      setLiveItems((prev) => [
        {
          id: `op-${op.block}-${op.type}-${Date.now()}-${Math.random()}`,
          type: 'operation' as const,
          timestamp: new Date(),
          data: op,
        },
        ...prev.slice(0, 99), // Keep last 100 items
      ]);
    });

    const unsubscribeState = wsClient.on('state', (message) => {
      // Map the WS state payload (accounts/comments/witnesses/last_block) onto
      // the GlobalStats shape used by the store.
      const d = message.data as StateData;
      setStats({
        accounts: d.accounts,
        comments: d.comments,
        witnesses: d.witnesses,
        blocks: d.last_block,
        last_block: d.last_block,
        last_update: d.last_update,
      });
    });

    // Subscribe to channels
    wsClient.subscribe('props');
    wsClient.subscribe('blocks');
    wsClient.subscribe('state');
    wsClient.subscribe('operation');

    // Fallback: If WebSocket is disconnected and no data, fetch from REST API
    // every 10 seconds. Reads the live store state to avoid stale closures.
    const fallbackInterval = setInterval(() => {
      const { state } = useWebSocketStore.getState();
      const { props: p, stats: s, latestBlocks: blocks } = useBlockchainStore.getState();
      if (state === 'disconnected' && (!p || !s || blocks.length === 0)) {
        fetchDashboardData();
      }
    }, 10000);

    return () => {
      unsubscribeProps();
      unsubscribeBlocks();
      unsubscribeOps();
      unsubscribeState();
      wsClient.unsubscribe('props');
      wsClient.unsubscribe('blocks');
      wsClient.unsubscribe('state');
      wsClient.unsubscribe('operation');
      clearInterval(fallbackInterval);
    };
  }, [wsState, fetchDashboardData, setProps, setStats]);

  const getConnectionStatus = () => {
    switch (wsState) {
      case 'connected':
        return <Badge variant="success">Live</Badge>;
      case 'connecting':
        return <Badge variant="warning">Connecting</Badge>;
      case 'disconnected':
        return <Badge variant="destructive">Offline</Badge>;
      default:
        return <Badge variant="outline">Unknown</Badge>;
    }
  };

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">Dashboard</h1>
          <p className="text-muted-foreground">
            Real-time overview of the Steem blockchain
          </p>
        </div>
        <div className="flex items-center space-x-2">
          <Clock className="h-4 w-4 text-muted-foreground" />
          <span className="text-sm text-muted-foreground">
            {new Date().toLocaleTimeString()}
          </span>
          {getConnectionStatus()}
        </div>
      </div>

      {/* Stats Grid */}
      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
        <StatCard
          title="Current Block"
          value={props ? formatNumber(props.head_block_number) : '-'}
          description="Latest block height"
          icon={<Blocks className="h-4 w-4" />}
        />
        <StatCard
          title="Current Witness"
          value={props ? `@${props.current_witness}` : '-'}
          description="Block producer"
          icon={<Shield className="h-4 w-4" />}
        />
        <StatCard
          title="Total Accounts"
          value={stats ? formatNumber(stats.accounts) : '-'}
          description="Registered users"
          icon={<Users className="h-4 w-4" />}
        />
        <StatCard
          title="Virtual Supply"
          value={props ? formatCurrency(props.virtual_supply) : '-'}
          description="Total STEEM supply"
          icon={<Activity className="h-4 w-4" />}
        />
      </div>

      {/* Additional Stats */}
      <div className="grid gap-4 md:grid-cols-3">
        <Card>
          <CardHeader>
            <CardTitle className="text-sm font-medium">SBD Supply</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">
              {props ? formatCurrency(props.current_sbd_supply, 'SBD') : '-'}
            </div>
            <p className="text-xs text-muted-foreground">
              Current SBD in circulation
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-sm font-medium">Vesting Fund</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">
              {props ? formatCurrency(props.total_vesting_fund_steem) : '-'}
            </div>
            <p className="text-xs text-muted-foreground">
              Total vested STEEM
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-sm font-medium">Reward Fund</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">
              {props ? formatCurrency(props.total_reward_fund_steem) : '-'}
            </div>
            <p className="text-xs text-muted-foreground">
              Available for rewards
            </p>
          </CardContent>
        </Card>
      </div>

      {/* Network Performance, Reward Pool, and Global Properties */}
      <div className="grid gap-6 md:grid-cols-2 lg:grid-cols-3">
        <NetworkPerformance data={networkPerformance || undefined} />
        <RewardPool data={rewardPool || undefined} />
        <GlobalProperties data={props || undefined} />
      </div>

      {/* Activity Stream (merged from the former Live Feed page) */}
      <Card>
        <CardHeader>
          <CardTitle>Activity Stream</CardTitle>
          <CardDescription>
            {wsState === 'connected'
              ? 'Real-time blocks and operations from the blockchain'
              : 'Connect to WebSocket to see live updates'}
          </CardDescription>
        </CardHeader>
        <CardContent>
          {wsState !== 'connected' && feedItems.length === 0 ? (
            <div className="text-center py-12">
              <Activity className="h-12 w-12 mx-auto mb-4 text-muted-foreground opacity-50" />
              <p className="text-muted-foreground">WebSocket disconnected</p>
              <p className="text-sm text-muted-foreground mt-2">
                Connect to see live blockchain activity
              </p>
            </div>
          ) : feedItems.length === 0 ? (
            <div className="text-center py-12">
              <Activity className="h-12 w-12 mx-auto mb-4 text-muted-foreground opacity-50" />
              <p className="text-muted-foreground">Waiting for activity...</p>
            </div>
          ) : (
            <div className="space-y-3 max-h-[600px] overflow-y-auto">
              {feedItems.map((item) => (
                <FeedItemRow key={item.id} item={item} />
              ))}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
