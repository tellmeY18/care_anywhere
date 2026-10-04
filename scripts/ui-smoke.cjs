// Run with NODE_PATH pointing at a build-time Playwright installation.
const {chromium} = require('playwright');
const fs = require('node:fs');
const path = require('node:path');
const assert = require('node:assert/strict');
(async()=>{
  const browser = await chromium.launch();
  const context = await browser.newContext();
  const page = await context.newPage();
  const errors=[]; page.on('pageerror', e=>errors.push(e.message));
  const c = JSON.parse(fs.readFileSync(process.argv[2], 'utf8'));
  await page.goto(c.URL+'/#'+c.Token);
  await page.getByText('Ready on this computer',{exact:true}).waitFor({timeout:180000});
  assert(!page.url().includes(c.Token));
  for(const [width,height] of [[1100,800],[390,844]]){
    await page.setViewportSize({width,height});
    assert(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth));
    await page.screenshot({path:path.join('dist',`alpha-ui-${width}.png`),fullPage:true});
  }
  if(await page.locator('#setup').isVisible()){
    await page.locator('#username').fill('ui-alpha');
    await page.locator('#password').fill('synthetic-alpha-password-2026');
    await page.locator('#confirm').fill('different-password-2026');
    await page.getByRole('button',{name:'Create administrator'}).click();
    await page.getByRole('alert').filter({hasText:'do not match'}).waitFor();
    await page.locator('#confirm').fill('synthetic-alpha-password-2026');
    await page.getByRole('button',{name:'Create administrator'}).click();
  }
  await page.getByRole('link',{name:'Open CARE'}).waitFor({timeout:90000});
  const [care] = await Promise.all([context.waitForEvent('page'),page.getByRole('link',{name:'Open CARE'}).click()]);
  await care.waitForLoadState('networkidle');
  await care.getByRole('button',{name:'Log in as Staff'}).click();
  await care.getByLabel('Username',{exact:true}).fill('ui-alpha');
  await care.getByLabel('Password',{exact:true}).fill('synthetic-alpha-password-2026');
  await care.getByRole('button',{name:'Login',exact:true}).click();
  await care.waitForURL(url=>!url.pathname.includes('login'),{timeout:60000});
  await care.waitForLoadState('networkidle');
  await care.waitForFunction(()=>document.body.innerText.trim()!=='LOADING...' && document.body.innerText.trim().length>40);
  await care.screenshot({path:'dist/alpha-care-login.png',fullPage:true});
  console.log('CARE page:',(await care.locator('body').innerText()).slice(0,3000));
  if(process.env.CARE_EXPLORE_ADMIN==='1'){
    await care.getByRole('link',{name:'Admin Dashboard'}).click();
    await care.waitForLoadState('networkidle');
    console.log('Admin links:',await care.locator('a').evaluateAll(as=>as.map(a=>({text:a.innerText,href:a.getAttribute('href')}))));
    console.log('Admin page:',await care.locator('body').innerText());
  }
  assert.equal(errors.length,0, errors.join('\n'));
  console.log('PASS: responsive UI, no JS errors, password confirmation, setup, open CARE');
  await browser.close();
})().catch(e=>{console.error(e);process.exit(1)});
