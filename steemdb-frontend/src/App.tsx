import { useEffect } from 'react';
import { BrowserRouter as Router, Routes, Route } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { Layout } from './components/layout/Layout';
import { Dashboard } from './pages/Dashboard';
import { useThemeStore, useWebSocketStore } from './store';
import { wsClient } from './lib/websocket';

// Create a client
const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 1000 * 60 * 5, // 5 minutes
      gcTime: 1000 * 60 * 10, // 10 minutes
    },
  },
});

// Theme effect hook
function useThemeEffect() {
  const { theme } = useThemeStore();

  useEffect(() => {
    const root = window.document.documentElement;
    root.classList.remove('light', 'dark');

    if (theme === 'system') {
      const systemTheme = window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
      root.classList.add(systemTheme);
    } else {
      root.classList.add(theme);
    }
  }, [theme]);
}

// WebSocket connection hook
function useWebSocketConnection() {
  const { setState, setLastMessage } = useWebSocketStore();

  useEffect(() => {
    // Set up WebSocket state handlers
    const unsubscribeState = wsClient.onStateChange((state) => {
      setState(state);
    });

    // Set up message handler for the store
    const unsubscribeMessages = wsClient.on('all', (message) => {
      setLastMessage(message);
    });

    // Connect if not already connected
    if (!wsClient.isConnected()) {
      wsClient.connect().catch(error => {
        console.error('Failed to connect to WebSocket:', error);
      });
    }

    return () => {
      unsubscribeState();
      unsubscribeMessages();
    };
  }, [setState, setLastMessage]);
}

function App() {
  useThemeEffect();
  useWebSocketConnection();

  return (
    <QueryClientProvider client={queryClient}>
      <Router>
        <Routes>
          <Route path="/" element={<Layout />}>
            <Route index element={<Dashboard />} />
            <Route path="blocks" element={<div>Blocks page coming soon...</div>} />
            <Route path="accounts" element={<div>Accounts page coming soon...</div>} />
            <Route path="witnesses" element={<div>Witnesses page coming soon...</div>} />
            <Route path="stats" element={<div>Statistics page coming soon...</div>} />
            <Route path="live" element={<div>Live feed page coming soon...</div>} />
            <Route path="settings" element={<div>Settings page coming soon...</div>} />
            <Route path="*" element={<div>Page not found</div>} />
          </Route>
        </Routes>
      </Router>
    </QueryClientProvider>
  );
}

export default App;