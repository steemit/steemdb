import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { ChevronRight, ArrowUpDown } from 'lucide-react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../components/ui/Card';
import { Button } from '../components/ui/Button';
import { getBlocks } from '../lib/api';
import { formatNumber, formatTimeAgo } from '../lib/utils';

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
