import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { ArrowLeft, FlaskConical } from 'lucide-react';
import { Card, CardContent, CardHeader, CardTitle } from '../../components/ui/Card';
import { Button } from '../../components/ui/Button';
import { getClients } from '../../lib/api';
import { formatNumber, formatCurrency } from '../../lib/utils';
import type { Clients } from '../../types';

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
    .slice(0, 50);

  const sortedRewards = Object.entries(clients.rewards || {})
    .sort(([, a], [, b]) => b - a)
    .slice(0, 50);

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

      {/* Top by Posts */}
      <Card>
        <CardHeader>
          <CardTitle>Top Clients by Posts</CardTitle>
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
                  <div className="font-semibold">{formatCurrency(reward)}</div>
                </div>
              ))
            )}
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
