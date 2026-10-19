import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import './index.css';
import App from './App.tsx';
import { initAnalytics } from './lib/analytics';

// Boot third-party analytics from the backend's /api/v1/meta
// (steemdb-web/internal/api/meta_handler.go). Fire-and-forget: a failed
// fetch or a misconfigured provider only disables tracking — it must
// never delay or block the app render.
void initAnalytics();

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
