import { Navigation } from "./Navigation";
import { TopBar } from "./TopBar";

export function AppShell({ children }: { children: React.ReactNode }) {
  return (
    <div className="min-h-dvh md:pl-60">
      <Navigation />
      <TopBar />
      <main className="mx-auto w-full max-w-md px-4 pb-28 pt-5 md:max-w-2xl md:px-8 md:pb-12 md:pt-8">
        {children}
      </main>
    </div>
  );
}
