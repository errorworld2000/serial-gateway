const { test } = require('node:test');
const assert = require('node:assert/strict');
const { colorKernelLine, createLogWriter } = require('../internal/webui/public/log-colors.js');

test('echo, prompts, editing controls and split UTF-8 render without timers', () => {
    const output = [];
    const writer = createLogWriter(s => output.push(s), () => assert.fail('interactive output must not wait'));
    for (const text of ['root# ', 'l', 's', '\b \b', '\x1b[D']) {
        writer.push(text);
        assert.equal(output.at(-1), text);
    }
    const bytes = new TextEncoder().encode('设备');
    writer.push(bytes.slice(0, 2));
    writer.push(bytes.slice(2));
    assert.equal(output.at(-1), '设备');
    writer.close();
});

test('ambiguous log prefixes flush as soon as they become ordinary echo', () => {
    const output = []; let scheduled = 0, canceled = 0;
    const writer = createLogWriter(s => output.push(s), () => ++scheduled, () => canceled++);
    writer.push('[');
    assert.equal(output.length, 0);
    writer.push('A');
    assert.equal(output.join(''), '[A');
    assert.equal(scheduled, 1);
    assert.equal(canceled, 1);
    writer.close();
});

test('fragmented timestamped logs retain coloring and a bounded deadline', () => {
    const output = []; let flush, scheduled = 0;
    const writer = createLogWriter(s => output.push(s), (f, ms) => {
        assert.equal(ms, 24); flush = f; return ++scheduled;
    }, () => {});
    for (const part of ['<', '3>', '[ ', '12.', '5] ', 'failed']) writer.push(part);
    assert.equal(output.length, 0);
    assert.equal(scheduled, 1);
    writer.push('\n');
    assert.equal(output.join(''), colorKernelLine('<3>[ 12.5] failed\n'));
    writer.push('[2] unfinished');
    flush();
    assert.ok(output.at(-1).includes('unfinished'));
    writer.close();
});

test('a burst of 1000 log lines uses one terminal write and preserves every line', () => {
    const output = [];
    const writer = createLogWriter(s => output.push(s));
    const lines = Array.from({ length: 1000 }, (_, i) => `[${i}] device ready\r\n`);
    writer.push(lines.join(''));
    assert.equal(output.join(''), lines.map(colorKernelLine).join(''));
    assert.equal(output.length, 1);
    writer.close();
});
test('kernel timestamps, severity and unchanged text', () => {
    for (const [line, color] of [['[ 1.25] device ready\r\n', 37], ['[2.0] probe failed\n', 31], ['[3] timeout\n', 33], ['<3>[4] device stopped\n', 31]]) {
        const rendered = colorKernelLine(line);
        assert.ok(rendered.includes(`\x1b[${color}m`));
        assert.equal(rendered.replace(/\x1b\[\d+m/g, ''), line);
    }
    assert.equal(colorKernelLine('root@board:~# '), 'root@board:~# ');
    assert.equal(colorKernelLine('\x1b[32m[1] ready\x1b[0m\n'), '\x1b[32m[1] ready\x1b[0m\n');
});
test('fragmented UTF-8, prompt flush, and no partial-line recoloring', () => {
    const output = []; let flush;
    const writer = createLogWriter(s => output.push(s), f => { flush = f; return 1; }, () => {});
    const bytes = new TextEncoder().encode('[1] 设备 ready\n');
    writer.push(bytes.slice(0, 6)); writer.push(bytes.slice(6));
    assert.equal(output.join('').replace(/\x1b\[\d+m/g, ''), '[1] 设备 ready\n');
    writer.push('root# '); flush();
    writer.push('[2] typed by user\n');
    assert.equal(output.slice(1).join(''), 'root# [2] typed by user\n');
    writer.close();
});
test('native ANSI state across frames is retained', () => {
    const output = []; let flush;
    const writer = createLogWriter(s => output.push(s), f => { flush = f; return 1; }, () => {});
    writer.push('\x1b[32m'); writer.push('\n[1] ready\n');
    assert.equal(output.join(''), '\x1b[32m\n[1] ready\n');
    writer.push('\x1b[0m\n[2] failed\n');
    assert.ok(output.at(-1).includes('\x1b[31m'));
});
