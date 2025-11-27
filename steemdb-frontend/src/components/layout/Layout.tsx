import { Outlet } from 'react-router-dom';
import { Header } from './Header';
import { Sidebar } from './Sidebar';
import { cn } from '../../lib/utils';

export function Layout() {

  return (
    <div className="min-h-screen bg-background">
      <Header />
      <div className="flex">
        <Sidebar />
        <main
          className={cn(
            'flex-1 transition-all duration-200 ease-in-out',
            'md:ml-64', // Always offset by sidebar width on desktop
            'pt-4' // Top padding for content
          )}
        >
          <div className="container mx-auto px-4 pb-8">
            <Outlet />
          </div>
        </main>
      </div>
    </div>
  );
}
