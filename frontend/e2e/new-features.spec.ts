import { test, expect } from "@playwright/test";

const tag = () => Math.random().toString(36).slice(2, 7);

test("group class shows partial payment and its receipt downloads as PDF", async ({ page, request }, info) => {
  const trainee = await (await request.post("/api/trainees", { data: { name: `Class payer ${tag()}`, monthlyFee: 0, status: "active" } })).json();
  const start = new Date(Date.now() + (info.project.name === "phone" ? 72 : 33 * 24 + 4) * 3_600_000).toISOString();
  const created = await request.post("/api/sessions", { data: { title: `Group fees ${tag()}`, type: "group", start, durationMin: 60, fee: 30, attendees: [{ trainee: trainee.id }] } });
  expect(created.ok(), await created.text()).toBeTruthy();
  const session = await created.json();
  const recorded = await request.post("/api/payments", { data: { trainee: trainee.id, type: "dropin", amount: 10, sessionId: session.id } });
  expect(recorded.ok(), await recorded.text()).toBeTruthy();
  const payment = await recorded.json();

  await page.goto(`/schedule/${session.id}`);
  await expect(page.getByRole("heading", { name: "Group class payments" })).toBeVisible();
  await expect(page.getByText("1 partial", { exact: false })).toBeVisible();
  await expect(page.getByText("$20 of class fee not recorded", { exact: false })).toBeVisible();

  await page.goto(`/payments/${payment.id}/receipt`);
  await expect(page.getByRole("article", { name: /Receipt/ })).toBeVisible();
  const [download] = await Promise.all([page.waitForEvent("download"), page.getByRole("button", { name: "PDF" }).click()]);
  expect(download.suggestedFilename()).toMatch(/^payment-receipt-.+\.pdf$/);
  expect((await download.path())).toBeTruthy();
  if (process.env.PDF_QA_DIR) await download.saveAs(`${process.env.PDF_QA_DIR}/payment-receipt.pdf`);
});

test("monthly report downloads PDF with its selected month", async ({ page }) => {
  await page.goto("/reports");
  await expect(page.getByRole("heading", { name: "Monthly report" })).toBeVisible();
  const [download] = await Promise.all([page.waitForEvent("download"), page.getByRole("button", { name: "PDF" }).click()]);
  expect(download.suggestedFilename()).toMatch(/^monthly-report-\d{4}-\d{2}\.pdf$/);
  expect((await download.path())).toBeTruthy();
  if (process.env.PDF_QA_DIR) await download.saveAs(`${process.env.PDF_QA_DIR}/monthly-report.pdf`);
});

test("rules acknowledgement and opted-in class draft are visible", async ({ page, request }, info) => {
  const trainee = await (await request.post("/api/trainees", { data: { name: `Alerts ${tag()}`, monthlyFee: 0, status: "active", phone: "03 555 121" , whatsappOptIn: true } })).json();
  const published = await (await request.put("/api/studio-rules", { data: { groupPolicy: "Arrive before class.", privatePolicy: "Book ahead.", clientAgreement: "I agree." } })).json();
  await page.goto(`/trainees/${trainee.id}`);
  await expect(page.getByText(`Version ${published.version} has not been acknowledged`)).toBeVisible();
  await page.getByRole("button", { name: "Record acknowledgement" }).click();
  await expect(page.getByText(new RegExp(`Version ${published.version} recorded`))).toBeVisible();

  const alertHours = info.project.name === "phone" ? 120 : 12 * 24 + 3;
  const booking = await request.post("/api/sessions", { data: { title: `Boxing alerts ${tag()}`, type: "private", start: new Date(Date.now() + alertHours * 3_600_000).toISOString(), durationMin: 60, attendees: [{ trainee: trainee.id }] } });
  expect(booking.ok(), await booking.text()).toBeTruthy();
  await page.goto("/class-alerts");
  await expect(page.getByRole("heading", { name: "Class reminders" })).toBeVisible();
  await expect(page.getByRole("link", { name: "Open draft" }).first()).toBeVisible();
});
