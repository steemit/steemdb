import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { ChevronRight, Search, ArrowUpDown, Star } from 'lucide-react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../components/ui/Card';
import { Input } from '../components/ui/Input';
import { Button } from '../components/ui/Button';
import { Badge } from '../components/ui/Badge';
import { getAccounts, getAccount, getAccountHistory } from '../lib/api';
import { formatNumber, formatTimeAgo, formatDate, formatCurrency, formatReputation, getAvatarUrl, formatVests } from '../lib/utils';
import { useFavoritesStore } from '../store';

export function AccountsPage() {
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const [page, setPage] = useState(Number(searchParams.get('page')) || 1);
  const [search, setSearch] = useState(searchParams.get('search') || '');
  const [sortBy, setSortBy] = useState(searchParams.get('sort') || 'name');
  const [sortOrder, setSortOrder] = useState<'asc' | 'desc'>((searchParams.get('order') as 'asc' | 'desc') || 'asc');

  const { data, isLoading, error } = useQuery({
    queryKey: ['accounts', page, search, sortBy, sortOrder],
    queryFn: () => getAccounts({ page, limit: 20, search, sort: sortBy, order: sortOrder }),
  });

  const handleSort = (field: string) => {
    if (sortBy === field) {
      setSortOrder(sortOrder === 'asc' ? 'desc' : 'asc');
    } else {
      setSortBy(field);
      setSortOrder('asc');
    }
    setPage(1);
  };

  const handlePageChange = (newPage: number) => {
    setPage(newPage);
    setSearchParams({ page: newPage.toString(), sort: sortBy, order: sortOrder, ...(search && { search }) });
  };

  const handleSearch = (value: string) => {
    setSearch(value);
    setPage(1);
    setSearchParams({ page: '1', sort: sortBy, order: sortOrder, ...(value && { search: value }) });
  };

  if (isLoading) {
    return (
      <div className="space-y-6">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-3xl font-bold tracking-tight">Accounts</h1>
            <p className="text-muted-foreground">Browse Steem accounts</p>
          </div>
        </div>
        <div className="text-center py-12">
          <div className="text-muted-foreground">Loading accounts...</div>
        </div>
      </div>
    );
  }

  if (error || !data?.success) {
    return (
      <div className="space-y-6">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-3xl font-bold tracking-tight">Accounts</h1>
            <p className="text-muted-foreground">Browse Steem accounts</p>
          </div>
        </div>
        <Card>
          <CardContent className="py-12">
            <div className="text-center text-muted-foreground">
              Failed to load accounts. Please try again later.
            </div>
          </CardContent>
        </Card>
      </div>
    );
  }

  const accounts = data.data?.data || [];
  const totalPages = data.data?.totalPages || 1;

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">Accounts</h1>
          <p className="text-muted-foreground">Browse Steem accounts</p>
        </div>
      </div>

      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div>
              <CardTitle>Account List</CardTitle>
              <CardDescription>Search and browse Steem accounts</CardDescription>
            </div>
            <div className="w-64">
              <div className="relative">
                <Search className="absolute left-2 top-2.5 h-4 w-4 text-muted-foreground" />
                <Input
                  placeholder="Search accounts..."
                  value={search}
                  onChange={(e) => handleSearch(e.target.value)}
                  className="pl-8"
                />
              </div>
            </div>
          </div>
        </CardHeader>
        <CardContent>
          <div className="space-y-4">
            {/* Table */}
            <div className="overflow-x-auto">
              <table className="w-full">
                <thead>
                  <tr className="border-b">
                    <th className="text-left p-3">Account</th>
                    <th className="text-left p-3">
                      <button
                        onClick={() => handleSort('reputation')}
                        className="flex items-center space-x-1 hover:text-foreground"
                      >
                        <span>Reputation</span>
                        <ArrowUpDown className="h-3 w-3" />
                      </button>
                    </th>
                    <th className="text-left p-3">
                      <button
                        onClick={() => handleSort('balance')}
                        className="flex items-center space-x-1 hover:text-foreground"
                      >
                        <span>Balance</span>
                        <ArrowUpDown className="h-3 w-3" />
                      </button>
                    </th>
                    <th className="text-right p-3">Posts</th>
                    <th className="text-right p-3">Comments</th>
                    <th className="text-right p-3"></th>
                  </tr>
                </thead>
                <tbody>
                  {accounts.map((account) => (
                    <tr
                      key={account.name}
                      className="border-b hover:bg-accent/50 cursor-pointer"
                      onClick={() => navigate(`/accounts/${account.name}`)}
                    >
                      <td className="p-3">
                        <div className="flex items-center space-x-3">
                          <img
                            src={getAvatarUrl(account.name)}
                            alt={account.name}
                            className="w-8 h-8 rounded-full"
                            onError={(e) => {
                              (e.target as HTMLImageElement).src = `https://ui-avatars.com/api/?name=${account.name}&background=random`;
                            }}
                          />
                          <div>
                            <div className="font-medium">@{account.name}</div>
                            {account.created && (
                              <div className="text-xs text-muted-foreground">
                                Joined {formatTimeAgo(account.created)}
                              </div>
                            )}
                          </div>
                        </div>
                      </td>
                      <td className="p-3">
                        <Badge variant="outline">{formatReputation(account.reputation)}</Badge>
                      </td>
                      <td className="p-3">
                        <div className="text-sm">{formatCurrency(account.balance)}</div>
                      </td>
                      <td className="p-3 text-right">{formatNumber(account.post_count || 0)}</td>
                      <td className="p-3 text-right">{formatNumber(account.comment_count || 0)}</td>
                      <td className="p-3 text-right">
                        <ChevronRight className="h-4 w-4 text-muted-foreground" />
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>

            {/* Pagination */}
            {totalPages > 1 && (
              <div className="flex items-center justify-between pt-4">
                <div className="text-sm text-muted-foreground">
                  Page {page} of {totalPages}
                </div>
                <div className="flex space-x-2">
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => handlePageChange(page - 1)}
                    disabled={page === 1}
                  >
                    Previous
                  </Button>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => handlePageChange(page + 1)}
                    disabled={page >= totalPages}
                  >
                    Next
                  </Button>
                </div>
              </div>
            )}
          </div>
        </CardContent>
      </Card>
    </div>
  );
}

