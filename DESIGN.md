# Bronze Boxing — Design System ("Ringside")

Dark, warm, premium. The mood is a fight-night broadcast graphics package, not a
gritty gym poster. Sharp edges, confident numerals, bronze used sparingly so it
actually means "important." All colors OKLCH; neutrals tinted warm (hue ~65), never
pure black/white.

## Color tokens (OKLCH)

| Token | Value | Use |
|---|---|---|
| `canvas` | `oklch(0.165 0.006 68)` | Page background (warm near-black) |
| `surface` | `oklch(0.205 0.008 68)` | Cards, panels |
| `elevated` | `oklch(0.255 0.009 68)` | Inputs, raised chips, hover |
| `line` | `oklch(0.305 0.010 68)` | Borders, dividers |
| `fg` | `oklch(0.965 0.006 80)` | Primary text |
| `muted` | `oklch(0.730 0.010 75)` | Secondary text |
| `faint` | `oklch(0.560 0.010 70)` | Tertiary text, captions |
| `bronze` | `oklch(0.720 0.130 64)` | Accent: active nav, primary CTA, key figures |
| `bronze-strong` | `oklch(0.785 0.140 66)` | Bronze hover / emphasis |
| `bronze-ink` | `oklch(0.190 0.020 64)` | Text on bronze fills |
| `paid` | `oklch(0.730 0.140 162)` | Paid / attended / done (emerald) |
| `overdue` | `oklch(0.645 0.180 26)` | Overdue / no-show / danger (red-clay) |
| `partial` | `oklch(0.800 0.150 85)` | Partial / pending (amber) |
| `info` | `oklch(0.700 0.095 240)` | Private session / neutral info (steel) |

Status chips: `text-<role>` on `bg-<role>/15` with a `border-<role>/25`. Status is
**never** color alone, always paired with a word or icon.

## Typography

- **Display** (`font-display`): Oswald, condensed and athletic. Used for the wordmark,
  section titles, stat numerals, nav labels. Uppercase + tracking for labels.
- **Body** (`font-sans`): Geist. Everything readable: names, notes, form fields.
- Numerals in the ledger use `tabular-nums` so columns align.
- Scale steps keep ≥1.25 contrast; weight contrast does the hierarchy work.

## Layout & elevation

- Mobile-first. Content max-width ~`28rem` centered on larger screens; the shell keeps
  the phone-app feel even on desktop, with a left rail replacing the bottom nav ≥`md`.
- Elevation = lighter surface + 1px `line` border + a soft low shadow. No glassmorphism.
- Generous vertical rhythm; section headers are small uppercase bronze/faint labels.
- Radius: cards `rounded-2xl`, controls `rounded-xl`, chips `rounded-full`.

## Motion

- Ease-out only (`--ease-out-quart`). Press feedback `active:scale-[0.98]`. No bounce.
- Animate transform/opacity, never layout.

## Components

- **Card**: surface + line border + soft shadow, `rounded-2xl`, padded.
- **StatTile**: label (uppercase faint) + big display numeral + sublabel; accent variant
  fills bronze for the single most important figure (deliberately not a grid of equals).
- **Button**: `primary` (bronze fill), `ghost` (line border), `danger`. 44px min target.
- **Badge**: status chip per table above.
- **BottomNav** (mobile) / **SideRail** (desktop): 5 destinations, center is the bronze
  "quick add" action.
- **EmptyState, PageHeader, Field/Input/Select/Textarea, Avatar (initials).**
