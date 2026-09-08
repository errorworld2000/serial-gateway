(() => {
    const reduced = matchMedia('(prefers-reduced-motion: reduce)');
    const pet = document.getElementById('desk-pet');
    const caption = document.getElementById('pet-caption');
    let shell, reveal, resetTimer, lastKey = 0;
    function greet(message) {
        caption.textContent = message;
        clearTimeout(resetTimer);
        resetTimer = setTimeout(() => { caption.textContent = '陪你调试 · 点击打招呼'; }, 1800);
        if (reduced.matches) return;
        pet.getAnimations().forEach(a => a.cancel());
        pet.animate([{ transform: 'translateY(0)' }, { transform: 'translateY(-4px)', offset: .4 }, { transform: 'translateY(0)' }], { duration: 260, easing: 'ease-out' });
    }
    pet.addEventListener('click', () => greet('喵，串口交给你，我陪着。'));
    document.addEventListener('keydown', event => {
        if (!event.target.closest?.('.terminal-container') || event.ctrlKey || event.metaKey || event.altKey) return;
        if (performance.now() - lastKey < 700) return;
        lastKey = performance.now();
        greet('收到，继续敲。');
    }, true);
    function clearTransition() { shell?.remove(); shell = null; reveal?.cancel(); reveal = null; }
    document.addEventListener('terminal-switch', event => {
        clearTransition();
        if (reduced.matches || document.hidden) return;
        const container = event.detail.container;
        shell = document.createElement('div');
        shell.className = 'terminal-hologram';
        shell.setAttribute('aria-hidden', 'true');
        const globe = document.createElement('div');
        globe.className = 'hologram-globe';
        for (let i = 0; i < 6; i++) {
            const ring = document.createElement('i');
            ring.style.transform = `rotateY(${i * 30}deg)`;
            globe.append(ring);
        }
        for (let i = -1; i <= 1; i++) {
            const ring = document.createElement('i');
            ring.style.transform = `rotateX(90deg) translateZ(${i * 26}px) scale(${i ? .86 : 1})`;
            globe.append(ring);
        }
        shell.append(globe);
        container.parentElement.append(shell);
        const current = shell;
        shell.addEventListener('animationend', event => { if (event.target === current) { current.remove(); if (shell === current) shell = null; } });
        reveal = container.animate([
            { clipPath: 'circle(36px at 50% 50%)', opacity: .3 },
            { clipPath: 'circle(36px at 50% 50%)', opacity: .6, offset: .18 },
            { clipPath: 'circle(150% at 50% 50%)', opacity: 1 }
        ], { duration: 520, easing: 'cubic-bezier(.22,.75,.2,1)' });
    });
    reduced.addEventListener('change', () => { clearTransition(); pet.getAnimations().forEach(a => a.cancel()); });
    document.addEventListener('visibilitychange', () => { if (document.hidden) clearTransition(); });
})();
