import { expect, test } from "@playwright/test";

test("error screen shows the real detail and lets you open diagnostics", async ({ page }) => {
  await page.route("**/status", route => route.fulfill({ json: {
    configured: false, healthy: false, phase: "error",
    detail: "Not enough free disk space. CARE needs about 9 GB free to prepare your clinic. Free up space on this computer, then open Diagnostics below and press Try again.",
    platform: "darwin",
  }}));
  await page.route("**/logs", route => route.fulfill({ body: "2026/10/05 13:40:00 insufficient disk space for clinic data" }));
  let retried = false;
  await page.route("**/start", route => { retried = true; route.fulfill({ body: "Done" }); });
  await page.goto("/#test-token");

  await expect(page.getByRole("heading", { name: "CARE needs attention" })).toBeVisible();
  await expect(page.getByText("Not enough free disk space", { exact: false }).first()).toBeVisible();
  await expect(page.getByText("No software downloads are needed", { exact: false })).toHaveCount(0);

  await page.getByRole("button", { name: "Show diagnostics" }).click();
  await expect(page.getByText("insufficient disk space for clinic data")).toBeVisible();

  await page.getByRole("button", { name: "Try again" }).click();
  expect(retried).toBe(true);
});

test("waiting screen explains itself and still offers diagnostics", async ({ page }) => {
  await page.route("**/status", route => route.fulfill({ json: {
    configured: false, healthy: false, phase: "starting",
    detail: "Checking the appliance and preparing your clinic. First launch can take a few minutes.",
    platform: "darwin",
  }}));
  await page.route("**/logs", route => route.fulfill({ body: "boot log line" }));
  await page.goto("/#test-token");
  await expect(page.getByText("Everything is included")).toBeVisible();
  await expect(page.getByRole("status").filter({ hasText: "Working" })).toBeVisible();
  await page.getByRole("button", { name: "Show diagnostics" }).click();
  await expect(page.getByText("boot log line")).toBeVisible();
});
