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
    function clear(px, py) {
        return px >= padding && py >= padding && px + width <= arena.clientWidth - padding && py + height <= arena.clientHeight - padding &&
            obstacles.every(r => px + width + 4 <= r.x || px >= r.x + r.w + 4 || py + height + 4 <= r.y || py >= r.y + r.h + 4);
    }
    function placePet() {
        pet.style.left = `${x}px`; pet.style.top = `${y}px`;
        pet.style.setProperty('--pet-facing', Math.cos(angle) < 0 ? '-1' : '1');
    }
    function layout() {
        const rows = [...list.querySelectorAll('.command-row')];
        const columns = Math.max(1, Math.floor(arena.clientWidth / 210));
        const cell = arena.clientWidth / columns;
        arena.style.height = `${Math.max(220, Math.ceil(rows.length / columns) * 112 + 66)}px`;
        obstacles = [];
        rows.forEach((row, index) => {
            const w = Math.max(70, Math.min(140, cell - 72));
            const px = (index % columns) * cell + 6 + random(index * 2) * Math.max(0, cell - w - 68);
            const py = Math.floor(index / columns) * 112 + 18 + random(index * 2 + 1) * 18;
            Object.assign(row.style, { left: `${px}px`, top: `${py}px`, width: `${w}px` });
            obstacles.push({ x: px, y: py, w, h: 40 });
        });
        if (!clear(x, y)) { x = Math.max(padding, arena.clientWidth - width - 12); y = arena.clientHeight - height - 12; }
        placePet();
    }
    function tick(now) {
        frame = requestAnimationFrame(tick);
        const dt = Math.min((now - last) / 1000, .04); last = now;
        if (pet.matches(':hover, :focus') || !arena.getClientRects().length) return;
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
    new MutationObserver(() => { closeMenu(false); layout(); }).observe(list, { childList: true });
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
