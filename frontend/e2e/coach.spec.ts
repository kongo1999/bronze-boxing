import { test, expect } from "@playwright/test";

const tag = () => Math.random().toString(36).slice(2, 7);

test("trial becomes a member and follow-up can be snoozed", async ({ page, request }) => {
  const name = `Trial ${tag()}`;
  await page.goto("/trial-leads");
  await page.getByRole("button", { name: "+ Enquiry" }).click();
  await page.getByPlaceholder("Full name").fill(name);
  await page.getByRole("button", { name: "Save enquiry" }).click();
  const card = page.locator("li", { hasText: name }).last();
  await expect(card.getByText(name)).toBeVisible();
  await card.getByRole("link", { name: /Create member from this trial/ }).click();
  await expect(page.getByPlaceholder("Full name")).toHaveValue(name);
  await page.getByRole("button", { name: "Save", exact: true }).click();
  await expect(page).toHaveURL(/\/trainees\/[0-9a-f]{24}/);
  await page.goto("/trial-leads");
  await page.getByRole("button", { name: "converted", exact: true }).click();
  await expect(page.getByText(name)).toBeVisible();

  const dueName = `Follow up ${tag()}`;
  const trainee = await (await request.post("/api/trainees", { data: { name: dueName, monthlyFee: 50, status: "active" } })).json();
  await page.goto("/follow-ups");
  const row = page.locator("li", { hasText: dueName }).last();
  await expect(row).toBeVisible();
  await row.getByRole("button", { name: "Snooze 7d" }).click();
  await expect(row).not.toBeVisible();
  await page.getByRole("button", { name: "Show snoozed" }).click();
  await expect(page.locator("li", { hasText: dueName }).last()).toBeVisible();
  expect(trainee.id).toBeTruthy();
});

test("class closeout saves attendance and a note", async ({ page, request }) => {
  const name = `Closeout ${tag()}`;
  const t = await (await request.post("/api/trainees", { data: { name, monthlyFee: 0, status: "active" } })).json();
  const s = await (await request.post("/api/sessions", { data: { title: `Pads ${tag()}`, type: "private", start: new Date(Date.now() - 2 * 3_600_000).toISOString(), durationMin: 45, attendees: [{ trainee: t.id }] } })).json();
  await page.goto(`/schedule/${s.id}`);
  await expect(page.getByRole("heading", { name: "Class closeout" })).toBeVisible();
  await page.getByRole("button", { name: "Mark unreviewed attended" }).click();
  await page.getByPlaceholder("What went well? Focus for next class…").fill("Counter after jab");
  await page.getByRole("button", { name: "Close class" }).click();
  await expect(page.getByText("Class closed. Attendance and note saved.")).toBeVisible();
  const saved = await (await request.get(`/api/sessions/${s.id}`)).json();
  expect(saved.status).toBe("completed");
  expect(saved.attendees[0].status).toBe("attended");
  expect(saved.closeoutNote).toBe("Counter after jab");
});

test("payment promise appears on the member and a cancelled class place goes to the waitlist", async ({ page, request }) => {
  const one = await (await request.post("/api/trainees", { data: { name: `Booked ${tag()}`, monthlyFee: 60, status: "active" } })).json();
  const two = await (await request.post("/api/trainees", { data: { name: `Waiting ${tag()}`, monthlyFee: 0, status: "active" } })).json();
  await page.goto("/payments");
  await page.getByRole("button", { name: "+ Promise" }).click();
  await page.getByRole("button", { name: /Choose someone who owes/ }).click();
  await page.getByRole("combobox", { name: "Search trainees…" }).fill(one.name);
  await page.keyboard.press("Enter");
  await page.getByRole("button", { name: "Save promise" }).click();
  await expect(page.getByText(one.name).first()).toBeVisible();
  await page.goto(`/trainees/${one.id}`);
  await expect(page.getByRole("heading", { name: "Payment dates agreed" })).toBeVisible();
  await expect(page.getByText("$60 promised")).toBeVisible();

  const s = await (await request.post("/api/sessions", { data: { title: `Waitlist ${tag()}`, type: "private", start: new Date(Date.now() + 48 * 3_600_000).toISOString(), durationMin: 45, capacity: 1, attendees: [{ trainee: one.id }] } })).json();
  await page.goto(`/schedule/${s.id}`);
  await page.getByRole("button", { name: /Add to waitlist/ }).click();
  await page.getByRole("combobox", { name: "Search trainees…" }).fill(two.name);
  await page.keyboard.press("Enter");
  await page.getByRole("button", { name: "Add", exact: true }).click();
  await expect(page.getByText(two.name)).toBeVisible();
  page.once("dialog", (d) => d.accept());
  await page.getByText("Cancel a booking").click();
  await page.getByRole("button", { name: `Cancel ${one.name}` }).click();
  await expect(page.getByRole("button", { name: "Book place" })).toBeVisible();
  await page.getByRole("button", { name: "Book place" }).click();
  const booked = await (await request.get(`/api/sessions/${s.id}`)).json();
  expect(booked.attendees.map((a: { trainee: string }) => a.trainee)).toEqual([two.id]);
});
