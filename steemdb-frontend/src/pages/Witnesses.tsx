import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { ChevronRight, ArrowUpDown, Star, ExternalLink, CheckCircle, XCircle } from 'lucide-react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../components/ui/Card';
import { Button } from '../components/ui/Button';
import { Badge } from '../components/ui/Badge';
import { getWitnesses, getWitness } from '../lib/api';
import { formatNumber, formatDate, formatCurrency, getAvatarUrl } from '../lib/utils';
import { useFavoritesStore } from '../store';

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
                      onClick={() => navigate(`/witnesses/${witness.owner}`)}
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

export function WitnessDetailPage() {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const witnessName = searchParams.get('id') || window.location.pathname.split('/').pop() || '';
  const { addWitness, removeWitness, isWitnessFavorite } = useFavoritesStore();
  const isFavorite = isWitnessFavorite(witnessName);

  const { data, isLoading, error } = useQuery({
    queryKey: ['witness', witnessName],
    queryFn: () => getWitness(witnessName),
    enabled: !!witnessName,
  });

  if (isLoading) {
    return (
      <div className="space-y-6">
        <div className="text-center py-12">
          <div className="text-muted-foreground">Loading witness details...</div>
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
              Witness not found or failed to load.
            </div>
          </CardContent>
        </Card>
      </div>
    );
  }

  const witness = data.data;

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <Button variant="ghost" onClick={() => navigate('/witnesses')} className="mb-2">
            ← Back to Witnesses
          </Button>
          <div className="flex items-center space-x-4">
            <img
              src={getAvatarUrl(witness.owner)}
              alt={witness.owner}
              className="w-16 h-16 rounded-full"
              onError={(e) => {
                (e.target as HTMLImageElement).src = `https://ui-avatars.com/api/?name=${witness.owner}&background=random`;
              }}
            />
            <div>
              <div className="flex items-center space-x-2">
                <h1 className="text-3xl font-bold tracking-tight">@{witness.owner}</h1>
                <Button
                  variant="ghost"
                  size="icon"
                  onClick={() => {
                    if (isFavorite) {
                      removeWitness(witness.owner);
                    } else {
                      addWitness(witness.owner);
                    }
                  }}
                >
                  <Star className={`h-4 w-4 ${isFavorite ? 'fill-yellow-400 text-yellow-400' : ''}`} />
                </Button>
              </div>
              <p className="text-muted-foreground">Witness details and statistics</p>
            </div>
          </div>
        </div>
      </div>

      <div className="grid gap-4 md:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle>Witness Information</CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            <div className="flex justify-between">
              <span className="text-muted-foreground">Votes</span>
              <span className="font-medium">{formatNumber(parseFloat(witness.votes || '0'))}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-muted-foreground">Total Missed</span>
              <div className="flex items-center space-x-2">
                {witness.total_missed === 0 ? (
                  <CheckCircle className="h-4 w-4 text-green-500" />
                ) : (
                  <XCircle className="h-4 w-4 text-red-500" />
                )}
                <span className="font-medium">{formatNumber(witness.total_missed || 0)}</span>
              </div>
            </div>
            <div className="flex justify-between">
              <span className="text-muted-foreground">Last Confirmed Block</span>
              <span className="font-medium">#{formatNumber(witness.last_confirmed_block_num || 0)}</span>
            </div>
            {witness.url && (
              <div className="flex justify-between">
                <span className="text-muted-foreground">URL</span>
                <a
                  href={witness.url}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="font-medium text-primary hover:underline flex items-center space-x-1"
                >
                  <span>Visit</span>
                  <ExternalLink className="h-3 w-3" />
                </a>
              </div>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Technical Details</CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            <div className="flex justify-between">
              <span className="text-muted-foreground">Running Version</span>
              <Badge variant="outline">{witness.running_version || 'N/A'}</Badge>
            </div>
            <div className="flex justify-between">
              <span className="text-muted-foreground">Hardfork Version</span>
              <Badge variant="outline">{witness.hardfork_version_vote || 'N/A'}</Badge>
            </div>
            <div className="flex justify-between">
              <span className="text-muted-foreground">Last ASlot</span>
              <span className="font-medium">{formatNumber(witness.last_aslot || 0)}</span>
            </div>
            {witness.created && (
              <div className="flex justify-between">
                <span className="text-muted-foreground">Created</span>
                <span className="font-medium text-sm">{formatDate(witness.created)}</span>
              </div>
            )}
          </CardContent>
        </Card>

        {witness.props && (
          <Card>
            <CardHeader>
              <CardTitle>Witness Properties</CardTitle>
            </CardHeader>
            <CardContent className="space-y-3">
              <div className="flex justify-between">
                <span className="text-muted-foreground">Account Creation Fee</span>
                <span className="font-medium">{formatCurrency(witness.props.account_creation_fee)}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">Maximum Block Size</span>
                <span className="font-medium">{formatNumber(witness.props.maximum_block_size || 0)} bytes</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">SBD Interest Rate</span>
                <span className="font-medium">{witness.props.sbd_interest_rate || 0}%</span>
              </div>
            </CardContent>
          </Card>
        )}
      </div>
    </div>
  );
}
