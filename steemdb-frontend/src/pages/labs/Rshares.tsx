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

/**
 * The date control is a UTC day: the API window is [00:00, 24:00) UTC, so the
 * default is today-in-UTC (not the browser's local day, which would ask for a
 * day that has not started yet during CST early hours).
 */
function utcToday(): string {
  return new Date().toISOString().split('T')[0];
}

export function RsharesPage() {
  const [date, setDate] = useState(utcToday());

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

  const board = data.data;
  const allocations = board?.allocations || [];
  // Percentages come from the server-side total: the rows are capped, so
  // summing `allocations` would make the shares add up to far more than 100%.
  const totalRshares = board?.total_rshares ?? 0;
  const dataAsOf = board?.data_as_of ? new Date(board.data_as_of) : undefined;

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
          <p className="text-muted-foreground">
            Rshares received by posts created on the selected day — including votes cast on
            later days
          </p>
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
            <span className="text-sm text-muted-foreground">UTC day</span>
          </div>
          <p className="mt-3 text-sm text-muted-foreground">
            A post pays out over 7 days, so a day&apos;s rshares keep growing after it ends.
          </p>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Rshares Allocation</CardTitle>
          <CardDescription>
            Total rshares: {formatNumber(totalRshares)} from {formatNumber(board?.total_votes ?? 0)}{' '}
            votes by {formatNumber(board?.voters ?? 0)} voters
            {dataAsOf && (
              <>
                {' · '}data as of {dataAsOf.toISOString().slice(0, 16).replace('T', ' ')} UTC
              </>
            )}
          </CardDescription>
        </CardHeader>
        <CardContent>
          {dataAsOf && (
            <p className="mb-4 text-sm text-muted-foreground">
              Vote snapshots refresh on a ~6 hour cycle, so the most recent hours of an ongoing
              day are not counted yet.
            </p>
          )}
          <div className="space-y-3">
            {allocations.length === 0 ? (
              <div className="text-center py-12 text-muted-foreground">
                No rshares data found for this day. The explorer only holds comments from the
                moment its database starts, so days before that are empty rather than quiet.
              </div>
            ) : (
              <>
                {allocations.length < (board?.voters ?? allocations.length) && (
                  <div className="pb-2 text-sm text-muted-foreground">
                    Showing top {formatNumber(allocations.length)} of{' '}
                    {formatNumber(board?.voters ?? allocations.length)} voters
                  </div>
                )}
                {allocations.map((allocation: RsharesAllocation, index: number) => (
                  <div
                    key={allocation.voter}
                    className="flex items-center justify-between p-4 border rounded-lg"
                  >
                    <div className="flex items-center gap-4">
                      <div className="flex items-center justify-center w-8 h-8 rounded-full bg-primary/10">
                        <span className="text-sm font-semibold text-primary">{index + 1}</span>
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
                ))}
              </>
            )}
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
