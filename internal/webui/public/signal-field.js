import * as THREE from './vendor/three/three.module.min.js';

const canvas = document.getElementById('signal-canvas');
const toggle = document.getElementById('signal-toggle');
const stage = canvas.parentElement;
const reduced = matchMedia('(prefers-reduced-motion: reduce)');
let renderer;
try {
    renderer = new THREE.WebGLRenderer({ canvas, alpha: true, antialias: false, powerPreference: 'low-power' });
} catch {
    stage.classList.add('signal-fallback');
    toggle.disabled = true;
    toggle.textContent = '3D 动效不可用';
}

if (renderer) {
    renderer.setPixelRatio(Math.min(devicePixelRatio, 1.5));
    const scene = new THREE.Scene();
    const camera = new THREE.PerspectiveCamera(42, 1, .1, 30);
    camera.position.z = 5;
    const group = new THREE.Group();
    scene.add(group);
    const geometry = new THREE.TorusKnotGeometry(.78, .19, 100, 7);
    const wire = new THREE.WireframeGeometry(geometry);
    const material = new THREE.LineBasicMaterial({ color: 0x71b6bb, transparent: true, opacity: .24 });
    const knot = new THREE.LineSegments(wire, material);
    group.add(knot);
    const positions = new Float32Array(160 * 3);
    for (let i = 0; i < positions.length; i += 3) {
        positions[i] = (Math.random() - .5) * 9;
        positions[i + 1] = (Math.random() - .5) * 4;
        positions[i + 2] = (Math.random() - .5) * 4;
    }
    const dustGeometry = new THREE.BufferGeometry();
    dustGeometry.setAttribute('position', new THREE.BufferAttribute(positions, 3));
    const dustMaterial = new THREE.PointsMaterial({ color: 0x99cdd5, size: .022, transparent: true, opacity: .5 });
    const dust = new THREE.Points(dustGeometry, dustMaterial);
    group.add(dust);
    let enabled = true;
    try { enabled = localStorage.getItem('serial-gateway.motion') !== 'off'; } catch { /* Optional preference. */ }
    let frame = 0;
    let last = 0;
    let time = 0;
    let pointerX = 0;
    let pointerY = 0;
    let pulse = 0;
    let disposed = false;
    function render() { renderer.render(scene, camera); }
    function tick(now) {
        frame = requestAnimationFrame(tick);
        if (now - last < 1000 / 30) return;
        const delta = Math.min((now - last) / 1000, .06);
        last = now;
        time += delta;
        knot.rotation.y = time * .18;
        knot.rotation.z = Math.sin(time * .3) * .15;
        dust.rotation.y = time * -.025;
        group.rotation.y += (pointerX * .14 - group.rotation.y) * .07;
        group.rotation.x += (pointerY * .1 - group.rotation.x) * .07;
        pulse *= .93;
        material.opacity = .24 + pulse * .3;
        knot.scale.setScalar(1 + pulse * .06);
        render();
    }
    function sync() {
        cancelAnimationFrame(frame);
        frame = 0;
        const moving = enabled && !reduced.matches;
        toggle.setAttribute('aria-pressed', String(moving));
        toggle.textContent = reduced.matches ? '3D 动效：跟随系统静止' : `3D 动效：${enabled ? '开' : '关'}`;
        if (disposed || document.hidden) return;
        render();
        if (moving) { last = performance.now(); frame = requestAnimationFrame(tick); }
    }
    function resize() {
        const width = stage.clientWidth;
        const height = stage.clientHeight;
        if (!width || !height) return;
        renderer.setSize(width, height, false);
        camera.aspect = width / height;
        camera.updateProjectionMatrix();
        if (!document.hidden) render();
    }
    const observer = new ResizeObserver(resize);
    observer.observe(stage);
    stage.addEventListener('pointermove', event => {
        const rect = stage.getBoundingClientRect();
        pointerX = (event.clientX - rect.left) / rect.width * 2 - 1;
        pointerY = (event.clientY - rect.top) / rect.height * 2 - 1;
    });
    stage.addEventListener('pointerleave', () => { pointerX = pointerY = 0; });
    document.getElementById('command-list').addEventListener('click', event => {
        if (event.target.closest('.command-run')) pulse = 1;
    });
    toggle.addEventListener('click', () => {
        enabled = !enabled;
        try { localStorage.setItem('serial-gateway.motion', enabled ? 'on' : 'off'); } catch { /* Optional preference. */ }
        sync();
    });
    document.addEventListener('visibilitychange', sync);
    reduced.addEventListener('change', sync);
    canvas.addEventListener('webglcontextlost', event => {
        event.preventDefault();
        cancelAnimationFrame(frame);
        stage.classList.add('signal-fallback');
    });
    canvas.addEventListener('webglcontextrestored', () => { stage.classList.remove('signal-fallback'); sync(); });
    window.addEventListener('pagehide', event => {
        cancelAnimationFrame(frame);
        if (event.persisted) return;
        disposed = true;
        observer.disconnect();
        document.removeEventListener('visibilitychange', sync);
        reduced.removeEventListener('change', sync);
        geometry.dispose(); wire.dispose(); material.dispose();
        dustGeometry.dispose(); dustMaterial.dispose(); renderer.dispose();
    });
    window.addEventListener('pageshow', sync);
    resize();
    sync();
}
