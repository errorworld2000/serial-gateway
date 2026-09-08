// Send String macros; these are serial text, never JavaScript or shell code.
function parseCommand(source) {
    const chunks = [];
    let text = '';
    const escapes = { r: '\r', n: '\n', t: '\t', e: '\x1b', b: '\b', '\\': '\\' };
    for (let i = 0; i < source.length; i++) {
        const char = source[i];
        if (char === '\\' && i + 1 < source.length) {
            const next = source[++i];
            if (next === 'p') {
                if (text) chunks.push(text);
                chunks.push(null);
                text = '';
            } else {
                text += escapes[next] ?? '\\' + next;
            }
        } else if (char === '\n') {
            text += '\r';
        } else if (char === '\r') {
            text += '\r';
            if (source[i + 1] === '\n') i++;
        } else text += char;
    }
    if (text) chunks.push(text);
    return chunks;
}

function initCommands(getSession) {
    const byId = id => document.getElementById(id);
    const list = byId('command-list');
    const editor = byId('command-editor');
    const status = byId('command-status');
    const stop = byId('command-stop');
    const storageKey = 'serial-gateway.commands.v1';
    const colors = ['#4A9E5C', '#5195db', '#ce9e45', '#D71921'];
    let commands = [];
    let editing = -1;
    let running = null;
    try {
        const saved = JSON.parse(localStorage.getItem(storageKey) || '[]');
        if (!Array.isArray(saved) || saved.some(c => !c || typeof c.name !== 'string' || typeof c.text !== 'string')) throw new Error('Invalid data');
        commands = saved;
    } catch {
        status.textContent = '无法读取已保存的按钮；可新建按钮。';
    }

    function save(next) {
        try { localStorage.setItem(storageKey, JSON.stringify(next)); }
        catch { status.textContent = '保存失败：请检查浏览器存储权限或剩余空间。'; return false; }
        commands = next;
        render();
        return true;
    }

    function edit(index) {
        editing = index;
        const command = commands[index];
        byId('command-name').value = command?.name || '';
        byId('command-text').value = command?.text || '';
        byId('command-color').value = colors.includes(command?.color) ? command.color : colors[0];
        editor.hidden = false;
        byId('command-name').focus();
    }

    function render() {
        list.replaceChildren();
        if (!commands.length) {
            const empty = document.createElement('p');
            empty.className = 'command-help';
            empty.textContent = '还没有命令，点击「新建」添加。';
            list.append(empty);
        }
        commands.forEach((command, index) => {
            const row = document.createElement('div');
            row.className = 'command-row';
            const run = document.createElement('button');
            run.type = 'button';
            run.className = 'action-button command-run';
            run.textContent = command.name;
            run.title = command.text;
            run.style.setProperty('--command-color', colors.includes(command.color) ? command.color : colors[0]);
            run.disabled = !!running;
            run.onclick = () => execute(command);
            const editButton = document.createElement('button');
            editButton.type = 'button';
            editButton.className = 'action-button';
            editButton.textContent = '编辑';
            editButton.setAttribute('aria-label', `编辑 ${command.name}`);
            editButton.onclick = () => edit(index);
            const remove = document.createElement('button');
            remove.type = 'button';
            remove.className = 'action-button';
            remove.textContent = '×';
            remove.setAttribute('aria-label', `删除 ${command.name}`);
            remove.onclick = () => {
                if (confirm(`删除按钮「${command.name}」？`) && save(commands.filter((_, i) => i !== index))) editor.hidden = true;
            };
            row.append(run, editButton, remove);
            list.append(row);
        });
    }

    async function execute(command) {
        if (running) return;
        const session = getSession();
        const socket = session?.socket;
        if (!socket || socket.readyState !== WebSocket.OPEN) {
            status.textContent = '请先选择已连接的串口。';
            return;
        }
        const job = { cancelled: false };
        running = job;
        stop.hidden = false;
        render();
        status.textContent = `正在向 ${session.portName} 发送：${command.name}`;
        try {
            for (const chunk of parseCommand(command.text)) {
                if (job.cancelled) throw new Error('已停止后续发送。');
                if (getSession() !== session || session.socket !== socket || socket.readyState !== WebSocket.OPEN) {
                    throw new Error('串口已切换或连接已断开，已停止后续发送。');
                }
                if (chunk === null) await new Promise(resolve => setTimeout(resolve, 1000));
                else socket.send(chunk);
            }
            if (job.cancelled) throw new Error('已停止后续发送。');
            status.textContent = `已发送到 ${session.portName}：${command.name}`;
            if (getSession() === session) session.term.focus();
        } catch (error) {
            status.textContent = error.message;
        } finally {
            running = null;
            stop.hidden = true;
            render();
        }
    }

    byId('command-add').onclick = () => edit(-1);
    byId('command-cancel').onclick = () => { editor.hidden = true; };
    stop.onclick = () => { if (running) running.cancelled = true; };
    editor.onsubmit = event => {
        event.preventDefault();
        const command = { name: byId('command-name').value.trim(), text: byId('command-text').value, color: byId('command-color').value };
        if (!command.name || !command.text) return;
        const next = [...commands];
        if (editing < 0) next.push(command);
        else next[editing] = command;
        if (save(next)) { editor.hidden = true; status.textContent = '按钮已保存。'; }
    };
    render();
}
