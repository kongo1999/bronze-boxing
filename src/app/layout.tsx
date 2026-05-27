import type { Metadata, Viewport } from "next";
import { Geist, Oswald } from "next/font/google";
import "./globals.css";
import { AppShell } from "@/components/shell/AppShell";
import { APP_NAME, APP_TAGLINE } from "@/lib/constants";

const geist = Geist({ variable: "--font-geist", subsets: ["latin"] });
const oswald = Oswald({
  variable: "--font-oswald",
  subsets: ["latin"],
  weight: ["500", "600", "700"],
});

export const metadata: Metadata = {
  title: `${APP_NAME} — ${APP_TAGLINE}`,
  description: "Schedule, roster, payments and reminders for the boxing studio.",
};

export const viewport: Viewport = {
  themeColor: "oklch(0.165 0.006 68)",
  width: "device-width",
  initialScale: 1,
};

export default function RootLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="en" className={`${geist.variable} ${oswald.variable} antialiased`}>
      <body>
        <AppShell>{children}</AppShell>
      </body>
    </html>
  );
}
