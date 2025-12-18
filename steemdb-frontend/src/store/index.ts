import { create } from 'zustand';
import { devtools } from 'zustand/middleware';
import type { 
  Theme, 
  WebSocketState, 
  BlockchainProps, 
  GlobalStats, 
  Block,
  WebSocketMessage,
  NetworkPerformance,
  RewardPool
} from '../types';

// Theme store
interface ThemeStore {
  theme: Theme;
  setTheme: (theme: Theme) => void;
}

export const useThemeStore = create<ThemeStore>()(
  devtools(
    (set) => ({
      theme: 'system',
      setTheme: (theme) => set({ theme }),
    }),
    { name: 'theme-store' }
  )
);

// WebSocket store
interface WebSocketStore {
  state: WebSocketState;
  lastMessage: WebSocketMessage | null;
  subscriptions: string[];
  setState: (state: WebSocketState) => void;
  setLastMessage: (message: WebSocketMessage) => void;
  addSubscription: (channel: string) => void;
  removeSubscription: (channel: string) => void;
  clearSubscriptions: () => void;
}

export const useWebSocketStore = create<WebSocketStore>()(
  devtools(
    (set) => ({
      state: 'disconnected',
      lastMessage: null,
      subscriptions: [],
      setState: (state) => set({ state }),
      setLastMessage: (message) => set({ lastMessage: message }),
      addSubscription: (channel) => 
        set((state) => ({
          subscriptions: [...state.subscriptions.filter(s => s !== channel), channel]
        })),
      removeSubscription: (channel) =>
        set((state) => ({
          subscriptions: state.subscriptions.filter(s => s !== channel)
        })),
      clearSubscriptions: () => set({ subscriptions: [] }),
    }),
    { name: 'websocket-store' }
  )
);

// Blockchain data store
interface BlockchainStore {
  props: BlockchainProps | null;
  stats: GlobalStats | null;
  latestBlocks: Block[];
  currentBlock: number;
  networkPerformance: NetworkPerformance | null;
  rewardPool: RewardPool | null;
  setProps: (props: BlockchainProps) => void;
  setStats: (stats: GlobalStats) => void;
  setLatestBlocks: (blocks: Block[]) => void;
  addBlock: (block: Block) => void;
  setCurrentBlock: (blockNumber: number) => void;
  setNetworkPerformance: (perf: NetworkPerformance | null) => void;
  setRewardPool: (pool: RewardPool | null) => void;
}

export const useBlockchainStore = create<BlockchainStore>()(
  devtools(
    (set) => ({
      props: null,
      stats: null,
      latestBlocks: [],
      currentBlock: 0,
      networkPerformance: null,
      rewardPool: null,
      setProps: (props) => set({ props, currentBlock: props.head_block_number }),
      setStats: (stats) => set({ stats }),
      setLatestBlocks: (blocks) => set({ latestBlocks: blocks }),
      addBlock: (block) =>
        set((state) => ({
          latestBlocks: [block, ...state.latestBlocks.slice(0, 9)], // Keep only 10 latest
          currentBlock: Math.max(state.currentBlock, block.number),
        })),
      setCurrentBlock: (blockNumber) => set({ currentBlock: blockNumber }),
      setNetworkPerformance: (perf) => set({ networkPerformance: perf }),
      setRewardPool: (pool) => set({ rewardPool: pool }),
    }),
    { name: 'blockchain-store' }
  )
);

// Navigation store
interface NavigationStore {
  sidebarOpen: boolean;
  searchOpen: boolean;
  searchQuery: string;
  setSidebarOpen: (open: boolean) => void;
  setSearchOpen: (open: boolean) => void;
  setSearchQuery: (query: string) => void;
  toggleSidebar: () => void;
  toggleSearch: () => void;
}

export const useNavigationStore = create<NavigationStore>()(
  devtools(
    (set) => ({
      sidebarOpen: false,
      searchOpen: false,
      searchQuery: '',
      setSidebarOpen: (open) => set({ sidebarOpen: open }),
      setSearchOpen: (open) => set({ searchOpen: open }),
      setSearchQuery: (query) => set({ searchQuery: query }),
      toggleSidebar: () => set((state) => ({ sidebarOpen: !state.sidebarOpen })),
      toggleSearch: () => set((state) => ({ searchOpen: !state.searchOpen })),
    }),
    { name: 'navigation-store' }
  )
);

// Notifications store
interface Notification {
  id: string;
  type: 'success' | 'error' | 'warning' | 'info';
  title: string;
  message?: string;
  duration?: number;
  timestamp: number;
}

interface NotificationStore {
  notifications: Notification[];
  addNotification: (notification: Omit<Notification, 'id' | 'timestamp'>) => void;
  removeNotification: (id: string) => void;
  clearNotifications: () => void;
}

export const useNotificationStore = create<NotificationStore>()(
  devtools(
    (set) => ({
      notifications: [],
      addNotification: (notification) =>
        set((state) => ({
          notifications: [
            ...state.notifications,
            {
              ...notification,
              id: Math.random().toString(36).substr(2, 9),
              timestamp: Date.now(),
            },
          ],
        })),
      removeNotification: (id) =>
        set((state) => ({
          notifications: state.notifications.filter((n) => n.id !== id),
        })),
      clearNotifications: () => set({ notifications: [] }),
    }),
    { name: 'notification-store' }
  )
);

// Favorites store (for accounts, witnesses, etc.)
interface FavoritesStore {
  accounts: string[];
  witnesses: string[];
  addAccount: (username: string) => void;
  removeAccount: (username: string) => void;
  addWitness: (username: string) => void;
  removeWitness: (username: string) => void;
  isAccountFavorite: (username: string) => boolean;
  isWitnessFavorite: (username: string) => boolean;
}

export const useFavoritesStore = create<FavoritesStore>()(
  devtools(
    (set, get) => ({
      accounts: JSON.parse(localStorage.getItem('favorite-accounts') || '[]'),
      witnesses: JSON.parse(localStorage.getItem('favorite-witnesses') || '[]'),
      addAccount: (username) =>
        set((state) => {
          const accounts = [...state.accounts.filter(a => a !== username), username];
          localStorage.setItem('favorite-accounts', JSON.stringify(accounts));
          return { accounts };
        }),
      removeAccount: (username) =>
        set((state) => {
          const accounts = state.accounts.filter(a => a !== username);
          localStorage.setItem('favorite-accounts', JSON.stringify(accounts));
          return { accounts };
        }),
      addWitness: (username) =>
        set((state) => {
          const witnesses = [...state.witnesses.filter(w => w !== username), username];
          localStorage.setItem('favorite-witnesses', JSON.stringify(witnesses));
          return { witnesses };
        }),
      removeWitness: (username) =>
        set((state) => {
          const witnesses = state.witnesses.filter(w => w !== username);
          localStorage.setItem('favorite-witnesses', JSON.stringify(witnesses));
          return { witnesses };
        }),
      isAccountFavorite: (username) => get().accounts.includes(username),
      isWitnessFavorite: (username) => get().witnesses.includes(username),
    }),
    { name: 'favorites-store' }
  )
);
