import { type ClassValue, clsx } from "clsx"
import { twMerge } from "tailwind-merge"

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

// Format numbers with commas
export function formatNumber(num: number): string {
  return new Intl.NumberFormat().format(num)
}

// Format large numbers with K, M, B suffixes
export function formatCompactNumber(num: number): string {
  const formatter = new Intl.NumberFormat('en', { notation: 'compact' })
  return formatter.format(num)
}

// Format currency amounts
export function formatCurrency(amount: string | number, currency: string = 'STEEM'): string {
  const num = typeof amount === 'string' ? parseFloat(amount) : amount
  if (isNaN(num)) return `${amount} ${currency}`
  
  // STEEM and SBD: 3 decimal places
  if (currency === 'STEEM' || currency === 'SBD') {
    return `${num.toFixed(3)} ${currency}`
  }
  // VESTS: 6 decimal places
  if (currency === 'VESTS') {
    return `${num.toFixed(6)} ${currency}`
  }
  // Default: use formatNumber
  return `${formatNumber(num)} ${currency}`
}

// Format VESTS amounts (6 decimal places)
export function formatVests(amount: string | number): string {
  const num = typeof amount === 'string' ? parseAmount(amount) : amount
  if (isNaN(num)) return String(amount)
  return `${num.toFixed(6)} VESTS`
}

// Format time ago
export function formatTimeAgo(date: Date | string): string {
  const now = new Date()
  const past = new Date(date)
  const diffInSeconds = Math.floor((now.getTime() - past.getTime()) / 1000)

  if (diffInSeconds < 60) {
    return `${diffInSeconds}s ago`
  } else if (diffInSeconds < 3600) {
    const minutes = Math.floor(diffInSeconds / 60)
    return `${minutes}m ago`
  } else if (diffInSeconds < 86400) {
    const hours = Math.floor(diffInSeconds / 3600)
    return `${hours}h ago`
  } else {
    const days = Math.floor(diffInSeconds / 86400)
    return `${days}d ago`
  }
}

// Format date
export function formatDate(date: Date | string): string {
  return new Intl.DateTimeFormat('en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(date))
}

// Parse Steem amount string (e.g., "123.456 STEEM" -> 123.456)
export function parseAmount(amountStr: string): number {
  const match = amountStr.match(/^(\d+\.?\d*)/);
  return match ? parseFloat(match[1]) : 0;
}

// Format reputation score
export function formatReputation(rep: string | number): number {
  const reputation = typeof rep === 'string' ? parseInt(rep) : rep;
  if (reputation === 0) return 25;
  
  const neg = reputation < 0;
  let reputationLevel = Math.log10(Math.abs(reputation));
  reputationLevel = Math.max(reputationLevel - 9, 0);
  reputationLevel = (neg ? -1 : 1) * reputationLevel;
  reputationLevel = reputationLevel * 9 + 25;
  
  return Math.floor(reputationLevel);
}

// Truncate text
export function truncateText(text: string, maxLength: number): string {
  if (text.length <= maxLength) return text;
  return text.substring(0, maxLength) + '...';
}

// Generate avatar URL
export function getAvatarUrl(username: string): string {
  return `https://images.hive.blog/u/${username}/avatar/small`;
}

// Validate Steem username
export function isValidSteemUsername(username: string): boolean {
  const regex = /^[a-z][a-z0-9\-\.]{2,15}$/;
  return regex.test(username);
}

// Convert vest to SP (approximate)
export function vestToSP(vests: string, totalVests: string, totalSteem: string): number {
  const vestAmount = parseAmount(vests);
  const totalVestAmount = parseAmount(totalVests);
  const totalSteemAmount = parseAmount(totalSteem);
  
  if (totalVestAmount === 0) return 0;
  return (vestAmount * totalSteemAmount) / totalVestAmount;
}

// Calculate voting power
export function calculateVotingPower(lastVoteTime: Date, currentTime: Date = new Date()): number {
  const timeDiff = currentTime.getTime() - lastVoteTime.getTime();
  const secondsDiff = timeDiff / 1000;
  
  // Voting power regenerates at 20% per day (0.2 / 86400 seconds)
  const regenRate = 0.2 / 86400;
  const regenAmount = secondsDiff * regenRate * 100;
  
  return Math.min(100, regenAmount);
}

// Debounce function
export function debounce<T extends (...args: any[]) => any>(
  func: T,
  wait: number
): (...args: Parameters<T>) => void {
  let timeout: number;
  return (...args: Parameters<T>) => {
    clearTimeout(timeout);
    timeout = window.setTimeout(() => func(...args), wait);
  };
}

// Copy to clipboard
export async function copyToClipboard(text: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch (err) {
    // Fallback for older browsers
    const textArea = document.createElement('textarea');
    textArea.value = text;
    document.body.appendChild(textArea);
    textArea.select();
    try {
      document.execCommand('copy');
      document.body.removeChild(textArea);
      return true;
    } catch (fallbackErr) {
      document.body.removeChild(textArea);
      return false;
    }
  }
}

// Format scientific notation to readable format
export function formatScientificNotation(value: number | string): string {
  const num = typeof value === 'string' ? parseFloat(value) : value;
  if (isNaN(num)) return String(value);
  
  // If it's a very large number in scientific notation
  if (Math.abs(num) >= 1e15) {
    const str = num.toExponential(3);
    const [base, exp] = str.split('e');
    const exponent = parseInt(exp);
    return `${base}E${exponent}`;
  }
  
  // For smaller numbers, use regular formatting
  return formatNumber(num);
}

// Format Unix timestamp (milliseconds)
export function formatTimestamp(timestamp: number | string): string {
  const ts = typeof timestamp === 'string' ? parseInt(timestamp) : timestamp;
  if (isNaN(ts)) return String(timestamp);
  
  // Handle both seconds and milliseconds
  const date = ts > 1e12 ? new Date(ts) : new Date(ts * 1000);
  return formatDate(date);
}

// Format key name (snake_case to Title Case)
export function formatKeyName(key: string): string {
  return key
    .split('_')
    .map(word => word.charAt(0).toUpperCase() + word.slice(1).toLowerCase())
    .join(' ');
}

// Format rate with 3 decimal places
export function formatRate(value: number): string {
  return value.toFixed(3);
}
