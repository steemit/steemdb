# SteemDB Frontend

Modern React-based frontend application for the SteemDB blockchain explorer, built with React 19, TypeScript, and Tailwind CSS.

## 🚀 Features

- **React 19**: Latest React with modern features and optimizations
- **TypeScript**: Full type safety and enhanced developer experience
- **Vite**: Lightning-fast build tool and dev server
- **Tailwind CSS**: Utility-first CSS framework for rapid UI development
- **Zustand**: Lightweight state management
- **TanStack Query**: Powerful data fetching and caching
- **React Router DOM**: Declarative routing
- **D3.js & Recharts**: Data visualization libraries

## 🛠 Technology Stack

- **Framework**: React 19
- **Language**: TypeScript
- **Build Tool**: Vite
- **Styling**: Tailwind CSS 3.4
- **State Management**: Zustand
- **Data Fetching**: TanStack Query (React Query)
- **Routing**: React Router DOM
- **Charts**: D3.js, Recharts
- **Package Manager**: pnpm

## 📁 Project Structure

```
steemdb-frontend/
├── src/
│   ├── components/          # Reusable UI components
│   │   ├── ui/              # Basic UI components (Button, Card, etc.)
│   │   └── layout/          # Layout components (Header, Sidebar, etc.)
│   ├── pages/               # Page components
│   ├── lib/                 # Utilities and helpers
│   │   ├── api.ts           # API client
│   │   ├── websocket.ts     # WebSocket client
│   │   └── utils.ts         # Utility functions
│   ├── store/               # Zustand stores
│   ├── types/               # TypeScript type definitions
│   ├── App.tsx              # Main app component
│   └── main.tsx             # Application entry point
├── public/                  # Static assets
├── dist/                   # Build output (generated)
├── index.html              # HTML template
├── package.json            # Dependencies and scripts
├── vite.config.ts          # Vite configuration
├── tailwind.config.js       # Tailwind CSS configuration
└── tsconfig.json           # TypeScript configuration
```

## 🚀 Quick Start

### Prerequisites

- Node.js 18 or later
- pnpm 8 or later

### Installation

1. **Install dependencies**
   ```bash
   pnpm install
   ```

2. **Start development server**
   ```bash
   pnpm run dev
   ```

3. **Open in browser**
   ```
   http://localhost:5173
   ```

### Development

The development server runs on `http://localhost:5173` with hot module replacement (HMR) enabled.

**Available Scripts:**

```bash
# Start development server
pnpm run dev

# Build for production
pnpm run build

# Preview production build
pnpm run preview

# Run linter
pnpm run lint

# Type check
pnpm run type-check
```

## 🔧 Configuration

### Environment Variables

Create a `.env` file in the project root:

```env
# API Configuration
VITE_API_BASE_URL=http://localhost/api/v1
VITE_WS_URL=ws://localhost/ws

# Feature Flags
VITE_ENABLE_ANALYTICS=false
```

### API Integration

The frontend communicates with the backend API through:

- **REST API**: Configured in `src/lib/api.ts`
- **WebSocket**: Configured in `src/lib/websocket.ts`

Both are configured to use relative URLs in production builds, ensuring compatibility with the Nginx reverse proxy setup.

## 🎨 Styling

### Tailwind CSS

The project uses Tailwind CSS for styling. Configuration is in `tailwind.config.js`.

**Customization:**
- Edit `tailwind.config.js` to customize theme, colors, and utilities
- Add custom CSS in `src/index.css`
- Use Tailwind utility classes directly in components

### Component Styling

Components are styled using:
- Tailwind utility classes
- CSS modules (if needed)
- Inline styles for dynamic values

## 📦 Building for Production

### Build Process

1. **Build the application**
   ```bash
   pnpm run build
   ```

2. **Output location**
   The built files are in the `dist/` directory, which is automatically copied to the `steemdb-web` Docker image during the build process.

### Production Deployment

The frontend is integrated into the `steemdb-web` service:

1. **Docker Build**: The frontend is built during the `steemdb-web` Docker image build
2. **Nginx Serving**: Static files are served by Nginx in the `steemdb-web` container
3. **SPA Routing**: Nginx is configured to handle React Router routes

See the main [README.md](../README.md) for deployment instructions.

## 🧪 Testing

### Running Tests

```bash
# Run tests (when test framework is added)
pnpm test

# Run tests in watch mode
pnpm test:watch

# Run tests with coverage
pnpm test:coverage
```

## 📚 Development Guidelines

### Component Structure

```tsx
// Example component structure
import { useState } from 'react';
import { Button } from '@/components/ui/Button';

export const MyComponent = () => {
  const [state, setState] = useState();

  return (
    <div className="container mx-auto">
      <Button onClick={() => setState('clicked')}>
        Click me
      </Button>
    </div>
  );
};
```

### State Management

Use Zustand for global state:

```tsx
import { useStore } from '@/store';

export const MyComponent = () => {
  const { data, setData } = useStore();
  
  return <div>{data}</div>;
};
```

### Data Fetching

Use TanStack Query for API calls:

```tsx
import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api';

export const MyComponent = () => {
  const { data, isLoading } = useQuery({
    queryKey: ['blocks'],
    queryFn: () => api.getBlocks(),
  });

  if (isLoading) return <div>Loading...</div>;
  return <div>{data}</div>;
};
```

### TypeScript

- Use TypeScript for all components
- Define types in `src/types/`
- Use strict type checking
- Avoid `any` types

## 🔗 Integration with Backend

### API Client

The API client (`src/lib/api.ts`) handles:
- Base URL configuration
- Request/response interceptors
- Error handling
- Type-safe API calls

### WebSocket Client

The WebSocket client (`src/lib/websocket.ts`) handles:
- Connection management
- Message subscription
- Automatic reconnection
- Event handling

## 🐛 Troubleshooting

### Common Issues

**Build Errors**
- Clear `node_modules` and reinstall: `rm -rf node_modules && pnpm install`
- Clear Vite cache: `rm -rf node_modules/.vite`

**Type Errors**
- Run type check: `pnpm run type-check`
- Ensure all dependencies are installed

**Styling Issues**
- Verify Tailwind is properly configured
- Check that PostCSS is set up correctly
- Ensure Tailwind directives are in `src/index.css`

## 🤝 Contributing

1. Create a feature branch
2. Make your changes
3. Ensure code follows the project's style guide
4. Test your changes
5. Submit a pull request

### Code Style

- Use TypeScript for all new code
- Follow React best practices
- Use functional components with hooks
- Keep components small and focused
- Use meaningful component and variable names

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](../LICENSE) file for details.

## 🆘 Support

- **Documentation**: See main [README.md](../README.md)
- **Issues**: [GitHub Issues](https://github.com/steemdb/steemdb/issues)
- **Discussions**: [GitHub Discussions](https://github.com/steemdb/steemdb/discussions)

---

**Part of the SteemDB project** - [Back to main README](../README.md)
