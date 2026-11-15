import { test as base, expect, APIResponse, Page, Response, TestInfo } from '@playwright/test';
import { ChildProcess, spawn, execFileSync } from 'node:child_process';
import { createServer } from 'node:net';
import { appendFileSync, mkdirSync } from 'node:fs';
import path from 'node:path';
import { parse, DefaultTreeAdapterMap } from 'parse5';

const root = path.resolve(__dirname, '..');
type Task = { id: number; title: string; done: boolean };
type SQLRow = Record<string, string | number | null>;

class AppServer {
  process?: ChildProcess;
  url = '';
  readonly db: string;
  readonly log: string;
  constructor(readonly info: TestInfo) {
    this.db = info.outputPath('tasks.db');
    this.log = info.outputPath('server.log');
    mkdirSync(path.dirname(this.db), { recursive: true });
  }
  sql(query: string, args: unknown[] = []): SQLRow[] {
    const script = `import sqlite3,json,sys
c=sqlite3.connect(sys.argv[1],timeout=5)
c.row_factory=sqlite3.Row
r=c.execute(sys.argv[2],json.loads(sys.argv[3]))
rows=[dict(x) for x in r.fetchall()] if r.description else []
c.commit()
c.close()
print(json.dumps(rows,ensure_ascii=False))`;
    return JSON.parse(execFileSync('python3', ['-c', script, this.db, query, JSON.stringify(args)], { encoding: 'utf8' }));
  }
  async start(): Promise<void> {
    const socket = createServer();
    await new Promise<void>((resolve, reject) => { socket.once('error', reject); socket.listen(0, '127.0.0.1', resolve); });
    const address = socket.address();
    if (!address || typeof address === 'string') throw new Error('No TCP port');
    const port = address.port;
    await new Promise<void>((resolve, reject) => socket.close(err => err ? reject(err) : resolve()));
    this.url = `http://127.0.0.1:${port}`;
    const binary = process.env.TASKS_BINARY || path.join(root, 'bin/tasks');
    const child = spawn(binary, [], { env: { ...process.env, ADDR: `127.0.0.1:${port}`, DB_PATH: this.db }, stdio: ['ignore', 'pipe', 'pipe'] });
    this.process = child;
    let spawnError: Error | undefined;
    child.once('error', error => { spawnError = error; });
    child.stdout?.on('data', data => appendFileSync(this.log, data));
    child.stderr?.on('data', data => appendFileSync(this.log, data));
    const end = Date.now() + 5_000;
    while (Date.now() < end) {
      if (spawnError) throw spawnError;
      if (child.exitCode !== null) throw new Error(`Server exited: ${child.exitCode}`);
      try {
        const response = await fetch(`${this.url}/api/tasks`, { signal: AbortSignal.timeout(500) });
        if (response.status === 200) return;
      } catch (_) { /* Poll process readiness, never replace HTTP. */ }
      await new Promise(resolve => setTimeout(resolve, 50));
    }
    throw new Error('Server readiness exceeded 5 seconds');
  }
  async stop(): Promise<void> {
    const child = this.process;
    if (!child || child.exitCode !== null || child.signalCode !== null) return;
    await new Promise<void>((resolve, reject) => {
      const timer = setTimeout(() => { child.kill('SIGKILL'); reject(new Error('Shutdown exceeded 5 seconds')); }, 5_000);
      child.once('exit', () => { clearTimeout(timer); resolve(); });
      child.kill('SIGTERM');
    });
    this.process = undefined;
  }
  seed(): void { this.sql('INSERT INTO tasks(id,title,done) VALUES(1,?,0)', ['Отчет']); }
  rows(): SQLRow[] { return this.sql('SELECT id,title,done FROM tasks ORDER BY id'); }
}

const test = base.extend<{ app: AppServer }>({
  app: async ({ browser }, use, info) => {
    await info.attach('browser-version', { body: JSON.stringify({ version: browser.version(), executableOverride: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || null }), contentType: 'application/json' });
    const app = new AppServer(info);
    try { await app.start(); await use(app); }
    finally { await app.stop(); }
  },
});
async function json(response: APIResponse | Response, status: number, body: unknown): Promise<void> {
  expect(response.status()).toBe(status);
  expect(response.headers()['content-type']).toBe('application/json; charset=utf-8');
  expect(await response.json()).toEqual(body);
}
const initial: Task = { id: 1, title: 'Отчет', done: false };
function waitResponse(page: Page, method: string, suffix: string): Promise<Response> {
  return page.waitForResponse(r => r.request().method() === method && new URL(r.url()).pathname === suffix, { timeout: 5_000 });
}

