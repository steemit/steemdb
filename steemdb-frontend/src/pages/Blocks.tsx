import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { ChevronRight, ArrowUpDown } from 'lucide-react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../components/ui/Card';
import { Button } from '../components/ui/Button';
import { Badge } from '../components/ui/Badge';
import { getBlocks, getBlock } from '../lib/api';
import { formatNumber, formatTimeAgo, formatDate } from '../lib/utils';

export function BlocksPage() {
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const [page, setPage] = useState(Number(searchParams.get('page')) || 1);
  const [sortBy, setSortBy] = useState(searchParams.get('sort') || 'number');
  const [sortOrder, setSortOrder] = useState<'asc' | 'desc'>((searchParams.get('order') as 'asc' | 'desc') || 'desc');

  const { data, isLoading, error } = useQuery({
    queryKey: ['blocks', page, sortBy, sortOrder],
    queryFn: () => getBlocks({ page, limit: 20, sort: sortBy, order: sortOrder }),
  });

  const handleSort = (field: string) => {
    if (sortBy === field) {
      setSortOrder(sortOrder === 'asc' ? 'desc' : 'asc');
    } else {
      setSortBy(field);
      setSortOrder('desc');
    }
    setPage(1);
  };

  const handlePageChange = (newPage: number) => {
    setPage(newPage);
    setSearchParams({ page: newPage.toString(), sort: sortBy, order: sortOrder });
  };

  if (isLoading) {
    return (
      <div className="space-y-6">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-3xl font-bold tracking-tight">Blocks</h1>
            <p className="text-muted-foreground">Browse blockchain blocks</p>
          </div>
        </div>
        <div className="text-center py-12">
          <div className="text-muted-foreground">Loading blocks...</div>
        </div>
      </div>
    );
  }

  if (error || !data?.success) {
    return (
      <div className="space-y-6">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-3xl font-bold tracking-tight">Blocks</h1>
            <p className="text-muted-foreground">Browse blockchain blocks</p>
          </div>
        </div>
        <Card>
          <CardContent className="py-12">
            <div className="text-center text-muted-foreground">
              Failed to load blocks. Please try again later.
            </div>
          </CardContent>
        </Card>
      </div>
    );
  }

  const blocks = data.data || [];
  const totalPages = data.meta?.total_pages || 1;

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">Blocks</h1>
          <p className="text-muted-foreground">Browse blockchain blocks</p>
        </div>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Block List</CardTitle>
          <CardDescription>Recent blocks on the Steem blockchain</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="space-y-4">
            {/* Table */}
            <div className="overflow-x-auto">
              <table className="w-full">
                <thead>
                  <tr className="border-b">
                    <th className="text-left p-3">
                      <button
                        onClick={() => handleSort('number')}
                        className="flex items-center space-x-1 hover:text-foreground"
                      >
                        <span>Block</span>
                        <ArrowUpDown className="h-3 w-3" />
                      </button>
                    </th>
                    <th className="text-left p-3">
                      <button
                        onClick={() => handleSort('timestamp')}
                        className="flex items-center space-x-1 hover:text-foreground"
                      >
                        <span>Time</span>
                        <ArrowUpDown className="h-3 w-3" />
                      </button>
                    </th>
                    <th className="text-left p-3">Witness</th>
                    <th className="text-right p-3">Transactions</th>
                    <th className="text-right p-3">Operations</th>
                    <th className="text-right p-3"></th>
                  </tr>
                </thead>
                <tbody>
                  {blocks.map((block) => (
                    <tr
                      key={block.block_num}
                      className="border-b hover:bg-accent/50 cursor-pointer"
                      onClick={() => navigate(`/blocks/${block.block_num}`)}
                    >
                      <td className="p-3">
                        <div className="font-medium">#{formatNumber(block.block_num)}</div>
                      </td>
                      <td className="p-3 text-sm text-muted-foreground">
                        {formatTimeAgo(block.timestamp)}
                      </td>
                      <td className="p-3">
                        <button
                          onClick={(e) => {
                            e.stopPropagation();
                            navigate(`/accounts/${block.witness}`);
                          }}
                          className="text-primary hover:underline"
                        >
                          @{block.witness}
                        </button>
                      </td>
                      <td className="p-3 text-right">{formatNumber(block.transaction_count || 0)}</td>
                      <td className="p-3 text-right">{formatNumber(block.operation_count || 0)}</td>
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

export function BlockDetailPage() {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const blockNumber = Number(searchParams.get('id') || window.location.pathname.split('/').pop());

  const { data, isLoading, error } = useQuery({
    queryKey: ['block', blockNumber],
    queryFn: () => getBlock(blockNumber),
    enabled: !!blockNumber,
  });

  if (isLoading) {
    return (
      <div className="space-y-6">
        <div className="text-center py-12">
          <div className="text-muted-foreground">Loading block details...</div>
        </div>
      </div>
    );
  }

  if (error || !data?.success || !data.data) {
    return (
      <div className="space-y-6">
        <Card>
          <CardContent className="py-12">
            <div className="text-center text-muted-foreground">
              Block not found or failed to load.
            </div>
          </CardContent>
        </Card>
      </div>
    );
  }

  const block = data.data;

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <Button variant="ghost" onClick={() => navigate('/blocks')} className="mb-2">
            ← Back to Blocks
          </Button>
          <h1 className="text-3xl font-bold tracking-tight">Block #{formatNumber(block.block_num)}</h1>
          <p className="text-muted-foreground">Block details and transactions</p>
        </div>
      </div>

      <div className="grid gap-4 md:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle>Block Information</CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            <div className="flex justify-between">
              <span className="text-muted-foreground">Block Number</span>
              <span className="font-medium">#{formatNumber(block.block_num)}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-muted-foreground">Timestamp</span>
              <span className="font-medium">{formatDate(block.timestamp)}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-muted-foreground">Witness</span>
              <button
                onClick={() => navigate(`/accounts/${block.witness}`)}
                className="font-medium text-primary hover:underline"
              >
                @{block.witness}
              </button>
            </div>
            <div className="flex justify-between">
              <span className="text-muted-foreground">Previous Block</span>
              <button
                onClick={() => navigate(`/blocks/${Number(block.block_num) - 1}`)}
                className="font-medium text-primary hover:underline"
              >
                #{formatNumber(Number(block.block_num) - 1)}
              </button>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Statistics</CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            <div className="flex justify-between">
              <span className="text-muted-foreground">Transactions</span>
              <span className="font-medium">{formatNumber(block.transaction_count || 0)}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-muted-foreground">Operations</span>
              <span className="font-medium">{formatNumber(block.operation_count || 0)}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-muted-foreground">Merkle Root</span>
              <span className="font-mono text-xs">{block.transaction_merkle_root?.substring(0, 16)}...</span>
            </div>
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Transactions</CardTitle>
          <CardDescription>
            {block.transactions?.length || 0} transactions in this block
          </CardDescription>
        </CardHeader>
        <CardContent>
          {!block.transactions || block.transactions.length === 0 ? (
            <div className="text-center py-8 text-muted-foreground">
              No transactions in this block (virtual operations only).
            </div>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full">
                <thead>
                  <tr className="border-b">
                    <th className="text-left p-3">#</th>
                    <th className="text-left p-3">Transaction ID</th>
                    <th className="text-left p-3">Operations</th>
                  </tr>
                </thead>
                <tbody>
                  {block.transactions.map((tx, index) => (
                    <tr key={tx.transaction_id} className="border-b">
                      <td className="p-3 text-muted-foreground">{index}</td>
                      <td className="p-3 font-mono text-xs">{tx.transaction_id}</td>
                      <td className="p-3">
                        <div className="flex flex-wrap gap-1">
                          {tx.operations?.map((op) => (
                            <Badge key={op.id || `${op.trx_index}-${op.op_index}`} variant="outline">
                              {op.op_type}
                            </Badge>
                          ))}
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
