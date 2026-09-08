import * as THREE from './vendor/three/three.module.min.js';
const canvas = document.getElementById('signal-canvas');
const reduced = matchMedia('(prefers-reduced-motion: reduce)');
let renderer;
try { renderer = new THREE.WebGLRenderer({ canvas, antialias: false, powerPreference: 'low-power' }); }
catch { canvas.hidden = true; }
if (renderer) {
    const scene = new THREE.Scene();
    const camera = new THREE.OrthographicCamera(-1, 1, 1, -1, 0, 1);
    const geometry = new THREE.PlaneGeometry(2, 2);
    const uniforms = { time: { value: 0 }, resolution: { value: new THREE.Vector2(1, 1) }, pointer: { value: new THREE.Vector2() }, energy: { value: 0 } };
    // A fixed 16px monochrome dot matrix, with a restrained scanning highlight.
    const material = new THREE.ShaderMaterial({ uniforms, depthTest: false, depthWrite: false,
        vertexShader: 'varying vec2 uvPos; void main(){uvPos=uv;gl_Position=vec4(position.xy,0.,1.);}',
        fragmentShader: `precision mediump float;
        varying vec2 uvPos; uniform float time, energy; uniform vec2 pointer, resolution;
        void main(){
            vec2 pixel=uvPos*resolution;
            vec2 cell=floor(pixel/16.);
            vec2 center=(cell+.5)*16.;
            float dotMask=1.-smoothstep(.75,1.45,length(pixel-center));
            float scan=exp(-pow((mod(center.x+center.y*.3-time*24.,resolution.x+resolution.y*.3)-80.)/48.,2.));
            vec2 mouse=(pointer*.5+.5)*resolution;
            float nearby=exp(-length(center-mouse)/110.);
            float light=.12+scan*.075+nearby*.04+energy*.09;
            gl_FragColor=vec4(vec3(dotMask*light),1.);
        }`
    });
    scene.add(new THREE.Mesh(geometry, material));
    let frame = 0, last = 0, lost = false, disposed = false, lastKey = 0;
    const target = new THREE.Vector2();
    const ripples = new Set();
    const moving = () => !reduced.matches && !document.hidden && !lost && !disposed;
    const render = () => renderer.render(scene, camera);
    function tick(now) {
        frame = requestAnimationFrame(tick);
        if (now - last < 1000 / 30) return;
        const delta = Math.min((now - last) / 1000, .07); last = now;
        uniforms.time.value += delta; uniforms.energy.value *= Math.exp(-delta * 4);
        uniforms.pointer.value.lerp(target, .06); render();
    }
    function sync() {
        cancelAnimationFrame(frame); frame = 0;
        if (!moving()) { ripples.forEach(r => r.remove()); ripples.clear(); uniforms.energy.value = 0; }
        if (disposed || lost || document.hidden) return;
        render();
        if (moving()) { last = performance.now(); frame = requestAnimationFrame(tick); }
    }
    function resize() {
        renderer.setPixelRatio(Math.min(devicePixelRatio, 1.25)); renderer.setSize(innerWidth, innerHeight, false);
        uniforms.resolution.value.set(innerWidth, innerHeight);
        if (!lost && !disposed && !document.hidden) render();
    }
    function pointerMove(event) { target.set(event.clientX / innerWidth * 2 - 1, 1 - event.clientY / innerHeight * 2); }
    function feedback(event) {
        if (!moving()) return;
        uniforms.energy.value = .8;
        if (ripples.size >= 10) { const oldest = ripples.values().next().value; oldest.remove(); ripples.delete(oldest); }
        const ripple = document.createElement('span'); ripple.className = 'signal-ripple'; ripple.setAttribute('aria-hidden', 'true');
        ripple.style.left = `${event.clientX}px`; ripple.style.top = `${event.clientY}px`;
        document.body.append(ripple); ripples.add(ripple);
        ripple.addEventListener('animationend', () => { ripple.remove(); ripples.delete(ripple); }, { once: true });
    }
    function keyFeedback(event) {
        if (!moving() || event.ctrlKey || event.metaKey || event.altKey || event.isComposing) return;
        if (!(event.key.length === 1 || ['Enter', 'Backspace', 'Delete', 'Tab'].includes(event.key))) return;
        const now = performance.now(); if (now - lastKey < 70) return; lastKey = now;
        uniforms.energy.value = Math.min(1, uniforms.energy.value + .35);
        const terminal = event.target.closest?.('.terminal-container');
        if (terminal?.animate) {
            terminal.getAnimations().forEach(animation => animation.cancel());
            terminal.animate([{ boxShadow: 'inset 0 -1px 0 #e8e8e870' }, { boxShadow: 'inset 0 -1px 0 #e8e8e800' }], { duration: 260, easing: 'ease-out' });
        }
    }
    document.addEventListener('pointermove', pointerMove, { passive: true });
    document.addEventListener('pointerdown', feedback, { passive: true });
    document.addEventListener('keydown', keyFeedback, true);
    document.addEventListener('visibilitychange', sync); reduced.addEventListener('change', sync);
    window.addEventListener('resize', resize);
    canvas.addEventListener('webglcontextlost', event => { event.preventDefault(); lost = true; sync(); });
    canvas.addEventListener('webglcontextrestored', () => { lost = false; resize(); sync(); });
    window.addEventListener('pagehide', event => {
        cancelAnimationFrame(frame); if (event.persisted) return; disposed = true;
        document.removeEventListener('pointermove', pointerMove); document.removeEventListener('pointerdown', feedback);
        document.removeEventListener('keydown', keyFeedback, true); document.removeEventListener('visibilitychange', sync);
        window.removeEventListener('resize', resize); reduced.removeEventListener('change', sync);
        geometry.dispose(); material.dispose(); renderer.dispose();
    });
    window.addEventListener('pageshow', sync); resize(); sync();
}
