# Design

Captured from the live code (frontend/src/style.css, "Ringside" system) so new work stays on-brand.

## Theme

Dark only. Carbon-black canvas with a fixed "velvet glow": a soft royal-purple radial light from the top edge melting into the canvas. Depth comes from lightness steps and shadows, not borders alone.

## Color

OKLCH throughout. Neutrals carry a faint cool tint (hue ~285).

| Role | Token | Value |
|---|---|---|
| Canvas | `--color-canvas` | `oklch(0.14 0.006 285)` |
| Surface (cards, bars) | `--color-surface` | `oklch(0.185 0.008 285)` |
| Elevated (inputs, tiles) | `--color-elevated` | `oklch(0.235 0.009 285)` |
| Hairline | `--color-line` | `oklch(0.3 0.01 285)` |
| Foreground | `--color-fg` | `oklch(0.96 0.004 285)` |
| Muted text | `--color-muted` | `oklch(0.72 0.008 285)` |
| Faint text | `--color-faint` | `oklch(0.55 0.01 285)` |
| Bronze (secondary: accents, active nav, key figures) | `--color-bronze` | `oklch(0.63 0.1 63)` |
| Bronze strong | `--color-bronze-strong` | `oklch(0.71 0.11 64)` |
| Purple (tertiary: CTAs/FAB only) | `--color-purple` | `oklch(0.44 0.16 298)` |
| Purple strong | `--color-purple-strong` | `oklch(0.52 0.17 298)` |
| Paid / Overdue / Partial / Info | status tokens | `0.74 0.14 162` / `0.645 0.18 26` / `0.8 0.15 85` / `0.7 0.11 285` |

Strategy: Restrained. Bronze is THE accent (≤10% of surface); purple is reserved for primary action buttons and brand moments. Status colors only on status.

## Typography

- Body: Geist Sans.
- Display: Oswald (`.font-display`) for headings, key figures, wordmarks. Condensed, punchy: fits the boxing identity.
- Eyebrow labels: `.label-eyebrow` = Oswald, uppercase, 0.12em tracking, weight 600, small sizes (0.625rem).
- Numbers: `.tnum` tabular figures everywhere money or counts align.

## Shape & Elevation

- Radii: xl = 0.875rem, 2xl = 1.125rem. Cards are rounded-2xl; inputs/tiles rounded-xl.
- Shadows: `--shadow-soft` (resting cards), `--shadow-lift` (overlays, FAB), `--shadow-btn`/`--shadow-btn-hover` (beveled buttons: inset top highlight + drop).
- Hairline borders `border-line` on surfaces; brighten to `bronze/40` on hover.

## Components

- Buttons (`components/ui/button.ts`): primary = purple gradient bevel; ghost = transparent with hover elevate. Sizes sm/md.
- Cards (`Card.vue`): surface bg, line border, rounded-2xl, soft shadow.
- Tiles/list rows: `rounded-xl border border-line bg-surface px-3 py-2.5`.
- Badges, StatTile, EmptyState, Skeleton, MonthPicker, Toast: see `components/ui/`.
- Nav: desktop side rail (w-60) + mobile top bar + bottom tab bar with center purple FAB.

## Motion

- Single easing: `--ease-out-quart` `cubic-bezier(0.25, 1, 0.5, 1)`.
- Route cross-fade with 4-6px vertical drift, 180ms.
- Hover: -translate-y-0.5 + shadow swap, 150ms. Active: scale ~0.97.
- `prefers-reduced-motion` kills page transitions and pulses.

## Brand assets

- `src/assets/logo.png`: club badge, transparent, 465x368 (wider than tall: wings). Size by height (`h-* w-auto`).
- `public/favicon.png`: 64px square version.
