import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { ArrowLeft, Calendar } from 'lucide-react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../../components/ui/Card';
import { Button } from '../../components/ui/Button';
import { getPowerDowns } from '../../lib/api';
import { formatCurrency, getAvatarUrl } from '../../lib/utils';
import type { PowerDown } from '../../types';

const dayNames = ['', 'Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];

export function PowerDownPage() {
  const { data, isLoading, error } = useQuery({
    queryKey: ['powerdowns'],
    queryFn: () => getPowerDowns(),
  });

  if (isLoading) {
    return (
      <div className="space-y-6">
        <div className="text-center py-12">
          <div className="text-muted-foreground">Loading power downs...</div>
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
              Failed to load power downs.
            </div>
          </CardContent>
        </Card>
      </div>
    );
  }

  const powerDown = data.data as PowerDown;

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-4">
        <Link to="/labs">
          <Button variant="ghost" size="icon">
            <ArrowLeft className="h-4 w-4" />
          </Button>
        </Link>
        <div>
          <h1 className="text-3xl font-bold tracking-tight">Power Down</h1>
          <p className="text-muted-foreground">STEEM power down statistics</p>
        </div>
      </div>

      {/* Props */}
      <Card>
        <CardHeader>
          <CardTitle>Blockchain Properties</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-3 gap-4">
            <div>
              <div className="text-sm text-muted-foreground">Current Supply</div>
              <div className="font-semibold">{formatCurrency(powerDown.props.current)}</div>
            </div>
            <div>
              <div className="text-sm text-muted-foreground">Vesting</div>
              <div className="font-semibold">{formatCurrency(powerDown.props.vesting)}</div>
            </div>
            <div>
              <div className="text-sm text-muted-foreground">Liquid</div>
              <div className="font-semibold">{formatCurrency(powerDown.props.liquid)}</div>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Upcoming */}
      <Card>
        <CardHeader>
          <CardTitle>Upcoming Power Downs</CardTitle>
          <CardDescription>
            Total: {formatCurrency(powerDown.upcoming_total)}
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="space-y-3">
            {powerDown.upcoming.map((day, index) => (
              <div key={index} className="flex items-center justify-between p-3 border rounded">
                <div className="flex items-center gap-3">
                  <Calendar className="h-4 w-4 text-muted-foreground" />
                  <div>
                    <div className="font-medium">
                      {dayNames[day.dow]} {day.month}/{day.day}/{day.year}
                    </div>
                    <div className="text-sm text-muted-foreground">
                      {day.count} withdrawal{day.count !== 1 ? 's' : ''}
                    </div>
                  </div>
                </div>
                <div className="font-semibold">{formatCurrency(day.withdrawn)}</div>
              </div>
            ))}
          </div>
        </CardContent>
      </Card>

      {/* Previous */}
      <Card>
        <CardHeader>
          <CardTitle>Previous Power Downs (Last 7 Days)</CardTitle>
          <CardDescription>
            Total: {formatCurrency(powerDown.previous_total)}
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="space-y-3">
            {powerDown.previous.map((day, index) => (
              <div key={index} className="flex items-center justify-between p-3 border rounded">
                <div className="flex items-center gap-3">
                  <Calendar className="h-4 w-4 text-muted-foreground" />
                  <div>
                    <div className="font-medium">
                      {dayNames[day.dow]} {day.month}/{day.day}/{day.year}
                    </div>
                    <div className="text-sm text-muted-foreground">
                      {day.count} withdrawal{day.count !== 1 ? 's' : ''}
                    </div>
                  </div>
                </div>
                <div className="text-right">
                  <div className="font-semibold">{formatCurrency(day.withdrawn)}</div>
                  {day.deposited && day.deposited > 0 && (
                    <div className="text-sm text-muted-foreground">
                      Deposited: {formatCurrency(day.deposited)}
                    </div>
                  )}
                </div>
              </div>
            ))}
          </div>
        </CardContent>
      </Card>

      {/* Top Power Down Users */}
      <Card>
        <CardHeader>
          <CardTitle>Top Power Down Users (Last 30 Days)</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="space-y-3">
            {powerDown.powerdowns.slice(0, 50).map((user, index) => (
              <div key={user.user} className="flex items-center justify-between p-3 border rounded">
                <div className="flex items-center gap-4">
                  <div className="flex items-center justify-center w-8 h-8 rounded-full bg-primary/10">
                    <span className="text-sm font-semibold text-primary">
                      {index + 1}
                    </span>
                  </div>
                  <div className="flex items-center gap-3">
                    {user.account && (
                      <img
                        src={getAvatarUrl(user.user)}
                        alt={user.user}
                        className="h-8 w-8 rounded-full"
                        onError={(e) => {
                          (e.target as HTMLImageElement).style.display = 'none';
                        }}
                      />
                    )}
                    <Link
                      to={`/accounts/${user.user}`}
                      className="font-medium hover:underline"
                    >
                      @{user.user}
                    </Link>
                    <div className="text-sm text-muted-foreground">
                      {user.count} withdrawal{user.count !== 1 ? 's' : ''}
                    </div>
                  </div>
                </div>
                <div className="text-right">
                  <div className="font-semibold">{formatCurrency(user.withdrawn)}</div>
                  {user.deposited && user.deposited > 0 && (
                    <div className="text-sm text-muted-foreground">
                      Deposited: {formatCurrency(user.deposited)}
                    </div>
                  )}
                </div>
              </div>
            ))}
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
