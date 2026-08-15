import React, { useEffect } from 'react';
import { Activity, Blocks, Users, Shield, TrendingUp, Clock } from 'lucide-react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../components/ui/Card';
import { Badge } from '../components/ui/Badge';
import { NetworkPerformance } from '../components/dashboard/NetworkPerformance';
import { RewardPool } from '../components/dashboard/RewardPool';
import { GlobalProperties } from '../components/dashboard/GlobalProperties';
import { useBlockchainStore, useWebSocketStore } from '../store';
import type { BlockSummary } from '../types';
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

interface RecentBlockProps {
  block: BlockSummary;
}

function RecentBlock({ block }: RecentBlockProps) {
  return (
    <div className="flex items-center justify-between p-3 border rounded-lg">
      <div className="flex items-center space-x-3">
        <div className="flex items-center justify-center w-8 h-8 bg-primary/10 rounded-full">
          <Blocks className="h-4 w-4 text-primary" />
        </div>
        <div>
          <div className="font-medium">Block #{formatNumber(block.number)}</div>
          <div className="text-sm text-muted-foreground">
            by @{block.witness}
          </div>
        </div>
      </div>
      <div className="text-right">
        <div className="text-sm font-medium">
          {block.transactions ?? block.transaction_count ?? 0} txs,{' '}
          {block.operations ?? block.operation_count ?? 0} ops
        </div>
        <div className="text-xs text-muted-foreground">
          {formatTimeAgo(block.timestamp)}
        </div>
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

  // Fetch dashboard data from REST API as fallback
  const fetchDashboardData = async () => {
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
  };

  useEffect(() => {
    // Initial fetch from REST API
    fetchDashboardData();

    // Subscribe to real-time data via WebSocket
    const unsubscribeProps = wsClient.on('props', (message) => {
      setProps(message.data);
    });

    const unsubscribeBlocks = wsClient.on('blocks', (message) => {
      useBlockchainStore.getState().addBlock(message.data);
    });

    const unsubscribeState = wsClient.on('state', (message) => {
      // Map the WS state payload (accounts/comments/witnesses/last_block) onto
      // the GlobalStats shape used by the store.
      const d = message.data;
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

    // Fallback: If WebSocket is disconnected and no data, fetch from REST API every 10 seconds
    const fallbackInterval = setInterval(() => {
      if (wsState === 'disconnected' && (!props || !stats || latestBlocks.length === 0)) {
        fetchDashboardData();
      }
    }, 10000);

    return () => {
      unsubscribeProps();
      unsubscribeBlocks();
      unsubscribeState();
      wsClient.unsubscribe('props');
      wsClient.unsubscribe('blocks');
      wsClient.unsubscribe('state');
      clearInterval(fallbackInterval);
    };
  }, [wsState]);

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

      {/* Recent Blocks */}
      <Card>
        <CardHeader>
          <CardTitle>Recent Blocks</CardTitle>
          <CardDescription>
            Latest blocks produced on the Steem blockchain
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="space-y-3">
            {latestBlocks.length > 0 ? (
              latestBlocks.slice(0, 5).map((block) => (
                <RecentBlock key={block.number} block={block} />
              ))
            ) : (
              <div className="text-center py-6 text-muted-foreground">
                <Blocks className="h-8 w-8 mx-auto mb-2 opacity-50" />
                <p>No recent blocks available</p>
                <p className="text-sm">Connect to see live data</p>
              </div>
            )}
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
