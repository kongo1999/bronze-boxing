import { cn } from "@/lib/utils";
import { initials } from "@/lib/utils";

export function Avatar({
  name,
  className,
}: {
  name: string;
  className?: string;
}) {
  return (
    <span
      className={cn(
        "inline-flex h-10 w-10 shrink-0 items-center justify-center rounded-full border border-line bg-elevated font-display text-sm font-semibold text-muted",
        className,
      )}
      aria-hidden
    >
      {initials(name) || "?"}
    </span>
  );
}
