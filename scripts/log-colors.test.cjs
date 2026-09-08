const { test } = require('node:test');
const assert = require('node:assert/strict');
const { colorKernelLine, createLogWriter } = require('../internal/webui/public/log-colors.js');
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
    writer.push('\x1b[32m'); flush(); writer.push('\n[1] ready\n');
    assert.equal(output.join(''), '\x1b[32m\n[1] ready\n');
    writer.push('\x1b[0m\n[2] failed\n');
    assert.ok(output.at(-1).includes('\x1b[31m'));
});
