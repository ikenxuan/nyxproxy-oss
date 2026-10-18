import type { Metadata, Viewport } from 'next';
import type { ReactNode } from 'react';
import './globals.css';
import { Providers } from './providers';

export const metadata: Metadata = {
  title: {
    default: 'NyxProxy Console',
    template: '%s · NyxProxy',
  },
  description: 'NyxProxy web console',
  applicationName: 'NyxProxy',
};

export const viewport: Viewport = {
  width: 'device-width',
  initialScale: 1,
  themeColor: [
    { media: '(prefers-color-scheme: light)', color: '#f4fafc' },
    { media: '(prefers-color-scheme: dark)', color: '#0b1216' },
  ],
};

// Applies the saved theme before first paint so dark mode never flashes light.
// Both the class and data attribute are set because HeroUI reads either.
const themeScript = `(function(){try{var t=localStorage.getItem('nyxproxy-theme');var d=t==='dark'||((!t||t==='system')&&window.matchMedia('(prefers-color-scheme: dark)').matches);var r=document.documentElement;r.classList.toggle('dark',d);r.dataset.theme=d?'dark':'light';}catch(e){}})();`;

export default function RootLayout({ children }: { children: ReactNode }) {
  return (
    <html lang="en" data-theme="light" suppressHydrationWarning>
      <head>
        <script dangerouslySetInnerHTML={{ __html: themeScript }} />
      </head>
      <body className="antialiased">
        <Providers>{children}</Providers>
      </body>
    </html>
  );
}
