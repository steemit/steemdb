import { useQuery } from '@tanstack/react-query';
import { TrendingUp, Activity, Users, Blocks } from 'lucide-react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../components/ui/Card';
import { getGlobalStats, getBlockchainProps, getAccountGrowthData, getBlockProductionData, getTransactionVolumeData } from '../lib/api';
import { formatNumber, formatCurrency } from '../lib/utils';
import { LineChart, Line, BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip, Legend, ResponsiveContainer } from 'recharts';

export function StatisticsPage() {
  const { data: statsData } = useQuery({
    queryKey: ['global-stats'],
    queryFn: () => getGlobalStats(),
  });

  const { data: propsData } = useQuery({
    queryKey: ['blockchain-props'],
    queryFn: () => getBlockchainProps(),
  });

  const { data: accountGrowthData } = useQuery({
    queryKey: ['account-growth'],
    queryFn: () => getAccountGrowthData(30),
  });

  const { data: blockProductionData } = useQuery({
    queryKey: ['block-production'],
    queryFn: () => getBlockProductionData(7),
  });

  const { data: transactionVolumeData } = useQuery({
    queryKey: ['transaction-volume'],
    queryFn: () => getTransactionVolumeData(30),
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
            <div className="text-2xl font-bold">{stats ? formatNumber(stats.blocks) : '-'}</div>
            <p className="text-xs text-muted-foreground">Processed blocks</p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Total Transactions</CardTitle>
            <Activity className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{stats ? formatNumber(stats.transactions) : '-'}</div>
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
        {accountGrowthData?.success && accountGrowthData.data && (
          <Card>
            <CardHeader>
              <CardTitle>Account Growth (30 days)</CardTitle>
              <CardDescription>New accounts over time</CardDescription>
            </CardHeader>
            <CardContent>
              <ResponsiveContainer width="100%" height={300}>
                <LineChart data={accountGrowthData.data}>
                  <CartesianGrid strokeDasharray="3 3" />
                  <XAxis dataKey="date" />
                  <YAxis />
                  <Tooltip />
                  <Legend />
                  <Line type="monotone" dataKey="count" stroke="#8884d8" name="New Accounts" />
                </LineChart>
              </ResponsiveContainer>
            </CardContent>
          </Card>
        )}

        {blockProductionData?.success && blockProductionData.data && (
          <Card>
            <CardHeader>
              <CardTitle>Block Production (7 days)</CardTitle>
              <CardDescription>Blocks produced per day</CardDescription>
            </CardHeader>
            <CardContent>
              <ResponsiveContainer width="100%" height={300}>
                <BarChart data={blockProductionData.data}>
                  <CartesianGrid strokeDasharray="3 3" />
                  <XAxis dataKey="date" />
                  <YAxis />
                  <Tooltip />
                  <Legend />
                  <Bar dataKey="blocks" fill="#8884d8" name="Blocks" />
                </BarChart>
              </ResponsiveContainer>
            </CardContent>
          </Card>
        )}

        {transactionVolumeData?.success && transactionVolumeData.data && (
          <Card className="md:col-span-2">
            <CardHeader>
              <CardTitle>Transaction Volume (30 days)</CardTitle>
              <CardDescription>Daily transaction count</CardDescription>
            </CardHeader>
            <CardContent>
              <ResponsiveContainer width="100%" height={300}>
                <LineChart data={transactionVolumeData.data}>
                  <CartesianGrid strokeDasharray="3 3" />
                  <XAxis dataKey="date" />
                  <YAxis />
                  <Tooltip />
                  <Legend />
                  <Line type="monotone" dataKey="transactions" stroke="#82ca9d" name="Transactions" />
                </LineChart>
              </ResponsiveContainer>
            </CardContent>
          </Card>
        )}
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
