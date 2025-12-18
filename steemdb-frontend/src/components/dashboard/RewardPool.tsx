import { Coins } from 'lucide-react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../ui/Card';
import { formatScientificNotation, formatTimestamp, formatKeyName, parseAmount } from '../../lib/utils';
import type { RewardPool } from '../../types';

interface RewardPoolProps {
  data?: RewardPool;
}

export function RewardPool({ data }: RewardPoolProps) {
  if (!data || Object.keys(data).length === 0) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Coins className="h-5 w-5" />
            Reward Pool
          </CardTitle>
          <CardDescription>Reward distribution parameters</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="text-center py-6 text-muted-foreground">
            No data available
          </div>
        </CardContent>
      </Card>
    );
  }

  // Filter out internal fields and format values
  const formatValue = (key: string, value: any): string => {
    // Handle timestamps
    if (key.includes('update') || key.includes('time') || key.includes('timestamp')) {
      if (typeof value === 'number' || typeof value === 'string') {
        return formatTimestamp(value);
      }
    }

    // Handle percentages
    if (key.includes('percent')) {
      if (typeof value === 'number') {
        return `${(value / 100).toFixed(2)}%`;
      }
    }

    // Handle currency strings (STEEM, SBD, VESTS)
    if (typeof value === 'string') {
      if (value.includes('STEEM')) {
        const num = parseAmount(value);
        if (!isNaN(num)) {
          return `${num.toFixed(3)} STEEM`;
        }
      } else if (value.includes('SBD')) {
        const num = parseAmount(value);
        if (!isNaN(num)) {
          return `${num.toFixed(3)} SBD`;
        }
      } else if (value.includes('VESTS')) {
        const num = parseAmount(value);
        if (!isNaN(num)) {
          return `${num.toFixed(6)} VESTS`;
        }
      }
      
      // Check if it's a number string
      const num = parseFloat(value);
      if (!isNaN(num)) {
        if (Math.abs(num) >= 1e10) {
          return formatScientificNotation(num);
        }
        return num.toLocaleString('en-US', { maximumFractionDigits: 3 });
      }
      return value;
    }

    // Handle numbers (scientific notation)
    if (typeof value === 'number') {
      if (Math.abs(value) >= 1e10) {
        return formatScientificNotation(value);
      }
      return value.toLocaleString('en-US', { maximumFractionDigits: 3 });
    }

    return String(value);
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Coins className="h-5 w-5" />
          Reward Pool
        </CardTitle>
        <CardDescription>Reward distribution parameters</CardDescription>
      </CardHeader>
      <CardContent>
        <dl className="space-y-3">
          {Object.entries(data)
            .filter(([key]) => !['_id', 'id', 'name'].includes(key))
            .map(([key, value]) => (
              <div key={key} className="flex items-start justify-between border-b pb-2 last:border-0">
                <dt className="text-sm font-medium text-muted-foreground pr-4">
                  {formatKeyName(key)}
                </dt>
                <dd className="text-sm font-medium text-right break-all">
                  {formatValue(key, value)}
                </dd>
              </div>
            ))}
        </dl>
      </CardContent>
    </Card>
  );
}
