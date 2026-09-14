const { test } = require('node:test');
const assert = require('node:assert/strict');
const { readFileSync } = require('node:fs');
const { resolve } = require('node:path');
const vm = require('node:vm');

// Small DOM adapter: runs the actual scripts, including lifecycle and action locks.
class Element {
  constructor() {
    this.listeners = {};
    this.attrs = {};
    this.dataset = {};
    this.disabled = false;
    this.innerHTML = 'original';
    const classes = new Set();
    this.classList = {
      add: (...names) => names.forEach(name => classes.add(name)),
      remove: (...names) => names.forEach(name => classes.delete(name)),
      contains: name => classes.has(name),
    };
  }
  addEventListener(type, fn, options = {}) {
    (this.listeners[type] ||= []).push({ fn, options });
  }
  fire(type, fields = {}) {
    const event = {
      target: this, button: 0, defaultPrevented: false,
      preventDefault() { this.defaultPrevented = true; },
      stopPropagation() {},
      stopImmediatePropagation() { this.stopped = true; },
      ...fields,
    };
    for (const listener of this.listeners[type] || []) {
      if (!listener.options.signal?.aborted) listener.fn(event);
      if (event.stopped) break;
    }
    return event;
  }
  setAttribute(name, value) { this.attrs[name] = value; }
  getAttribute(name) { return this.attrs[name] ?? null; }
  removeAttribute(name) { delete this.attrs[name]; }
  hasAttribute(name) { return name in this.attrs; }
  querySelector() { return null; }
  querySelectorAll() { return []; }
  matches() { return true; }
  contains() { return false; }
  closest() { return null; }
  focus() { this.focused = true; }
}

function setup() {
  const document = new Element();
  document.body = new Element();
  document.readyState = 'complete';
  const elements = { 'main-content': new Element(), 'page-modals': new Element() };
  document.getElementById = id => elements[id] || null;
  const window = new Element();
  window.location = { href: 'http://localhost/?tag=work', origin: 'http://localhost' };
  window.scrollX = 0;
  window.scrollY = 450;
  window.setTimeout = setTimeout;
  window.clearTimeout = clearTimeout;
  window.clearInterval = clearInterval;
  window.scrollTo = position => { window.lastScroll = position; };
  const pushes = [];
  window.history = { pushState: (_, __, url) => {
    pushes.push(url);
    window.location.href = url;
  } };
  const errors = [];
  window.BridopenToast = { error: message => errors.push(message) };
  window.BridopenSidebar = { syncActive: path => { window.activePath = path; } };
  const requests = [];
  const context = vm.createContext({
    window, document, console, AbortController, URL, URLSearchParams,
    FormData: class { constructor(form) { return form.fields; } },
    fetch: (url, options) => new Promise((resolve, reject) => {
      requests.push({ url, options, resolve, reject });
    }),
  });
  const load = path => vm.runInContext(readFileSync(resolve(__dirname, '../views/static/js', path), 'utf8'), context);
  load('core/page-lifecycle.js');
  return { document, elements, window, requests, errors, pushes, load };
}

function navigation() {
  const env = setup();
  env.load('core/action-lock.js');
  env.load('core/partial-nav.js');
  return env;
}

function formFor(action) {
  const form = new Element();
  form.method = 'post';
  form.action = 'http://localhost' + action;
  form.fields = [['gorilla.csrf.Token', 'csrf-token'], ['redirect', '/?tag=work']];
  form.button = new Element();
  form.querySelector = () => form.button;
  return form;
}

function respond(request, url = 'http://localhost/?tag=work', extra = {}) {
  request.resolve({
    ok: true, redirected: true, url,
    headers: { get: () => 'application/json' },
    json: async () => ({ html: 'updated', modals: '', title: 'Notes' }),
    ...extra,
  });
}
const tick = () => new Promise(resolve => setTimeout(resolve, 10));

test('account menu opens after partial navigation, closes outside/Escape, and remounts once', () => {
  const env = setup();
  // Load on Home, where no account controls exist yet.
  env.load('account/edit-personal-info.js');
  const trigger = new Element();
  trigger.setAttribute('aria-controls', 'accountSettingsMenu');
  const menu = new Element();
  menu.classList.add('hidden');
  env.elements.accountSettingsMenu = menu;
  env.document.querySelectorAll = () => [trigger];
  for (let i = 0; i < 3; i++) {
    // Each partial response supplies fresh elements.
    trigger.listeners = {};
    menu.listeners = {};
    env.window.BridopenPage.unmountAll();
    env.window.BridopenPage.mountAll();
    trigger.fire('click');
    assert.equal(menu.classList.contains('hidden'), false);
    assert.equal(trigger.getAttribute('aria-expanded'), 'true');
    env.document.fire('keydown', { key: 'Escape' });
    assert.equal(menu.getAttribute('aria-hidden'), 'true');
    trigger.fire('click');
    env.document.fire('click');
    assert.equal(menu.classList.contains('hidden'), true);
  }
});

