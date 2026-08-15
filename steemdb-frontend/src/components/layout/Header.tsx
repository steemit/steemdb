import { useEffect, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { Menu, Search, Moon, Sun, Monitor, User, Box, ArrowLeftRight, Loader2 } from 'lucide-react';
import { Button } from '../ui/Button';
import { Input } from '../ui/Input';
import { useThemeStore, useNavigationStore } from '../../store';
import { cn } from '../../lib/utils';
import { apiClient } from '../../lib/api';
import type { SearchResult } from '../../lib/api';

export function Header() {
  const { theme, setTheme } = useThemeStore();
  const { searchOpen, toggleSidebar, toggleSearch } = useNavigationStore();
  const navigate = useNavigate();

  const [searchQuery, setSearchQuery] = useState('');
  const [results, setResults] = useState<SearchResult[]>([]);
  const [searching, setSearching] = useState(false);
  const [dropdownOpen, setDropdownOpen] = useState(false);

  // Debounced search: fires 300ms after typing stops, minimum 2 characters.
  // All state updates happen inside the timer callback (never synchronously
  // in the effect body).
  useEffect(() => {
    const q = searchQuery.trim();
    const timer = setTimeout(async () => {
      if (q.length < 2) {
        setResults([]);
        setSearching(false);
        setDropdownOpen(false);
        return;
      }

      setSearching(true);
      const response = await apiClient.search(q);
      setResults(response.success && response.data ? response.data : []);
      setSearching(false);
      setDropdownOpen(true);
    }, 300);

    return () => clearTimeout(timer);
  }, [searchQuery]);

  const handleSelect = (result: SearchResult) => {
    setDropdownOpen(false);
    if (result.type === 'account') {
      navigate(`/accounts/${result.id}`);
    } else if (result.type === 'block') {
      navigate(`/blocks/${result.id}`);
    } else if (result.type === 'transaction' && result.subtitle) {
      // Transaction detail pages don't exist yet — jump to the containing
      // block (search returns the block number in subtitle).
      navigate(`/blocks/${result.subtitle}`);
    }
  };

  const resultIcon = (type: string) => {
    switch (type) {
      case 'account':
        return <User className="h-3.5 w-3.5" />;
      case 'block':
        return <Box className="h-3.5 w-3.5" />;
      default:
        return <ArrowLeftRight className="h-3.5 w-3.5" />;
    }
  };

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
            to="/accounts"
            className="transition-colors hover:text-foreground/80 text-foreground/60"
          >
            accounts
          </Link>
          <Link
            to="/posts"
            className="transition-colors hover:text-foreground/80 text-foreground/60"
          >
            posts
          </Link>
          <Link
            to="/witnesses"
            className="transition-colors hover:text-foreground/80 text-foreground/60"
          >
            witnesses
          </Link>
          <Link
            to="/labs"
            className="transition-colors hover:text-foreground/80 text-foreground/60"
          >
            labs
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
                <>
                  <Input
                    type="search"
                    placeholder="Search accounts, blocks, txs..."
                    value={searchQuery}
                    onChange={(e) => setSearchQuery(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === 'Escape') {
                        setSearchQuery('');
                        setDropdownOpen(false);
                        toggleSearch();
                      }
                      if (e.key === 'Enter' && results.length > 0) {
                        handleSelect(results[0]);
                      }
                    }}
                    className="w-full"
                    autoFocus
                  />
                  {dropdownOpen && (searching || results.length > 0 || searchQuery.trim().length >= 2) && (
                    <div className="absolute top-10 right-0 w-80 max-h-80 overflow-y-auto rounded-md border bg-popover text-popover-foreground shadow-md">
                      {searching && (
                        <div className="flex items-center space-x-2 px-3 py-2 text-sm text-muted-foreground">
                          <Loader2 className="h-3.5 w-3.5 animate-spin" />
                          <span>Searching...</span>
                        </div>
                      )}
                      {!searching && results.length === 0 && (
                        <div className="px-3 py-2 text-sm text-muted-foreground">
                          No results found
                        </div>
                      )}
                      {!searching &&
                        results.map((result) => (
                          <button
                            key={`${result.type}:${result.id}`}
                            className="flex w-full items-center space-x-2 px-3 py-2 text-left text-sm hover:bg-accent"
                            onClick={() => handleSelect(result)}
                          >
                            {resultIcon(result.type)}
                            <span className="flex-1 truncate">{result.title}</span>
                            <span className="text-xs text-muted-foreground">{result.type}</span>
                          </button>
                        ))}
                    </div>
                  )}
                </>
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
