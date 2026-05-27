# Bronze Boxing — Product Context

**Register:** product (the design serves the tool; clarity and speed beat decoration)

## Product purpose

A studio management app for a single boxing coach to run the business side of his
gym from his phone. It replaces the notebook and mental math: who trains when, who
has paid this month, what private sessions are booked, and what he must remember
today and this week. Cash-only for now (no payment gateway). v1 is single-admin
(only the coach logs in, later); trainees are records, not users yet.

## Users

- **Primary: the coach.** Owns and runs the gym. Not a "software person." Checks the
  app in short bursts between classes, often standing, one-handed, phone in hand, in
  a gym with uneven lighting. Wants answers in a glance: money in, who owes, what's
  next. Low tolerance for menus, forms, and clutter.
- (v2) Trainees viewing their own schedule and dues. Out of scope now, but the data
  model must not preclude it.

## Jobs to be done

1. See today at a glance: classes, income this month, who's overdue, reminders.
2. Keep the trainee roster (contact, skill level, monthly fee, notes).
3. Schedule regular group classes (recurring) and one-off private sessions; mark who
   attended.
4. Record cash payments and instantly know who has / hasn't paid this month's dues.
5. Set day and week reminders and check them off.

## Tone & voice

Direct, confident, gym-floor plain. Short labels. Numbers do the talking. No corporate
filler, no cute mascots, no exclamation marks. It should feel like a tool a serious
coach is proud to pull out, not a consumer fitness toy.

## Anti-references (what to avoid)

- Generic SaaS dashboard: identical card grid, big-number-with-gradient hero, pastel.
- Consumer fitness-app gamification: streaks, confetti, badges, motivational quotes.
- Gritty "boxing" cliché: grunge textures, distressed fonts, fake leather, red gloves.
- Anything that needs a tutorial. The coach should understand a screen in 3 seconds.

## Strategic principles

- **Glanceable over comprehensive.** The home screen answers the four daily questions
  before any tap.
- **Status is the product.** Paid / overdue, attended / no-show, scheduled / done must
  read instantly via color + label (never color alone).
- **Thumb-first.** Primary actions reachable bottom-of-screen; bottom nav on mobile.
- **Fast.** Server-rendered data, minimal client JS, no spinners for primary content.
