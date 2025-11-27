import type { WebSocketMessage, SubscriptionRequest, WebSocketState } from '../types';

export type WebSocketEventHandler = (message: WebSocketMessage) => void;
export type WebSocketStateHandler = (state: WebSocketState) => void;

class WebSocketClient {
  private ws: WebSocket | null = null;
  private url: string;
  private reconnectAttempts = 0;
  private maxReconnectAttempts = 5;
  private reconnectDelay = 1000;
  private subscriptions = new Set<string>();
  private eventHandlers = new Map<string, Set<WebSocketEventHandler>>();
  private stateHandlers = new Set<WebSocketStateHandler>();
  private currentState: WebSocketState = 'disconnected';

  constructor(url?: string) {
    this.url = url || this.getWebSocketUrl();
  }

  private getWebSocketUrl(): string {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const host = import.meta.env.VITE_WS_HOST || window.location.host;
    const path = import.meta.env.VITE_WS_PATH || '/ws';
    return `${protocol}//${host}${path}`;
  }

  private setState(state: WebSocketState) {
    if (this.currentState !== state) {
      this.currentState = state;
      this.stateHandlers.forEach(handler => handler(state));
    }
  }

  private emit(channel: string, message: WebSocketMessage) {
    const handlers = this.eventHandlers.get(channel);
    if (handlers) {
      handlers.forEach(handler => handler(message));
    }

    // Also emit to 'all' channel for global listeners
    const allHandlers = this.eventHandlers.get('all');
    if (allHandlers) {
      allHandlers.forEach(handler => handler(message));
    }
  }

  connect(): Promise<void> {
    return new Promise((resolve, reject) => {
      if (this.ws?.readyState === WebSocket.OPEN) {
        resolve();
        return;
      }

      this.setState('connecting');

      try {
        this.ws = new WebSocket(this.url);

        this.ws.onopen = () => {
          console.log('WebSocket connected');
          this.setState('connected');
          this.reconnectAttempts = 0;

          // Resubscribe to all channels
          this.subscriptions.forEach(channel => {
            this.sendSubscription('subscribe', channel);
          });

          resolve();
        };

        this.ws.onmessage = (event) => {
          try {
            const message: WebSocketMessage = JSON.parse(event.data);
            this.emit(message.channel || message.type, message);
          } catch (error) {
            console.error('Failed to parse WebSocket message:', error);
          }
        };

        this.ws.onclose = (event) => {
          console.log('WebSocket closed:', event.code, event.reason);
          this.setState('disconnected');
          this.ws = null;

          // Attempt to reconnect if not a clean close
          if (event.code !== 1000 && this.reconnectAttempts < this.maxReconnectAttempts) {
            this.scheduleReconnect();
          }
        };

        this.ws.onerror = (error) => {
          console.error('WebSocket error:', error);
          this.setState('error');
          reject(new Error('WebSocket connection failed'));
        };

      } catch (error) {
        this.setState('error');
        reject(error);
      }
    });
  }

  private scheduleReconnect() {
    this.reconnectAttempts++;
    const delay = this.reconnectDelay * Math.pow(2, this.reconnectAttempts - 1);
    
    console.log(`Attempting to reconnect in ${delay}ms (attempt ${this.reconnectAttempts}/${this.maxReconnectAttempts})`);
    
    setTimeout(() => {
      this.connect().catch(error => {
        console.error('Reconnection failed:', error);
      });
    }, delay);
  }

  disconnect() {
    if (this.ws) {
      this.ws.close(1000, 'Client disconnect');
      this.ws = null;
    }
    this.setState('disconnected');
  }

  private sendSubscription(action: 'subscribe' | 'unsubscribe', channel: string) {
    if (this.ws?.readyState === WebSocket.OPEN) {
      const request: SubscriptionRequest = { action, channel };
      this.ws.send(JSON.stringify(request));
    }
  }

  subscribe(channel: string): void {
    this.subscriptions.add(channel);
    this.sendSubscription('subscribe', channel);
  }

  unsubscribe(channel: string): void {
    this.subscriptions.delete(channel);
    this.sendSubscription('unsubscribe', channel);
  }

  on(channel: string, handler: WebSocketEventHandler): () => void {
    if (!this.eventHandlers.has(channel)) {
      this.eventHandlers.set(channel, new Set());
    }
    this.eventHandlers.get(channel)!.add(handler);

    // Return unsubscribe function
    return () => {
      const handlers = this.eventHandlers.get(channel);
      if (handlers) {
        handlers.delete(handler);
        if (handlers.size === 0) {
          this.eventHandlers.delete(channel);
        }
      }
    };
  }

  onStateChange(handler: WebSocketStateHandler): () => void {
    this.stateHandlers.add(handler);

    // Return unsubscribe function
    return () => {
      this.stateHandlers.delete(handler);
    };
  }

  getState(): WebSocketState {
    return this.currentState;
  }

  isConnected(): boolean {
    return this.currentState === 'connected';
  }

  getSubscriptions(): string[] {
    return Array.from(this.subscriptions);
  }
}

// Create and export WebSocket client instance
export const wsClient = new WebSocketClient();

// Auto-connect when the module is imported
if (typeof window !== 'undefined') {
  // Connect after a short delay to allow the app to initialize
  setTimeout(() => {
    wsClient.connect().catch(error => {
      console.error('Initial WebSocket connection failed:', error);
    });
  }, 1000);
}

export default wsClient;
