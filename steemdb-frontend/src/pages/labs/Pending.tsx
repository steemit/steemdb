import { useQuery } from '@tanstack/react-query';
import { Link, useNavigate } from 'react-router-dom';
import { ArrowLeft, Clock, User } from 'lucide-react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../../components/ui/Card';
import { Button } from '../../components/ui/Button';
import { getPending } from '../../lib/api';
import { formatCurrency, formatTimeAgo, getAvatarUrl } from '../../lib/utils';
import type { PendingPost } from '../../types';

export function PendingPage() {
  const navigate = useNavigate();

  const { data, isLoading, error } = useQuery({
    queryKey: ['pending'],
    queryFn: () => getPending(),
  });

  if (isLoading) {
    return (
      <div className="space-y-6">
        <div className="text-center py-12">
          <div className="text-muted-foreground">Loading pending posts...</div>
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
              Failed to load pending posts.
            </div>
          </CardContent>
        </Card>
      </div>
    );
  }

  const pending = data.data || [];

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-4">
        <Link to="/labs">
          <Button variant="ghost" size="icon">
            <ArrowLeft className="h-4 w-4" />
          </Button>
        </Link>
        <div>
          <h1 className="text-3xl font-bold tracking-tight">Pending Posts</h1>
          <p className="text-muted-foreground">Posts awaiting payout (6-7 days old)</p>
        </div>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Pending Payout Posts</CardTitle>
          <CardDescription>
            Posts created 6-7 days ago, sorted by pending payout value
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="space-y-3">
            {pending.length === 0 ? (
              <div className="text-center py-12 text-muted-foreground">
                No pending posts found.
              </div>
            ) : (
              pending.map((post: PendingPost) => (
                <div
                  key={post.id}
                  className="border rounded-lg p-4 hover:bg-accent/50 transition-colors cursor-pointer"
                  onClick={() => navigate(`/posts/${post.author}/${post.permlink}`)}
                >
                  <div className="flex items-start justify-between">
                    <div className="flex-1 min-w-0">
                      <div className="flex items-center gap-2 mb-2">
                        <img
                          src={getAvatarUrl(post.author)}
                          alt={post.author}
                          className="h-6 w-6 rounded-full"
                          onError={(e) => {
                            (e.target as HTMLImageElement).style.display = 'none';
                          }}
                        />
                        <Link
                          to={`/accounts/${post.author}`}
                          onClick={(e) => e.stopPropagation()}
                          className="font-medium hover:underline flex items-center gap-1"
                        >
                          <User className="h-3 w-3" />
                          @{post.author}
                        </Link>
                        <span className="text-xs text-muted-foreground flex items-center gap-1">
                          <Clock className="h-3 w-3" />
                          {formatTimeAgo(post.created)}
                        </span>
                      </div>
                      <h3 className="font-semibold mb-2 line-clamp-2">
                        {post.title || post.permlink}
                      </h3>
                    </div>
                    <div className="text-right ml-4">
                      <div className="font-semibold text-lg">
                        {formatCurrency(post.total_pending_payout_value)}
                      </div>
                      <div className="text-xs text-muted-foreground">
                        Pending
                      </div>
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
