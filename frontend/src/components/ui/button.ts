export type BtnVariant = "primary" | "ghost" | "danger";
export type BtnSize = "md" | "sm" | "icon";

// Shared button classes — usable on <Button>, <router-link>, or <a>.
// Tactile: bevel + drop shadow for depth, hover lift, press-down on active.
export function btnClasses(variant: BtnVariant = "primary", size: BtnSize = "md"): string {
  const base =
    "inline-flex select-none items-center justify-center gap-2 rounded-xl font-medium " +
    "shadow-[var(--shadow-btn)] " +
    "transition-[transform,box-shadow,background-color,border-color] duration-150 ease-[var(--ease-out-quart)] " +
    "hover:-translate-y-0.5 hover:shadow-[var(--shadow-btn-hover)] " +
    "active:translate-y-0 active:scale-[0.97] active:shadow-[var(--shadow-btn)] " +
    "disabled:pointer-events-none disabled:opacity-50 disabled:shadow-none disabled:hover:translate-y-0";
  const sizes: Record<BtnSize, string> = {
    md: "h-11 px-4 text-sm",
    sm: "h-9 px-3 text-sm",
    icon: "h-10 w-10",
  };
  const variants: Record<BtnVariant, string> = {
    // Tertiary purple: vertical gradient (lighter top → base) reads as a lit face.
    primary:
      "border border-purple-strong/50 bg-gradient-to-b from-purple-strong to-purple text-purple-ink hover:from-purple hover:to-purple-strong",
    ghost:
      "border border-line bg-elevated/50 text-fg hover:border-purple/40 hover:bg-elevated hover:text-purple",
    danger:
      "border border-overdue/40 bg-overdue/10 text-overdue hover:bg-overdue/20",
  };
  return `${base} ${sizes[size]} ${variants[variant]}`;
}
