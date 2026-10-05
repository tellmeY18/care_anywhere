import { expect, test } from "@playwright/test";

test("reused panel authenticates, stops and restarts through the VM adapter", async ({ page }) => {
  let running = true;
  const operations: string[] = [];
  await page.route("**/status", async route => {
    expect(route.request().headers().authorization).toBe("Bearer test-token");
    await route.fulfill({ json: { configured: true, healthy: running, phase: running ? "ready" : "stopped", detail: "Test clinic", platform: "linux", backupDir: "/test/backups", stateDir: "/test/clinic" } });
  });
  await page.route("**/backups", route => route.fulfill({json:[]}));
  await page.route("**/storage", route => route.fulfill({json:{checked_at:1,level:"ok",drives:[],backup:{},last_run:{},stale:false}}));
  for (const operation of ["start", "stop"]) {
    await page.route(`**/${operation}`, async route => {
      expect(route.request().method()).toBe("POST");
      expect(route.request().headers().authorization).toBe("Bearer test-token");
      operations.push(operation); running = operation === "start";
      await route.fulfill({body:"Done"});
    });
  }
  await page.goto("/#test-token");
  await expect(page.getByRole("button",{name:"Stop",exact:true})).toBeVisible();
  expect(page.url()).not.toContain("test-token");
  await expect(page.getByText("Staff can open CARE on the clinic Wi-Fi.")).toHaveCount(0);
  await page.getByRole("button",{name:"Stop",exact:true}).click();
  await expect(page.getByRole("button",{name:"Start clinic",exact:true})).toBeVisible();
  await page.getByRole("button",{name:"Start clinic",exact:true}).click();
  await expect(page.getByRole("button",{name:"Stop",exact:true})).toBeVisible();
  expect(operations).toEqual(["stop","start"]);
  await page.getByRole("button",{name:"Backups",exact:true}).click();
  await expect(page.getByText("Manual backups in this alpha")).toBeVisible();
  await expect(page.getByText("Automatic every 0 hours")).toHaveCount(0);
});

test("setup lets you choose a username, requires matching passwords, and submits them", async ({page}) => {
  await page.route("**/status",route=>route.fulfill({json:{configured:false,healthy:true,phase:"ready",detail:"Ready",platform:"linux"}}));
  const requests: unknown[]=[];
  await page.route("**/setup",async route=>{requests.push(route.request().postDataJSON());await route.fulfill({status:400,body:"Test setup failure"})});
  await page.goto("/#test-token");
  await expect(page.locator("#adminusername")).toHaveValue("admin");
  await page.locator("#adminusername").fill("clinicadmin");
  await page.locator("#adminpw").fill("test-password-123");
  await page.locator("#adminpw-confirm").fill("different-password");
  await expect(page.getByText("These don't match.",{exact:false})).toBeVisible();
  await page.locator("#adminpw-confirm").fill("test-password-123");
  await page.getByRole("button",{name:"Create administrator"}).click();
  await expect(page.getByText("Test setup failure",{exact:false})).toBeVisible();
  expect(requests).toEqual([{username:"clinicadmin",password:"test-password-123"}]);
});

test("reset password card validates and submits to the real endpoint", async ({page}) => {
  let running = true;
  await page.route("**/status", route => route.fulfill({ json: { configured: true, healthy: running, phase: "ready", detail: "Test clinic", platform: "linux" } }));
  await page.route("**/backups", route => route.fulfill({json:[]}));
  await page.route("**/storage", route => route.fulfill({json:{checked_at:1,level:"ok",drives:[],backup:{},last_run:{},stale:false}}));
  const requests: unknown[] = [];
  await page.route("**/reset-password", async route => { requests.push(route.request().postDataJSON()); await route.fulfill({body:"Password reset"}); });
  await page.goto("/#test-token");
  await page.getByRole("button",{name:"Reset a password"}).click();
  await page.locator("#reset-username").fill("admin");
  await page.locator("#reset-password").fill("a-new-password-123");
  await page.locator("#reset-confirm").fill("a-new-password-123");
  await page.getByRole("button",{name:"Reset password",exact:true}).click();
  await expect(page.getByText("Password updated")).toBeVisible();
  expect(requests).toEqual([{username:"admin",password:"a-new-password-123"}]);
});
