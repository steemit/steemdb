import { Link } from 'react-router-dom';
import { FlaskConical, TrendingUp, TrendingDown, BarChart3, Award, UserCheck, Users, Gift, Clock } from 'lucide-react';
import { Card, CardDescription, CardHeader, CardTitle } from '../components/ui/Card';

const labsFeatures = [
  {
    id: 'powerup',
    name: 'Power Up',
    description: 'Statistics on STEEM power ups (vesting deposits)',
    icon: TrendingUp,
    href: '/labs/powerup',
  },
  {
    id: 'powerdown',
    name: 'Power Down',
    description: 'Statistics on STEEM power downs (vesting withdrawals)',
    icon: TrendingDown,
    href: '/labs/powerdown',
  },
  {
    id: 'rshares',
    name: 'Rshares Allocation',
    description: 'Analysis of rshares distribution',
    icon: BarChart3,
    href: '/labs/rshares',
  },
  {
    id: 'curation',
    name: 'Curation Leaderboard',
    description: 'Top curators by reward',
    icon: Award,
    href: '/labs/curation',
  },
  {
    id: 'author',
    name: 'Author Leaderboard',
    description: 'Top authors by reward',
    icon: UserCheck,
    href: '/labs/author',
  },
  {
    id: 'flags',
    name: 'Flags',
    description: 'Accounts receiving downvotes',
    icon: Users,
    href: '/labs/flags',
  },
  {
    id: 'clients',
    name: 'Clients',
    description: 'Client usage statistics',
    icon: FlaskConical,
    href: '/labs/clients',
  },
  {
    id: 'benefactors',
    name: 'Benefactors',
    description: 'Benefactor reward statistics',
    icon: Gift,
    href: '/labs/benefactors',
  },
  {
    id: 'pending',
    name: 'Pending Posts',
    description: 'Posts awaiting payout',
    icon: Clock,
    href: '/labs/pending',
  },
];

export function LabsPage() {
  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-3xl font-bold tracking-tight">Labs</h1>
        <p className="text-muted-foreground">
          Experimental features and analytics
        </p>
      </div>

      <div className="grid gap-6 md:grid-cols-2 lg:grid-cols-3">
        {labsFeatures.map((feature) => (
          <Link key={feature.id} to={feature.href}>
            <Card className="h-full hover:bg-accent/50 transition-colors cursor-pointer">
              <CardHeader>
                <div className="flex items-center gap-3">
                  <div className="p-2 rounded-md bg-primary/10">
                    <feature.icon className="h-5 w-5 text-primary" />
                  </div>
                  <CardTitle className="text-lg">{feature.name}</CardTitle>
                </div>
                <CardDescription>{feature.description}</CardDescription>
              </CardHeader>
            </Card>
          </Link>
        ))}
      </div>
    </div>
  );
}
