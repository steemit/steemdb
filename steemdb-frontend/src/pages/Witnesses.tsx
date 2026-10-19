import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { ChevronRight, ArrowUpDown, ExternalLink, CheckCircle, XCircle } from 'lucide-react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../components/ui/Card';
import { Button } from '../components/ui/Button';
import { Badge } from '../components/ui/Badge';
import { getWitnesses } from '../lib/api';
import { formatNumber, getAvatarUrl } from '../lib/utils';

export function WitnessesPage() {
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const [page, setPage] = useState(Number(searchParams.get('page')) || 1);
  const [sortBy, setSortBy] = useState(searchParams.get('sort') || 'votes');
  const [sortOrder, setSortOrder] = useState<'asc' | 'desc'>((searchParams.get('order') as 'asc' | 'desc') || 'desc');

  const { data, isLoading, error } = useQuery({
    queryKey: ['witnesses', page, sortBy, sortOrder],
    queryFn: () => getWitnesses({ page, limit: 20, sort: sortBy, order: sortOrder }),
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
            <h1 className="text-3xl font-bold tracking-tight">Witnesses</h1>
            <p className="text-muted-foreground">Block producers on the Steem blockchain</p>
          </div>
        </div>
        <div className="text-center py-12">
          <div className="text-muted-foreground">Loading witnesses...</div>
        </div>
      </div>
    );
  }

  if (error || !data?.success) {
    return (
      <div className="space-y-6">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-3xl font-bold tracking-tight">Witnesses</h1>
            <p className="text-muted-foreground">Block producers on the Steem blockchain</p>
          </div>
        </div>
        <Card>
          <CardContent className="py-12">
            <div className="text-center text-muted-foreground">
              Failed to load witnesses. Please try again later.
            </div>
          </CardContent>
        </Card>
      </div>
    );
  }

  const witnesses = data.data || [];
  const totalPages = data.meta?.total_pages || 1;

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">Witnesses</h1>
          <p className="text-muted-foreground">Block producers on the Steem blockchain</p>
        </div>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Witness List</CardTitle>
          <CardDescription>Active and top witnesses</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="space-y-4">
            {/* Table */}
            <div className="overflow-x-auto">
              <table className="w-full">
                <thead>
                  <tr className="border-b">
                    <th className="text-left p-3">Witness</th>
                    <th className="text-left p-3">
                      <button
                        onClick={() => handleSort('votes')}
                        className="flex items-center space-x-1 hover:text-foreground"
                      >
                        <span>Votes</span>
                        <ArrowUpDown className="h-3 w-3" />
                      </button>
                    </th>
                    <th className="text-left p-3">Missed Blocks</th>
                    <th className="text-left p-3">Version</th>
                    <th className="text-right p-3"></th>
                  </tr>
                </thead>
                <tbody>
                  {witnesses.map((witness) => (
                    <tr
                      key={witness.owner}
                      className="border-b hover:bg-accent/50 cursor-pointer"
                      onClick={() => navigate(`/accounts/${witness.owner}`)}
                    >
                      <td className="p-3">
                        <div className="flex items-center space-x-3">
                          <img
                            src={getAvatarUrl(witness.owner)}
                            alt={witness.owner}
                            className="w-8 h-8 rounded-full"
                            onError={(e) => {
                              (e.target as HTMLImageElement).src = `https://ui-avatars.com/api/?name=${witness.owner}&background=random`;
                            }}
                          />
                          <div>
                            <div className="font-medium">@{witness.owner}</div>
                            {witness.url && (
                              <a
                                href={witness.url}
                                target="_blank"
                                rel="noopener noreferrer"
                                onClick={(e) => e.stopPropagation()}
                                className="text-xs text-muted-foreground hover:text-primary flex items-center space-x-1"
                              >
                                <span>{witness.url.replace(/^https?:\/\//, '').substring(0, 30)}</span>
                                <ExternalLink className="h-3 w-3" />
                              </a>
                            )}
                          </div>
                        </div>
                      </td>
                      <td className="p-3">
                        <div className="text-sm">{formatNumber(parseFloat(witness.votes || '0'))}</div>
                      </td>
                      <td className="p-3">
                        <div className="flex items-center space-x-2">
                          {witness.total_missed === 0 ? (
                            <CheckCircle className="h-4 w-4 text-green-500" />
                          ) : (
                            <XCircle className="h-4 w-4 text-red-500" />
                          )}
                          <span className="text-sm">{formatNumber(witness.total_missed || 0)}</span>
                        </div>
                      </td>
                      <td className="p-3">
                        <Badge variant="outline">{witness.running_version || 'N/A'}</Badge>
                      </td>
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
