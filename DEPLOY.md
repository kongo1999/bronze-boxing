# Deploying Bronze Boxing

The app runs in the cloud, so it no longer touches your laptop's resources. You need
two free accounts: **MongoDB Atlas** (database) and **Vercel** (app). Railway or Render
work too (see the bottom).

> ⚠️ **There is no login yet** (auth is v2). Anyone with the URL can view and edit data.
> Before sharing the URL, add protection — either Vercel's deployment password, or ask
> me to add a simple password gate / build the real login.

## 1. Database — MongoDB Atlas (free)

1. Create an account at https://www.mongodb.com/atlas and make a **free M0 cluster**.
2. **Database Access** → add a database user (username + password).
3. **Network Access** → Add IP → **Allow access from anywhere** (`0.0.0.0/0`).
   Serverless hosts use changing IPs, so this is required.
4. **Connect → Drivers** → copy the connection string. It looks like:
   ```
   mongodb+srv://<user>:<password>@<cluster>.mongodb.net/bronze-boxing?retryWrites=true&w=majority
   ```
   Replace `<user>`/`<password>` and keep `/bronze-boxing` as the database name.

## 2. Push this repo to GitHub

This project is already committed locally. Create an empty repo on GitHub (no README),
then from the project folder:

```bash
git remote add origin https://github.com/<you>/bronze-boxing.git
git branch -M main
git push -u origin main
```

## 3. App — Vercel (free)

1. Sign in at https://vercel.com with GitHub and **Import** the repo.
2. Framework is auto-detected (Next.js). Leave build settings default.
3. **Environment Variables** — add:
   | Name | Value |
   |---|---|
   | `MONGODB_URI` | your Atlas connection string from step 1 |
   | `NEXT_PUBLIC_CURRENCY` | `$` (or your symbol) |
4. **Deploy.** You'll get a live URL like `https://bronze-boxing.vercel.app`.

### Seed live data (optional)
With the Atlas URI in your local `.env.local`, run `npm run seed` once to load sample
data into the cloud database. Or just add real trainees through the app.

## Alternatives (always-on server instead of serverless)

- **Railway** (https://railway.app) — "New Project → Deploy from GitHub", add the same
  two env vars. Runs `next start` persistently.
- **Render** (https://render.com) — "New → Web Server", build `npm run build`, start
  `npm run start`, add the env vars.

Both still use MongoDB Atlas for the database.

## Notes
- No Docker anywhere — the app connects to whatever `MONGODB_URI` points at.
- `.env.local` is gitignored and never deployed; set env vars in the host dashboard.
- Times are stored UTC and shown in the viewer's local timezone.
