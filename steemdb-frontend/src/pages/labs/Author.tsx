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

  const leaderboard: AuthorLeaderboard[] = data?.success ? data.data || [] : [];
  // The failure state is rendered inside the results card, not instead of the
  // page: a single failed request used to unmount the filters too, leaving
  // "Failed to load author leaderboard." as the only thing on screen with no
  // way back to another day or grouping.
  const failed = Boolean(error) || Boolean(data && !data.success);

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
                // `date` always holds YYYY-MM-DD so switching grouping can
                // never send a value the API rejects; the month control reads
                // and writes the YYYY-MM prefix of the same state.
                value={grouping === 'monthly' ? date.slice(0, 7) : date}
                onChange={(e) =>
                  setDate(grouping === 'monthly' ? `${e.target.value}-01` : e.target.value)
                }
                className="w-48"
              />
              <span className="text-sm text-muted-foreground">UTC</span>
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
          <p className="mt-3 text-sm text-muted-foreground">
            Top 200 authors per window. Rewards are converted to STEEM at the current vests ratio
            and median SBD price, so all three payout legs are comparable.
          </p>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Top Authors</CardTitle>
          <CardDescription>
            {failed
              ? 'Failed to load author leaderboard.'
              : `Top ${leaderboard.length} authors`}
          </CardDescription>
        </CardHeader>
        <CardContent>
          {failed ? (
            <div className="text-center py-12 text-muted-foreground">
              The leaderboard for {grouping === 'monthly' ? date.slice(0, 7) : date} (UTC) could
              not be loaded. Pick another day or grouping above.
            </div>
          ) : isLoading ? (
            <div className="text-center py-12 text-muted-foreground">
              Loading author leaderboard...
            </div>
          ) : (
            <div className="space-y-4">
              {leaderboard.length === 0 ? (
                <div className="text-center py-12 text-muted-foreground">
                  No author data found.
                </div>
              ) : (
                leaderboard.map((entry: AuthorLeaderboard, index: number) => (
                  <div key={entry.author} className="border rounded-lg p-4">
                    <div className="flex items-start justify-between mb-3">
                      <div className="flex items-center gap-4">
                        <div className="flex items-center justify-center w-8 h-8 rounded-full bg-primary/10">
                          <span className="text-sm font-semibold text-primary">{index + 1}</span>
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
                        {/* STEEM equivalent of all three payout legs — the
                            value the board is ranked by. Showing only steem+sbd
                            here printed "0.000" for 100% power-up authors. */}
                        <div className="font-semibold text-lg">
                          {formatCurrency(entry.total_steem)}
                        </div>
                        <div className="text-xs text-muted-foreground">
                          {formatCurrency(entry.vest, 'VESTS')} +{' '}
                          {formatCurrency(entry.steem)} + {formatCurrency(entry.sbd, 'SBD')}
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
                          {formatCurrency(entry.post_vest, 'VESTS')}
                        </div>
                      </div>
                      <div>
                        <div className="text-muted-foreground flex items-center gap-1">
                          <MessageSquare className="h-3 w-3" />
                          Replies
                        </div>
                        <div className="font-medium">{entry.replies}</div>
                        <div className="text-xs text-muted-foreground">
                          {formatCurrency(entry.reply_vest, 'VESTS')}
                        </div>
                      </div>
                      <div>
                        <div className="text-muted-foreground">Liquid Rewards</div>
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
          )}
        </CardContent>
      </Card>
    </div>
  );
}
