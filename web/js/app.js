'use strict';

// app.js — main application shell: session management, tab switcher, navigation.

(function() {

  // --- App state ---
  const state = {
    sessionID: null,
    tabs: [],      // [{tabID, url, title, snapID, snap, history, historyIdx}]
    activeTabIdx: -1,
  };

  // --- DOM refs ---
  const addressBar    = document.getElementById('address-bar');
  const btnBack       = document.getElementById('btn-back');
  const btnForward    = document.getElementById('btn-forward');
  const btnReload     = document.getElementById('btn-reload');
  const btnGo         = document.getElementById('btn-go');
  const btnSettings   = document.getElementById('btn-settings');
  const statusBar     = document.getElementById('status-bar');
  const pageContent   = document.getElementById('page-content');
  const loadingOverlay = document.getElementById('loading-overlay');
  const tabList       = document.getElementById('tab-list');
  const btnNewTab     = document.getElementById('btn-new-tab');
  const settingsPanel = document.getElementById('settings-panel');
  const btnSettingsClose = document.getElementById('btn-settings-close');
  const btnSettingsSave  = document.getElementById('btn-settings-save');
  const btnClearSession  = document.getElementById('btn-clear-session');
  const settingsStatus   = document.getElementById('settings-status');
  const btnArchive       = document.getElementById('btn-archive');
  const btnArchives      = document.getElementById('btn-archives');
  const archivesPanel    = document.getElementById('archives-panel');
  const btnArchivesClose = document.getElementById('btn-archives-close');
  const archivesList     = document.getElementById('archives-list');

  // --- Helpers ---
  function setStatus(msg, type) {
    statusBar.textContent = msg;
    statusBar.className = type || '';
  }

  function setLoading(on) {
    loadingOverlay.className = on ? 'visible' : '';
  }

  function activeTab() {
    return state.tabs[state.activeTabIdx] || null;
  }

  function normalizeURL(raw) {
    raw = (raw || '').trim();
    if (!raw) return null;
    if (!/^https?:\/\//i.test(raw)) raw = 'https://' + raw;
    try { return new URL(raw).href; } catch(e) { return raw; }
  }

  function escHtml(s) {
    return String(s)
      .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
  }

  // --- Session ---
  async function ensureSession() {
    if (state.sessionID) return;
    setStatus('Creating session…', 'loading');
    const s = Settings.get();
    const data = await MiniAPI.createSession(s.deviceProfile);
    state.sessionID = data.session_id;
    setStatus('Session ready');
  }

  async function clearSession() {
    if (state.sessionID) {
      try { await MiniAPI.deleteSession(state.sessionID); } catch(e) {}
    }
    state.sessionID = null;
    state.tabs = [];
    state.activeTabIdx = -1;
    renderTabBar();
    pageContent.innerHTML = '';
    setStatus('Session cleared. Navigate to start a new one.');
  }

  // --- Tabs ---
  function renderTabBar() {
    tabList.innerHTML = '';
    state.tabs.forEach((tab, idx) => {
      const pill = document.createElement('div');
      pill.className = 'tab-pill' + (idx === state.activeTabIdx ? ' active' : '');
      pill.dataset.idx = String(idx);
      pill.title = tab.url || '';

      if (tab.faviconURL) {
        const fav = document.createElement('img');
        fav.className = 'tab-favicon';
        fav.src = tab.faviconURL;
        fav.width = 16;
        fav.height = 16;
        fav.onerror = () => { fav.style.display = 'none'; };
        pill.appendChild(fav);
      }

      const label = document.createElement('span');
      label.className = 'tab-label';
      label.textContent = tab.title || tab.url || 'New tab';
      pill.appendChild(label);

      const closeBtn = document.createElement('button');
      closeBtn.className = 'tab-close';
      closeBtn.textContent = '×';
      closeBtn.onclick = e => { e.stopPropagation(); closeTab(idx); };
      pill.appendChild(closeBtn);

      pill.addEventListener('click', () => switchTab(idx));
      tabList.appendChild(pill);
    });
  }

  async function openNewTab(url) {
    await ensureSession();
    const data = await MiniAPI.createTab(state.sessionID, url || '');
    const tab = {
      tabID: data.tab_id,
      url: url || '',
      title: url ? '…' : 'New tab',
      faviconURL: '',
      snapID: 0,
      snap: null,
      history: url ? [url] : [],
      historyIdx: url ? 0 : -1,
    };
    state.tabs.push(tab);
    state.activeTabIdx = state.tabs.length - 1;
    renderTabBar();

    if (url) {
      await loadSnapshot();
    } else {
      pageContent.innerHTML = '<div style="padding:24px;color:#888;text-align:center">Enter a URL above to browse.</div>';
      setStatus('New tab');
    }
  }

  async function closeTab(idx) {
    const tab = state.tabs[idx];
    if (!tab) return;
    try { await MiniAPI.closeTab(state.sessionID, tab.tabID); } catch(e) {}
    state.tabs.splice(idx, 1);
    if (state.activeTabIdx >= state.tabs.length) {
      state.activeTabIdx = state.tabs.length - 1;
    }
    renderTabBar();
    if (activeTab()) {
      renderTabContent(activeTab().snap);
    } else {
      pageContent.innerHTML = '';
    }
  }

  function switchTab(idx) {
    state.activeTabIdx = idx;
    renderTabBar();
    const tab = activeTab();
    if (tab) {
      addressBar.value = tab.url || '';
      document.title = tab.title ? tab.title + ' — MiniNext' : 'MiniNext';
      renderTabContent(tab.snap);
      updateNavButtons();
    }
  }

  function updateNavButtons() {
    const tab = activeTab();
    btnBack.disabled    = !tab || tab.historyIdx <= 0;
    btnForward.disabled = !tab || tab.historyIdx >= tab.history.length - 1;
  }

  // --- Navigation ---
  async function navigate(url, opts) {
    opts = opts || {};
    await ensureSession();

    if (!activeTab()) {
      await openNewTab(url);
      return;
    }

    const tab = activeTab();
    setLoading(true);
    setStatus('Loading ' + url + '…', 'loading');
    addressBar.value = url;

    try {
      await MiniAPI.navigate(state.sessionID, tab.tabID, url);
      await loadSnapshot(opts.pushHistory !== false);
    } catch(e) {
      setStatus('Error: ' + e.message, 'error');
      pageContent.innerHTML = '<div style="padding:16px;color:#c00"><b>Error:</b> ' + escHtml(e.message) + '</div>';
    } finally {
      setLoading(false);
    }
  }

  async function loadSnapshot(pushHistory) {
    const tab = activeTab();
    if (!tab) return;

    setLoading(true);
    try {
      const snap = await MiniAPI.getSnapshot(state.sessionID, tab.tabID, tab.snapID || 0);
      if (!snap) { return; } // null means delta without base; caller should retry
      tab.snap   = snap;
      tab.snapID = snap.snapshot_id || 0;
      const newURL = snap.url || tab.url;

      if (pushHistory !== false && newURL && newURL !== tab.history[tab.historyIdx]) {
        tab.history = tab.history.slice(0, tab.historyIdx + 1);
        tab.history.push(newURL);
        tab.historyIdx = tab.history.length - 1;
      }

      tab.url        = newURL;
      tab.title      = snap.title || newURL;
      tab.faviconURL = snap.favicon_url || tab.faviconURL;
      addressBar.value = newURL;
      document.title = (snap.title || 'MiniNext') + ' — MiniNext';
      updateNavButtons();
      renderTabBar();
      renderTabContent(snap);
      setStatus(snap.title || newURL || '');
    } finally {
      setLoading(false);
    }
  }

  function renderTabContent(snap) {
    if (!snap) {
      pageContent.innerHTML = '';
      return;
    }
    // Clear find highlights before re-rendering (marks become stale after DOM replacement).
    clearFinds();
    const tab = activeTab();
    const getResourceURL = (tab && state.sessionID)
      ? resID => MiniAPI.getResource(state.sessionID, tab.tabID, resID)
      : null;
    MiniRenderer.render(snap, pageContent, onInteract, getResourceURL);
  }

  // --- Interaction ---
  async function onInteract(event) {
    const tab = activeTab();
    if (!tab || !state.sessionID) return;

    setLoading(true);
    setStatus('Interacting…', 'loading');

    try {
      const result = await MiniAPI.interact(
        state.sessionID, tab.tabID, tab.snapID, event
      );

      if (result && result.snapshot) {
        const snap = result.snapshot;
        tab.snap   = snap;
        tab.snapID = snap.snapshot_id || tab.snapID;
        const newURL = snap.url || tab.url;

        if (newURL && newURL !== tab.history[tab.historyIdx]) {
          tab.history = tab.history.slice(0, tab.historyIdx + 1);
          tab.history.push(newURL);
          tab.historyIdx = tab.history.length - 1;
        }

        tab.url        = newURL;
        tab.title      = snap.title || newURL;
        tab.faviconURL = snap.favicon_url || tab.faviconURL;
        addressBar.value = newURL;
        document.title = (snap.title || 'MiniNext') + ' — MiniNext';
        updateNavButtons();
        renderTabBar();
        renderTabContent(snap);
        setStatus(snap.title || newURL || '');
      }
    } catch(e) {
      setStatus('Error: ' + e.message, 'error');
    } finally {
      setLoading(false);
    }
  }

  // --- Settings panel ---
  function openSettings() {
    const s = Settings.get();
    document.getElementById('set-endpoint').value    = s.endpoint || '';
    document.getElementById('set-token').value       = s.authToken || '';
    document.getElementById('set-quality').value     = s.imageQuality || 'medium';
    document.getElementById('set-format').value      = s.imageFormat || 'jpeg';
    document.getElementById('set-page-format').value = s.pageFormat || 'minidom-text';
    document.getElementById('set-device').value      = s.deviceProfile || 'phone-modern';
    document.getElementById('set-rendering').value   = s.renderingProfile || 'box';
    document.getElementById('set-adblock').checked   = !!s.adBlockEnabled;
    settingsPanel.classList.remove('hidden');
    settingsStatus.textContent = '';
  }

  function saveSettings() {
    const s = Settings.get();
    s.endpoint      = document.getElementById('set-endpoint').value.trim();
    s.authToken     = document.getElementById('set-token').value.trim();
    s.imageQuality  = document.getElementById('set-quality').value;
    s.imageFormat   = document.getElementById('set-format').value;
    s.pageFormat        = document.getElementById('set-page-format').value;
    s.deviceProfile     = document.getElementById('set-device').value;
    s.renderingProfile  = document.getElementById('set-rendering').value;
    s.adBlockEnabled    = document.getElementById('set-adblock').checked;
    Settings.save(s);
    settingsStatus.textContent = 'Saved.';
    setTimeout(() => { settingsStatus.textContent = ''; }, 2000);
  }

  // --- Event wiring ---
  btnGo.addEventListener('click', () => {
    const url = normalizeURL(addressBar.value);
    if (url) navigate(url);
  });

  addressBar.addEventListener('keydown', e => {
    if (e.key === 'Enter') {
      const url = normalizeURL(addressBar.value);
      if (url) navigate(url);
    }
  });

  btnBack.addEventListener('click', () => {
    const tab = activeTab();
    if (!tab || tab.historyIdx <= 0) return;
    tab.historyIdx--;
    navigate(tab.history[tab.historyIdx], { pushHistory: false });
  });

  btnForward.addEventListener('click', () => {
    const tab = activeTab();
    if (!tab || tab.historyIdx >= tab.history.length - 1) return;
    tab.historyIdx++;
    navigate(tab.history[tab.historyIdx], { pushHistory: false });
  });

  btnReload.addEventListener('click', async () => {
    const tab = activeTab();
    const url = (tab && tab.url) || normalizeURL(addressBar.value);
    if (!url) return;
    setLoading(true);
    try {
      await MiniAPI.navigate(state.sessionID, tab.tabID, url);
      await loadSnapshot(false);
    } catch(e) {
      setStatus('Error: ' + e.message, 'error');
    } finally {
      setLoading(false);
    }
  });

  btnNewTab.addEventListener('click', () => openNewTab(''));

  btnSettings.addEventListener('click', openSettings);
  btnSettingsClose.addEventListener('click', () => settingsPanel.classList.add('hidden'));
  btnSettingsSave.addEventListener('click', saveSettings);

  // Ad block toggle: applies immediately to the live session (no save needed).
  document.getElementById('set-adblock').addEventListener('change', async (e) => {
    const enabled = e.target.checked;
    const s = Settings.get();
    s.adBlockEnabled = enabled;
    Settings.save(s);
    if (state.sessionID) {
      try {
        await MiniAPI.setAdBlock(state.sessionID, enabled);
        setStatus('Ad blocking ' + (enabled ? 'enabled' : 'disabled'));
      } catch(err) {
        setStatus('Ad block toggle failed: ' + err.message, 'error');
      }
    }
  });

  btnClearSession.addEventListener('click', async () => {
    await clearSession();
    settingsPanel.classList.add('hidden');
  });

  // --- Archive ---
  function swMessage(msg) {
    if (navigator.serviceWorker && navigator.serviceWorker.controller) {
      navigator.serviceWorker.controller.postMessage(msg);
    }
  }

  btnArchive && btnArchive.addEventListener('click', async () => {
    const tab = activeTab();
    if (!tab || !state.sessionID) {
      setStatus('No active tab to archive.', 'error');
      return;
    }
    setStatus('Saving page…', 'loading');
    try {
      const result = await MiniAPI.archivePage(state.sessionID, tab.tabID);
      setStatus('Saved: ' + (result.title || result.url));
      // Tell the service worker to pre-warm the SW cache for offline access.
      swMessage({ type: 'CACHE_ARCHIVE', archiveID: result.archive_id });
    } catch(e) {
      setStatus('Archive failed: ' + e.message, 'error');
    }
  });

  async function openArchivesPanel() {
    archivesPanel.classList.remove('hidden');
    archivesList.innerHTML = '<em>Loading…</em>';
    try {
      const items = await MiniAPI.listArchives();
      if (!items || items.length === 0) {
        archivesList.innerHTML = '<p style="color:#888">No saved pages yet.</p>';
        return;
      }
      archivesList.innerHTML = '';
      items.forEach(item => {
        const row = document.createElement('div');
        row.className = 'archive-row';

        const link = document.createElement('a');
        link.className = 'archive-link';
        link.href = '#';
        link.textContent = item.title || item.url;
        link.title = item.url;
        link.onclick = async (e) => {
          e.preventDefault();
          archivesPanel.classList.add('hidden');
          setLoading(true);
          setStatus('Opening archive…', 'loading');
          try {
            const snap = await MiniAPI.openArchive(item.id);
            // Build resource URL function: use inline data if present (for offline use),
            // otherwise fall back to the live resource proxy (requires network).
            const inlineMap = {};
            for (const res of (snap.resources || [])) {
              if (res.inline_data && res.resource_id) {
                // JSON encodes []byte as base64 string.
                inlineMap[res.resource_id] =
                  `data:${res.mime_type || 'image/jpeg'};base64,${res.inline_data}`;
              }
            }
            const getArchiveResource = (resID) => inlineMap[resID] || null;
            // Render the archived snapshot.
            pageContent.innerHTML = '';
            MiniRenderer.render(snap, pageContent, null, getArchiveResource);
            addressBar.value = item.url;
            document.title = (item.title || 'Archive') + ' — MiniNext [offline]';
            setStatus((item.title || item.url) + ' [archived]');
          } catch(err) {
            setStatus('Open failed: ' + err.message, 'error');
          } finally {
            setLoading(false);
          }
        };

        const meta = document.createElement('span');
        meta.className = 'archive-meta';
        const kb = Math.round(item.size / 1024);
        const d = new Date(item.created_at);
        meta.textContent = `${kb} KB · ${d.toLocaleDateString()}`;

        const del = document.createElement('button');
        del.className = 'archive-delete';
        del.textContent = '×';
        del.title = 'Delete this archive';
        del.onclick = async () => {
          try {
            await MiniAPI.deleteArchive(item.id);
            swMessage({ type: 'EVICT_ARCHIVE', archiveID: item.id });
            row.remove();
          } catch(e) {
            setStatus('Delete failed: ' + e.message, 'error');
          }
        };

        row.appendChild(link);
        row.appendChild(meta);
        row.appendChild(del);
        archivesList.appendChild(row);
      });
    } catch(e) {
      archivesList.innerHTML = '<p style="color:#c00">Error: ' + escHtml(e.message) + '</p>';
    }
  }

  btnArchives && btnArchives.addEventListener('click', openArchivesPanel);
  btnArchivesClose && btnArchivesClose.addEventListener('click', () => {
    archivesPanel.classList.add('hidden');
  });

  // --- Find in page ---
  const findBar    = document.getElementById('find-bar');
  const findInput  = document.getElementById('find-input');
  const findCount  = document.getElementById('find-count');
  const findPrev   = document.getElementById('find-prev');
  const findNext   = document.getElementById('find-next');
  const findClose  = document.getElementById('find-close');

  let findMatches = [];  // array of {node: TextNode, start: int, end: int} ranges
  let findCurrent = -1;  // index into findMatches
  let findMarks   = [];  // <mark> elements currently in the DOM

  function openFind() {
    findBar.classList.remove('hidden');
    findInput.focus();
    findInput.select();
    if (findInput.value) runFind(findInput.value);
  }

  function closeFind() {
    findBar.classList.add('hidden');
    clearFinds();
    findInput.classList.remove('no-match');
  }

  function clearFinds() {
    // Unwrap all <mark> elements, restoring original text nodes.
    for (const mark of findMarks) {
      const parent = mark.parentNode;
      if (!parent) continue;
      while (mark.firstChild) parent.insertBefore(mark.firstChild, mark);
      parent.removeChild(mark);
      parent.normalize();
    }
    findMarks = [];
    findMatches = [];
    findCurrent = -1;
    findCount.textContent = '';
  }

  function runFind(query) {
    clearFinds();
    if (!query) return;

    const q = query.toLowerCase();
    const walker = document.createTreeWalker(
      pageContent,
      NodeFilter.SHOW_TEXT,
      { acceptNode: n => n.textContent.trim() ? NodeFilter.FILTER_ACCEPT : NodeFilter.FILTER_REJECT }
    );

    const ranges = [];
    let node;
    while ((node = walker.nextNode())) {
      const text = node.textContent.toLowerCase();
      let pos = 0;
      while ((pos = text.indexOf(q, pos)) !== -1) {
        ranges.push({ node, start: pos, end: pos + q.length });
        pos += q.length;
      }
    }

    if (ranges.length === 0) {
      findCount.textContent = '0 matches';
      findInput.classList.add('no-match');
      return;
    }

    findInput.classList.remove('no-match');

    // Wrap matches in <mark> elements (process in reverse within each text node
    // to preserve offsets).
    // Group by text node.
    const byNode = new Map();
    for (const r of ranges) {
      if (!byNode.has(r.node)) byNode.set(r.node, []);
      byNode.get(r.node).push(r);
    }

    for (const [textNode, nodeRanges] of byNode) {
      // Process ranges in reverse so splitting doesn't affect earlier offsets.
      nodeRanges.sort((a, b) => b.start - a.start);
      let current = textNode;
      for (const r of nodeRanges) {
        const after  = current.splitText(r.end);
        const mid    = current.splitText(r.start);
        const mark   = document.createElement('mark');
        mark.className = 'find-highlight';
        mid.parentNode.insertBefore(mark, after);
        mark.appendChild(mid);
        findMarks.push(mark);
        current = after;
      }
    }

    // findMarks is in reverse-per-node order; sort by DOM position.
    findMarks.sort((a, b) => {
      const pos = a.compareDocumentPosition(b);
      return pos & Node.DOCUMENT_POSITION_FOLLOWING ? -1 : 1;
    });

    findMatches = findMarks;
    findCurrent = 0;
    scrollToMatch(0);
    updateFindCount();
  }

  function scrollToMatch(idx) {
    for (const m of findMarks) m.classList.remove('find-current');
    if (idx < 0 || idx >= findMarks.length) return;
    findMarks[idx].classList.add('find-current');
    findMarks[idx].scrollIntoView({ block: 'center', behavior: 'smooth' });
  }

  function updateFindCount() {
    if (findMarks.length === 0) {
      findCount.textContent = '0 matches';
    } else {
      findCount.textContent = `${findCurrent + 1}/${findMarks.length}`;
    }
  }

  findInput.addEventListener('input', () => {
    runFind(findInput.value);
  });
  findInput.addEventListener('keydown', e => {
    if (e.key === 'Enter') {
      if (e.shiftKey) {
        findCurrent = (findCurrent - 1 + findMarks.length) % findMarks.length;
      } else {
        findCurrent = (findCurrent + 1) % findMarks.length;
      }
      scrollToMatch(findCurrent);
      updateFindCount();
    } else if (e.key === 'Escape') {
      closeFind();
    }
  });
  findNext.addEventListener('click', () => {
    if (!findMarks.length) return;
    findCurrent = (findCurrent + 1) % findMarks.length;
    scrollToMatch(findCurrent);
    updateFindCount();
  });
  findPrev.addEventListener('click', () => {
    if (!findMarks.length) return;
    findCurrent = (findCurrent - 1 + findMarks.length) % findMarks.length;
    scrollToMatch(findCurrent);
    updateFindCount();
  });
  findClose.addEventListener('click', closeFind);

  // Ctrl+F / Cmd+F opens find bar.
  document.addEventListener('keydown', e => {
    if ((e.ctrlKey || e.metaKey) && e.key === 'f') {
      e.preventDefault();
      if (findBar.classList.contains('hidden')) {
        openFind();
      } else {
        findInput.focus();
        findInput.select();
      }
    }
  });

  // --- Init ---
  updateNavButtons();
  setStatus('Ready — enter a URL to begin');

})();
