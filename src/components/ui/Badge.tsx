import { cn } from "@/lib/utils";

export type BadgeTone = "paid" | "overdue" | "partial" | "info" | "bronze" | "neutral";

const tones: Record<BadgeTone, string> = {
  paid: "text-paid bg-paid/15 border-paid/25",
  overdue: "text-overdue bg-overdue/15 border-overdue/30",
  partial: "text-partial bg-partial/15 border-partial/30",
  info: "text-info bg-info/15 border-info/25",
  bronze: "text-bronze bg-bronze/15 border-bronze/25",
  neutral: "text-muted bg-elevated border-line",
};

export function Badge({
  tone = "neutral",
  className,
  ...props
}: React.ComponentProps<"span"> & { tone?: BadgeTone }) {
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1 rounded-full border px-2.5 py-0.5 text-xs font-medium",
        tones[tone],
        className,
      )}
      {...props}
    />
  );
}
