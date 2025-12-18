import { Link } from 'react-router-dom';
import { Menu, Search, Moon, Sun, Monitor } from 'lucide-react';
import { Button } from '../ui/Button';
import { Input } from '../ui/Input';
import { useThemeStore, useNavigationStore } from '../../store';
import { cn } from '../../lib/utils';

export function Header() {
  const { theme, setTheme } = useThemeStore();
  const { searchOpen, searchQuery, toggleSidebar, toggleSearch, setSearchQuery } = useNavigationStore();

  const cycleTheme = () => {
    const themes = ['light', 'dark', 'system'] as const;
    const currentIndex = themes.indexOf(theme);
    const nextTheme = themes[(currentIndex + 1) % themes.length];
    setTheme(nextTheme);
  };

  const getThemeIcon = () => {
    switch (theme) {
      case 'light':
        return <Sun className="h-4 w-4" />;
      case 'dark':
        return <Moon className="h-4 w-4" />;
      default:
        return <Monitor className="h-4 w-4" />;
    }
  };

  return (
    <header className="sticky top-0 z-50 w-full border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60">
      <div className="container flex h-14 items-center">
        {/* Mobile menu button */}
        <Button
          variant="ghost"
          size="icon"
          className="mr-2 md:hidden"
          onClick={toggleSidebar}
        >
          <Menu className="h-4 w-4" />
        </Button>

        {/* Logo */}
        <Link to="/" className="mr-6 flex items-center space-x-2">
          <span className="hidden font-bold sm:inline-block">SteemDB</span>
        </Link>

        {/* Navigation */}
        <nav className="hidden md:flex items-center space-x-6 text-sm font-medium">
          <Link
            to="/"
            className="transition-colors hover:text-foreground/80 text-foreground/60"
          >
            Dashboard
          </Link>
          <Link
            to="/blocks"
            className="transition-colors hover:text-foreground/80 text-foreground/60"
          >
            Blocks
          </Link>
          <Link
            to="/accounts"
            className="transition-colors hover:text-foreground/80 text-foreground/60"
          >
            Accounts
          </Link>
          <Link
            to="/witnesses"
            className="transition-colors hover:text-foreground/80 text-foreground/60"
          >
            Witnesses
          </Link>
        </nav>

        <div className="flex flex-1 items-center justify-end space-x-2">
          {/* Search */}
          <div className="relative">
            <div
              className={cn(
                'flex items-center transition-all duration-200',
                searchOpen ? 'w-64' : 'w-auto'
              )}
            >
              {searchOpen ? (
                <Input
                  type="search"
                  placeholder="Search accounts, blocks..."
                  value={searchQuery}
                  onChange={(e) => setSearchQuery(e.target.value)}
                  className="w-full"
                  onBlur={() => {
                    if (!searchQuery) {
                      toggleSearch();
                    }
                  }}
                  autoFocus
                />
              ) : (
                <Button
                  variant="ghost"
                  size="icon"
                  onClick={toggleSearch}
                >
                  <Search className="h-4 w-4" />
                </Button>
              )}
            </div>
          </div>

          {/* Theme toggle */}
          <Button
            variant="ghost"
            size="icon"
            onClick={cycleTheme}
            title={`Current theme: ${theme}`}
          >
            {getThemeIcon()}
          </Button>
        </div>
      </div>
    </header>
  );
}
