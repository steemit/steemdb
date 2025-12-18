import type { 
  ApiResponse, 
  Account, 
  Block, 
  Witness, 
  GlobalStats, 
  BlockchainProps,
  NetworkPerformance,
  RewardPool,
  Post,
  Vote,
  Reblog,
  PowerUp,
  PowerDown,
  RsharesAllocation,
  CurationLeaderboard,
  AuthorLeaderboard,
  Flags,
  Clients,
  Benefactors,
  PendingPost,
  PaginatedResponse,
  PaginationParams 
} from '../types';

const API_BASE_URL = import.meta.env.VITE_API_URL || 'http://localhost:8080/api';

class ApiClient {
  private baseUrl: string;

  constructor(baseUrl: string = API_BASE_URL) {
    this.baseUrl = baseUrl;
  }

  private async request<T>(endpoint: string, options?: RequestInit): Promise<ApiResponse<T>> {
    try {
      const response = await fetch(`${this.baseUrl}${endpoint}`, {
        headers: {
          'Content-Type': 'application/json',
          ...options?.headers,
        },
        ...options,
      });

      if (!response.ok) {
        throw new Error(`HTTP error! status: ${response.status}`);
      }

      const data = await response.json();
      return data;
    } catch (error) {
      console.error('API request failed:', error);
      return {
        success: false,
        error: error instanceof Error ? error.message : 'Unknown error occurred',
      };
    }
  }

  // Account endpoints
  async getAccount(username: string): Promise<ApiResponse<Account>> {
    return this.request<Account>(`/accounts/${username}`);
  }

  async getAccounts(params: PaginationParams & { search?: string }): Promise<ApiResponse<PaginatedResponse<Account>>> {
    const searchParams = new URLSearchParams({
      page: params.page.toString(),
      limit: params.limit.toString(),
      ...(params.sort && { sort: params.sort }),
      ...(params.order && { order: params.order }),
      ...(params.search && { search: params.search }),
    });

    return this.request<PaginatedResponse<Account>>(`/accounts?${searchParams}`);
  }

  async getAccountHistory(username: string, params: PaginationParams): Promise<ApiResponse<any[]>> {
    const searchParams = new URLSearchParams({
      page: params.page.toString(),
      limit: params.limit.toString(),
    });

    return this.request<any[]>(`/accounts/${username}/history?${searchParams}`);
  }

  // Block endpoints
  async getBlock(blockNumber: number): Promise<ApiResponse<Block>> {
    return this.request<Block>(`/blocks/${blockNumber}`);
  }

  async getBlocks(params: PaginationParams): Promise<ApiResponse<PaginatedResponse<Block>>> {
    const searchParams = new URLSearchParams({
      page: params.page.toString(),
      limit: params.limit.toString(),
      ...(params.sort && { sort: params.sort }),
      ...(params.order && { order: params.order }),
    });

    return this.request<PaginatedResponse<Block>>(`/blocks?${searchParams}`);
  }

  async getLatestBlocks(limit: number = 10): Promise<ApiResponse<Block[]>> {
    return this.request<Block[]>(`/blocks/latest?limit=${limit}`);
  }

  // Witness endpoints
  async getWitnesses(params: PaginationParams): Promise<ApiResponse<PaginatedResponse<Witness>>> {
    const searchParams = new URLSearchParams({
      page: params.page.toString(),
      limit: params.limit.toString(),
      ...(params.sort && { sort: params.sort }),
      ...(params.order && { order: params.order }),
    });

    return this.request<PaginatedResponse<Witness>>(`/witnesses?${searchParams}`);
  }

  async getWitness(username: string): Promise<ApiResponse<Witness>> {
    return this.request<Witness>(`/witnesses/${username}`);
  }

  async getTopWitnesses(limit: number = 21): Promise<ApiResponse<Witness[]>> {
    return this.request<Witness[]>(`/witnesses/top?limit=${limit}`);
  }

  // Statistics endpoints
  async getGlobalStats(): Promise<ApiResponse<GlobalStats>> {
    return this.request<GlobalStats>('/stats/global');
  }

  async getBlockchainProps(): Promise<ApiResponse<BlockchainProps>> {
    return this.request<BlockchainProps>('/stats/props');
  }

  // Dashboard endpoint
  async getDashboard(): Promise<ApiResponse<{
    props: BlockchainProps;
    latest_blocks: Block[];
    stats: GlobalStats;
    network_performance?: NetworkPerformance;
    reward_pool?: RewardPool;
    is_from_upstream: boolean;
  }>> {
    return this.request(`/v1/dashboard`);
  }

  // Search endpoint
  async search(query: string, type?: 'account' | 'block' | 'transaction'): Promise<ApiResponse<any[]>> {
    const searchParams = new URLSearchParams({
      q: query,
      ...(type && { type }),
    });

    return this.request<any[]>(`/search?${searchParams}`);
  }

  // Chart data endpoints
  async getAccountGrowthData(days: number = 30): Promise<ApiResponse<any[]>> {
    return this.request<any[]>(`/charts/accounts/growth?days=${days}`);
  }

  async getBlockProductionData(days: number = 7): Promise<ApiResponse<any[]>> {
    return this.request<any[]>(`/charts/blocks/production?days=${days}`);
  }

