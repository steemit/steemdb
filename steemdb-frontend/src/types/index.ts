// API Response types
export interface ApiResponse<T> {
  success: boolean;
  data?: T;
  error?: string;
  message?: string;
}

// Blockchain types
export interface Block {
  number: number;
  timestamp: string;
  witness: string;
  transactions: number;
  operations: number;
  previous: string;
  transaction_merkle_root: string;
  witness_signature: string;
}

export interface Transaction {
  ref_block_num: number;
  ref_block_prefix: number;
  expiration: string;
  operations: Operation[];
  extensions: any[];
  signatures: string[];
  transaction_id: string;
  block_num: number;
  transaction_num: number;
}

export interface Operation {
  type: string;
  value: any;
  block: number;
  timestamp: string;
  transaction_id: string;
}

// Account types
export interface Account {
  id: number;
  name: string;
  created: string;
  reputation: string;
  post_count: number;
  comment_count: number;
  voting_power: number;
  balance: string;
  sbd_balance: string;
  vesting_shares: string;
  delegated_vesting_shares: string;
  received_vesting_shares: string;
  last_post: string;
  last_vote_time: string;
  witness_votes: string[];
  json_metadata: string;
  proxy: string;
  recovery_account: string;
}

export interface AccountHistory {
  account: string;
  timestamp: string;
  balance: string;
  sbd_balance: string;
  vesting_shares: string;
  reputation: string;
  post_count: number;
  followers: number;
  following: number;
}

// Witness types
export interface Witness {
  id: number;
  owner: string;
  created: string;
  url: string;
  votes: string;
  total_missed: number;
  last_aslot: number;
  last_confirmed_block_num: number;
  signing_key: string;
  running_version: string;
  hardfork_version_vote: string;
  props: WitnessProps;
  sbd_exchange_rate: ExchangeRate;
}

export interface WitnessProps {
  account_creation_fee: string;
  maximum_block_size: number;
  sbd_interest_rate: number;
}

export interface ExchangeRate {
  base: string;
  quote: string;
}

// Statistics types
export interface GlobalStats {
  accounts: number;
  blocks: number;
  transactions: number;
  operations: number;
  witnesses: number;
  last_block: number;
  last_update: string;
}

export interface NetworkPerformance {
  transactions_24h: number;
  transactions_1h: number;
  transactions_per_sec_24h: number;
  transactions_per_sec_1h: number;
  operations_24h: number;
  operations_1h: number;
  operations_per_sec_24h: number;
  operations_per_sec_1h: number;
}

export interface RewardPool {
  [key: string]: any; // Dynamic fields
}

export interface BlockchainProps {
  head_block_number: number;
  head_block_id: string;
  time: string;
  current_witness: string;
  total_pow: number;
  num_pow_witnesses: number;
  virtual_supply: string;
  current_supply: string;
  confidential_supply: string;
  current_sbd_supply: string;
  confidential_sbd_supply: string;
  total_vesting_fund_steem: string;
  total_vesting_shares: string;
  total_reward_fund_steem: string;
  total_reward_shares2: string;
  pending_rewarded_vesting_shares: string;
  pending_rewarded_vesting_steem: string;
  sbd_interest_rate: number;
  sbd_print_rate: number;
  maximum_block_size: number;
  current_aslot: number;
  recent_slots_filled: string;
  participation_count: number;
  last_irreversible_block_num: number;
  vote_power_reserve_rate: number;
}

// WebSocket types
export interface WebSocketMessage {
  type: 'block' | 'props' | 'state' | 'operation';
  channel: string;
  data: any;
  timestamp: string;
}

export interface SubscriptionRequest {
  action: 'subscribe' | 'unsubscribe';
  channel: string;
}

export interface BlockData {
  number: number;
  timestamp: string;
  witness: string;
  transactions: number;
  operations: number;
}

export interface PropsData extends BlockchainProps {}

export interface StateData {
  accounts: number;
  comments: number;
  witnesses: number;
  last_block: number;
  last_update: string;
}

export interface OperationData {
  type: string;
  block: number;
  timestamp: string;
  data: any;
  accounts?: string[];
}

// Chart data types
export interface ChartDataPoint {
  timestamp: string;
  value: number;
  label?: string;
}

export interface TimeSeriesData {
  name: string;
  data: ChartDataPoint[];
  color?: string;
}

// UI types
export interface Tab {
  id: string;
  label: string;
  content: React.ReactNode;
}

export interface MenuItem {
  id: string;
  label: string;
  href: string;
  icon?: React.ReactNode;
  children?: MenuItem[];
}

export interface SearchResult {
  type: 'account' | 'block' | 'transaction';
  id: string;
  title: string;
  subtitle?: string;
  url: string;
}

// Pagination types
export interface PaginationParams {
  page: number;
  limit: number;
  sort?: string;
  order?: 'asc' | 'desc';
}

export interface PaginatedResponse<T> {
  data: T[];
  total: number;
  page: number;
  limit: number;
  totalPages: number;
}

// Filter types
export interface DateRange {
  start: Date;
  end: Date;
}

export interface AccountFilter {
  reputation?: [number, number];
  balance?: [number, number];
  created?: DateRange;
  active?: DateRange;
}

export interface BlockFilter {
  witness?: string;
  dateRange?: DateRange;
  minTransactions?: number;
  maxTransactions?: number;
}

// Theme types
export type Theme = 'light' | 'dark' | 'system';

// WebSocket connection states
export type WebSocketState = 'connecting' | 'connected' | 'disconnected' | 'error';

// Loading states
export type LoadingState = 'idle' | 'loading' | 'success' | 'error';

// Sort options
export type SortOption = {
  field: string;
  direction: 'asc' | 'desc';
  label: string;
};
