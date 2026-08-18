import { useEffect, useState } from 'react';
import { Activity, Blocks } from 'lucide-react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../components/ui/Card';
import { Badge } from '../components/ui/Badge';
import { useBlockchainStore, useWebSocketStore } from '../store';
import { wsClient } from '../lib/websocket';
import { formatNumber, formatTimeAgo } from '../lib/utils';
import type { BlockData, OperationData } from '../types';

interface FeedItem {
  id: string;
  type: 'block' | 'operation';
  timestamp: Date;
  data: BlockData | OperationData;
}

export function LiveFeedPage() {
  const { latestBlocks } = useBlockchainStore();
  const { state: wsState } = useWebSocketStore();
  const [feedItems, setFeedItems] = useState<FeedItem[]>([]);

  useEffect(() => {
    // Subscribe to real-time updates
    const unsubscribeBlocks = wsClient.on('blocks', (message) => {
      const block = message.data as BlockData;
      setFeedItems((prev) => [
        {
          id: `block-${block.number}`,
          type: 'block',
          timestamp: new Date(),
          data: block,
        },
        ...prev.slice(0, 99), // Keep last 100 items
      ]);
    });

    const unsubscribeOps = wsClient.on('operation', (message) => {
      const op = message.data as OperationData;
      setFeedItems((prev) => [
        {
          id: `op-${Date.now()}-${Math.random()}`,
          type: 'operation',
          timestamp: new Date(),
          data: op,
        },
        ...prev.slice(0, 99),
      ]);
    });

    // Subscribe to channels
    wsClient.subscribe('blocks');
    wsClient.subscribe('operation');

    return () => {
      unsubscribeBlocks();
      unsubscribeOps();
      wsClient.unsubscribe('blocks');
      wsClient.unsubscribe('operation');
    };
  }, []);

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

  const renderFeedItem = (item: FeedItem) => {
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
    } else {
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
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">Live Feed</h1>
          <p className="text-muted-foreground">Real-time blockchain activity</p>
        </div>
        <div className="flex items-center space-x-2">
          {getConnectionStatus()}
        </div>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Activity Stream</CardTitle>
          <CardDescription>
            {wsState === 'connected'
              ? 'Real-time updates from the blockchain'
              : 'Connect to WebSocket to see live updates'}
          </CardDescription>
        </CardHeader>
        <CardContent>
          {wsState !== 'connected' ? (
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
                <div key={item.id}>{renderFeedItem(item)}</div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>

      {/* Recent Blocks Summary */}
      {latestBlocks.length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle>Recent Blocks</CardTitle>
            <CardDescription>Latest blocks from the blockchain</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="space-y-3">
              {latestBlocks.slice(0, 10).map((block) => (
                <div
                  key={block.number}
                  className="flex items-center justify-between p-3 border rounded-lg hover:bg-accent/50"
                >
                  <div className="flex items-center space-x-3">
                    <Blocks className="h-4 w-4 text-muted-foreground" />
                    <div>
                      <div className="font-medium">Block #{formatNumber(block.number)}</div>
                      <div className="text-sm text-muted-foreground">
                        by @{block.witness} • {formatTimeAgo(block.timestamp)}
                      </div>
                    </div>
                  </div>
                  <div className="text-sm text-muted-foreground">
                    {block.transactions ?? block.transaction_count ?? 0} txs,{' '}
                    {block.operations ?? block.operation_count ?? 0} ops
                  </div>
                </div>
              ))}
            </div>
          </CardContent>
        </Card>
      )}
    </div>
  );
}
