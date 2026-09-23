import { test, expect, type APIRequestContext, type Page } from "@playwright/test";

// The release's acceptance flows, driven through the real UI against a real
// API and database (see playwright.config.ts). Every record gets a unique
// name, so the phone and desktop runs share the database without clashing.

const uniq = () => Math.random().toString(36).slice(2, 6);
const dayPlus = (n: number) => new Date(Date.now() + n * 86_400_000).toISOString().slice(0, 10);

async function addTrainee(request: APIRequestContext, name: string, fee: number, phone = "") {
  const res = await request.post("/api/trainees", { data: { name, monthlyFee: fee, status: "active", phone } });
  expect(res.ok()).toBeTruthy();
  return (await res.json()) as { id: string; name: string };
}

async function dueRow(page: Page, traineeId: string) {
  await page.goto("/payments");
  const row = page.locator(`#due-${traineeId}`);
  await expect(row).toBeVisible();
  return row;
}

test("a partial payer stays partial — Money, Home and the report agree — until the rest is collected", async ({ page, request }) => {
  const t = await addTrainee(request, `Maya Partial ${uniq()}`, 50);

  // Collect $20 of $50.
  let row = await dueRow(page, t.id);
  await row.getByRole("link", { name: /Collect \$50/ }).click();
  const amount = page.getByLabel(/Amount/);
  await expect(amount).toHaveValue("50");
  await amount.fill("20");
  await page.getByRole("button", { name: "Record payment" }).click();
  await expect(page).toHaveURL(/\/payments/);
  row = page.locator(`#due-${t.id}`);
  await expect(row.getByText("Partial", { exact: true })).toBeVisible();
  await expect(row.getByText("$30 left")).toBeVisible();

  // Home counts partial payers apart from unpaid ones.
  await page.goto("/");
  await expect(page.getByRole("link", { name: /partly paid/ })).toBeVisible();

  // The month's report lists them as partly paid, not paid.
  await page.goto("/reports");
  const partial = page.locator("div", { hasText: "Partly paid — not counted as paid" }).last();
  await expect(partial.getByText(t.name)).toBeVisible();

  // Collect the remaining $30: now paid.
  row = await dueRow(page, t.id);
  await row.getByRole("link", { name: /Collect \$30/ }).click();
  await expect(page.getByLabel(/Amount/)).toHaveValue("30");
  await page.getByRole("button", { name: "Record payment" }).click();
  await expect(page.locator(`#due-${t.id}`).getByText("Paid", { exact: true })).toBeVisible();

  // Its receipt prints/shares.
  const pays = await (await request.get(`/api/payments?trainee=${t.id}`)).json();
  await page.goto(`/payments/${pays[0].id}/receipt`);
  await expect(page.getByRole("article", { name: /Receipt/ })).toBeVisible();
  await expect(page.getByRole("button", { name: /Print/ })).toBeVisible();
});

test("search forgives typos and opens the exact record; the picker works by keyboard and phone digits", async ({ page, request }) => {
  const tag = uniq();
  const digits = String(100000 + Math.floor(Math.random() * 899999));
  const t = await addTrainee(request, `Zeina Khalil${tag}`, 0, `71 ${digits.slice(0, 3)} ${digits.slice(3)}`);

  await page.goto("/search");
  await page.getByLabel("Search everything").fill(`zeyna khalil${tag}`);
  const hit = page.getByRole("link", { name: new RegExp(`Zeina Khalil${tag}`) });
  await expect(hit).toBeVisible();
  await expect(page.getByText(/Did you mean/)).toBeVisible();
  await hit.click();
  await expect(page).toHaveURL(new RegExp(`/trainees/${t.id}`));

  await page.goto("/payments/new");
  const picker = page.getByRole("button", { name: /Trainee:/ });
  await picker.focus();
  await page.keyboard.press("ArrowDown");
  const box = page.getByRole("combobox", { name: "Search by name or phone…" });
  await expect(box).toBeFocused();
  await box.fill(digits); // part of the phone number, typed without its spaces
  await page.keyboard.press("Enter");
  await expect(picker).toContainText(`Zeina Khalil${tag}`);
  await expect(picker).toBeFocused();
});

test("a recurring series previews its dates and is created with its planned count", async ({ page, request }, info) => {
  const title = `Morning Pads ${uniq()}`;
  // Group classes may not overlap: each project books its own weeks.
  const start = info.project.name === "phone" ? 7 : 49;
  await page.goto(`/schedule/new?day=${dayPlus(start)}`);
  await page.getByPlaceholder("Evening Group Class").fill(title);
  await page.getByRole("switch").click();
  await page.getByRole("button", { name: "Mon", exact: true }).click();
  await page.getByRole("button", { name: "Wed", exact: true }).click();
  await page.getByRole("button", { name: "4 weeks", exact: true }).click();
  await expect(page.getByText("8 sessions", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Create 8 sessions" }).click();
  await expect(page).toHaveURL(/\/schedule/);

  const sessions = (await (await request.get(`/api/sessions?limit=50&q=${encodeURIComponent(title)}`)).json()).items as { title: string; seriesId: string }[];
  const mine = sessions.filter((s) => s.title === title);
  expect(mine).toHaveLength(8);
  const [progress] = await (await request.get(`/api/sessions/series?ids=${mine[0].seriesId}`)).json();
  expect(progress.planned).toBe(8);
  expect(progress.completed).toBe(0);
});

test("selling asks for confirmation, moves stock, and gives a receipt", async ({ page, request }) => {
  const name = `Speed Rope ${uniq()}`;
  const item = await (await request.post("/api/inventory", { data: { name, price: 10, stock: 5, lowStockThreshold: 1 } })).json();

  await page.goto(`/inventory/${item.id}`);
  await page.getByRole("button", { name: "Sell", exact: true }).click();
  await page.getByRole("button", { name: "One more" }).click();
  await page.getByRole("button", { name: /Review sale/ }).click();
  await expect(page.getByText("Record this sale?")).toBeVisible();
  await expect(page.getByText("Left in stock")).toBeVisible();
  await page.getByRole("button", { name: /Record sale — \$20/ }).click();

  await expect(page).toHaveURL(/\/sales\/.+\/receipt/);
  await expect(page.getByRole("article", { name: /Receipt S-/ })).toBeVisible();
  const after = await (await request.get(`/api/inventory/${item.id}`)).json();
  expect(after.stock).toBe(3);
});

test("the monthly report exports the same month as CSV", async ({ page }) => {
  await page.goto("/reports");
  await expect(page.getByRole("heading", { name: "Monthly report" })).toBeVisible();
  const [download] = await Promise.all([page.waitForEvent("download"), page.getByRole("button", { name: "CSV" }).click()]);
  expect(download.suggestedFilename()).toMatch(/^report-\d{4}-\d{2}\.csv$/);
});
