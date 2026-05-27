import mongoose from "mongoose";

/**
 * Globally-cached Mongoose connection.
 *
 * Next.js (and especially dev hot-reload) can evaluate this module many times.
 * Without caching we'd open a new connection on every reload and exhaust the
 * MongoDB connection pool. We stash the connection (and its in-flight promise)
 * on `globalThis` so it survives module re-evaluation.
 */

type MongooseCache = {
  conn: typeof mongoose | null;
  promise: Promise<typeof mongoose> | null;
};

declare global {
  var _mongooseCache: MongooseCache | undefined;
}

const cached: MongooseCache =
  global._mongooseCache ?? (global._mongooseCache = { conn: null, promise: null });

export async function dbConnect(): Promise<typeof mongoose> {
  if (cached.conn) return cached.conn;

  if (!cached.promise) {
    const uri = process.env.MONGODB_URI;
    if (!uri) {
      throw new Error(
        "MONGODB_URI is not set. Add it to .env.local (see .env.example).",
      );
    }

    cached.promise = mongoose.connect(uri, {
      bufferCommands: false, // fail fast instead of queueing when disconnected
      maxPoolSize: 10,
      serverSelectionTimeoutMS: 10_000,
      heartbeatFrequencyMS: 30_000,
    });
  }

  try {
    cached.conn = await cached.promise;
  } catch (err) {
    // Reset so the next request can retry a fresh connection.
    cached.promise = null;
    throw err;
  }

  return cached.conn;
}
