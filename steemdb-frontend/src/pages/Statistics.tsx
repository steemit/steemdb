import { useQuery } from '@tanstack/react-query';
import { TrendingUp, Activity, Users, Blocks, Loader2 } from 'lucide-react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../components/ui/Card';
import { getGlobalStats, getBlockchainProps, getAccountGrowthData, getBlockProductionData, getTransactionVolumeData, getWitnessVotingData } from '../lib/api';
import { formatNumber, formatCurrency } from '../lib/utils';
import { LineChart, Line, BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip, Legend, ResponsiveContainer } from 'recharts';

import type { ReactNode } from 'react';

// compactVotes renders huge vest-based vote totals (1e18+) readably
function compactVotes(value: number): string {
  return new Intl.NumberFormat('en', { notation: 'compact', maximumFractionDigits: 2 }).format(value);
}

// ChartCard wraps a chart with consistent loading / error / empty states so
// charts never silently disappear when an endpoint fails.
function ChartCard({
  title,
  description,
  isLoading,
  isError,
  isEmpty,
  emptyMessage,
  className,
  children,
}: {
  title: string;
  description: string;
  isLoading: boolean;
  isError: boolean;
  isEmpty: boolean;
  emptyMessage: string;
  className?: string;
  children: ReactNode;
}) {
  return (
    <Card className={className}>
      <CardHeader>
        <CardTitle>{title}</CardTitle>
        <CardDescription>{description}</CardDescription>
      </CardHeader>
      <CardContent>
        {isLoading ? (
          <div className="flex h-[300px] items-center justify-center text-muted-foreground">
            <Loader2 className="mr-2 h-4 w-4 animate-spin" />
            Loading chart data...
          </div>
        ) : isError ? (
          <div className="flex h-[300px] items-center justify-center text-muted-foreground">
            Failed to load chart data. Please try again later.
          </div>
        ) : isEmpty ? (
          <div className="flex h-[300px] items-center justify-center text-muted-foreground">
            {emptyMessage}
          </div>
        ) : (
          children
        )}
      </CardContent>
    </Card>
  );
}

