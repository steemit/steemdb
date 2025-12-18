import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { ArrowLeft, Calendar } from 'lucide-react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../../components/ui/Card';
import { Button } from '../../components/ui/Button';
import { Input } from '../../components/ui/Input';
import { getRshares } from '../../lib/api';
import { formatNumber, getAvatarUrl } from '../../lib/utils';
import type { RsharesAllocation } from '../../types';

export function RsharesPage() {
  const [date, setDate] = useState(new Date().toISOString().split('T')[0]);

  const { data, isLoading, error } = useQuery({
    queryKey: ['rshares', date],
    queryFn: () => getRshares(date),
  });

  if (isLoading) {
    return (
      <div className="space-y-6">
        <div className="text-center py-12">
          <div className="text-muted-foreground">Loading rshares allocation...</div>
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
              Failed to load rshares allocation.
            </div>
          </CardContent>
        </Card>
      </div>
    );
  }

  const allocations = data.data || [];
  const totalRshares = allocations.reduce((sum, a) => sum + a.rshares, 0);

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-4">
        <Link to="/labs">
          <Button variant="ghost" size="icon">
            <ArrowLeft className="h-4 w-4" />
          </Button>
        </Link>
        <div>
          <h1 className="text-3xl font-bold tracking-tight">Rshares Allocation</h1>
          <p className="text-muted-foreground">Rshares distribution analysis</p>
        </div>
      </div>

      {/* Date selector */}
      <Card>
        <CardHeader>
          <CardTitle>Select Date</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex items-center gap-4">
            <Calendar className="h-4 w-4 text-muted-foreground" />
            <Input
              type="date"
              value={date}
              onChange={(e) => setDate(e.target.value)}
              className="w-48"
            />
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Rshares Allocation</CardTitle>
          <CardDescription>
            Total rshares: {formatNumber(totalRshares)}
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="space-y-3">
            {allocations.length === 0 ? (
              <div className="text-center py-12 text-muted-foreground">
                No rshares data found for this date.
              </div>
            ) : (
              allocations.map((allocation: RsharesAllocation, index: number) => (
                <div
                  key={allocation.voter}
                  className="flex items-center justify-between p-4 border rounded-lg"
                >
                  <div className="flex items-center gap-4">
                    <div className="flex items-center justify-center w-8 h-8 rounded-full bg-primary/10">
                      <span className="text-sm font-semibold text-primary">
                        {index + 1}
                      </span>
                    </div>
                    <div className="flex items-center gap-3">
                      {allocation.account && (
                        <img
                          src={getAvatarUrl(allocation.voter)}
                          alt={allocation.voter}
                          className="h-8 w-8 rounded-full"
                          onError={(e) => {
                            (e.target as HTMLImageElement).style.display = 'none';
                          }}
                        />
                      )}
                      <div>
                        <Link
                          to={`/accounts/${allocation.voter}`}
                          className="font-medium hover:underline"
                        >
                          @{allocation.voter}
                        </Link>
                        <div className="text-sm text-muted-foreground">
                          {allocation.votes} vote{allocation.votes !== 1 ? 's' : ''}
                        </div>
                      </div>
                    </div>
                  </div>
                  <div className="text-right">
                    <div className="font-semibold text-lg">
                      {formatNumber(allocation.rshares)} rshares
                    </div>
                    {totalRshares > 0 && (
                      <div className="text-sm text-muted-foreground">
                        {((allocation.rshares / totalRshares) * 100).toFixed(2)}%
                      </div>
                    )}
                  </div>
                </div>
              ))
            )}
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
