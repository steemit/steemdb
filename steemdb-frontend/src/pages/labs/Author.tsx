import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { ArrowLeft, Calendar, FileText, MessageSquare } from 'lucide-react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../../components/ui/Card';
import { Button } from '../../components/ui/Button';
import { Input } from '../../components/ui/Input';
import { getAuthor } from '../../lib/api';
import { formatCurrency, getAvatarUrl } from '../../lib/utils';
import type { AuthorLeaderboard } from '../../types';

export function AuthorPage() {
  const [date, setDate] = useState(new Date().toISOString().split('T')[0]);
  const [grouping, setGrouping] = useState<'daily' | 'monthly'>('daily');

  const { data, isLoading, error } = useQuery({
    queryKey: ['author', date, grouping],
    queryFn: () => getAuthor(date, grouping),
  });

  if (isLoading) {
    return (
      <div className="space-y-6">
        <div className="text-center py-12">
          <div className="text-muted-foreground">Loading author leaderboard...</div>
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
              Failed to load author leaderboard.
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
          <h1 className="text-3xl font-bold tracking-tight">Author Leaderboard</h1>
          <p className="text-muted-foreground">Top authors by reward</p>
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
          <CardTitle>Top Authors</CardTitle>
          <CardDescription>
            Showing top {leaderboard.length} authors
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="space-y-4">
            {leaderboard.length === 0 ? (
              <div className="text-center py-12 text-muted-foreground">
                No author data found.
              </div>
            ) : (
              leaderboard.map((entry: AuthorLeaderboard, index: number) => (
                <div
                  key={entry.author}
                  className="border rounded-lg p-4"
                >
                  <div className="flex items-start justify-between mb-3">
                    <div className="flex items-center gap-4">
                      <div className="flex items-center justify-center w-8 h-8 rounded-full bg-primary/10">
                        <span className="text-sm font-semibold text-primary">
                          {index + 1}
                        </span>
                      </div>
                      <div className="flex items-center gap-3">
                        {entry.account && (
                          <img
                            src={getAvatarUrl(entry.author)}
                            alt={entry.author}
                            className="h-8 w-8 rounded-full"
                            onError={(e) => {
                              (e.target as HTMLImageElement).style.display = 'none';
                            }}
                          />
                        )}
                        <Link
                          to={`/accounts/${entry.author}`}
                          className="font-medium hover:underline"
                        >
                          @{entry.author}
                        </Link>
                      </div>
                    </div>
                    <div className="text-right">
                      <div className="font-semibold text-lg">
                        {formatCurrency(entry.vest)} VESTS
                      </div>
                    </div>
                  </div>
                  <div className="grid grid-cols-2 md:grid-cols-4 gap-4 text-sm">
                    <div>
                      <div className="text-muted-foreground flex items-center gap-1">
                        <FileText className="h-3 w-3" />
                        Posts
                      </div>
                      <div className="font-medium">{entry.posts}</div>
                      <div className="text-xs text-muted-foreground">
                        {formatCurrency(entry.post_vest)} VESTS
                      </div>
                    </div>
                    <div>
                      <div className="text-muted-foreground flex items-center gap-1">
                        <MessageSquare className="h-3 w-3" />
                        Replies
                      </div>
                      <div className="font-medium">{entry.replies}</div>
                      <div className="text-xs text-muted-foreground">
                        {formatCurrency(entry.reply_vest)} VESTS
                      </div>
                    </div>
                    <div>
                      <div className="text-muted-foreground">Total Rewards</div>
                      <div className="font-medium">{formatCurrency(entry.steem)}</div>
                      <div className="text-xs text-muted-foreground">
                        {formatCurrency(entry.sbd, 'SBD')}
                      </div>
                    </div>
                    <div>
                      <div className="text-muted-foreground">Total Count</div>
                      <div className="font-medium">{entry.count}</div>
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
