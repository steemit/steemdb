import { Globe } from 'lucide-react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../ui/Card';
import { formatKeyName, formatCurrency, formatScientificNotation } from '../../lib/utils';
import type { BlockchainProps } from '../../types';

interface GlobalPropertiesProps {
  data?: BlockchainProps;
}

// Fields to exclude from display (already shown elsewhere)
const EXCLUDED_FIELDS = [
  'head_block_number',
  'head_block_id',
  'recent_slots_filled',
  'steem_per_mvests',
  'reversible_blocks',
];

export function GlobalProperties({ data }: GlobalPropertiesProps) {
  if (!data) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Globe className="h-5 w-5" />
            Global Properties
          </CardTitle>
          <CardDescription>Blockchain configuration and state</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="text-center py-6 text-muted-foreground">
            No data available
          </div>
        </CardContent>
      </Card>
    );
  }

  const formatValue = (key: string, value: any): string => {
    // Handle VESTS fields (vesting_shares) - 6 decimal places
    if (key.includes('vesting_shares') || key.includes('shares') && !key.includes('fund')) {
      if (typeof value === 'string') {
        const num = parseFloat(value);
        if (!isNaN(num)) {
          return `${num.toFixed(6)} VESTS`;
        }
      }
      if (typeof value === 'number') {
        return `${value.toFixed(6)} VESTS`;
      }
    }
    
    // Handle currency fields (STEEM/SBD) - 3 decimal places
    if (key.includes('supply') || key.includes('balance') || key.includes('fund')) {
      if (typeof value === 'string') {
        // Check if it contains STEEM or SBD
        if (value.includes('STEEM')) {
          const num = parseFloat(value);
          if (!isNaN(num)) {
            return `${num.toFixed(3)} STEEM`;
          }
        } else if (value.includes('SBD')) {
          const num = parseFloat(value);
          if (!isNaN(num)) {
            return `${num.toFixed(3)} SBD`;
          }
        }
        return formatCurrency(value);
      }
    }

    // Handle percentages
    if (key.includes('rate') || key.includes('percent')) {
      if (typeof value === 'number') {
        return `${(value / 100).toFixed(2)}%`;
      }
    }

    // Handle time
    if (key === 'time' && typeof value === 'string') {
      return new Date(value).toLocaleString();
    }

    // Handle large numbers
    if (typeof value === 'number') {
      if (Math.abs(value) >= 1e10) {
        return formatScientificNotation(value);
      }
      return value.toLocaleString('en-US');
    }

    // Handle strings
    if (typeof value === 'string') {
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

    return String(value);
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Globe className="h-5 w-5" />
          Global Properties
        </CardTitle>
        <CardDescription>Blockchain configuration and state</CardDescription>
      </CardHeader>
      <CardContent>
        <dl className="space-y-3">
          {Object.entries(data)
            .filter(([key]) => !EXCLUDED_FIELDS.includes(key))
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
