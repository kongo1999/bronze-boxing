// Phone helpers. Numbers are stored as typed (local format reads best), and
// turned into call / WhatsApp links only when they're usable.

const COUNTRY = ((import.meta.env.VITE_PHONE_COUNTRY_CODE as string) || "961").replace(/\D/g, "");

export function digits(p?: string): string {
  return (p ?? "").replace(/\D/g, "");
}

/** tel: link, or "" when there aren't enough digits to dial. */
export function telHref(p?: string): string {
  const d = digits(p);
  if (d.length < 6) return "";
  return `tel:${(p ?? "").trim().startsWith("+") ? "+" : ""}${d}`;
}

/**
 * WhatsApp needs the full international number. "+961 3 111 222" is used as
 * is; a local "03 111 222" gets the studio's country code with the trunk 0
 * dropped. Returns "" when the result can't be a real number.
 */
export function whatsappHref(p?: string): string {
  const raw = (p ?? "").trim();
  let d = digits(raw);
  if (!d) return "";
  if (raw.startsWith("+")) {
    // already international
  } else if (d.startsWith("00")) {
    d = d.slice(2);
  } else {
    d = COUNTRY + d.replace(/^0+/, "");
  }
  if (d.length < 8 || d.length > 15) return "";
  return `https://wa.me/${d}`;
}
