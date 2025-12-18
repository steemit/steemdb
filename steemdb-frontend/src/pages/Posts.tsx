import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useNavigate, useSearchParams, Link } from 'react-router-dom';
import { ArrowUpDown, MessageSquare, ThumbsUp, Clock } from 'lucide-react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../components/ui/Card';
import { Button } from '../components/ui/Button';
import { Badge } from '../components/ui/Badge';
import { getPosts } from '../lib/api';
import { formatNumber, formatTimeAgo, formatCurrency } from '../lib/utils';
import type { Post } from '../types';

export function PostsPage() {
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const [page, setPage] = useState(Number(searchParams.get('page')) || 1);
  const [sortBy, setSortBy] = useState(searchParams.get('sort_by') || 'created');
  const [sortOrder, setSortOrder] = useState<'asc' | 'desc'>((searchParams.get('sort_order') as 'asc' | 'desc') || 'desc');

  const { data, isLoading, error } = useQuery({
    queryKey: ['posts', page, sortBy, sortOrder],
    queryFn: () => getPosts({ page, limit: 20, sort_by: sortBy, sort_order: sortOrder }),
  });

  const handleSort = (field: string) => {
    if (sortBy === field) {
      setSortOrder(sortOrder === 'asc' ? 'desc' : 'asc');
    } else {
      setSortBy(field);
      setSortOrder('desc');
    }
    setPage(1);
  };

  const handlePageChange = (newPage: number) => {
    setPage(newPage);
    setSearchParams({ page: newPage.toString(), sort_by: sortBy, sort_order: sortOrder });
  };

  if (isLoading) {
    return (
      <div className="space-y-6">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-3xl font-bold tracking-tight">Posts</h1>
            <p className="text-muted-foreground">Browse Steem posts</p>
          </div>
        </div>
        <div className="text-center py-12">
          <div className="text-muted-foreground">Loading posts...</div>
        </div>
      </div>
    );
  }

  if (error || !data?.success) {
    return (
      <div className="space-y-6">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-3xl font-bold tracking-tight">Posts</h1>
            <p className="text-muted-foreground">Browse Steem posts</p>
          </div>
        </div>
        <Card>
          <CardContent className="py-12">
            <div className="text-center text-muted-foreground">
              Failed to load posts. Please try again later.
            </div>
          </CardContent>
        </Card>
      </div>
    );
  }

  const posts = data.data?.data || [];
  const totalPages = data.data?.totalPages || 1;
  const total = data.data?.total || 0;

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">Posts</h1>
          <p className="text-muted-foreground">Browse Steem posts and comments</p>
        </div>
      </div>

      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div>
              <CardTitle>Post List</CardTitle>
              <CardDescription>
                Showing {formatNumber((page - 1) * 20 + 1)}-{formatNumber(Math.min(page * 20, total))} of {formatNumber(total)} posts
              </CardDescription>
            </div>
          </div>
        </CardHeader>
        <CardContent>
          <div className="space-y-4">
            {/* Sort controls */}
            <div className="flex items-center gap-2 flex-wrap">
              <span className="text-sm text-muted-foreground">Sort by:</span>
              <Button
                variant="outline"
                size="sm"
                onClick={() => handleSort('created')}
                className="flex items-center gap-1"
              >
                Date
                {sortBy === 'created' && <ArrowUpDown className="h-3 w-3" />}
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={() => handleSort('net_votes')}
                className="flex items-center gap-1"
              >
                Votes
                {sortBy === 'net_votes' && <ArrowUpDown className="h-3 w-3" />}
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={() => handleSort('total_payout_value')}
                className="flex items-center gap-1"
              >
                Payout
                {sortBy === 'total_payout_value' && <ArrowUpDown className="h-3 w-3" />}
              </Button>
            </div>

            {/* Posts list */}
            {posts.length === 0 ? (
              <div className="text-center py-12 text-muted-foreground">
                No posts found.
              </div>
            ) : (
              <div className="space-y-4">
                {posts.map((post: Post) => (
                  <div
                    key={post.id}
                    className="border rounded-lg p-4 hover:bg-accent/50 transition-colors cursor-pointer"
                    onClick={() => navigate(`/posts/${post.author}/${post.permlink}`)}
                  >
                    <div className="flex items-start justify-between gap-4">
                      <div className="flex-1 min-w-0">
                        <div className="flex items-center gap-2 mb-2">
                          <Link
                            to={`/accounts/${post.author}`}
                            onClick={(e) => e.stopPropagation()}
                            className="font-medium hover:underline"
                          >
                            @{post.author}
                          </Link>
                          {post.category && (
                            <Badge variant="outline" className="text-xs">
                              {post.category}
                            </Badge>
                          )}
                          <span className="text-xs text-muted-foreground flex items-center gap-1">
                            <Clock className="h-3 w-3" />
                            {formatTimeAgo(post.created)}
                          </span>
                        </div>
                        <h3 className="font-semibold text-lg mb-2 line-clamp-2">
                          {post.title || post.permlink}
                        </h3>
                        {post.body && (
                          <p className="text-sm text-muted-foreground line-clamp-2 mb-2">
                            {post.body}
                          </p>
                        )}
                        <div className="flex items-center gap-4 text-sm text-muted-foreground">
                          <span className="flex items-center gap-1">
                            <ThumbsUp className="h-4 w-4" />
                            {formatNumber(post.net_votes)}
                          </span>
                          <span className="flex items-center gap-1">
                            <MessageSquare className="h-4 w-4" />
                            {formatNumber(post.children)}
                          </span>
                          <span>
                            {formatCurrency(post.total_payout_value + (post.pending_payout_value || 0))}
                          </span>
                        </div>
                      </div>
                    </div>
                  </div>
                ))}
              </div>
            )}

            {/* Pagination */}
            {totalPages > 1 && (
              <div className="flex items-center justify-center gap-2 pt-4">
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => handlePageChange(page - 1)}
                  disabled={page === 1}
                >
                  Previous
                </Button>
                <span className="text-sm text-muted-foreground">
                  Page {page} of {totalPages}
                </span>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => handlePageChange(page + 1)}
                  disabled={page >= totalPages}
                >
                  Next
                </Button>
              </div>
            )}
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