  async getTransactionVolumeData(days: number = 30): Promise<ApiResponse<any[]>> {
    return this.request<any[]>(`/charts/transactions/volume?days=${days}`);
  }

  async getWitnessVotingData(): Promise<ApiResponse<any[]>> {
    return this.request<any[]>('/charts/witnesses/voting');
  }

  // Health check
  async healthCheck(): Promise<ApiResponse<{ status: string; timestamp: number }>> {
    return this.request<{ status: string; timestamp: number }>('/health');
  }

  // Post/Comment endpoints
  async getPosts(params: PaginationParams & { sort_by?: string; sort_order?: 'asc' | 'desc' }): Promise<ApiResponse<PaginatedResponse<Post>>> {
    const searchParams = new URLSearchParams({
      page: params.page.toString(),
      limit: params.limit.toString(),
      ...(params.sort_by && { sort_by: params.sort_by }),
      ...(params.sort_order && { sort_order: params.sort_order }),
    });

    return this.request<PaginatedResponse<Post>>(`/v1/posts?${searchParams}`);
  }

  async getPost(author: string, permlink: string): Promise<ApiResponse<Post>> {
    return this.request<Post>(`/v1/posts/${author}/${permlink}`);
  }

  async getPostsByDate(date: string, tag?: string, sort?: string): Promise<ApiResponse<Post[]>> {
    const searchParams = new URLSearchParams({
      date,
      ...(tag && { tag }),
      ...(sort && { sort }),
    });

    return this.request<Post[]>(`/v1/posts/daily?${searchParams}`);
  }

  async getPostReplies(author: string, permlink: string): Promise<ApiResponse<Post[]>> {
    return this.request<Post[]>(`/v1/posts/${author}/${permlink}/replies`);
  }

  async getPostVotes(author: string, permlink: string): Promise<ApiResponse<Vote[]>> {
    return this.request<Vote[]>(`/v1/posts/${author}/${permlink}/votes`);
  }

  async getPostReblogs(author: string, permlink: string): Promise<ApiResponse<Reblog[]>> {
    return this.request<Reblog[]>(`/v1/posts/${author}/${permlink}/reblogs`);
  }

  // Labs endpoints
  async getLabsIndex(): Promise<ApiResponse<{ features: string[] }>> {
    return this.request<{ features: string[] }>('/v1/labs');
  }

  async getPowerUps(filter?: string): Promise<ApiResponse<PowerUp[]>> {
    const searchParams = new URLSearchParams();
    if (filter) {
      searchParams.set('filter', filter);
    }
    return this.request<PowerUp[]>(`/v1/labs/powerup?${searchParams}`);
  }

  async getPowerDowns(): Promise<ApiResponse<PowerDown>> {
    return this.request<PowerDown>('/v1/labs/powerdown');
  }

  async getRshares(date?: string): Promise<ApiResponse<RsharesAllocation[]>> {
    const searchParams = new URLSearchParams();
    if (date) {
      searchParams.set('date', date);
    }
    return this.request<RsharesAllocation[]>(`/v1/labs/rshares?${searchParams}`);
  }

  async getCuration(date?: string, grouping?: string): Promise<ApiResponse<CurationLeaderboard[]>> {
    const searchParams = new URLSearchParams();
    if (date) {
      searchParams.set('date', date);
    }
    if (grouping) {
      searchParams.set('grouping', grouping);
    }
    return this.request<CurationLeaderboard[]>(`/v1/labs/curation?${searchParams}`);
  }

  async getAuthor(date?: string, grouping?: string): Promise<ApiResponse<AuthorLeaderboard[]>> {
    const searchParams = new URLSearchParams();
    if (date) {
      searchParams.set('date', date);
    }
    if (grouping) {
      searchParams.set('grouping', grouping);
    }
    return this.request<AuthorLeaderboard[]>(`/v1/labs/author?${searchParams}`);
  }

  async getFlags(): Promise<ApiResponse<Flags[]>> {
    return this.request<Flags[]>('/v1/labs/flags');
  }

  async getClients(): Promise<ApiResponse<Clients>> {
    return this.request<Clients>('/v1/labs/clients');
  }

  async getBenefactors(): Promise<ApiResponse<Benefactors>> {
    return this.request<Benefactors>('/v1/labs/benefactors');
  }

  async getPending(): Promise<ApiResponse<PendingPost[]>> {
    return this.request<PendingPost[]>('/v1/labs/pending');
  }
}

// Create and export API client instance
export const apiClient = new ApiClient();

// Export individual methods for easier use
export const {
  getAccount,
  getAccounts,
  getAccountHistory,
  getBlock,
  getBlocks,
  getLatestBlocks,
  getWitnesses,
  getWitness,
  getTopWitnesses,
  getGlobalStats,
  getBlockchainProps,
  getDashboard,
  search,
  getAccountGrowthData,
  getBlockProductionData,
  getTransactionVolumeData,
  getWitnessVotingData,
  healthCheck,
  getPosts,
  getPost,
  getPostsByDate,
  getPostReplies,
  getPostVotes,
  getPostReblogs,
  getLabsIndex,
  getPowerUps,
  getPowerDowns,
  getRshares,
  getCuration,
  getAuthor,
  getFlags,
  getClients,
  getBenefactors,
  getPending,
} = apiClient;