test('A01 creation and whitespace rejection', async ({ app, page }) => {
  await page.goto(app.url);
  await expect(page.locator('#title-input')).not.toHaveAttribute('required');
  await page.locator('#title-input').fill('  Отчет  ');
  let pending = waitResponse(page, 'POST', '/api/tasks');
  await page.locator('#create-button').click();
  await json(await pending, 201, initial);
  const row = page.locator('#tasks-body tr[data-id="1"]');
  await expect(row).toHaveCount(1);
  await expect(row.locator('.title')).toHaveText('Отчет', { useInnerText: false });
  await expect(row.locator('.status')).toHaveText('Не выполнена');
  await expect(page.locator('#title-input')).toHaveValue('');
  await page.locator('#title-input').fill('   ');
  pending = waitResponse(page, 'POST', '/api/tasks');
  await page.locator('#create-button').click();
  await json(await pending, 400, { error: 'title_length' });
  await expect(page.locator('#error-banner')).toHaveText('Название: от 1 до 100 символов');
  await expect(page.locator('#title-input')).toHaveValue('   ');
  await expect(page.locator('#tasks-body tr')).toHaveCount(1);
  expect(app.rows()).toEqual([{ id: 1, title: 'Отчет', done: 0 }]);
});

test('A02 empty and ID-ordered list', async ({ app, page, request }) => {
  await page.goto(app.url);
  await json(await request.get(`${app.url}/api/tasks`), 200, []);
  await expect(page.locator('#tasks-body tr')).toHaveCount(0);
  await json(await request.post(`${app.url}/api/tasks`, { data: { title: 'Б' } }), 201, { id: 1, title: 'Б', done: false });
  await json(await request.post(`${app.url}/api/tasks`, { data: { title: 'А' } }), 201, { id: 2, title: 'А', done: false });
  await page.reload();
  await json(await request.get(`${app.url}/api/tasks`), 200, [{ id: 1, title: 'Б', done: false }, { id: 2, title: 'А', done: false }]);
  await expect(page.locator('#tasks-body tr')).toHaveCount(2);
  expect(await page.locator('#tasks-body tr').evaluateAll(rows => rows.map(r => r.getAttribute('data-id')))).toEqual(['1', '2']);
  expect(await page.locator('#tasks-body .title').allTextContents()).toEqual(['Б', 'А']);
});

test('A03 completion persists across real process restart', async ({ app, page, request }) => {
  app.seed();
  await page.goto(app.url);
  const completed = { ...initial, done: true };
  const pending = waitResponse(page, 'PATCH', '/api/tasks/1/done');
  await page.locator('.done-button').click();
  await json(await pending, 200, completed);
  await expect(page.locator('.status')).toHaveText('Выполнена');
  await expect(page.locator('.done-button')).toBeDisabled();
  await json(await request.patch(`${app.url}/api/tasks/1/done`), 200, completed);
  const oldPID = app.process?.pid;
  await app.stop();
  await app.start();
  expect(app.process?.pid).not.toBe(oldPID);
  await page.goto(app.url);
  await json(await request.get(`${app.url}/api/tasks`), 200, [completed]);
  await expect(page.locator('#tasks-body tr[data-id="1"]')).toHaveCount(1);
  await expect(page.locator('.title')).toHaveText('Отчет');
  await expect(page.locator('.status')).toHaveText('Выполнена');
  await expect(page.locator('.done-button')).toBeDisabled();
  expect(app.rows()).toEqual([{ id: 1, title: 'Отчет', done: 1 }]);
});

test('A04 physical deletion and repeat 404', async ({ app, page, request }) => {
  // Observe the real Fetch body: Chromium CDP does not expose 204 bodies.
  await page.addInitScript(() => {
    const original = window.fetch.bind(window);
    window.fetch = async (...args: Parameters<typeof fetch>) => {
      const response = await original(...args);
      if (response.status === 204) {
        (window as unknown as { __deleteBytes: number }).__deleteBytes = (await response.clone().arrayBuffer()).byteLength;
      }
      return response;
    };
  });
  app.seed();await page.goto(app.url);
  const pending = waitResponse(page, 'DELETE', '/api/tasks/1');
  await page.locator('.delete-button').click();
  const response = await pending;
  expect(response.status()).toBe(204);
  await expect.poll(() => page.evaluate(() => (window as unknown as { __deleteBytes: number }).__deleteBytes)).toBe(0);
  await expect(page.locator('tr[data-id="1"]')).toHaveCount(0);
  await json(await request.delete(`${app.url}/api/tasks/1`), 404, { error: 'not_found' });
  await json(await request.get(`${app.url}/api/tasks`), 200, []);
  expect(app.rows()).toEqual([]);
});

