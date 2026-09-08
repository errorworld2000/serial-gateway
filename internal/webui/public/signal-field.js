import * as THREE from './vendor/three/three.module.min.js';
const canvas = document.getElementById('signal-canvas');
const toggle = document.getElementById('signal-toggle');
const reduced = matchMedia('(prefers-reduced-motion: reduce)');
let enabled = true;
try { enabled = localStorage.getItem('serial-gateway.motion') !== 'off'; } catch {}
let renderer;
try { renderer = new THREE.WebGLRenderer({ canvas, antialias: false, powerPreference: 'low-power' }); }
catch { canvas.hidden = true; toggle.disabled = true; toggle.textContent = '动效不可用'; }
if (renderer) {
    const scene = new THREE.Scene();
    const camera = new THREE.OrthographicCamera(-1, 1, 1, -1, 0, 1);
    const geometry = new THREE.PlaneGeometry(2, 2);
    const uniforms = { time: { value: 0 }, aspect: { value: 1 }, pointer: { value: new THREE.Vector2() }, energy: { value: 0 } };
    // A single draw call blends flowing ribbons and sparse stars into the page.
    const material = new THREE.ShaderMaterial({ uniforms, depthTest: false, depthWrite: false,
        vertexShader: 'varying vec2 uvPos; void main(){uvPos=uv;gl_Position=vec4(position.xy,0.,1.);}',
        fragmentShader: `precision mediump float;
        varying vec2 uvPos; uniform float time, aspect, energy; uniform vec2 pointer;
        float hash(vec2 p){return fract(sin(dot(p,vec2(127.1,311.7)))*43758.5453);}
        void main(){
            vec2 p=uvPos+pointer*.012; vec3 color=vec3(.014,.025,.044);
            for(int i=0;i<3;i++){
                float f=float(i);
                float wave=.25+f*.23+.12*sin(p.x*4.+time*.16+f*1.7)+.06*sin(p.x*8.-time*.1+f);
                float d=abs(p.y-wave);
                float glow=exp(-d*18.)*.10+exp(-d*100.)*.06;
                vec3 tint=mix(vec3(.22,.65,.73),vec3(.38,.32,.66),f*.4);
                color+=tint*glow*(.65+.35*sin(p.x*5.+f))*(1.+energy*.7);
            }
            vec2 grid=p*vec2(100.*aspect,100.); float seed=hash(floor(grid));
            float star=(1.-smoothstep(0.,.09,length(fract(grid)-.5)))*step(.985,seed);
            color+=vec3(.35,.58,.65)*star*(.25+.15*sin(time*.5+seed*100.));
            color*=.7+.3*(1.-length(uvPos-.5)); gl_FragColor=vec4(color,1.);
        }`
    });
    scene.add(new THREE.Mesh(geometry, material));
    let frame = 0, last = 0, lost = false, disposed = false, lastKey = 0;
    const target = new THREE.Vector2();
    const ripples = new Set();
    const moving = () => enabled && !reduced.matches && !document.hidden && !lost && !disposed;
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
        toggle.setAttribute('aria-pressed', String(enabled && !reduced.matches));
        toggle.textContent = reduced.matches ? '动效：系统静止' : `动效：${enabled ? '开' : '关'}`;
        if (!moving()) { ripples.forEach(r => r.remove()); ripples.clear(); uniforms.energy.value = 0; }
        if (disposed || lost || document.hidden) return;
        render();
        if (moving()) { last = performance.now(); frame = requestAnimationFrame(tick); }
    }
    function resize() {
        renderer.setPixelRatio(Math.min(devicePixelRatio, 1.25)); renderer.setSize(innerWidth, innerHeight, false);
        uniforms.aspect.value = innerWidth / Math.max(innerHeight, 1);
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
            terminal.animate([{ boxShadow: 'inset 0 -2px 22px #78d7e025' }, { boxShadow: 'inset 0 -2px 22px #78d7e000' }], { duration: 260, easing: 'ease-out' });
        }
    }
    toggle.addEventListener('click', () => { enabled = !enabled; try { localStorage.setItem('serial-gateway.motion', enabled ? 'on' : 'off'); } catch {} sync(); });
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
