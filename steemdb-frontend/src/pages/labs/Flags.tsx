import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { ArrowLeft, AlertTriangle } from 'lucide-react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../../components/ui/Card';
import { Button } from '../../components/ui/Button';
import { Badge } from '../../components/ui/Badge';
import { getFlags } from '../../lib/api';
import type { Flags } from '../../types';

export function FlagsPage() {
  const { data, isLoading, error } = useQuery({
    queryKey: ['flags'],
    queryFn: () => getFlags(),
  });

  if (isLoading) {
    return (
      <div className="space-y-6">
        <div className="text-center py-12">
          <div className="text-muted-foreground">Loading flags...</div>
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
              Failed to load flags.
            </div>
          </CardContent>
        </Card>
      </div>
    );
  }

  const flags = data.data || [];

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-4">
        <Link to="/labs">
          <Button variant="ghost" size="icon">
            <ArrowLeft className="h-4 w-4" />
          </Button>
        </Link>
        <div>
          <h1 className="text-3xl font-bold tracking-tight">Flags</h1>
          <p className="text-muted-foreground">Accounts receiving downvotes</p>
        </div>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Flagged Accounts</CardTitle>
          <CardDescription>
            Accounts with the most downvotes
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="space-y-4">
            {flags.length === 0 ? (
              <div className="text-center py-12 text-muted-foreground">
                No flags found.
              </div>
            ) : (
              flags.map((flag: Flags) => (
                <div key={flag.author} className="border rounded-lg p-4">
                  <div className="flex items-start justify-between mb-3">
                    <div className="flex items-center gap-3">
                      <div className="flex items-center justify-center w-8 h-8 rounded-full bg-destructive/10">
                        <AlertTriangle className="h-4 w-4 text-destructive" />
                      </div>
                      <div>
                        <Link
                          to={`/accounts/${flag.author}`}
                          className="font-medium hover:underline"
                        >
                          @{flag.author}
                        </Link>
                        <div className="text-sm text-muted-foreground">
                          {flag.count} downvote{flag.count !== 1 ? 's' : ''}
                        </div>
                      </div>
                    </div>
                    <Badge variant="destructive">{flag.count}</Badge>
                  </div>
                  {flag.voters && Object.keys(flag.voters).length > 0 && (
                    <div className="mt-3 pt-3 border-t">
                      <div className="text-sm text-muted-foreground mb-2">Top Flaggers:</div>
                      <div className="flex flex-wrap gap-2">
                        {Object.entries(flag.voters)
                          .slice(0, 10)
                          .map(([voter, count]) => (
                            <Link
                              key={voter}
                              to={`/accounts/${voter}`}
                              className="text-xs hover:underline"
                            >
                              @{voter} ({count})
                            </Link>
                          ))}
                      </div>
                    </div>
                  )}
                  {flag.posts && flag.posts.length > 0 && (
                    <div className="mt-2 text-xs text-muted-foreground">
                      {flag.posts.length} post{flag.posts.length !== 1 ? 's' : ''} flagged
                    </div>
                  )}
                </div>
              ))
            )}
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