test('A05 live DROP TABLE is displayed as storage failure', async ({ app, page }) => {
  await page.goto(app.url);await page.locator('#title-input').fill('Отчет');
  app.sql('DROP TABLE tasks');
  const pending = waitResponse(page, 'POST', '/api/tasks');
  await page.locator('#create-button').click();
  await json(await pending, 500, { error: 'storage' });
  await expect(page.locator('#error-banner')).toHaveText('Хранилище недоступно');
  await expect(page.locator('#title-input')).toHaveValue('Отчет');
  await expect(page.locator('#tasks-body tr')).toHaveCount(0);
  expect(app.sql("SELECT COUNT(*) AS n FROM sqlite_master WHERE type='table' AND name='tasks'")).toEqual([{ n: 0 }]);
});

async function prepareXSS(page: Page): Promise<{ count: number }> {
  await page.addInitScript(() => { (window as unknown as { __xss: number }).__xss = 0; });
  const dialogs = { count: 0 };
  page.on('dialog', async dialog => { dialogs.count++; await dialog.dismiss(); });
  return dialogs;
}
async function xssState(page: Page, payload: string, dialogs: { count: number }): Promise<void> {
  expect(await page.locator('.title').textContent()).toBe(payload);
  await expect(page.locator('.title img, .title script')).toHaveCount(0);
  expect(await page.evaluate(() => (window as unknown as { __xss: number }).__xss)).toBe(0);
  expect(dialogs.count).toBe(0);
}

test('S04 fetch appends XSS payload as text', async ({ app, page }) => {
  const payload = '<img src=x onerror=window.__xss=1>';
  const dialogs = await prepareXSS(page);
  await page.goto(app.url);await page.locator('#title-input').fill(payload);
  const pending = waitResponse(page, 'POST', '/api/tasks');
  await page.locator('#create-button').click();
  await json(await pending, 201, { id: 1, title: payload, done: false });
  await expect(page.locator('#tasks-body tr')).toHaveCount(1);
  await xssState(page, payload, dialogs);
  await page.waitForTimeout(500);
  await xssState(page, payload, dialogs);
  expect(app.rows()).toEqual([{ id: 1, title: payload, done: 0 }]);
});

type Node = DefaultTreeAdapterMap['node'];
function descendants(node: Node): Node[] {
  return [node, ...('childNodes' in node ? node.childNodes.flatMap(descendants) : [])];
}
function text(node: Node): string {
  if (node.nodeName === '#text' && 'value' in node) return node.value;
  return 'childNodes' in node ? node.childNodes.map(text).join('') : '';
}

test('S05 raw server HTML escapes stored script', async ({ app, page, request }) => {
  const payload = '<script>window.__xss=1</script>';
  await json(await request.post(`${app.url}/api/tasks`, { data: { title: payload } }), 201, { id: 1, title: payload, done: false });
  const raw = await request.get(app.url);
  expect(raw.status()).toBe(200);
  expect(raw.headers()['content-type']).toBe('text/html; charset=utf-8');
  const nodes = descendants(parse(await raw.text()));
  const cells = nodes.filter(n => 'tagName' in n && n.tagName === 'td' && n.attrs.some(a => a.name === 'class' && a.value === 'title'));
  expect(cells).toHaveLength(1);
  expect(text(cells[0])).toBe(payload);
  expect(descendants(cells[0]).filter(n => 'tagName' in n && ['script', 'img'].includes(n.tagName))).toHaveLength(0);
  const dialogs = await prepareXSS(page);
  await page.goto(app.url);
  await expect(page.locator('#tasks-body tr')).toHaveCount(1);
  await xssState(page, payload, dialogs);
  await page.waitForTimeout(500);
  await xssState(page, payload, dialogs);
  expect(app.rows()).toEqual([{ id: 1, title: payload, done: 0 }]);
});
