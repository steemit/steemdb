import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { ArrowLeft, FlaskConical } from 'lucide-react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../../components/ui/Card';
import { Button } from '../../components/ui/Button';
import { getClients } from '../../lib/api';
import { formatNumber, formatCurrency } from '../../lib/utils';
import type { Clients } from '../../types';

const TOP_CLIENTS = 50;

/** YYYY-MM-DD of an ISO timestamp, for window captions. */
function dayOf(iso?: string): string | undefined {
  return iso ? iso.slice(0, 10) : undefined;
}

export function ClientsPage() {
  const { data, isLoading, error } = useQuery({
    queryKey: ['clients'],
    queryFn: () => getClients(),
  });

  if (isLoading) {
    return (
      <div className="space-y-6">
        <div className="text-center py-12">
          <div className="text-muted-foreground">Loading client statistics...</div>
        </div>
      </div>
    );
  }

  if (error || !data?.success) {
    return (
      <div className="space-y-6">
        <Card>
          <CardContent className="py-12">
            <div className="text-center text-muted-foreground">
              Failed to load client statistics.
            </div>
          </CardContent>
        </Card>
      </div>
    );
  }

  const clients = data.data as Clients;

  // Sort clients by posts
  const sortedPosts = Object.entries(clients.posts || {})
    .sort(([, a], [, b]) => b - a)
    .slice(0, TOP_CLIENTS);

  const sortedRewards = Object.entries(clients.rewards || {})
    .sort(([, a], [, b]) => b - a)
    .slice(0, TOP_CLIENTS);

  const clientCount = Object.keys(clients.posts || {}).length;
  const windowFrom = dayOf(clients.window_from);
  const windowTo = dayOf(clients.window_to);
  const updatedAt = clients.updated_at
    ? `${clients.updated_at.slice(0, 16).replace('T', ' ')} UTC`
    : undefined;
  // Both counters count comments (posts + replies). A snapshot written before
  // the metadata existed has no counters at all, so both read 0 — that must not
  // render as "0 of 0 comments name a client", which reads as a coverage
  // finding instead of a missing field. There is no way to tell such a snapshot
  // from an empty window, and claiming nothing is the honest option for both.
  const unattributed = clients.posts_scanned - clients.posts_with_app;
  const attributedShare =
    clients.posts_scanned > 0
      ? ((clients.posts_with_app / clients.posts_scanned) * 100).toFixed(1)
      : undefined;

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-4">
        <Link to="/labs">
          <Button variant="ghost" size="icon">
            <ArrowLeft className="h-4 w-4" />
          </Button>
        </Link>
        <div>
          <h1 className="text-3xl font-bold tracking-tight">Clients</h1>
          <p className="text-muted-foreground">Client usage statistics</p>
        </div>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Coverage</CardTitle>
        </CardHeader>
        <CardContent className="space-y-1 text-sm text-muted-foreground">
          <p>
            {windowFrom && windowTo ? `${windowFrom} to ${windowTo} (UTC)` : 'Last 90 days (UTC)'}
            {' · '}
            {formatNumber(clientCount)} clients
            {clientCount > TOP_CLIENTS ? ` · top ${TOP_CLIENTS} shown` : ''}
          </p>
          {clients.posts_scanned > 0 && (
            <p>
              Attribution comes from each comment&apos;s <code>json_metadata.app</code>:{' '}
              {formatNumber(clients.posts_with_app)} of {formatNumber(clients.posts_scanned)}{' '}
              comments (posts and replies) name a client
              {attributedShare ? ` (${attributedShare}%)` : ''}
              {unattributed > 0
                ? `, the other ${formatNumber(unattributed)} are not counted below`
                : ''}
              .
            </p>
          )}
          {updatedAt && <p>Snapshot refreshed {updatedAt}.</p>}
        </CardContent>
      </Card>

      {/* Top by Posts */}
      <Card>
        <CardHeader>
          <CardTitle>Top Clients by Posts</CardTitle>
          <CardDescription>Posts published by each client in the window</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="space-y-3">
            {sortedPosts.length === 0 ? (
              <div className="text-center py-12 text-muted-foreground">
                No client data available.
              </div>
            ) : (
              sortedPosts.map(([client, count]) => (
                <div
                  key={client}
                  className="flex items-center justify-between p-3 border rounded"
                >
                  <div className="flex items-center gap-3">
                    <div className="flex items-center justify-center w-8 h-8 rounded-full bg-primary/10">
                      <FlaskConical className="h-4 w-4 text-primary" />
                    </div>
                    <div>
                      <div className="font-medium">{client}</div>
                    </div>
                  </div>
                  <div className="font-semibold">{formatNumber(count)} posts</div>
                </div>
              ))
            )}
          </div>
        </CardContent>
      </Card>

      {/* Top by Rewards */}
      <Card>
        <CardHeader>
          <CardTitle>Top Clients by Rewards</CardTitle>
          {/* SBD, not STEEM: the snapshot sums total_payout_value +
              pending_payout_value, both SBD assets (labelled STEEM before,
              inflating every figure). */}
          <CardDescription>
            Author rewards (SBD) earned by posts published through each client
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="space-y-3">
            {sortedRewards.length === 0 ? (
              <div className="text-center py-12 text-muted-foreground">
                No reward data available.
              </div>
            ) : (
              sortedRewards.map(([client, reward]) => (
                <div
                  key={client}
                  className="flex items-center justify-between p-3 border rounded"
                >
                  <div className="flex items-center gap-3">
                    <div className="flex items-center justify-center w-8 h-8 rounded-full bg-primary/10">
                      <FlaskConical className="h-4 w-4 text-primary" />
                    </div>
                    <div>
                      <div className="font-medium">{client}</div>
                    </div>
                  </div>
                  <div className="font-semibold">{formatCurrency(reward, 'SBD')}</div>
                </div>
              ))
            )}
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
