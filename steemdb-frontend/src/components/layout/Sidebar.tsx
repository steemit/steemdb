import { Link, useLocation } from 'react-router-dom';
import { 
  Home, 
  Blocks, 
  Users, 
  Shield, 
  BarChart3, 
  Activity,
  Star,
  Settings,
  X
} from 'lucide-react';
import { Button } from '../ui/Button';
import { Badge } from '../ui/Badge';
import { useNavigationStore, useWebSocketStore, useFavoritesStore } from '../../store';
import { cn } from '../../lib/utils';

const navigation = [
  { name: 'Dashboard', href: '/', icon: Home },
  { name: 'Blocks', href: '/blocks', icon: Blocks },
  { name: 'Accounts', href: '/accounts', icon: Users },
  { name: 'Witnesses', href: '/witnesses', icon: Shield },
  { name: 'Statistics', href: '/stats', icon: BarChart3 },
  { name: 'Live Feed', href: '/live', icon: Activity },
];

export function Sidebar() {
  const location = useLocation();
  const { sidebarOpen, setSidebarOpen } = useNavigationStore();
  const { state: wsState } = useWebSocketStore();
  const { accounts: favoriteAccounts, witnesses: favoriteWitnesses } = useFavoritesStore();

  const getConnectionBadge = () => {
    switch (wsState) {
      case 'connected':
        return <Badge variant="success" className="text-xs">Live</Badge>;
      case 'connecting':
        return <Badge variant="warning" className="text-xs">Connecting</Badge>;
      case 'disconnected':
        return <Badge variant="destructive" className="text-xs">Offline</Badge>;
      default:
        return <Badge variant="outline" className="text-xs">Unknown</Badge>;
    }
  };

  return (
    <>
      {/* Mobile overlay */}
      {sidebarOpen && (
        <div
          className="fixed inset-0 z-40 bg-background/80 backdrop-blur-sm md:hidden"
          onClick={() => setSidebarOpen(false)}
        />
      )}

      {/* Sidebar */}
      <aside
        className={cn(
          'fixed left-0 top-14 z-50 h-[calc(100vh-3.5rem)] w-64 transform border-r bg-background transition-transform duration-200 ease-in-out md:translate-x-0',
          sidebarOpen ? 'translate-x-0' : '-translate-x-full'
        )}
      >
        <div className="flex h-full flex-col">
          {/* Close button for mobile */}
          <div className="flex items-center justify-between p-4 md:hidden">
            <span className="text-lg font-semibold">Menu</span>
            <Button
              variant="ghost"
              size="icon"
              onClick={() => setSidebarOpen(false)}
            >
              <X className="h-4 w-4" />
            </Button>
          </div>

          {/* Connection status */}
          <div className="flex items-center justify-between px-4 py-2 border-b">
            <span className="text-sm text-muted-foreground">Connection</span>
            {getConnectionBadge()}
          </div>

          {/* Navigation */}
          <nav className="flex-1 space-y-1 p-4">
            {navigation.map((item) => {
              const isActive = location.pathname === item.href;
              return (
                <Link
                  key={item.name}
                  to={item.href}
                  className={cn(
                    'flex items-center rounded-md px-3 py-2 text-sm font-medium transition-colors',
                    isActive
                      ? 'bg-accent text-accent-foreground'
                      : 'text-muted-foreground hover:bg-accent hover:text-accent-foreground'
                  )}
                  onClick={() => setSidebarOpen(false)}
                >
                  <item.icon className="mr-3 h-4 w-4" />
                  {item.name}
                </Link>
              );
            })}
          </nav>

          {/* Favorites section */}
          {(favoriteAccounts.length > 0 || favoriteWitnesses.length > 0) && (
            <div className="border-t p-4">
              <div className="flex items-center mb-3">
                <Star className="mr-2 h-4 w-4 text-muted-foreground" />
                <span className="text-sm font-medium">Favorites</span>
              </div>

              {favoriteAccounts.length > 0 && (
                <div className="mb-3">
                  <div className="text-xs text-muted-foreground mb-1">Accounts</div>
                  <div className="space-y-1">
                    {favoriteAccounts.slice(0, 5).map((account) => (
                      <Link
                        key={account}
                        to={`/accounts/${account}`}
                        className="block px-2 py-1 text-xs text-muted-foreground hover:text-foreground rounded"
                        onClick={() => setSidebarOpen(false)}
                      >
                        @{account}
                      </Link>
                    ))}
                    {favoriteAccounts.length > 5 && (
                      <div className="text-xs text-muted-foreground px-2">
                        +{favoriteAccounts.length - 5} more
                      </div>
                    )}
                  </div>
                </div>
              )}

              {favoriteWitnesses.length > 0 && (
                <div>
                  <div className="text-xs text-muted-foreground mb-1">Witnesses</div>
                  <div className="space-y-1">
                    {favoriteWitnesses.slice(0, 5).map((witness) => (
                      <Link
                        key={witness}
                        to={`/witnesses/${witness}`}
                        className="block px-2 py-1 text-xs text-muted-foreground hover:text-foreground rounded"
                        onClick={() => setSidebarOpen(false)}
                      >
                        @{witness}
                      </Link>
                    ))}
                    {favoriteWitnesses.length > 5 && (
                      <div className="text-xs text-muted-foreground px-2">
                        +{favoriteWitnesses.length - 5} more
                      </div>
                    )}
                  </div>
                </div>
              )}
            </div>
          )}

          {/* Settings */}
          <div className="border-t p-4">
            <Link
              to="/settings"
              className="flex items-center rounded-md px-3 py-2 text-sm font-medium text-muted-foreground hover:bg-accent hover:text-accent-foreground"
              onClick={() => setSidebarOpen(false)}
            >
              <Settings className="mr-3 h-4 w-4" />
              Settings
            </Link>
          </div>
        </div>
      </aside>
    </>
  );
}
