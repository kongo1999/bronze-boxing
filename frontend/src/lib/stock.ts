// Shop vocabulary shared by the inventory screens.
import type { InventoryItem, StockMovement } from "./types";

export type Shortage = "out" | "low" | "";

/** "out" at zero, "low" at or under the item's threshold, else "". Mirrors the API. */
export function shortage(i: InventoryItem): Shortage {
  if (i.stock <= 0) return "out";
  if (i.lowStockThreshold && i.stock <= i.lowStockThreshold) return "low";
  return "";
}

/** Shortages first (out, then low), then by name. */
export function byUrgency(a: InventoryItem, b: InventoryItem): number {
  const rank = (i: InventoryItem) => (!i.active ? 3 : shortage(i) === "out" ? 0 : shortage(i) === "low" ? 1 : 2);
  return rank(a) - rank(b) || a.name.localeCompare(b.name);
}

export const MOVE_LABEL: Record<StockMovement["kind"], string> = {
  opening: "Opening stock",
  sale: "Sold",
  return: "Returned",
  restock: "Restocked",
  damage: "Written off",
  correction: "Count corrected",
  sale_edit: "Sale corrected",
  sale_void: "Sale voided",
};

export const DAMAGE_REASONS = ["Damaged", "Lost", "Expired", "Used in class"];
export const COUNT_REASONS = ["Shelf count", "Found extra", "Entered wrong"];
export const PRICE_REASONS = ["Discount", "Member price", "Bundle", "Damaged packaging"];
export const RETURN_REASONS = ["Wrong size", "Defective", "Changed mind"];
