import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useSearchParams, Link } from 'react-router-dom';
import { ArrowLeft, User } from 'lucide-react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../../components/ui/Card';
import { Button } from '../../components/ui/Button';
import { getPowerUps } from '../../lib/api';
import { formatCurrency, getAvatarUrl } from '../../lib/utils';
import type { PowerUp } from '../../types';

export function PowerUpPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const [filter, setFilter] = useState(searchParams.get('filter') || '');

  const { data, isLoading, error } = useQuery({
    queryKey: ['powerups', filter],
    queryFn: () => getPowerUps(filter || undefined),
  });

  const handleFilterChange = (newFilter: string) => {
    setFilter(newFilter);
    if (newFilter) {
      setSearchParams({ filter: newFilter });
    } else {
      setSearchParams({});
    }
  };

  if (isLoading) {
    return (
      <div className="space-y-6">
        <div className="text-center py-12">
          <div className="text-muted-foreground">Loading power ups...</div>
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
              Failed to load power ups.
            </div>
          </CardContent>
        </Card>
      </div>
    );
  }

  const powerUps = data.data || [];

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-4">
        <Link to="/labs">
          <Button variant="ghost" size="icon">
            <ArrowLeft className="h-4 w-4" />
          </Button>
        </Link>
        <div>
          <h1 className="text-3xl font-bold tracking-tight">Power Up</h1>
          <p className="text-muted-foreground">STEEM power up statistics</p>
        </div>
      </div>

      {/* Filter buttons */}
      <div className="flex items-center gap-2">
        <span className="text-sm text-muted-foreground">Filter:</span>
        <Button
          variant={filter === '' ? 'default' : 'outline'}
          size="sm"
          onClick={() => handleFilterChange('')}
        >
          All (30 days)
        </Button>
        <Button
          variant={filter === 'week' ? 'default' : 'outline'}
          size="sm"
          onClick={() => handleFilterChange('week')}
        >
          Week
        </Button>
        <Button
          variant={filter === 'day' ? 'default' : 'outline'}
          size="sm"
          onClick={() => handleFilterChange('day')}
        >
          Day
        </Button>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Top Power Ups</CardTitle>
          <CardDescription>
            Showing top {powerUps.length} accounts by total power up amount
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="space-y-4">
            {powerUps.length === 0 ? (
              <div className="text-center py-12 text-muted-foreground">
                No power ups found.
              </div>
            ) : (
              <div className="space-y-3">
                {powerUps.map((powerUp: PowerUp, index: number) => (
                  <div
                    key={powerUp.user}
                    className="flex items-center justify-between p-4 border rounded-lg"
                  >
                    <div className="flex items-center gap-4">
                      <div className="flex items-center justify-center w-8 h-8 rounded-full bg-primary/10">
                        <span className="text-sm font-semibold text-primary">
                          {index + 1}
                        </span>
                      </div>
                      <div className="flex items-center gap-3">
                        {powerUp.account && (
                          <img
                            src={getAvatarUrl(powerUp.user)}
                            alt={powerUp.user}
                            className="h-8 w-8 rounded-full"
                            onError={(e) => {
                              (e.target as HTMLImageElement).style.display = 'none';
                            }}
                          />
                        )}
                        <div>
                          <Link
                            to={`/accounts/${powerUp.user}`}
                            className="font-medium hover:underline flex items-center gap-2"
                          >
                            <User className="h-4 w-4" />
                            @{powerUp.user}
                          </Link>
                          <div className="text-sm text-muted-foreground">
                            {powerUp.count} power up{powerUp.count !== 1 ? 's' : ''}
                          </div>
                        </div>
                      </div>
                    </div>
                    <div className="text-right">
                      <div className="font-semibold text-lg">
                        {formatCurrency(powerUp.total)}
                      </div>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
