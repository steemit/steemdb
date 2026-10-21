/**
 * Client-side analytics bootstrap.
 *
 * The web service keeps NO analytics snippet in the static HTML (the
 * previous mechanism sed-replaced a `__GA_ID__` placeholder in index.html
 * on every container start). It publishes the selection via
 * GET /api/v1/meta (steemdb-web/internal/api/meta_handler.go) and this
 * module loads the matching third-party script on the client:
 *
 *   - "ga"    -> Google Analytics 4 (gtag)
 *   - "umami" -> self-hosted Umami (<base>/script.js)
 *   - absent  -> no tracking
 *
 * Analytics must never break the app: a failed /api/v1/meta fetch, a
 * malformed response, or a script-load error only disables tracking for
 * this visit. The pageview is sent when the config arrives rather than at
 * DOMContentLoaded — a short, imperceptible delay for an SPA.
 */
import { apiClient } from './api';
import type { MetaInfo } from './api';

// Mirrors the guard the old inline index.html script used before loading
// gtag (and the server-side check in steemdb-web Normalize): GA4
// measurement IDs are "G-" + alphanumerics.
const gaIDPattern = /^G-[A-Z0-9]+$/i;

let initialized = false;

export async function initAnalytics(): Promise<void> {
  if (initialized) return;
  initialized = true;

  let meta: MetaInfo | undefined;
  try {
    meta = (await apiClient.getMeta()) as MetaInfo | undefined;
  } catch {
    return; // failed fetch -> no tracking, never block the app
  }

  const a = meta?.analytics;
  if (!a) return;

  try {
    if (a.provider === 'ga' && a.id && gaIDPattern.test(a.id)) {
      loadGoogleTag(a.id);
    } else if (a.provider === 'umami' && a.url && /^https?:\/\//.test(a.url)) {
      loadUmami(a.url);
    }
    // Unknown/misconfigured provider: silently no-op. The server already
    // fails closed to "none"; this is defense in depth.
  } catch (err) {
    console.warn('Failed to initialize analytics:', err);
  }
}

// Google Analytics 4 (gtag). Mirrors the snippet the old inline index.html
// script injected, minus the placeholder machinery.
function loadGoogleTag(id: string): void {
  const w = window as unknown as {
    dataLayer?: unknown[];
    gtag?: (...args: unknown[]) => void;
  };
  w.dataLayer = w.dataLayer || [];
  w.gtag = function (...args: unknown[]) {
    w.dataLayer!.push(args);
  };
  const s = document.createElement('script');
  s.async = true;
  s.src = `https://www.googletagmanager.com/gtag/js?id=${encodeURIComponent(id)}`;
  document.head.appendChild(s);
  w.gtag('js', new Date());
  w.gtag('config', id);
}

// Self-hosted Umami: script.js lives at the instance root and identifies the
// site by the requesting hostname.
function loadUmami(baseURL: string): void {
  const u = new URL(baseURL);
  const s = document.createElement('script');
  s.defer = true;
  s.src = `${u.origin}/script.js`;
  document.head.appendChild(s);
}
