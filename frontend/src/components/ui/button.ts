export type BtnVariant = "primary" | "ghost" | "danger";
export type BtnSize = "md" | "sm" | "icon";

// Shared button classes — usable on <Button>, <router-link>, or <a>.
export function btnClasses(variant: BtnVariant = "primary", size: BtnSize = "md"): string {
  const base =
    "inline-flex items-center justify-center gap-2 rounded-xl font-medium transition-[background-color,border-color,transform] active:scale-[0.98] disabled:pointer-events-none disabled:opacity-50";
  const sizes: Record<BtnSize, string> = {
    md: "h-11 px-4 text-sm",
    sm: "h-9 px-3 text-sm",
    icon: "h-10 w-10",
  };
  const variants: Record<BtnVariant, string> = {
    primary: "bg-bronze text-bronze-ink hover:bg-bronze-strong",
    ghost: "border border-line text-fg hover:bg-elevated",
    danger: "border border-overdue/30 text-overdue hover:bg-overdue/10",
  };
  return `${base} ${sizes[size]} ${variants[variant]}`;
}
