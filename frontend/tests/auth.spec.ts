import { test, expect, type Page } from "@playwright/test";

test("anonymous layouts and keyboard controls", async ({ page }, testInfo) => {
  await page.route("**/api/auth/me", route => route.fulfill({
    status: 401, contentType: "application/json",
    body: JSON.stringify({ error: "Silakan masuk", code: "UNAUTHENTICATED" }),
  }));
  await page.goto("/login");
  await expect(page.getByRole("heading", { name: "Selamat datang kembali." })).toBeVisible();
  for (const width of [320, 360, 768, 1280]) {
    await page.setViewportSize({ width, height: 900 });
    for (const theme of ["dark", "light"]) {
      await page.getByRole("button", { name: theme === "dark" ? "Tema gelap" : "Tema terang" }).click();
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBeTruthy();
      await page.screenshot({ path: testInfo.outputPath(`login-${width}-${theme}.png`), fullPage: true });
    }
  }
  await page.evaluate(() => { document.documentElement.style.fontSize = "200%"; });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBeTruthy();
  await page.evaluate(() => { document.documentElement.style.removeProperty("font-size"); });
  await page.getByLabel("Email atau username").focus();
  await page.keyboard.press("Tab");
  await expect(page.getByLabel("Kata sandi", { exact: true })).toBeFocused();
  await page.keyboard.press("Tab");
  await expect(page.getByRole("button", { name: "Tampilkan", exact: true })).toBeFocused();
  await page.keyboard.press("Enter");
  await expect(page.getByLabel("Kata sandi", { exact: true })).toHaveAttribute("type", "text");
  await page.getByRole("link", { name: "Daftar akun", exact: true }).first().click();
  await page.setViewportSize({ width: 360, height: 900 });
  await expect(page.getByLabel("Konfirmasi kata sandi")).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBeTruthy();
  await page.screenshot({ path: testInfo.outputPath("register-mobile.png"), fullPage: true });
});

const password = "AssesmentPass!2026";

async function login(page: Page, identifier: string, secret = password) {
  await page.goto("/login");
  await page.getByLabel("Email atau username").fill(identifier);
  await page.getByLabel("Kata sandi", { exact: true }).fill(secret);
  await page.getByRole("button", { name: "Masuk ke akun" }).click();
  await expect(page).toHaveURL(/\/profile$/);
}

