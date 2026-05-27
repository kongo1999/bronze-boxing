/**
 * Seed the database with realistic sample data for development/demo.
 * Run with: npm run seed
 *
 * WARNING: this clears the trainees, sessions, payments and reminders collections.
 */
import { config } from "dotenv";
config({ path: ".env.local" });

import { addDays, setHours, setMinutes, setSeconds, setMilliseconds, startOfWeek } from "date-fns";
import { randomUUID } from "crypto";
import mongoose, { type Types } from "mongoose";
import { dbConnect } from "./db";
import { Payment, Reminder, Session, Trainee } from "./models";
import type { AttendanceStatus, SessionStatus, SessionType } from "./constants";
import { monthKey } from "./utils";

/** Uniform shape so the seed array unifies cleanly for Session.create(). */
type SeedSession = {
  title: string;
  type: SessionType;
  start: Date;
  durationMin: number;
  location?: string;
  capacity?: number;
  fee?: number;
  seriesId?: string;
  status: SessionStatus;
  attendees: { trainee: Types.ObjectId; status: AttendanceStatus }[];
};

function at(date: Date, h: number, m = 0): Date {
  return setMilliseconds(setSeconds(setMinutes(setHours(date, h), m), 0), 0);
}

async function run() {
  await dbConnect();
  console.log("Clearing existing data…");
  await Promise.all([
    Trainee.deleteMany({}),
    Session.deleteMany({}),
    Payment.deleteMany({}),
    Reminder.deleteMany({}),
  ]);

  const now = new Date();
  const month = monthKey(now);
  const weekStart = startOfWeek(now, { weekStartsOn: 1 }); // Monday

  console.log("Creating trainees…");
  const trainees = await Trainee.create([
    { name: "Karim Haddad", phone: "03 111 222", skillLevel: "intermediate", monthlyFee: 120, notes: "Southpaw. Working on the jab." },
    { name: "Rami Khoury", phone: "03 333 444", skillLevel: "beginner", monthlyFee: 100 },
    { name: "Jad Saliba", phone: "70 555 666", skillLevel: "advanced", monthlyFee: 120, notes: "Prepping for amateur bout." },
    { name: "Nour Aoun", phone: "76 777 888", skillLevel: "beginner", monthlyFee: 80 },
    { name: "Tarek Mansour", phone: "71 999 000", skillLevel: "intermediate", monthlyFee: 100 },
    { name: "Sami Dabboussi", phone: "03 121 212", skillLevel: "beginner", monthlyFee: 0, notes: "Drop-in only." },
    { name: "Lara Fares", phone: "78 343 434", skillLevel: "intermediate", monthlyFee: 90 },
    { name: "Omar Zein", phone: "70 565 656", skillLevel: "beginner", monthlyFee: 0, status: "inactive", notes: "On a break." },
  ]);
  const [karim, rami, jad, nour, tarek, sami, lara] = trainees;

  console.log("Creating sessions…");
  const seriesId = randomUUID();
  const groupDays = [0, 2, 4]; // Mon, Wed, Fri offsets within the week
  const groupSessions: SeedSession[] = groupDays.map((offset): SeedSession => {
    const day = addDays(weekStart, offset);
    const start = at(day, 18, 0);
    const past = start < now;
    return {
      title: "Evening Group Class",
      type: "group",
      start,
      durationMin: 60,
      location: "Main floor",
      capacity: 12,
      seriesId,
      status: past ? "completed" : "scheduled",
      attendees: [karim, rami, jad, nour, lara].map((t, i) => ({
        trainee: t._id,
        status: past ? (i === 1 ? "no_show" : "attended") : "booked",
      })),
    };
  });

  const privateToday: SeedSession = {
    title: "Private — Jad",
    type: "private",
    start: at(now, 16, 30),
    durationMin: 45,
    location: "Ring 1",
    fee: 35,
    status: "scheduled",
    attendees: [{ trainee: jad._id, status: "booked" }],
  };
  const privateLater: SeedSession = {
    title: "Private — Lara",
    type: "private",
    start: at(addDays(now, 1), 17, 0),
    durationMin: 45,
    location: "Ring 1",
    fee: 35,
    status: "scheduled",
    attendees: [{ trainee: lara._id, status: "booked" }],
  };
  // A general group class today so the dashboard always has "today".
  const groupToday: SeedSession = {
    title: "Morning Conditioning",
    type: "group",
    start: at(now, 7, 30),
    durationMin: 50,
    location: "Main floor",
    capacity: 10,
    status: now > at(now, 8, 20) ? "completed" : "scheduled",
    attendees: [nour, tarek, sami].map((t) => ({ trainee: t._id, status: "booked" })),
  };

  await Session.create([...groupSessions, groupToday, privateToday, privateLater]);

  console.log("Creating payments…");
  await Payment.create([
    { trainee: karim._id, amount: 120, type: "subscription", periodMonth: month, date: now },
    { trainee: nour._id, amount: 80, type: "subscription", periodMonth: month, date: now },
    { trainee: lara._id, amount: 90, type: "subscription", periodMonth: month, date: now },
    { trainee: rami._id, amount: 50, type: "subscription", periodMonth: month, date: now, note: "Half now, half later" },
    // Jad and Tarek intentionally unpaid -> overdue.
    { trainee: jad._id, amount: 35, type: "private", date: now },
    { trainee: sami._id, amount: 20, type: "dropin", date: now },
  ]);

  console.log("Creating reminders…");
  await Reminder.create([
    { title: "Fix the speed-bag bracket", dueDate: addDays(now, -1), priority: "high" },
    { title: "Call Rami about his schedule", dueDate: now, priority: "normal" },
    { title: "Order new gloves (size M)", dueDate: addDays(now, 2), priority: "normal" },
    { title: "Renew gym insurance", dueDate: addDays(now, 4), priority: "high" },
    { title: "Wipe down the ring canvas", dueDate: addDays(now, -2), priority: "low", done: true },
  ]);

  const counts = {
    trainees: await Trainee.countDocuments(),
    sessions: await Session.countDocuments(),
    payments: await Payment.countDocuments(),
    reminders: await Reminder.countDocuments(),
  };
  console.log("Seed complete:", counts);
  await mongoose.disconnect();
}

run().catch((err) => {
  console.error(err);
  process.exit(1);
});
