const { test } = require('node:test');
const assert = require('node:assert/strict');
const vm = require('node:vm');
const fs = require('node:fs');
const path = require('node:path');

test('pet movement does not read layout or write layout positions each frame', () => {
    let reads = 0, positionWrites = 0, frame;
    const element = () => ({
        style: new Proxy({ setProperty() {} }, { set(target, key, value) {
            if (key === 'left' || key === 'top') positionWrites++;
            target[key] = value; return true;
        } }),
        get clientWidth() { reads++; return 300; },
        get clientHeight() { reads++; return 160; },
        getClientRects() { reads++; return [{}]; },
        before() {}, append() {}, addEventListener() {}, setAttribute() {},
        querySelectorAll() { return []; }, matches() { return false; },
        getContext() { return {}; }
    });
    const list = element(), pet = element();
    vm.runInNewContext(fs.readFileSync(path.join(__dirname, '../internal/webui/public/command-habitat.js'), 'utf8'), {
        document: { hidden: false, body: element(), getElementById: id => id === 'command-list' ? list : pet,
            createElement: element, addEventListener() {} },
        window: { addEventListener() {} }, localStorage: { getItem() { return null; }, setItem() {} },
        matchMedia: () => ({ matches: false, addEventListener() {} }),
        ResizeObserver: class { observe() {} }, MutationObserver: class { observe() {} },
        performance: { now: () => 0 }, requestAnimationFrame: f => { frame = f; return 1; }, cancelAnimationFrame() {}
    });
    reads = 0; positionWrites = 0;
    for (let i = 1; i <= 120; i++) frame(i * 1000 / 60);
    assert.equal(reads, 0, 'steady movement must use cached arena dimensions');
    assert.equal(positionWrites, 0, 'movement must use composited translation');
    assert.ok(pet.style.translate, 'pet still moves');
});