test("register, session eviction, role guard, admin force logout, and responsive themes", async ({
  browser,
}, testInfo) => {
  const errors: string[] = [];
  const contexts = await Promise.all(
    Array.from({ length: 4 }, () =>
      browser.newContext({ baseURL: "http://localhost:3000" }),
    ),
  );
  const pages = await Promise.all(contexts.map((context) => context.newPage()));
  pages.forEach((page) =>
    page.on("pageerror", (error) => errors.push(error.message)),
  );
  const [first, second, third, admin] = pages;
  const username = `qa_${Date.now()}`;
  try {
    await first.goto("/profile");
    await expect(first).toHaveURL(/\/login$/);
    await first.screenshot({
      path: testInfo.outputPath("login-desktop.png"),
      fullPage: true,
    });
    await first
      .getByRole("link", { name: "Daftar akun", exact: true })
      .first()
      .click();
    await first
      .getByLabel("Email", { exact: true })
      .fill(`${username}@example.com`);
    await first.getByLabel("Username", { exact: true }).fill(username);
    await first.getByLabel("Kata sandi", { exact: true }).fill(password);
    await first.getByLabel("Konfirmasi kata sandi").fill("DifferentPassword");
    await first.getByRole("button", { name: "Buat akun", exact: true }).click();
    await expect(first.getByRole("alert")).toContainText("belum sama");
    await first.getByLabel("Konfirmasi kata sandi").fill(password);
    await first.getByRole("button", { name: "Tampilkan", exact: true }).click();
    await expect(
      first.getByLabel("Kata sandi", { exact: true }),
    ).toHaveAttribute("type", "text");
    await first
      .getByRole("button", { name: "Sembunyikan", exact: true })
      .click();
    await first.getByRole("button", { name: "Buat akun", exact: true }).click();
    await expect(first).toHaveURL(/\/profile$/);
    await expect(
      first.getByRole("heading", { name: `Halo, ${username}.` }),
    ).toBeVisible();
    await expect(first.getByText("1/2", { exact: true })).toBeVisible();
    await first.getByRole("button", { name: "Muat ulang", exact: true }).click();
    await expect(first.getByText("1/2", { exact: true })).toBeVisible();
    await first.screenshot({
      path: testInfo.outputPath("profile-desktop.png"),
      fullPage: true,
    });
    await expect(
      first.getByRole("link", { name: "Pengguna", exact: true }),
    ).toHaveCount(0);
    await first.goto("/admin/users");
    await expect(
      first.getByRole("heading", { name: "Akses terbatas." }),
    ).toBeVisible();
    await first.getByRole("link", { name: "Kembali ke profil" }).click();

    await login(second, username);
    await login(third, `${username}@example.com`);
    await expect(third.getByText("2/2", { exact: true })).toBeVisible();
    expect((await contexts[0].request.get("/api/auth/me")).status()).toBe(401);
    await first.goto("/profile");
    await expect(first).toHaveURL(/\/login$/);

    await third
      .getByRole("button", { name: /Akhiri sesi .*perangkat lain/ })
      .click();
    await expect(third.getByRole("dialog")).toBeVisible();
    await third.keyboard.press("Escape");
    await expect(third.getByRole("dialog")).toHaveCount(0);
    await third
      .getByRole("button", { name: /Akhiri sesi .*perangkat lain/ })
      .click();
    await third.getByRole("button", { name: "Batal", exact: true }).click();
    await third
      .getByRole("button", { name: /Akhiri sesi .*perangkat lain/ })
      .click();
    await third
      .getByRole("dialog")
      .getByRole("button", { name: "Akhiri sesi", exact: true })
      .click();
    await expect(third.getByText("1/2", { exact: true })).toBeVisible();
    await second.reload();
    await expect(second).toHaveURL(/\/login$/);
    await login(second, username);

    await login(
      admin,
      process.env.ADMIN_USERNAME || "admin",
      process.env.ADMIN_PASSWORD || "ChangeMeNow!2026",
    );
    await admin.getByRole("link", { name: "Pengguna", exact: true }).click();
    await admin
      .getByLabel("Cari email atau username")
      .fill("no_matching_user_123456789");
    await admin.getByRole("button", { name: "Cari", exact: true }).click();
    await expect(
      admin.getByRole("heading", { name: "Tidak ada pengguna yang cocok." }),
    ).toBeVisible();
    await admin
      .getByRole("button", { name: "Hapus filter", exact: true })
      .click();
    await admin.getByLabel("Cari email atau username").fill(username);
    await admin.getByRole("button", { name: "Cari", exact: true }).click();
    await expect(
      admin.getByRole("row").filter({ hasText: username }),
    ).toBeVisible();

    for (const width of [360, 768, 1280]) {
      await admin.setViewportSize({ width, height: 900 });
      for (const theme of ["dark", "light"]) {
        const button = admin.getByRole("button", {
          name: theme === "dark" ? "Tema gelap" : "Tema terang",
        });
        await button.click();
        await expect(admin.locator("html")).toHaveAttribute(
          "data-theme",
          theme,
        );
        expect(
          await admin.evaluate(
            () => document.documentElement.scrollWidth <= window.innerWidth,
          ),
        ).toBeTruthy();
        if (width === 360)
          await admin.screenshot({
            path: testInfo.outputPath(`users-mobile-${theme}.png`),
            fullPage: true,
          });
      }
    }
    await admin.screenshot({
      path: testInfo.outputPath("users-desktop.png"),
      fullPage: true,
    });
    await admin
      .getByRole("button", { name: `Force logout ${username}`, exact: true })
      .click();
    await admin
      .getByRole("dialog")
      .getByRole("button", { name: "Cabut seluruh sesi" })
      .click();
    await expect(admin.getByRole("status")).toContainText(
      `Seluruh sesi ${username} telah dicabut.`,
    );
    expect((await contexts[2].request.get("/api/auth/me")).status()).toBe(401);
    await third.goto("/profile");
    await expect(third).toHaveURL(/\/login$/);
    await second.reload();
    await expect(second).toHaveURL(/\/login$/);

    await login(third, username);
    await third
      .getByRole("button", { name: /Akhiri sesi .*perangkat ini/ })
      .click();
    await third
      .getByRole("dialog")
      .getByRole("button", { name: "Akhiri sesi", exact: true })
      .click();
    await expect(third).toHaveURL(/\/login$/);

    await admin.getByRole("button", { name: "Keluar", exact: true }).click();
    await expect(admin).toHaveURL(/\/login$/);
    await admin.getByRole("button", { name: "Masuk ke akun" }).click();
    expect(
      await admin
        .getByLabel("Email atau username")
        .evaluate((input) => (input as HTMLInputElement).validity.valueMissing),
    ).toBeTruthy();
    await admin.getByLabel("Email atau username").fill(username);
    await admin
      .getByLabel("Kata sandi", { exact: true })
      .fill("IncorrectPassword");
    await admin.getByRole("button", { name: "Masuk ke akun" }).click();
    await expect(admin.getByRole("alert")).toContainText("tidak sesuai");
    expect(errors).toEqual([]);
  } finally {
    await Promise.all(contexts.map((context) => context.close()));
  }
});

test("network failure shows recoverable state", async ({ page }) => {
  await page.route("**/api/auth/me", (route) => route.abort("failed"));
  await page.goto("/profile");
  await expect(
    page.getByRole("heading", { name: "Koneksi belum tersedia." }),
  ).toBeVisible();
  await expect(page.getByRole("alert")).toContainText("Tidak dapat terhubung");
  await page.unroute("**/api/auth/me");
  await page.getByRole("button", { name: "Coba hubungkan kembali" }).click();
  await expect(page).toHaveURL(/\/login$/);
});
