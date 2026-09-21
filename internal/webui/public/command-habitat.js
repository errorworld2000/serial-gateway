(() => {
    const list = document.getElementById('command-list');
    const pet = document.getElementById('desk-pet');
    const arena = document.createElement('div');
    arena.className = 'command-habitat';
    list.before(arena);
    arena.append(list, pet);
    const reduced = matchMedia('(prefers-reduced-motion: reduce)');
    let seed = Math.random() * 100000;
    try { seed = Number(localStorage.getItem('serial-gateway.habitat-seed')) || seed; localStorage.setItem('serial-gateway.habitat-seed', String(seed)); } catch {}
    const random = n => { const x = Math.sin(n * 127.1 + seed) * 43758.5453; return x - Math.floor(x); };
    let obstacles = [], x = 0, y = 0, angle = 0, frame = 0, last = 0, turnAt = 0;
    const width = 38, height = 34, padding = 5;
    let arenaWidth = 0, arenaHeight = 0;
    function clear(px, py) {
        return px >= padding && py >= padding && px + width <= arenaWidth - padding && py + height <= arenaHeight - padding &&
            obstacles.every(r => px + width + 4 <= r.x || px >= r.x + r.w + 4 || py + height + 4 <= r.y || py >= r.y + r.h + 4);
    }
    function placePet() {
        // Translation does not invalidate layout and composes with the greeting transform.
        pet.style.translate = `${x}px ${y}px`;
        pet.style.setProperty('--pet-facing', Math.cos(angle) < 0 ? '-1' : '1');
    }
    const positionsKey = 'serial-gateway.command-positions.v1';
    let positions = {};
    try { const saved = JSON.parse(localStorage.getItem(positionsKey) || '{}'); if (saved && typeof saved === 'object' && !Array.isArray(saved)) positions = saved; } catch {}
    const measure = document.createElement('canvas').getContext('2d');
    measure.font = '11px Consolas, monospace';
    const snap = value => Math.round(value / 8) * 8;
    const overlaps = (a, b) => a.x < b.x + b.w + 8 && a.x + a.w + 8 > b.x && a.y < b.y + b.h + 8 && a.y + a.h + 8 > b.y;
    function relocatePet() {
        if (clear(x, y)) return;
        for (let py = arenaHeight - height - 8; py >= 8; py -= 8) {
            for (let px = arenaWidth - width - 8; px >= 8; px -= 8) {
                if (clear(px, py)) { x = px; y = py; placePet(); return; }
            }
        }
    }
    let dragging = null, suppressClick = false;
    function layout() {
        if (dragging) return;
        arenaWidth = arena.clientWidth;
        const rows = [...list.querySelectorAll('.command-row')];
        obstacles = [];
        rows.forEach((row, index) => {
            const button = row.querySelector('button');
            const textWidth = measure.measureText(button.textContent).width + 14;
            const w = Math.min(Math.max(40, Math.ceil(textWidth / 8) * 8), 128, Math.max(40, snap(arena.clientWidth - 64)));
            Object.assign(row.style, { width: `${w}px`, height: 'auto' });
            button.style.height = 'auto';
            const h = Math.max(24, Math.ceil(button.scrollHeight / 8) * 8);
            button.style.height = '100%';
            const saved = positions[row.dataset.commandId];
            let rect = { x: saved && Number.isFinite(saved.x) ? snap(Math.max(0, Math.min(1, saved.x)) * Math.max(0, arena.clientWidth - w - 16)) + 8 : 8 + snap(random(index) * Math.max(0, arena.clientWidth - w - 72)), y: saved && Number.isFinite(saved.y) ? Math.min(10000, Math.max(8, snap(saved.y))) : 8, w, h, row };
            // Pack tightly, retaining saved positions whenever they still fit.
            while (obstacles.some(other => overlaps(rect, other))) rect.y += 8;
            Object.assign(row.style, { left: `${rect.x}px`, top: `${rect.y}px`, height: `${h}px` });
            obstacles.push(rect);
        });
        arena.style.height = `${Math.max(160, ...obstacles.map(r => r.y + r.h + 64))}px`;
        arenaHeight = arena.clientHeight;
        relocatePet(); placePet();
    }
    list.addEventListener('pointerdown', event => {
        const row = event.target.closest('.command-row');
        if (!row || event.button !== 0 || event.target.disabled) return;
        const rect = obstacles.find(r => r.row === row);
        if (!rect) return;
        dragging = { rect, pointer: event.pointerId, startX: event.clientX, startY: event.clientY, x: rect.x, y: rect.y, active: false };
    });
    list.addEventListener('pointermove', event => {
        if (!dragging || dragging.pointer !== event.pointerId) return;
        const d = dragging;
        if (!d.active && Math.hypot(event.clientX - d.startX, event.clientY - d.startY) < 10) return;
        if (!d.active) { d.active = true; d.rect.row.classList.add('dragging'); list.setPointerCapture(event.pointerId); closeMenu(); }
        event.preventDefault();
        const candidate = { ...d.rect, x: Math.max(8, Math.min(Math.floor((arena.clientWidth - d.rect.w - 8) / 8) * 8, snap(d.x + event.clientX - d.startX))), y: Math.max(8, Math.min(Math.floor((arena.clientHeight - d.rect.h - 8) / 8) * 8, snap(d.y + event.clientY - d.startY))) };
        if (obstacles.some(r => r !== d.rect && overlaps(candidate, r))) return;
        d.rect.x = candidate.x; d.rect.y = candidate.y;
        Object.assign(d.rect.row.style, { left: `${candidate.x}px`, top: `${candidate.y}px` });
        relocatePet();
    });
    function endDrag(event) {
        if (!dragging || event.pointerId !== dragging.pointer) return;
        const d = dragging; dragging = null;
        d.rect.row.classList.remove('dragging');
        if (list.hasPointerCapture(event.pointerId)) list.releasePointerCapture(event.pointerId);
        if (!d.active) return;
        suppressClick = true; setTimeout(() => { suppressClick = false; }, 0);
        if (event.type === 'pointercancel') { d.rect.x = d.x; d.rect.y = d.y; layout(); return; }
        positions[d.rect.row.dataset.commandId] = { x: (d.rect.x - 8) / Math.max(1, arena.clientWidth - d.rect.w - 16), y: d.rect.y };
        try { localStorage.setItem(positionsKey, JSON.stringify(positions)); }
        catch { document.getElementById('command-status').textContent = '位置暂未保存：浏览器存储不可用。'; }
        layout();
    }
    list.addEventListener('pointerup', endDrag);
    list.addEventListener('pointercancel', endDrag);
    list.addEventListener('click', event => { if (suppressClick) { event.preventDefault(); event.stopImmediatePropagation(); } }, true);
    function tick(now) {
        frame = requestAnimationFrame(tick);
        const dt = Math.min((now - last) / 1000, .04); last = now;
        if (pet.matches(':hover, :focus') || !arenaWidth || !arenaHeight) return;
        if (now > turnAt) { angle += (Math.random() - .5) * 1.5; turnAt = now + 1500 + Math.random() * 2000; }
        let nx = x + Math.cos(angle) * 28 * dt, ny = y + Math.sin(angle) * 28 * dt;
        if (!clear(nx, ny)) {
            // Try alternate headings; every step tests the full cat rectangle.
            for (let i = 1; i <= 12; i++) {
                const candidate = angle + i * Math.PI / 6;
                nx = x + Math.cos(candidate) * 28 * dt; ny = y + Math.sin(candidate) * 28 * dt;
                if (clear(nx, ny)) { angle = candidate; break; }
            }
        }
        if (clear(nx, ny)) { x = nx; y = ny; placePet(); }
    }
    function sync() { cancelAnimationFrame(frame); if (!document.hidden && !reduced.matches) { last = performance.now(); frame = requestAnimationFrame(tick); } }
    new ResizeObserver(layout).observe(arena);
    new MutationObserver(() => {
        if (dragging) {
            if (list.hasPointerCapture(dragging.pointer)) list.releasePointerCapture(dragging.pointer);
            dragging = null;
        }
        closeMenu(false); layout();
    }).observe(list, { childList: true });
    reduced.addEventListener('change', sync);
    document.addEventListener('visibilitychange', sync);
    layout(); sync();

    const menu = document.createElement('div');
    menu.className = 'command-context'; menu.hidden = true; menu.setAttribute('role', 'menu');
    document.body.append(menu);
    let owner;
    function closeMenu(focus = false) { menu.hidden = true; if (focus && owner?.isConnected) owner.focus(); }
    window.openCommandMenu = (event, button, edit, remove) => {
        owner = button; menu.replaceChildren();
        for (const [label, action] of [['编辑', edit], ['删除', remove]]) {
            const item = document.createElement('button'); item.type = 'button'; item.textContent = label;
            item.setAttribute('role', 'menuitem');
            item.onclick = () => { closeMenu(true); action(); };
            menu.append(item);
        }
        menu.hidden = false;
        const rect = button.getBoundingClientRect();
        const px = event.clientX || rect.left, py = event.clientY || rect.bottom;
        menu.style.left = `${Math.max(4, Math.min(px, innerWidth - menu.offsetWidth - 4))}px`;
        menu.style.top = `${Math.max(4, Math.min(py, innerHeight - menu.offsetHeight - 4))}px`;
        menu.firstElementChild.focus();
    };
    document.addEventListener('pointerdown', event => { if (!menu.contains(event.target)) closeMenu(); });
    document.addEventListener('scroll', () => closeMenu(), true);
    window.addEventListener('resize', () => closeMenu());
    menu.addEventListener('keydown', event => {
        if (event.key === 'Escape' || event.key === 'Tab') { closeMenu(true); if (event.key === 'Escape') event.preventDefault(); }
        if (event.key === 'ArrowDown' || event.key === 'ArrowUp') { event.preventDefault(); (document.activeElement === menu.firstElementChild ? menu.lastElementChild : menu.firstElementChild).focus(); }
    });
})();
