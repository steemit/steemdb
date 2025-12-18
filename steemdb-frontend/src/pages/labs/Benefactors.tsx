import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { ArrowLeft, Gift, Calendar } from 'lucide-react';
import { Card, CardContent, CardHeader, CardTitle } from '../../components/ui/Card';
import { Button } from '../../components/ui/Button';
import { getBenefactors } from '../../lib/api';
import { formatCurrency } from '../../lib/utils';
import type { Benefactors } from '../../types';

const dayNames = ['', 'Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];

export function BenefactorsPage() {
  const { data, isLoading, error } = useQuery({
    queryKey: ['benefactors'],
    queryFn: () => getBenefactors(),
  });

  if (isLoading) {
    return (
      <div className="space-y-6">
        <div className="text-center py-12">
          <div className="text-muted-foreground">Loading benefactor statistics...</div>
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
              Failed to load benefactor statistics.
            </div>
          </CardContent>
        </Card>
      </div>
    );
  }

  const benefactors = data.data as Benefactors;

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-4">
        <Link to="/labs">
          <Button variant="ghost" size="icon">
            <ArrowLeft className="h-4 w-4" />
          </Button>
        </Link>
        <div>
          <h1 className="text-3xl font-bold tracking-tight">Benefactors</h1>
          <p className="text-muted-foreground">Benefactor reward statistics</p>
        </div>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Benefactor Rewards by Date</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="space-y-6">
            {benefactors.dates.length === 0 ? (
              <div className="text-center py-12 text-muted-foreground">
                No benefactor data available.
              </div>
            ) : (
              benefactors.dates.map((date, index) => (
                <div key={index} className="border rounded-lg p-4">
                  <div className="flex items-center justify-between mb-4">
                    <div className="flex items-center gap-3">
                      <Calendar className="h-4 w-4 text-muted-foreground" />
                      <div>
                        <div className="font-medium">
                          {dayNames[date.dow]} {date.month}/{date.day}/{date.year}
                        </div>
                        <div className="text-sm text-muted-foreground">
                          {date.total} reward{date.total !== 1 ? 's' : ''}
                        </div>
                      </div>
                    </div>
                    <div className="text-right">
                      <div className="font-semibold text-lg">
                        {formatCurrency(date.reward)}
                      </div>
                    </div>
                  </div>
                  <div className="space-y-2">
                    {date.benefactors.slice(0, 10).map((benefactor, idx) => (
                      <div
                        key={idx}
                        className="flex items-center justify-between p-2 bg-accent/50 rounded"
                      >
                        <div className="flex items-center gap-2">
                          <Gift className="h-3 w-3 text-muted-foreground" />
                          <Link
                            to={`/accounts/${benefactor.benefactor}`}
                            className="text-sm font-medium hover:underline"
                          >
                            @{benefactor.benefactor}
                          </Link>
                          <span className="text-xs text-muted-foreground">
                            ({benefactor.count})
                          </span>
                        </div>
                        <div className="text-sm font-medium">
                          {formatCurrency(benefactor.reward)}
                        </div>
                      </div>
                    ))}
                    {date.benefactors.length > 10 && (
                      <div className="text-xs text-muted-foreground text-center pt-2">
                        +{date.benefactors.length - 10} more benefactors
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
