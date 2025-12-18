import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { ArrowLeft, Award, Calendar } from 'lucide-react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../../components/ui/Card';
import { Button } from '../../components/ui/Button';
import { Input } from '../../components/ui/Input';
import { getCuration } from '../../lib/api';
import { formatCurrency, getAvatarUrl } from '../../lib/utils';
import type { CurationLeaderboard } from '../../types';

export function CurationPage() {
  const [date, setDate] = useState(new Date().toISOString().split('T')[0]);
  const [grouping, setGrouping] = useState<'daily' | 'monthly'>('daily');

  const { data, isLoading, error } = useQuery({
    queryKey: ['curation', date, grouping],
    queryFn: () => getCuration(date, grouping),
  });

  if (isLoading) {
    return (
      <div className="space-y-6">
        <div className="text-center py-12">
          <div className="text-muted-foreground">Loading curation leaderboard...</div>
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
              Failed to load curation leaderboard.
            </div>
          </CardContent>
        </Card>
      </div>
    );
  }

  const leaderboard = data.data || [];

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-4">
        <Link to="/labs">
          <Button variant="ghost" size="icon">
            <ArrowLeft className="h-4 w-4" />
          </Button>
        </Link>
        <div>
          <h1 className="text-3xl font-bold tracking-tight">Curation Leaderboard</h1>
          <p className="text-muted-foreground">Top curators by reward</p>
        </div>
      </div>

      {/* Controls */}
      <Card>
        <CardHeader>
          <CardTitle>Filters</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex items-center gap-4">
            <div className="flex items-center gap-2">
              <Calendar className="h-4 w-4 text-muted-foreground" />
              <Input
                type={grouping === 'monthly' ? 'month' : 'date'}
                value={date}
                onChange={(e) => setDate(e.target.value)}
                className="w-48"
              />
            </div>
            <div className="flex items-center gap-2">
              <span className="text-sm text-muted-foreground">Grouping:</span>
              <Button
                variant={grouping === 'daily' ? 'default' : 'outline'}
                size="sm"
                onClick={() => setGrouping('daily')}
              >
                Daily
              </Button>
              <Button
                variant={grouping === 'monthly' ? 'default' : 'outline'}
                size="sm"
                onClick={() => setGrouping('monthly')}
              >
                Monthly
              </Button>
            </div>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Top Curators</CardTitle>
          <CardDescription>
            Showing top {leaderboard.length} curators
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="space-y-3">
            {leaderboard.length === 0 ? (
              <div className="text-center py-12 text-muted-foreground">
                No curation data found.
              </div>
            ) : (
              leaderboard.map((entry: CurationLeaderboard) => (
                <div
                  key={entry.curator}
                  className="flex items-center justify-between p-4 border rounded-lg"
                >
                  <div className="flex items-center gap-4">
                    <div className="flex items-center justify-center w-8 h-8 rounded-full bg-primary/10">
                      <Award className="h-4 w-4 text-primary" />
                    </div>
                    <div className="flex items-center gap-3">
                      {entry.account && (
                        <img
                          src={getAvatarUrl(entry.curator)}
                          alt={entry.curator}
                          className="h-8 w-8 rounded-full"
                          onError={(e) => {
                            (e.target as HTMLImageElement).style.display = 'none';
                          }}
                        />
                      )}
                      <div>
                        <Link
                          to={`/accounts/${entry.curator}`}
                          className="font-medium hover:underline"
                        >
                          @{entry.curator}
                        </Link>
                        <div className="text-sm text-muted-foreground">
                          {entry.count} curation{entry.count !== 1 ? 's' : ''}
                          {entry.authors && entry.authors.length > 0 && (
                            <> • {entry.authors.length} author{entry.authors.length !== 1 ? 's' : ''}</>
                          )}
                        </div>
                      </div>
                    </div>
                  </div>
                  <div className="text-right">
                    <div className="font-semibold text-lg">
                      {formatCurrency(entry.total)}
                    </div>
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
