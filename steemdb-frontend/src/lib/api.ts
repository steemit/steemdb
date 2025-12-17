import type { 
  ApiResponse, 
  Account, 
  Block, 
  Witness, 
  GlobalStats, 
  BlockchainProps,
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
} = apiClient;
