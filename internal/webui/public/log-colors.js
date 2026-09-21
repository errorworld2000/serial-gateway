// Only decorate plain, timestamped kernel logs. Never send these escapes to the device.
function colorKernelLine(line) {
    if (/[\x00-\x08\x0b\x0c\x0e-\x1f\x7f-\x9f]/.test(line)) return line;
    const match = /^(\r?)(?:<(\d{1,3})>)?(\[\s*\d+(?:\.\d+)?\]\s*)(.*?)(\r?\n)?$/.exec(line);
    if (!match) return line;
    const [, start, priority, stamp, message, ending = ''] = match;
    const level = priority === undefined ? null : Number(priority) % 8;
    const color = level !== null ? (level <= 3 ? 31 : level === 4 ? 33 : 37)
        : /\b(error|failed|failure|panic|oops|fatal|call trace)\b/i.test(message) ? 31
        : /\b(warn(?:ing)?|timeout|timed out|deprecated)\b/i.test(message) ? 33 : 37;
    return `${start}\x1b[90m${priority === undefined ? '' : `<${priority}>`}${stamp}\x1b[${color}m${message}\x1b[0m${ending}`;
}

function createLogWriter(write, schedule = setTimeout, cancel = clearTimeout) {
    const decoder = new TextDecoder();
    let pending = '', timer = null, continuation = false, ansiActive = false;
    // Keep a small tail so a reset split across network frames is recognized.
    let controlTail = '';
    function emit(text, complete, output = write) {
        const native = ansiActive || text.includes('\x1b') || /[\x80-\x9f]/.test(text);
        output(!continuation && !native ? colorKernelLine(text) : text);
        const controls = controlTail + text;
        for (const match of controls.matchAll(/\x1b(?:\[(?:0)?m)?/g)) {
            ansiActive = match[0] !== '\x1b[0m' && match[0] !== '\x1b[m';
        }
        controlTail = controls.slice(-8);
        continuation = !complete;
    }
    function flush() {
        if (timer !== null) cancel(timer);
        timer = null;
        if (pending) { emit(pending, false); pending = ''; }
    }
    function push(data) {
        pending += typeof data === 'string' ? data : decoder.decode(data, { stream: true });
        let end;
        const batch = [];
        while ((end = pending.indexOf('\n')) !== -1) {
            const line = pending.slice(0, end + 1); pending = pending.slice(end + 1);
            emit(line, true, text => batch.push(text));
        }
        if (batch.length) write(batch.join(''));
        if (pending.length > 8192) flush();
        // Bound prompt/interactive latency; continuous traffic cannot postpone this timer.
        if (pending && timer === null) timer = schedule(flush, 24);
    }
    return { push, close() { pending += decoder.decode(); flush(); } };
}
if (typeof module !== 'undefined') module.exports = { colorKernelLine, createLogWriter };
