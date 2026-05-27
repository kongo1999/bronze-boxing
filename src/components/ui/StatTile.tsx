import { cn } from "@/lib/utils";

/**
 * A single headline figure. The `accent` variant fills bronze and is reserved for
 * the one most important number on a screen, so the eye has a clear anchor (we
 * deliberately avoid a grid of identical-weight tiles).
 */
export function StatTile({
  label,
  value,
  sub,
  accent = false,
  className,
}: {
  label: string;
  value: React.ReactNode;
  sub?: React.ReactNode;
  accent?: boolean;
  className?: string;
}) {
  return (
    <div
      className={cn(
        "rounded-2xl border p-4 shadow-soft",
        accent
          ? "border-transparent bg-bronze text-bronze-ink"
          : "border-line bg-surface",
        className,
      )}
    >
      <p
        className={cn(
          "label-eyebrow text-[0.625rem]",
          accent ? "text-bronze-ink/70" : "text-faint",
        )}
      >
        {label}
      </p>
      <p className="mt-1 font-display text-3xl font-semibold tracking-tight tnum">
        {value}
      </p>
      {sub ? (
        <p className={cn("mt-0.5 text-sm", accent ? "text-bronze-ink/80" : "text-muted")}>
          {sub}
        </p>
      ) : null}
    </div>
  );
}