export function StatisticsPage() {
  const { data: statsData } = useQuery({
    queryKey: ['global-stats'],
    queryFn: () => getGlobalStats(),
  });

  const { data: propsData } = useQuery({
    queryKey: ['blockchain-props'],
    queryFn: () => getBlockchainProps(),
  });

  const accountGrowth = useQuery({
    queryKey: ['account-growth'],
    queryFn: () => getAccountGrowthData(30),
  });

  const blockProduction = useQuery({
    queryKey: ['block-production'],
    queryFn: () => getBlockProductionData(7),
  });

  const transactionVolume = useQuery({
    queryKey: ['transaction-volume'],
    queryFn: () => getTransactionVolumeData(30),
  });

  const witnessVoting = useQuery({
    queryKey: ['witness-voting'],
    queryFn: () => getWitnessVotingData(30),
  });

  const stats = statsData?.data;
  const props = propsData?.data;

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">Statistics</h1>
          <p className="text-muted-foreground">Blockchain statistics and analytics</p>
        </div>
      </div>

      {/* Overview Cards */}
      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Total Accounts</CardTitle>
            <Users className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{stats ? formatNumber(stats.accounts) : '-'}</div>
            <p className="text-xs text-muted-foreground">Registered users</p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Total Blocks</CardTitle>
            <Blocks className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{stats ? formatNumber(stats.blocks ?? 0) : "-"}</div>
            <p className="text-xs text-muted-foreground">Processed blocks</p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Total Transactions</CardTitle>
            <Activity className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{stats ? formatNumber(stats.transactions ?? 0) : "-"}</div>
            <p className="text-xs text-muted-foreground">All transactions</p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Virtual Supply</CardTitle>
            <TrendingUp className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{props ? formatCurrency(props.virtual_supply) : '-'}</div>
            <p className="text-xs text-muted-foreground">Total STEEM supply</p>
          </CardContent>
        </Card>
      </div>

      {/* Charts */}
      <div className="grid gap-4 md:grid-cols-2">
        <ChartCard
          title="Account Growth (30 days)"
          description="New accounts over time"
          isLoading={accountGrowth.isLoading}
          isError={accountGrowth.isError || accountGrowth.data?.success === false}
          isEmpty={(accountGrowth.data?.data?.length ?? 0) === 0}
          emptyMessage="No new accounts in the last 30 days"
        >
          <ResponsiveContainer width="100%" height={300}>
            <LineChart data={accountGrowth.data?.data}>
              <CartesianGrid strokeDasharray="3 3" />
              <XAxis dataKey="date" />
              <YAxis />
              <Tooltip />
              <Legend />
              <Line type="monotone" dataKey="count" stroke="#8884d8" name="New Accounts" />
            </LineChart>
          </ResponsiveContainer>
        </ChartCard>

        <ChartCard
          title="Block Production (7 days)"
          description="Blocks produced per day"
          isLoading={blockProduction.isLoading}
          isError={blockProduction.isError || blockProduction.data?.success === false}
          isEmpty={(blockProduction.data?.data?.length ?? 0) === 0}
          emptyMessage="No blocks in the last 7 days"
        >
          <ResponsiveContainer width="100%" height={300}>
            <BarChart data={blockProduction.data?.data}>
              <CartesianGrid strokeDasharray="3 3" />
              <XAxis dataKey="date" />
              <YAxis />
              <Tooltip />
              <Legend />
              <Bar dataKey="blocks" fill="#8884d8" name="Blocks" />
            </BarChart>
          </ResponsiveContainer>
        </ChartCard>

        <ChartCard
          className="md:col-span-2"
          title="Transaction Volume (30 days)"
          description="Daily transaction count"
          isLoading={transactionVolume.isLoading}
          isError={transactionVolume.isError || transactionVolume.data?.success === false}
          isEmpty={(transactionVolume.data?.data?.length ?? 0) === 0}
          emptyMessage="No transactions in the last 30 days"
        >
          <ResponsiveContainer width="100%" height={300}>
            <LineChart data={transactionVolume.data?.data}>
              <CartesianGrid strokeDasharray="3 3" />
              <XAxis dataKey="date" />
              <YAxis />
              <Tooltip />
              <Legend />
              <Line type="monotone" dataKey="transactions" stroke="#82ca9d" name="Transactions" />
            </LineChart>
          </ResponsiveContainer>
        </ChartCard>

        <ChartCard
          className="md:col-span-2"
          title="Witness Voting (30 days)"
          description="Total voting weight across snapshotted witnesses per day"
          isLoading={witnessVoting.isLoading}
          isError={witnessVoting.isError || witnessVoting.data?.success === false}
          isEmpty={(witnessVoting.data?.data?.length ?? 0) === 0}
          emptyMessage="No witness voting snapshots in the last 30 days"
        >
          <ResponsiveContainer width="100%" height={300}>
            <LineChart data={witnessVoting.data?.data}>
              <CartesianGrid strokeDasharray="3 3" />
              <XAxis dataKey="date" />
              <YAxis tickFormatter={compactVotes} />
              <Tooltip formatter={(value: number) => compactVotes(value)} />
              <Legend />
              <Line type="monotone" dataKey="votes" stroke="#ffc658" name="Total Votes" />
            </LineChart>
          </ResponsiveContainer>
        </ChartCard>
      </div>

      {/* Additional Stats */}
      {props && (
        <div className="grid gap-4 md:grid-cols-2">
          <Card>
            <CardHeader>
              <CardTitle>Supply Information</CardTitle>
            </CardHeader>
            <CardContent className="space-y-3">
              <div className="flex justify-between">
                <span className="text-muted-foreground">Current Supply</span>
                <span className="font-medium">{formatCurrency(props.current_supply)}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">SBD Supply</span>
                <span className="font-medium">{formatCurrency(props.current_sbd_supply, 'SBD')}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">Vesting Fund</span>
                <span className="font-medium">{formatCurrency(props.total_vesting_fund_steem)}</span>
              </div>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>Network Information</CardTitle>
            </CardHeader>
            <CardContent className="space-y-3">
              <div className="flex justify-between">
                <span className="text-muted-foreground">Head Block</span>
                <span className="font-medium">#{formatNumber(props.head_block_number)}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">Last Irreversible Block</span>
                <span className="font-medium">#{formatNumber(props.last_irreversible_block_num)}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">Maximum Block Size</span>
                <span className="font-medium">{formatNumber(props.maximum_block_size)} bytes</span>
              </div>
            </CardContent>
          </Card>
        </div>
      )}
    </div>
  );
}
