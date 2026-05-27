"use client";

import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useRef, useTransition } from "react";
import { Search } from "lucide-react";

export function TraineeSearch({ initialQuery }: { initialQuery?: string }) {
  const router = useRouter();
  const pathname = usePathname();
  const params = useSearchParams();
  const [, startTransition] = useTransition();
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);

  function onChange(value: string) {
    clearTimeout(timer.current);
    timer.current = setTimeout(() => {
      const next = new URLSearchParams(params);
      if (value.trim()) next.set("q", value.trim());
      else next.delete("q");
      startTransition(() => router.replace(`${pathname}?${next}`, { scroll: false }));
    }, 250);
  }

  return (
    <div className="relative">
      <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-faint" />
      <input
        type="search"
        defaultValue={initialQuery}
        onChange={(e) => onChange(e.target.value)}
        placeholder="Search crew…"
        aria-label="Search trainees"
        className="h-11 w-full rounded-xl border border-line bg-elevated pl-9 pr-3 text-fg placeholder:text-faint focus:border-bronze focus:outline-none"
      />
    </div>
  );
}
