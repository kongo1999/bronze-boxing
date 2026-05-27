import { dbConnect } from "@/lib/db";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

/**
 * Lightweight health/diagnostics endpoint.
 * Reports whether the DB env var is present and whether a connection succeeds,
 * without ever exposing credentials (only the host is shown).
 */
export async function GET() {
  const uri = process.env.MONGODB_URI;
  const hasUri = Boolean(uri);

  let uriHost: string | null = null;
  if (uri) {
    try {
      uriHost = new URL(uri).host;
    } catch {
      uriHost = "unparseable";
    }
  }

  let connected = false;
  let error: string | null = null;
  if (!hasUri) {
    error = "MONGODB_URI is not set in this environment";
  } else {
    try {
      await dbConnect();
      connected = true;
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    }
  }

  return Response.json({
    ok: connected,
    hasUri,
    uriHost,
    connected,
    error,
    currency: process.env.NEXT_PUBLIC_CURRENCY ?? null,
    time: new Date().toISOString(),
  });
}
