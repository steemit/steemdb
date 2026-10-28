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
  // The oldest vote snapshot among the day's posts: the instant up to which the
  // whole day is complete. Absent means no post of that day has been rescanned,
  // and the page must then claim no freshness at all.
  const votesCompleteAsOf = board?.votes_complete_as_of
    ? new Date(board.votes_complete_as_of)
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
          <h1 className="text-3xl font-bold tracking-tight">Rshares Allocation</h1>
          <p className="text-muted-foreground">
            Cumulative rshares of posts created on the selected day — including the votes they
            receive over the following 7 days
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
        </CardContent>
      </Card>

      {/* What the board counts — the label alone ("rshares of a day") reads as
          "votes cast that day", which is a different and much smaller number. */}
      <Card>
        <CardHeader>
          <CardTitle>What This Measures</CardTitle>
          <CardDescription>
            How to read a row of this board, and what it leaves out
          </CardDescription>
        </CardHeader>
        <CardContent>
          <ul className="list-disc space-y-3 pl-5 text-sm text-muted-foreground">
            <li>
              <span className="font-medium text-foreground">Rshares are vote weight, not money.</span>{' '}
              A vote turns part of the voter&apos;s mana into rshares — a full-weight vote spends 2%
              of it — denominated in millionths of a VEST. They only decide how large a slice of the
              reward fund a post can claim when it pays out; they are not a STEEM, SBD or SP amount.
            </li>
            <li>
              <span className="font-medium text-foreground">
                This is the content view, not the voting view.
              </span>{' '}
              Each row is the rshares a voter put into posts <em>published</em> on the selected day.
              A vote cast today on a post from six days ago counts under that post&apos;s
              publication day, not today — so these totals are not &quot;rshares cast on this
              day&quot;, and the two figures differ substantially.
            </li>
            <li>
              <span className="font-medium text-foreground">The day keeps growing for a week.</span>{' '}
              A post pays out over 7 days, so a day&apos;s total keeps rising for 7 days after it
              ends and only then settles: a recent day is a partial figure next to a settled one.
              The numbers always describe each post&apos;s vote list as it stands now, not a
              historical snapshot — Steem keeps no such series on chain.
            </li>
            <li>
              <span className="font-medium text-foreground">Only upvotes are counted.</span>{' '}
              Downvotes are left out of both the totals and the vote counts, so the board shows
              gross upvote weight, not a net figure.
            </li>
            <li>
              <span className="font-medium text-foreground">Percentages are of the whole day.</span>{' '}
              A share is measured against every voter of that day, not against the rows listed
              here — only the top rows are shown.
            </li>
          </ul>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Rshares Allocation</CardTitle>
          <CardDescription>
            Total rshares: {formatNumber(totalRshares)} from {formatNumber(board?.total_votes ?? 0)}{' '}
            votes by {formatNumber(board?.voters ?? 0)} voters
            {votesCompleteAsOf && (
              <>
                {' · '}votes complete as of{' '}
                {votesCompleteAsOf.toISOString().slice(0, 16).replace('T', ' ')} UTC
              </>
            )}
          </CardDescription>
        </CardHeader>
        <CardContent>
          {votesCompleteAsOf && (
            <p className="mb-4 text-sm text-muted-foreground">
              Each post&apos;s votes are refreshed on a ~6 hour cycle. The timestamp above is the
              oldest snapshot among this day&apos;s posts, so the day is complete up to that point:
              later votes — and posts not yet snapshotted at all — may be missing.
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
