(() => {
  'use strict';

  const historyStateKey = 'contentcloudWorkbenchSession';
  const state = { capability: '', csrf: '', expiresAt: '', snapshot: null, query: {view: 'workspace_summary'}, claim: null, pendingClaims: {}, proposal: null, runID: '', lastEventID: 0, closed: false, eventsConnected: false };
  const elements = {
    app: document.querySelector('#app'), workspace: document.querySelector('#workspace-name'), revision: document.querySelector('#revision'),
    kind: document.querySelector('#view-kind'), title: document.querySelector('#view-title'), summary: document.querySelector('#view-summary'),
    content: document.querySelector('#view-content'), facts: document.querySelector('#facts'), checks: document.querySelector('#checks'),
    activity: document.querySelector('#activity-text'), activityDot: document.querySelector('#activity-state'), refresh: document.querySelector('#refresh-view'), edit: document.querySelector('#edit-view'), close: document.querySelector('#close-session'),
    ownership: document.querySelector('#ownership-state'), dialog: document.querySelector('#confirm-dialog'), confirmKind: document.querySelector('#confirm-kind'),
    confirmTitle: document.querySelector('#confirm-title'), confirmMessage: document.querySelector('#confirm-message'), confirmEffects: document.querySelector('#confirm-effects'), confirmAccept: document.querySelector('#confirm-accept')
  };

  function activity(message, tone = 'progress') {
    elements.activity.textContent = message;
    elements.activityDot.className = `activity-dot${tone === 'error' ? ' is-error' : tone === 'idle' ? ' is-idle' : ''}`;
  }

  async function api(url, options = {}) {
    const headers = new Headers(options.headers || {});
    headers.set('Authorization', `Bearer ${state.capability}`);
	const method = options.method || 'GET';
    if (method !== 'GET' && method !== 'HEAD') {
      headers.set('X-Workbench-CSRF', state.csrf);
      if (!headers.has('Content-Type')) headers.set('Content-Type', 'application/json');
      if (!headers.has('Idempotency-Key')) headers.set('Idempotency-Key', idempotencyKey());
    }
    const response = await fetch(url, {...options, headers, cache: 'no-store'});
    if (!response.ok) {
      if (response.status === 401) { state.closed = true; clearPersistedSession(); }
      let message = `请求失败 (${response.status})`;
      try { const body = await response.json(); message = body.error?.message || message; } catch (_) {}
      throw new Error(message);
    }
    return response;
  }

  async function exchange() {
    const params = new URLSearchParams(location.hash.slice(1));
    const token = params.get('handoff');
    if (!token) {
      const restored = history.state?.[historyStateKey];
      if (!restored || typeof restored.capability !== 'string' || typeof restored.csrf !== 'string') {
        throw new Error('本地会话入口已失效，请从 Codex 重新打开。');
      }
      state.capability = restored.capability;
      state.csrf = restored.csrf;
      state.expiresAt = typeof restored.expiresAt === 'string' ? restored.expiresAt : '';
      state.pendingClaims = restored.pendingClaims && typeof restored.pendingClaims === 'object' ? restored.pendingClaims : {};
      return;
    }
    clearPersistedSession();
    const response = await fetch('/api/v1/session/exchange', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({token}), cache: 'no-store'});
    if (!response.ok) throw new Error('本地会话入口已失效，请从 Codex 重新打开。');
    const body = await response.json();
    state.capability = body.capability;
    state.csrf = body.csrf;
    state.expiresAt = body.expires_at;
    state.pendingClaims = body.claims || {};
    persistSession();
  }

  function persistSession() {
    const current = history.state && typeof history.state === 'object' ? history.state : {};
    history.replaceState({...current, [historyStateKey]: {
      capability: state.capability, csrf: state.csrf, expiresAt: state.expiresAt, pendingClaims: state.pendingClaims
    }}, '', '/');
  }

  function clearPersistedSession() {
    const current = history.state && typeof history.state === 'object' ? {...history.state} : {};
    delete current[historyStateKey];
    history.replaceState(Object.keys(current).length ? current : null, '', '/');
  }

  async function prepareServiceWorker() {
    if (!('serviceWorker' in navigator)) return false;
    await navigator.serviceWorker.register('/assets/sw.js', {scope: '/'});
    await navigator.serviceWorker.ready;
    if (!navigator.serviceWorker.controller) {
      await new Promise((resolve) => navigator.serviceWorker.addEventListener('controllerchange', resolve, {once: true}));
    }
    const controller = navigator.serviceWorker.controller;
    if (!controller) return false;
    await new Promise((resolve, reject) => {
      const channel = new MessageChannel();
      const timeout = setTimeout(() => { channel.port1.close(); reject(new Error('媒体授权初始化超时')); }, 5000);
      channel.port1.addEventListener('message', (event) => {
        if (event.data?.type !== 'workbench-capability-ready') return;
        clearTimeout(timeout);
        channel.port1.close();
        resolve();
      }, {once: true});
      channel.port1.start();
      controller.postMessage({type: 'workbench-capability', capability: state.capability}, [channel.port2]);
    });
    return true;
  }

  async function acceptBrowserHandoff() {
    await exchange();
    await prepareServiceWorker();
    await reloadServerView();
    if (!state.eventsConnected) {
      state.eventsConnected = true;
      connectEvents();
    }
  }

  async function reloadServerView() {
    state.snapshot = null;
    state.query = {view: 'workspace_summary'};
    await load();
  }

  async function load(query = state.query) {
    activity('正在读取本地工作区');
	const resolvedQuery = {...query};
	if (state.runID && !resolvedQuery.run_id) resolvedQuery.run_id = state.runID;
    const params = new URLSearchParams(Object.entries(resolvedQuery).filter(([key, value]) => key !== 'view' && value !== '' && value !== undefined));
    const initialBootstrap = resolvedQuery.view === 'workspace_summary' && state.snapshot === null;
    const response = await api(initialBootstrap ? '/api/v1/bootstrap' : `/api/v1/views/${encodeURIComponent(resolvedQuery.view)}?${params}`);
    state.snapshot = await response.json();
	state.runID = state.snapshot.view.run_id || state.runID;
	state.query = initialBootstrap ? queryFromSnapshot(state.snapshot) : {...query};
	const persistedToken = state.pendingClaims[state.runID];
	if (!state.claim && persistedToken && state.snapshot.ownership?.claimed && state.snapshot.ownership.owner_id === state.snapshot.workbench_id) {
		state.claim = {...state.snapshot.ownership, token: persistedToken, owner_id: state.snapshot.workbench_id};
	}
	if (state.snapshot.ownership?.owner_id !== state.claim?.owner_id || state.snapshot.ownership?.epoch !== state.claim?.epoch) state.claim = null;
    render(state.snapshot);
    activity('本地工作区已同步', 'idle');
  }

  function queryFromSnapshot(snapshot) {
    const view = snapshot.view.view;
    return {view: view.kind, ...(view.ref ? {ref: view.ref} : {})};
  }

  function render(snapshot) {
    const view = snapshot.view.view;
    elements.app.setAttribute('aria-busy', 'false');
    elements.workspace.textContent = `${snapshot.workspace_id} / ${snapshot.project_id}`;
    elements.revision.textContent = `revision ${snapshot.view.context_revision || '--'}`;
    elements.kind.textContent = view.kind.replaceAll('_', ' ');
    elements.title.textContent = view.title;
    elements.summary.textContent = view.summary;
    renderContent(view, snapshot.resources || []);
    renderFacts(snapshot);
    renderChecks(view.checks || []);
	renderOwnership(snapshot.ownership);
	elements.edit.hidden = !canEdit(snapshot);
  }

  function renderContent(view, resources) {
    elements.content.replaceChildren();
    if (Array.isArray(view.data)) {
      const list = document.createElement('div');
      list.className = 'entries';
      for (const item of view.data) {
        const button = document.createElement('button');
        button.type = 'button';
        button.className = 'entry';
        const name = document.createElement('span'); name.className = 'entry-name'; name.textContent = item.ref;
        const kind = document.createElement('span'); kind.className = 'entry-kind'; kind.textContent = item.kind;
        const size = document.createElement('span'); size.className = 'entry-size mono'; size.textContent = formatBytes(item.byte_size || 0);
        button.append(name, kind, size);
        button.addEventListener('click', () => load({view: 'file', ref: item.ref}).catch(showError));
        list.append(button);
      }
      if (!view.data.length) list.append(empty('当前目录为空。'));
      elements.content.append(list);
      return;
    }
    const resource = resources[0];
    if (resource && resource.mime_type.startsWith('image/')) return elements.content.append(media('img', resource));
    if (resource && resource.mime_type.startsWith('audio/')) return elements.content.append(media('audio', resource));
    if (resource && resource.mime_type.startsWith('video/')) return elements.content.append(media('video', resource));
    if (resource && resource.mime_type === 'application/pdf') return elements.content.append(media('iframe', resource));
    if (view.text) {
      if (view.mime_type === 'text/markdown') {
        elements.content.append(renderMarkdown(view.text));
      } else {
        const pre = document.createElement('pre'); pre.className = 'document'; pre.textContent = view.text; elements.content.append(pre);
      }
      return;
    }
    if (view.data) {
      elements.content.append(renderStructured(view.data)); return;
    }
    elements.content.append(empty('当前视图没有可展示的内容。'));
  }

  function renderMarkdown(text) {
    const article = document.createElement('article'); article.className = 'markdown-document';
    for (const line of String(text).split(/\r?\n/)) {
      const trimmed = line.trim();
      if (!trimmed) continue;
      const heading = trimmed.match(/^(#{1,3})\s+(.+)$/);
      if (heading) { const node = document.createElement(`h${heading[1].length}`); node.textContent = heading[2]; article.append(node); continue; }
      if (/^[-*]\s+/.test(trimmed)) { const list = article.lastElementChild?.tagName === 'UL' ? article.lastElementChild : document.createElement('ul'); if (!list.parentNode) article.append(list); const item = document.createElement('li'); item.textContent = trimmed.replace(/^[-*]\s+/, ''); list.append(item); continue; }
      const paragraph = document.createElement('p'); paragraph.textContent = trimmed; article.append(paragraph);
    }
    return article;
  }

  function renderStructured(value) {
    const table = document.createElement('dl'); table.className = 'structured-facts';
    const entries = Array.isArray(value) ? value.map((item, index) => [String(index + 1), item]) : Object.entries(value);
    for (const [key, item] of entries) {
      const row = document.createElement('div'); const term = document.createElement('dt'); const detail = document.createElement('dd');
      term.textContent = key; detail.textContent = typeof item === 'string' ? item : JSON.stringify(item, null, 2); row.append(term, detail); table.append(row);
    }
    return table;
  }

  function renderOwnership(ownership) {
    if (!state.runID) {
      elements.ownership.textContent = '当前视图未绑定 LocalRun，仅可查看。';
      return;
    }
    if (!ownership?.claimed) {
      elements.ownership.textContent = ownership?.expired ? `租约已过期 / epoch ${ownership.epoch}` : '当前没有写入者。';
      return;
    }
    elements.ownership.textContent = `${ownership.owner_kind} / ${ownership.owner_id} / epoch ${ownership.epoch}`;
  }

  function canEdit(snapshot) {
    const view = snapshot.view.view;
    const ref = view.ref || '';
    const writable = (ref.startsWith('40-work/') && !ref.startsWith('40-work/runs/') && !ref.startsWith('40-work/handoffs/')) || ref.startsWith('50-production/');
    const documentValue = typeof view.text === 'string' && view.text !== '' || view.data && !Array.isArray(view.data);
    return Boolean(snapshot.view.run_id && snapshot.view.context_revision && snapshot.view.observed_digest && writable && documentValue);
  }

  async function editCurrentView() {
    if (!canEdit(state.snapshot)) return;
    const claim = await ensureBrowserOwnership();
    if (!claim) return;
    const view = state.snapshot.view.view;
    const editor = document.createElement('div'); editor.className = 'editor';
    const toolbar = document.createElement('div'); toolbar.className = 'editor-toolbar';
    const label = document.createElement('p'); label.textContent = `${view.ref} / epoch ${claim.epoch}`;
    const actions = document.createElement('div'); actions.className = 'editor-actions';
    const cancel = commandButton('取消', 'button button-quiet');
    const prepare = commandButton('检查变更', 'button button-primary');
    const textarea = document.createElement('textarea'); textarea.className = 'draft-editor'; textarea.spellcheck = false;
    textarea.value = view.text || JSON.stringify(view.data, null, 2) + '\n';
    cancel.addEventListener('click', () => render(state.snapshot));
    prepare.addEventListener('click', async () => {
      prepare.disabled = true;
      try { await prepareAndApply(textarea.value); } catch (error) { showError(error); } finally { prepare.disabled = false; }
    });
    actions.append(cancel, prepare); toolbar.append(label, actions); editor.append(toolbar, textarea);
    elements.content.replaceChildren(editor); textarea.focus();
  }

  async function ensureBrowserOwnership() {
    if (state.claim?.token && state.claim.owner_id === state.snapshot.workbench_id) return state.claim;
    const ownership = state.snapshot.ownership;
    const base = {run_id: state.runID, expected_context_revision: state.snapshot.view.context_revision};
    let response;
    if (ownership?.claimed) {
      const confirmed = await confirmAction({
        kind: 'OWNERSHIP', title: '接管本地写入权',
        message: '当前 LocalRun 由另一个写入者持有。接管会递增 epoch，并立即使旧 token 失效。', confirmLabel: '确认接管',
        effects: [['当前所有者', `${ownership.owner_kind} / ${ownership.owner_id}`], ['当前 epoch', String(ownership.epoch)], ['Run revision', String(state.snapshot.view.context_revision)]]
      });
      if (!confirmed) return null;
      response = await api('/api/v1/ownership/takeover', {method: 'POST', body: JSON.stringify({...base, expected_owner_kind: ownership.owner_kind, expected_owner_id: ownership.owner_id, expected_epoch: ownership.epoch})});
    } else {
      if (ownership?.expired) {
        const confirmed = await confirmAction({
          kind: 'OWNERSHIP', title: '接管已过期租约', message: '上一个写入者的租约已经过期。确认其不再写入后才能取得新的 epoch。', confirmLabel: '确认接管',
          effects: [['上一所有者', `${ownership.owner_kind} / ${ownership.owner_id}`], ['上一 epoch', String(ownership.epoch)], ['过期时间', ownership.expires_at || '--']]
        });
        if (!confirmed) return null;
        base.takeover_expired = true;
      }
      response = await api('/api/v1/ownership/claim', {method: 'POST', body: JSON.stringify(base)});
    }
	state.claim = await response.json();
	state.pendingClaims[state.runID] = state.claim.token;
	persistSession();
    state.snapshot.ownership = {claimed: true, owner_kind: state.claim.owner_kind, owner_id: state.claim.owner_id, epoch: state.claim.epoch, expires_at: state.claim.expires_at, expired: false};
    renderOwnership(state.snapshot.ownership);
    return state.claim;
  }

  async function prepareAndApply(content) {
    const view = state.snapshot.view.view;
    activity('正在校验草稿并生成 Proposal');
    const response = await api('/api/v1/proposals', {method: 'POST', body: JSON.stringify({
      run_id: state.runID, claim_token: state.claim.token, owner_epoch: state.claim.epoch,
      expected_context_revision: state.snapshot.view.context_revision, typed_action: 'workspace_file.replace',
      ref: view.ref, expected_digest: state.snapshot.view.observed_digest, content
    })});
    state.proposal = await response.json();
    const effect = state.proposal.effects[0];
    const confirmed = await confirmAction({
      kind: 'PROPOSAL', title: '应用到本地工作区',
      message: '这会保存当前草稿并推进 LocalRun revision，不会提交到云端。', confirmLabel: '应用变更',
      effects: [['路径', effect.ref], ['源 digest', effect.before_digest], ['目标 digest', effect.after_digest], ['字节变化', `${effect.before_bytes} -> ${effect.after_bytes}`], ['Owner fence', `${state.proposal.owner_kind} / epoch ${state.proposal.owner_epoch}`]]
    });
    if (!confirmed) { activity('Proposal 未应用', 'idle'); return; }
    const applied = await api(`/api/v1/proposals/${encodeURIComponent(state.proposal.proposal_id)}/apply`, {method: 'POST', body: JSON.stringify({
      claim_token: state.claim.token, owner_epoch: state.claim.epoch,
      expected_context_revision: state.proposal.base_context_revision, confirm: true
    })});
    const result = await applied.json();
    state.claim.context_revision = result.context_revision;
    state.proposal = null;
    await load(state.query);
    activity('本地草稿已保存', 'idle');
  }

  function confirmAction({kind, title, message, confirmLabel, effects}) {
    elements.confirmKind.textContent = kind;
    elements.confirmTitle.textContent = title;
    elements.confirmMessage.textContent = message;
    elements.confirmAccept.textContent = confirmLabel;
    elements.confirmEffects.replaceChildren();
    for (const [label, value] of effects) {
      const row = document.createElement('div'); const term = document.createElement('dt'); const detail = document.createElement('dd');
      term.textContent = label; detail.textContent = value; row.append(term, detail); elements.confirmEffects.append(row);
    }
    elements.dialog.showModal();
    return new Promise((resolve) => elements.dialog.addEventListener('close', () => resolve(elements.dialog.returnValue === 'confirm'), {once: true}));
  }

  function commandButton(label, className) {
    const button = document.createElement('button'); button.type = 'button'; button.className = className; button.textContent = label; return button;
  }

  function media(tag, resource) {
    const node = document.createElement(tag);
    node.className = 'media';
    node.src = resource.url;
    node.setAttribute('aria-label', resource.name);
    if (tag === 'audio' || tag === 'video') node.controls = true;
    if (tag === 'img') node.alt = resource.name;
    if (tag === 'iframe') node.title = resource.name;
    return node;
  }

  function empty(message) { const node = document.createElement('p'); node.className = 'empty'; node.textContent = message; return node; }

  function renderFacts(snapshot) {
    elements.facts.replaceChildren();
    const view = snapshot.view.view;
    const facts = [['Workspace', snapshot.workspace_id], ['Project', snapshot.project_id], ['Ref', view.ref || '--'], ['MIME', view.mime_type || '--'], ['Bytes', formatBytes(view.byte_size || 0)], ['Digest', snapshot.view.observed_digest || '--'], ['Generation', snapshot.session_generation]];
    for (const [label, value] of facts) {
      const wrapper = document.createElement('div'); const term = document.createElement('dt'); const detail = document.createElement('dd');
      term.textContent = label; detail.textContent = value; if (label === 'Digest' || label === 'Generation') detail.className = 'mono';
      wrapper.append(term, detail); elements.facts.append(wrapper);
    }
  }

  function renderChecks(checks) {
    elements.checks.replaceChildren();
    for (const check of checks) { const item = document.createElement('li'); item.textContent = `${check.name}: ${check.status}${check.detail ? ` / ${check.detail}` : ''}`; elements.checks.append(item); }
  }

  async function connectEvents() {
    while (!state.closed) {
      try {
        const headers = {Authorization: `Bearer ${state.capability}`};
        if (state.lastEventID) headers['Last-Event-ID'] = String(state.lastEventID);
        const response = await fetch('/api/v1/events', {headers, cache: 'no-store'});
        if (response.status === 401) { state.closed = true; clearPersistedSession(); activity('本地会话已失效，请从 Codex 重新打开。', 'error'); return; }
        if (!response.ok || !response.body) throw new Error('事件连接失败');
        const reader = response.body.getReader(); const decoder = new TextDecoder(); let buffer = '';
        while (!state.closed) {
          const chunk = await reader.read(); if (chunk.done) break; buffer += decoder.decode(chunk.value, {stream: true});
          const frames = buffer.split('\n\n'); buffer = frames.pop() || '';
          for (const frame of frames) {
            const id = frame.match(/^id: (\d+)$/m); const data = frame.match(/^data: (.+)$/m);
            if (id) state.lastEventID = Number(id[1]);
            if (data) {
              const event = JSON.parse(data[1]);
              if (event.topic === 'session.closed') { state.closed = true; clearPersistedSession(); activity('会话已关闭', 'idle'); return; }
              if (event.topic === 'view.invalidated' || event.topic === 'event.gap') await reloadServerView();
            }
          }
        }
      } catch (error) {
        if (!state.closed) { activity(error.message, 'error'); await new Promise((resolve) => setTimeout(resolve, 1500)); }
      }
    }
  }

  function showError(error) {
    elements.app.setAttribute('aria-busy', 'false'); elements.content.replaceChildren();
    const node = document.createElement('p'); node.className = 'error'; node.textContent = error.message || String(error); elements.content.append(node); activity(node.textContent, 'error');
  }

  function formatBytes(value) {
    if (!value) return '0 B';
    const units = ['B', 'KB', 'MB', 'GB']; const index = Math.min(Math.floor(Math.log(value) / Math.log(1024)), units.length - 1);
    return `${(value / (1024 ** index)).toFixed(index ? 1 : 0)} ${units[index]}`;
  }

  document.querySelectorAll('.nav-item').forEach((button) => button.addEventListener('click', () => {
    document.querySelectorAll('.nav-item').forEach((item) => item.classList.toggle('is-active', item === button));
    load({view: button.dataset.view, ref: button.dataset.ref || ''}).catch(showError);
  }));
  elements.refresh.addEventListener('click', () => load().catch(showError));
	elements.edit.addEventListener('click', () => editCurrentView().catch(showError));
  elements.close.addEventListener('click', async () => {
    try { await api('/api/v1/session', {method: 'DELETE', headers: {'Content-Type': 'application/json', 'Idempotency-Key': idempotencyKey()}, body: '{}'}); state.closed = true; clearPersistedSession(); elements.content.replaceChildren(empty('本地 Workbench 已关闭。')); activity('会话已关闭', 'idle'); } catch (error) { showError(error); }
  });

  if ('serviceWorker' in navigator) {
    navigator.serviceWorker.addEventListener('message', (event) => {
      if (event.data?.type !== 'workbench-capability-request' || !event.ports[0]) return;
      event.ports[0].postMessage({type: 'workbench-capability-response', capability: state.capability});
    });
  }

  window.addEventListener('hashchange', () => {
    if (new URLSearchParams(location.hash.slice(1)).has('handoff')) acceptBrowserHandoff().catch(showError);
  });

  (async () => {
    try { await acceptBrowserHandoff(); } catch (error) { showError(error); }
  })();

  function idempotencyKey() { return `wbk-${crypto.randomUUID()}`; }
})();