export function AccountDetailPage() {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const accountName = searchParams.get('id') || window.location.pathname.split('/').pop() || '';
  const { addAccount, removeAccount, isAccountFavorite } = useFavoritesStore();
  const isFavorite = isAccountFavorite(accountName);
  const [historyPage, setHistoryPage] = useState(1);

  const { data: accountData, isLoading: accountLoading } = useQuery({
    queryKey: ['account', accountName],
    queryFn: () => getAccount(accountName),
    enabled: !!accountName,
  });

  const { data: historyData, isLoading: historyLoading } = useQuery({
    queryKey: ['account-history', accountName, historyPage],
    queryFn: () => getAccountHistory(accountName, { page: historyPage, limit: 20 }),
    enabled: !!accountName,
  });


  if (accountLoading) {
    return (
      <div className="space-y-6">
        <div className="text-center py-12">
          <div className="text-muted-foreground">Loading account details...</div>
        </div>
      </div>
    );
  }

  if (!accountData?.success || !accountData.data) {
    return (
      <div className="space-y-6">
        <Card>
          <CardContent className="py-12">
            <div className="text-center text-muted-foreground">
              Account not found or failed to load.
            </div>
          </CardContent>
        </Card>
      </div>
    );
  }

  const account = accountData.data;

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <Button variant="ghost" onClick={() => navigate('/accounts')} className="mb-2">
            ← Back to Accounts
          </Button>
          <div className="flex items-center space-x-4">
            <img
              src={getAvatarUrl(account.name)}
              alt={account.name}
              className="w-16 h-16 rounded-full"
              onError={(e) => {
                (e.target as HTMLImageElement).src = `https://ui-avatars.com/api/?name=${account.name}&background=random`;
              }}
            />
            <div>
              <div className="flex items-center space-x-2">
                <h1 className="text-3xl font-bold tracking-tight">@{account.name}</h1>
                <Button
                  variant="ghost"
                  size="icon"
                  onClick={() => {
                    if (isFavorite) {
                      removeAccount(account.name);
                    } else {
                      addAccount(account.name);
                    }
                  }}
                >
                  <Star className={`h-4 w-4 ${isFavorite ? 'fill-yellow-400 text-yellow-400' : ''}`} />
                </Button>
              </div>
              <p className="text-muted-foreground">Account details and statistics</p>
            </div>
          </div>
        </div>
      </div>

      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
        <Card>
          <CardHeader>
            <CardTitle>Balance</CardTitle>
          </CardHeader>
          <CardContent className="space-y-2">
            <div className="flex justify-between">
              <span className="text-muted-foreground">STEEM</span>
              <span className="font-medium">{formatCurrency(account.balance)}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-muted-foreground">SBD</span>
              <span className="font-medium">{formatCurrency(account.sbd_balance, 'SBD')}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-muted-foreground">Vesting Shares</span>
              <span className="font-medium text-xs">{formatVests(account.vesting_shares)}</span>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Statistics</CardTitle>
          </CardHeader>
          <CardContent className="space-y-2">
            <div className="flex justify-between">
              <span className="text-muted-foreground">Reputation</span>
              <Badge variant="outline">{formatReputation(account.reputation)}</Badge>
            </div>
            <div className="flex justify-between">
              <span className="text-muted-foreground">Posts</span>
              <span className="font-medium">{formatNumber(account.post_count || 0)}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-muted-foreground">Comments</span>
              <span className="font-medium">{formatNumber(account.comment_count || 0)}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-muted-foreground">Voting Power</span>
              <span className="font-medium">{account.voting_power || 0}%</span>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Account Info</CardTitle>
          </CardHeader>
          <CardContent className="space-y-2">
            {account.created && (
              <div className="flex justify-between">
                <span className="text-muted-foreground">Created</span>
                <span className="font-medium text-sm">{formatDate(account.created)}</span>
              </div>
            )}
            {account.last_post && (
              <div className="flex justify-between">
                <span className="text-muted-foreground">Last Post</span>
                <span className="font-medium text-sm">{formatTimeAgo(account.last_post)}</span>
              </div>
            )}
            {account.proxy && (
              <div className="flex justify-between">
                <span className="text-muted-foreground">Proxy</span>
                <button
                  onClick={() => navigate(`/accounts/${account.proxy}`)}
                  className="font-medium text-sm text-primary hover:underline"
                >
                  @{account.proxy}
                </button>
              </div>
            )}
          </CardContent>
        </Card>
      </div>

      {account.witness_votes && account.witness_votes.length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle>Witness Votes</CardTitle>
            <CardDescription>Witnesses voted by this account</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="flex flex-wrap gap-2">
              {account.witness_votes.map((witness) => (
                <button
                  key={witness}
                  onClick={() => navigate(`/witnesses/${witness}`)}
                  className="text-sm text-primary hover:underline"
                >
                  @{witness}
                </button>
              ))}
            </div>
          </CardContent>
        </Card>
      )}

      <Card>
        <CardHeader>
          <CardTitle>Operation History</CardTitle>
          <CardDescription>
            Recent operations involving this account
            {historyData?.meta?.total !== undefined &&
              ` (${formatNumber(historyData.meta.total)} total)`}
          </CardDescription>
        </CardHeader>
        <CardContent>
          {historyLoading ? (
            <div className="text-center py-8 text-muted-foreground">
              Loading operation history...
            </div>
          ) : historyData?.success && historyData.data && historyData.data.length > 0 ? (
            <div className="space-y-4">
              <div className="overflow-x-auto">
                <table className="w-full">
                  <thead>
                    <tr className="border-b">
                      <th className="text-left p-3">Block</th>
                      <th className="text-left p-3">Type</th>
                      <th className="text-left p-3">Details</th>
                      <th className="text-left p-3">Time</th>
                    </tr>
                  </thead>
                  <tbody>
                    {historyData.data.map((op) => (
                      <tr
                        key={op.id}
                        className="border-b hover:bg-accent/50 cursor-pointer"
                        onClick={() => navigate(`/blocks/${op.block_num}`)}
                      >
                        <td className="p-3 font-medium">#{formatNumber(op.block_num)}</td>
                        <td className="p-3">
                          <Badge variant="outline">{op.op_type}</Badge>
                        </td>
                        <td className="p-3 text-sm text-muted-foreground">
                          {formatOpSummary(op.summary)}
                        </td>
                        <td className="p-3 text-sm">
                          {op.block_time ? formatDate(op.block_time) : '-'}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>

              {(historyData.meta?.total_pages || 1) > 1 && (
                <div className="flex items-center justify-between">
                  <div className="text-sm text-muted-foreground">
                    Page {historyPage} of {historyData.meta?.total_pages}
                  </div>
                  <div className="flex space-x-2">
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => setHistoryPage((p) => Math.max(1, p - 1))}
                      disabled={historyPage === 1}
                    >
                      Previous
                    </Button>
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => setHistoryPage((p) => p + 1)}
                      disabled={historyPage >= (historyData.meta?.total_pages || 1)}
                    >
                      Next
                    </Button>
                  </div>
                </div>
              )}
            </div>
          ) : (
            <div className="text-center py-8 text-muted-foreground">
              No operations found for this account.
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

// formatOpSummary renders the backend-generated summary map as readable text
function formatOpSummary(summary: Record<string, unknown> | null | undefined): string {
  if (!summary) return '-';
  const parts: string[] = [];
  for (const [key, value] of Object.entries(summary)) {
    if (value === null || value === undefined) continue;
    let rendered: unknown = value;
    if (typeof value === 'object') {
      // Asset objects ({amount, precision, nai}) or nested structures
      rendered = (value as Record<string, unknown>).amount ?? JSON.stringify(value);
    }
    parts.push(`${key}: ${rendered}`);
  }
  return parts.length > 0 ? parts.slice(0, 3).join(' · ') : '-';
}
