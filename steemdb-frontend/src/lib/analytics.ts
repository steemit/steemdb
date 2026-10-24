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
 *   - "umami" -> self-hosted Umami (<base>/script.js + data-website-id)
 *   - absent  -> no tracking
 *
 * Analytics must never break the app: a failed /api/v1/meta fetch, a
 * malformed response, or a script-load error only disables tracking for
 * this visit. The pageview is sent when the config arrives rather than at
 * DOMContentLoaded — a short, imperceptible delay for an SPA.
 *
 * The app is an SPA, but nothing here sends manual page_view events on
 * route changes: GA4 Enhanced Measurement (on by default for a web data
 * stream) watches the History API itself, and the Umami tracker script
 * hooks the History API on its own. Sending them manually ON TOP of
 * Enhanced Measurement double-counts every navigation.
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
    } else if (
      a.provider === 'umami' &&
      a.url &&
      a.website_id &&
      /^https?:\/\//.test(a.url)
    ) {
      loadUmami(a.url, a.website_id);
    }
    // Unknown/misconfigured provider: silently no-op. The server already
    // fails closed to "none"; this is defense in depth.
  } catch (err) {
    console.warn('Failed to initialize analytics:', err);
  }
}

// GA records the landing page automatically (gtag('config')); SPA route
// changes via the History API are covered by GA4 Enhanced Measurement and
// by the Umami tracker's own History hook — see the header comment. Events
// pushed before gtag.js finishes downloading sit in dataLayer and are
// flushed when it arrives.

// Google Analytics 4 (gtag). Mirrors the snippet the old inline index.html
// script injected, minus the placeholder machinery.
//
// The gtag shim below pushes the built-in `arguments` object — NOT a
// rest-args array — on purpose. This is the exact shape of Google's own
// snippet, and GTM's queue treats the two differently: with
// `(...args) => dataLayer.push(args)` the entry lands as a plain Array, and
// GTM's processing loop deterministically wedges right after the gtm.load
// event (google_tag_manager.sequence freezes, zero hits, zero console
// errors — every pageview silently lost). Verified by A/B on a minimal
// page: arguments shim -> seq 15 + collect requests; array shim -> seq 13,
// loop dead. Do not "modernize" this function.
function loadGoogleTag(id: string): void {
  const w = window as unknown as {
    dataLayer?: unknown[];
    gtag?: (...args: unknown[]) => void;
  };
  w.dataLayer = w.dataLayer || [];
  w.gtag = function () {
    // eslint-disable-next-line prefer-rest-params -- must push the built-in arguments object, not a rest array; see the comment above loadGoogleTag
    w.dataLayer!.push(arguments);
  };
  const s = document.createElement('script');
  s.async = true;
  s.src = `https://www.googletagmanager.com/gtag/js?id=${encodeURIComponent(id)}`;
  s.onerror = () => console.warn('Analytics: failed to load the Google tag (gtag.js)');
  document.head.appendChild(s);
  w.gtag('js', new Date());
  w.gtag('config', id);
}

// Self-hosted Umami: script.js lives at the instance's base URL (any
// configured path is preserved, so instances behind a subpath reverse
// proxy work). The tracker attributes events via the script tag's
// data-website-id attribute — it does NOT identify the site by hostname,
// so the website UUID from the Umami dashboard is mandatory.
function loadUmami(baseURL: string, websiteID: string): void {
  const s = document.createElement('script');
  s.defer = true;
  s.src = `${baseURL.replace(/\/+$/, '')}/script.js`;
  s.setAttribute('data-website-id', websiteID);
  s.onerror = () => console.warn(`Analytics: failed to load the Umami tracker from ${baseURL}`);
  document.head.appendChild(s);
}
