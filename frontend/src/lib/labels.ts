// One vocabulary for money labels across screens.

export const PAY_TYPE_LABEL: Record<string, string> = {
  subscription: "Subscription",
  private: "Private session",
  dropin: "Drop-in",
  sale: "Shop sale",
  return: "Return",
  other: "Other",
};

export const METHOD_LABEL: Record<string, string> = {
  cash: "Cash",
  card: "Card",
  bank_transfer: "Bank transfer",
  other: "Other",
  unspecified: "Unspecified",
};

/** Methods offered when recording money (a tiny fixed set → visible buttons). */
export const METHODS = [
  { v: "cash", l: "Cash" },
  { v: "card", l: "Card" },
  { v: "bank_transfer", l: "Transfer" },
  { v: "other", l: "Other" },
] as const;

export const CATEGORY_LABEL: Record<string, string> = {
  rent: "Rent",
  equipment: "Equipment",
  utilities: "Utilities",
  supplies: "Supplies",
  wages: "Wages",
  other: "Other",
};

export const methodLabel = (m?: string) => METHOD_LABEL[m || "unspecified"] ?? m ?? "Unspecified";
export const payTypeLabel = (t?: string) => PAY_TYPE_LABEL[t ?? ""] ?? t ?? "";
export const categoryLabel = (c?: string) => CATEGORY_LABEL[c ?? ""] ?? c ?? "";
