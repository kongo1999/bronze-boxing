import { cn } from "@/lib/utils";

type Variant = "primary" | "ghost" | "danger" | "subtle";
type Size = "sm" | "md" | "icon";

const base =
  "inline-flex items-center justify-center gap-2 rounded-xl font-medium transition-[transform,background-color,border-color] duration-150 ease-[var(--ease-out-quart)] active:scale-[0.98] disabled:pointer-events-none disabled:opacity-50 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-bronze";

const variants: Record<Variant, string> = {
  primary: "bg-bronze text-bronze-ink hover:bg-bronze-strong",
  ghost: "border border-line bg-transparent text-fg hover:bg-elevated",
  danger: "border border-overdue/30 bg-overdue/15 text-overdue hover:bg-overdue/25",
  subtle: "bg-elevated text-fg hover:bg-line/60",
};

const sizes: Record<Size, string> = {
  sm: "h-9 px-3 text-sm",
  md: "h-11 px-4 text-sm",
  icon: "h-11 w-11",
};

export function buttonClasses(variant: Variant = "primary", size: Size = "md") {
  return cn(base, variants[variant], sizes[size]);
}

export function Button({
  variant = "primary",
  size = "md",
  className,
  ...props
}: React.ComponentProps<"button"> & { variant?: Variant; size?: Size }) {
  return <button className={cn(buttonClasses(variant, size), className)} {...props} />;
}
