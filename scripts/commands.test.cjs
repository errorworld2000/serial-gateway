const { test } = require('node:test');
const assert = require('node:assert/strict');
const vm = require('node:vm');
const fs = require('node:fs');
const path = require('node:path');
const source = fs.readFileSync(path.join(__dirname, '../internal/webui/public/commands.js'), 'utf8');

function setup(saved = []) {
    const elements = new Map();
    const element = () => ({ value: '', children: [], style: { setProperty() {} }, append(...nodes) { this.children.push(...nodes); }, replaceChildren() { this.children = []; }, setAttribute() {}, focus() {} });
    const get = id => { if (!elements.has(id)) elements.set(id, element()); return elements.get(id); };
    let stored = JSON.stringify(saved);
    let session = { portName: 'COM1', socket: { readyState: 1, send(data) { sent.push(data); } }, term: { focus() {} } };
    const sent = [];
    const waits = [];
    const context = vm.createContext({ document: { getElementById: get, createElement: element }, localStorage: { getItem: () => stored, setItem: (_, value) => { stored = value; } }, WebSocket: { OPEN: 1 }, setTimeout: resolve => waits.push(resolve), confirm: () => true });
    vm.runInContext(source, context);
    context.initCommands(() => session);
    return { context, get, sent, waits, stored: () => JSON.parse(stored), switchSession: () => { session = null; } };
}

test('Send String escapes, pauses, literal backslashes and actual newlines', () => {
    const { context } = setup();
    assert.equal(JSON.stringify(context.parseCommand(String.raw`usb reset\r\pls usb 0:1\n\t\e\b\\\q`)), JSON.stringify(['usb reset\r', null, 'ls usb 0:1\n\t\x1b\b\\\\q']));
    assert.equal(JSON.stringify(context.parseCommand('a\r\nb\nc')), JSON.stringify(['a\rb\rc']));
    assert.equal(JSON.stringify(context.parseCommand('help')), '["help"]');
});

test('button sends in order and stops if active port changes during delay', async () => {
    const ui = setup([{ name: 'boot', text: String.raw`first\r\psecond\r` }]);
    const result = ui.get('command-list').children[0].children[0].onclick();
    assert.deepEqual(ui.sent, ['first\r']);
    ui.switchSession();
    ui.waits.shift()();
    await result;
    assert.deepEqual(ui.sent, ['first\r']);
    assert.match(ui.get('command-status').textContent, /停止/);
});

test('stop cancels remaining chunks', async () => {
    const ui = setup([{ name: 'boot', text: String.raw`first\psecond` }]);
    const result = ui.get('command-list').children[0].children[0].onclick();
    ui.get('command-stop').onclick();
    ui.waits.shift()();
    await result;
    assert.deepEqual(ui.sent, ['first']);
});

test('new button persists its exact source without sending it', () => {
    const ui = setup();
    ui.get('command-add').onclick();
    ui.get('command-name').value = 'load-usb';
    ui.get('command-text').value = String.raw`load usb 0:1 $load_addr\r`;
    ui.get('command-editor').onsubmit({ preventDefault() {} });
    assert.equal(ui.stored()[0].text, String.raw`load usb 0:1 $load_addr\r`);
    assert.deepEqual(ui.sent, []);
});