test('pin/unpin posts once with CSRF, retains shell, filters and scroll, and releases lock', async () => {
  const env = navigation();
  const main = env.elements['main-content'];
  for (const action of ['pin', 'unpin']) {
    const form = formFor('/note/1/' + action);
    const submit = () => env.document.fire('submit', { target: form, submitter: form.button });
    const count = env.requests.length;
    assert.equal(submit().defaultPrevented, true);
    await tick();
    assert.equal(form.button.disabled, true);
    submit();
    assert.equal(env.requests.length, count + 1);
    const request = env.requests.at(-1);
    assert.equal(request.options.method, 'POST');
    assert.equal(request.options.headers['X-Nav-Mode'], 'partial');
    assert.equal(request.options.body.get('gorilla.csrf.Token'), 'csrf-token');
    assert.equal(request.options.body.get('redirect'), '/?tag=work');
    assert.equal(main.classList.contains('is-navigating'), false);
    respond(request);
    await tick();
    assert.equal(env.elements['main-content'], main);
    assert.equal(main.innerHTML, 'updated');
    assert.equal(env.window.location.href, 'http://localhost/?tag=work');
    assert.equal(env.window.lastScroll.top, 450);
    assert.equal(form.button.disabled, false);
    assert.equal(main.hasAttribute('aria-busy'), false);
    assert.equal(env.pushes.length, 0);
  }
});

test('failed mutation retains content and URL, unlocks, and does not replay the POST', async () => {
  for (const failure of ['network', 'forbidden', 'invalid']) {
    const env = navigation();
    const form = formFor('/note/1/pin');
    env.document.fire('submit', { target: form, submitter: form.button });
    if (failure === 'network') env.requests[0].reject(new Error('offline'));
    else if (failure === 'forbidden') respond(env.requests[0], undefined, { ok: false });
    else respond(env.requests[0], undefined, { json: async () => ({ ok: true }) });
    await tick();
    assert.equal(env.elements['main-content'].innerHTML, 'original');
    assert.equal(env.window.location.href, 'http://localhost/?tag=work');
    assert.equal(env.requests.length, 1);
    assert.equal(env.errors.length, 1);
    assert.equal(form.button.disabled, false);
  }
});

test('archive uses its final redirect; unrelated POST forms remain native', async () => {
  const env = navigation();
  const native = formFor('/user/signout');
  assert.equal(env.document.fire('submit', { target: native }).defaultPrevented, false);
  assert.equal(env.requests.length, 0);
  env.document.fire('submit', { target: formFor('/note/1/archive') });
  respond(env.requests[0], 'http://localhost/notes/archive');
  await tick();
  assert.equal(env.window.location.href, 'http://localhost/notes/archive');
  assert.equal(env.window.activePath, '/notes/archive');
});

test('same-URL links and explicit refresh replace only page content without history duplication', async () => {
  const env = navigation();
  const link = new Element();
  link.href = env.window.location.href;
  link.origin = env.window.location.origin;
  link.setAttribute('href', '/?tag=work');
  const target = { closest: () => link };
  assert.equal(env.document.fire('click', { target }).defaultPrevented, true);
  respond(env.requests[0]);
  await tick();
  const refresh = env.window.BridopenNavigation.visit(link.href);
  respond(env.requests[1]);
  assert.equal(await refresh, true);
  assert.equal(env.pushes.length, 0);
});

test('late mutation response cannot replace a page reached by a newer navigation', async () => {
  const env = navigation();
  env.document.fire('submit', { target: formFor('/note/1/pin') });
  const visit = env.window.BridopenNavigation.visit('/tags');
  respond(env.requests[1], 'http://localhost/tags');
  await visit;
  respond(env.requests[0]);
  await tick();
  assert.equal(env.window.location.href, 'http://localhost/tags');
  assert.equal(env.window.activePath, '/tags');
});

test('slow partial response leaves existing content visible throughout the request', async () => {
  const env = navigation();
  const visit = env.window.BridopenNavigation.visit('/tags');
  await new Promise(resolve => setTimeout(resolve, 250));
  assert.equal(env.elements['main-content'].innerHTML, 'original');
  assert.equal(env.elements['main-content'].classList.contains('is-navigating'), false);
  respond(env.requests[0], 'http://localhost/tags');
  await visit;
});

test('delete success refreshes through partial navigation, including reload-on-success', async () => {
  const env = navigation();
  for (const id of ['note-delete-modal', 'note-delete-confirm', 'note-delete-cancel']) {
    env.elements[id] = new Element();
  }
  const modal = env.elements['note-delete-modal'];
  const confirm = env.elements['note-delete-confirm'];
  confirm.setAttribute('data-csrf-token', 'csrf-token');
  const messages = [];
  env.window.BridopenToast.show = message => messages.push(message);
  env.load('notes/note-delete-modal.js');
  const trigger = new Element();
  trigger.setAttribute('data-note-delete-url', '/note/1');
  trigger.setAttribute('data-note-delete-reload', 'true');
  trigger.setAttribute('data-note-delete-success', 'Deleted');
  trigger.closest = selector => selector === '[data-note-delete-action]' ? trigger : null;
  env.document.fire('click', { target: trigger });
  confirm.fire('click');
  assert.equal(env.requests[0].options.method, 'DELETE');
  respond(env.requests[0], undefined, { json: async () => ({ ok: true, message: 'Deleted' }) });
  await tick();
  assert.equal(modal.classList.contains('hidden'), true);
  assert.equal(env.requests[1].url, 'http://localhost/?tag=work');
  assert.equal(env.requests[1].options.headers['X-Nav-Mode'], 'partial');
  respond(env.requests[1]);
  await tick();
  assert.deepEqual(messages, ['Deleted']);
  assert.equal(env.pushes.length, 0);
});
