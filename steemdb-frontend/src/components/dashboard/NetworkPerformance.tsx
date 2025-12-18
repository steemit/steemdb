import { Activity, TrendingUp } from 'lucide-react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../ui/Card';
import { formatNumber, formatRate } from '../../lib/utils';
import type { NetworkPerformance as NetworkPerformanceType } from '../../types';

interface NetworkPerformanceProps {
  data?: NetworkPerformanceType;
}

export function NetworkPerformance({ data }: NetworkPerformanceProps) {
  if (!data) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Activity className="h-5 w-5" />
            Network Performance
          </CardTitle>
          <CardDescription>Transaction and operation metrics</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="text-center py-6 text-muted-foreground">
            No data available
          </div>
        </CardContent>
      </Card>
    );
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Activity className="h-5 w-5" />
          Network Performance
        </CardTitle>
        <CardDescription>Transaction and operation metrics</CardDescription>
      </CardHeader>
      <CardContent>
        <dl className="space-y-4">
          {/* Transactions per second */}
          <div className="flex items-center justify-between border-b pb-2">
            <dt className="text-sm font-medium text-muted-foreground flex items-center gap-2">
              <TrendingUp className="h-4 w-4" />
              Transactions per second (24h)
            </dt>
            <dd className="text-lg font-semibold">
              {formatRate(data.transactions_per_sec_24h)} tx/sec
            </dd>
          </div>

          <div className="flex items-center justify-between border-b pb-2">
            <dt className="text-sm font-medium text-muted-foreground">
              Transactions per second (1h)
            </dt>
            <dd className="text-lg font-semibold">
              {formatRate(data.transactions_per_sec_1h)} tx/sec
            </dd>
          </div>

          {/* Transactions totals */}
          <div className="flex items-center justify-between border-b pb-2">
            <dt className="text-sm font-medium text-muted-foreground">
              Transactions over 24h
            </dt>
            <dd className="text-base font-medium">
              {formatNumber(data.transactions_24h)} txs
            </dd>
          </div>

          <div className="flex items-center justify-between border-b pb-2">
            <dt className="text-sm font-medium text-muted-foreground">
              Transactions over 1h
            </dt>
            <dd className="text-base font-medium">
              {formatNumber(data.transactions_1h)} txs
            </dd>
          </div>

          {/* Operations totals */}
          <div className="flex items-center justify-between border-b pb-2">
            <dt className="text-sm font-medium text-muted-foreground">
              Operations over 24h
            </dt>
            <dd className="text-base font-medium">
              {formatNumber(data.operations_24h)} ops
            </dd>
          </div>

          <div className="flex items-center justify-between">
            <dt className="text-sm font-medium text-muted-foreground">
              Operations over 1h
            </dt>
            <dd className="text-base font-medium">
              {formatNumber(data.operations_1h)} ops
            </dd>
          </div>
        </dl>
      </CardContent>
    </Card>
  );
}
