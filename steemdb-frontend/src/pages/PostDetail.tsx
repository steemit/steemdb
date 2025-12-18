import { useQuery } from '@tanstack/react-query';
import { useParams, useNavigate, Link } from 'react-router-dom';
import { ArrowLeft, ThumbsUp, MessageSquare, Repeat2, Clock } from 'lucide-react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../components/ui/Card';
import { Button } from '../components/ui/Button';
import { Badge } from '../components/ui/Badge';
import { getPost, getPostReplies, getPostVotes, getPostReblogs } from '../lib/api';
import { formatNumber, formatTimeAgo, formatDate, formatCurrency, getAvatarUrl } from '../lib/utils';
import type { Post, Vote, Reblog } from '../types';

export function PostDetailPage() {
  const { author, permlink } = useParams<{ author: string; permlink: string }>();
  const navigate = useNavigate();

  const { data: postData, isLoading: postLoading, error: postError } = useQuery({
    queryKey: ['post', author, permlink],
    queryFn: () => getPost(author!, permlink!),
    enabled: !!author && !!permlink,
  });

  const { data: repliesData } = useQuery({
    queryKey: ['post-replies', author, permlink],
    queryFn: () => getPostReplies(author!, permlink!),
    enabled: !!author && !!permlink,
  });

  const { data: votesData } = useQuery({
    queryKey: ['post-votes', author, permlink],
    queryFn: () => getPostVotes(author!, permlink!),
    enabled: !!author && !!permlink,
  });

  const { data: reblogsData } = useQuery({
    queryKey: ['post-reblogs', author, permlink],
    queryFn: () => getPostReblogs(author!, permlink!),
    enabled: !!author && !!permlink,
  });

  if (postLoading) {
    return (
      <div className="space-y-6">
        <div className="text-center py-12">
          <div className="text-muted-foreground">Loading post...</div>
        </div>
      </div>
    );
  }

  if (postError || !postData?.success || !postData.data) {
    return (
      <div className="space-y-6">
        <Card>
          <CardContent className="py-12">
            <div className="text-center text-muted-foreground">
              Post not found or failed to load.
            </div>
            <div className="text-center mt-4">
              <Button onClick={() => navigate('/posts')}>Back to Posts</Button>
            </div>
          </CardContent>
        </Card>
      </div>
    );
  }

  const post = postData.data;
  const replies = repliesData?.data || [];
  const votes = votesData?.data || [];
  const reblogs = reblogsData?.data || [];

  return (
    <div className="space-y-6">
      {/* Back button */}
      <Button variant="ghost" onClick={() => navigate('/posts')} className="flex items-center gap-2">
        <ArrowLeft className="h-4 w-4" />
        Back to Posts
      </Button>

      {/* Post content */}
      <Card>
        <CardHeader>
          <div className="flex items-start justify-between">
            <div className="flex-1">
              <div className="flex items-center gap-2 mb-2">
                <Link
                  to={`/accounts/${post.author}`}
                  className="font-medium hover:underline flex items-center gap-2"
                >
                  <img
                    src={getAvatarUrl(post.author)}
                    alt={post.author}
                    className="h-8 w-8 rounded-full"
                    onError={(e) => {
                      (e.target as HTMLImageElement).style.display = 'none';
                    }}
                  />
                  @{post.author}
                </Link>
                {post.category && (
                  <Badge variant="outline">{post.category}</Badge>
                )}
                <span className="text-sm text-muted-foreground flex items-center gap-1">
                  <Clock className="h-3 w-3" />
                  {formatDate(post.created)}
                </span>
              </div>
              <CardTitle className="text-2xl mb-2">{post.title || post.permlink}</CardTitle>
              <CardDescription>
                <div className="flex items-center gap-4 mt-2">
                  <span className="flex items-center gap-1">
                    <ThumbsUp className="h-4 w-4" />
                    {formatNumber(post.net_votes)} votes
                  </span>
                  <span className="flex items-center gap-1">
                    <MessageSquare className="h-4 w-4" />
                    {formatNumber(post.children)} replies
                  </span>
                  <span className="flex items-center gap-1">
                    <Repeat2 className="h-4 w-4" />
                    {formatNumber(reblogs.length)} reblogs
                  </span>
                </div>
              </CardDescription>
            </div>
          </div>
        </CardHeader>
        <CardContent>
          <div className="prose max-w-none mb-6">
            <div className="whitespace-pre-wrap">{post.body}</div>
          </div>

          {/* Stats */}
          <div className="grid grid-cols-2 md:grid-cols-4 gap-4 pt-4 border-t">
            <div>
              <div className="text-sm text-muted-foreground">Total Payout</div>
              <div className="font-semibold">{formatCurrency(post.total_payout_value)}</div>
            </div>
            <div>
              <div className="text-sm text-muted-foreground">Pending Payout</div>
              <div className="font-semibold">{formatCurrency(post.pending_payout_value || 0)}</div>
            </div>
            <div>
              <div className="text-sm text-muted-foreground">Net Votes</div>
              <div className="font-semibold">{formatNumber(post.net_votes)}</div>
            </div>
            <div>
              <div className="text-sm text-muted-foreground">Replies</div>
              <div className="font-semibold">{formatNumber(post.children)}</div>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Votes */}
      {votes.length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle>Votes ({votes.length})</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="space-y-2">
              {votes.slice(0, 20).map((vote: Vote, index: number) => (
                <div key={index} className="flex items-center justify-between p-2 border rounded">
                  <Link
                    to={`/accounts/${vote.voter}`}
                    className="font-medium hover:underline"
                  >
                    @{vote.voter}
                  </Link>
                  <div className="text-sm text-muted-foreground">
                    {vote.weight > 0 ? '+' : ''}{vote.weight}%
                  </div>
                </div>
              ))}
              {votes.length > 20 && (
                <div className="text-center text-sm text-muted-foreground pt-2">
                  Showing first 20 of {formatNumber(votes.length)} votes
                </div>
              )}
            </div>
          </CardContent>
        </Card>
      )}

      {/* Reblogs */}
      {reblogs.length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle>Reblogs ({reblogs.length})</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="space-y-2">
              {reblogs.map((reblog: Reblog) => (
                <div key={reblog.id} className="flex items-center justify-between p-2 border rounded">
                  <Link
                    to={`/accounts/${reblog.account}`}
                    className="font-medium hover:underline"
                  >
                    @{reblog.account}
                  </Link>
                  <span className="text-sm text-muted-foreground">
                    {formatTimeAgo(reblog.timestamp)}
                  </span>
                </div>
              ))}
            </div>
          </CardContent>
        </Card>
      )}

      {/* Replies */}
      {replies.length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle>Replies ({replies.length})</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="space-y-4">
              {replies.map((reply: Post) => (
                <div key={reply.id} className="border rounded-lg p-4">
                  <div className="flex items-center gap-2 mb-2">
                    <Link
                      to={`/accounts/${reply.author}`}
                      className="font-medium hover:underline"
                    >
                      @{reply.author}
                    </Link>
                    <span className="text-sm text-muted-foreground">
                      {formatTimeAgo(reply.created)}
                    </span>
                  </div>
                  <div className="text-sm whitespace-pre-wrap">{reply.body}</div>
                  <div className="flex items-center gap-4 mt-2 text-sm text-muted-foreground">
                    <span>{formatNumber(reply.net_votes)} votes</span>
                    <span>{formatCurrency(reply.total_payout_value + (reply.pending_payout_value || 0))}</span>
                  </div>
                </div>
              ))}
            </div>
          </CardContent>
        </Card>
      )}
    </div>
  );
}
