'use strict';

// api.js — Promise-based wrappers for all MiniNext REST endpoints.

window.MiniAPI = (function() {

  async function request(method, path, body, binary) {
    const base = Settings.apiBase();
    const headers = Settings.authHeaders();
    if (binary) {
      delete headers['Content-Type'];
      headers['Accept'] = 'application/x-mbpf';
    }

    const opts = {
      method,
      headers,
    };
    if (body !== undefined) {
      opts.body = JSON.stringify(body);
    }

    const res = await fetch(base + path, opts);
    if (!res.ok) {
      let msg = res.statusText;
      try { const j = await res.json(); msg = j.error || msg; } catch(e) {}
      throw new Error(msg);
    }

    if (binary) return res.arrayBuffer();
    return res.json();
  }

  function createSession(deviceProfile) {
    const s = Settings.get();
    return request('POST', '/api/v1/sessions', {
      device_profile: deviceProfile || 'phone-modern',
      capabilities: {
        page_formats: ['minidom-text', 'mbpf'],
        compression: ['gzip', 'brotli', 'zstd'],
        image_formats: ['jpeg', 'webp', 'png', 'gif'],
        rendering_profiles: ['box', 'flow'],
        adblock: s.adBlockEnabled !== false, // default true
      }
    });
  }

  function deleteSession(sessionID) {
    return request('DELETE', `/api/v1/sessions/${sessionID}`, undefined);
  }

  function sleepSession(sessionID) {
    return request('POST', `/api/v1/sessions/${sessionID}/sleep`, {});
  }

  function resumeSession(sessionID) {
    return request('POST', `/api/v1/sessions/${sessionID}/resume`, {});
  }

  function setAdBlock(sessionID, enabled) {
    return request('POST', `/api/v1/sessions/${sessionID}/adblock`, { enabled });
  }

  function createTab(sessionID, url) {
    return request('POST', `/api/v1/sessions/${sessionID}/tabs`, { url });
  }

  function closeTab(sessionID, tabID) {
    return request('DELETE', `/api/v1/sessions/${sessionID}/tabs/${tabID}`, undefined);
  }

  function navigate(sessionID, tabID, url) {
    return request('POST', `/api/v1/sessions/${sessionID}/tabs/${tabID}/navigate`, { url });
  }

  // navigateAsync sends an async navigate (returns 202 immediately).
  // Completion is delivered via the tab's SSE stream.
  function navigateAsync(sessionID, tabID, url) {
    return request('POST', `/api/v1/sessions/${sessionID}/tabs/${tabID}/navigate`, { url, async: true });
  }

  // subscribeTabEvents opens an SSE connection for the given tab.
  // onEvent is called with each parsed event object.
  // Returns an object with a close() method.
  function subscribeTabEvents(sessionID, tabID, onEvent) {
    const base = Settings.apiBase();
    const s = Settings.get();
    let url = `${base}/api/v1/sessions/${sessionID}/tabs/${tabID}/events`;
    if (s.authToken) url += '?token=' + encodeURIComponent(s.authToken);

    const es = new EventSource(url);
    es.onmessage = function(e) {
      try { onEvent(JSON.parse(e.data)); } catch(err) {}
    };
    return { close: function() { es.close(); } };
  }

  // _lastSnaps caches {[sessionID+tabID]: snapshot} for delta application.
  const _lastSnaps = {};

  function _snapKey(sessionID, tabID) { return sessionID + '/' + tabID; }

  function _applyDelta(baseSnap, delta) {
    const nodeMap = new Map();
    for (const n of (baseSnap.nodes || [])) {
      if (n.stable_id) nodeMap.set(n.stable_id, Object.assign({}, n));
    }
    for (const inst of (delta.instructions || [])) {
      switch (inst.op) {
        case 1: case 3: // ADD or UPDATE
          if (inst.node) nodeMap.set(inst.stable_id, inst.node);
          break;
        case 2: // REMOVE
          nodeMap.delete(inst.stable_id);
          break;
      }
    }
    // Rebuild ordered list: base order first, then added nodes.
    const seen = new Set();
    const nodes = [];
    for (const bn of (baseSnap.nodes || [])) {
      if (!bn.stable_id) continue;
      const n = nodeMap.get(bn.stable_id);
      if (n) { nodes.push(n); seen.add(bn.stable_id); }
    }
    for (const inst of (delta.instructions || [])) {
      if (inst.op === 1 && !seen.has(inst.stable_id) && inst.node) {
        nodes.push(inst.node);
      }
    }
    return {
      format: baseSnap.format,
      version: baseSnap.version,
      snapshot_id: delta.snapshot_id,
      url: delta.url || baseSnap.url,
      title: delta.title || baseSnap.title,
      favicon_url: delta.favicon_url || baseSnap.favicon_url,
      nodes,
      resources: baseSnap.resources,
    };
  }

  async function getSnapshot(sessionID, tabID, lastSnapID) {
    const s = Settings.get();
    const base = Settings.apiBase();
    const headers = Settings.authHeaders();

    const useMBPF = s.pageFormat === 'mbpf';
    if (useMBPF) {
      headers['Accept'] = 'application/x-mbpf';
    } else {
      headers['Accept'] = 'application/minidom+json, application/minidom-delta+json';
    }
    headers['Accept-Encoding'] = 'gzip, br';

    const rendering = s.renderingProfile || 'box';
    const key = _snapKey(sessionID, tabID);
    const cachedSnap = _lastSnaps[key];

    // Build URL with optional since= for delta requests.
    let url = `${base}/api/v1/sessions/${sessionID}/tabs/${tabID}/snapshot?rendering=${rendering}`;
    if (lastSnapID && cachedSnap && cachedSnap.snapshot_id === lastSnapID) {
      url += `&since=${lastSnapID}`;
    }

    const res = await fetch(url, { headers });
    if (!res.ok) {
      let msg = res.statusText;
      try { const j = await res.json(); msg = j.error || msg; } catch(e) {}
      throw new Error(msg);
    }

    const snapshotID = parseInt(res.headers.get('X-Snapshot-Id') || '0', 10);
    const ct = res.headers.get('Content-Type') || '';

    let snap;
    if (ct.includes('application/minidom-delta+json')) {
      const delta = await res.json();
      if (cachedSnap) {
        snap = _applyDelta(cachedSnap, delta);
      } else {
        // Delta without base — fall back to full snapshot on next call.
        snap = null;
      }
    } else if (useMBPF) {
      const buf = await res.arrayBuffer();
      snap = MBPFDecoder.decode(buf);
    } else {
      snap = await res.json();
    }

    if (snap) {
      snap.snapshot_id = snapshotID;
      _lastSnaps[key] = snap;
    }
    return snap;
  }

  function interact(sessionID, tabID, snapshotID, event) {
    const s = Settings.get();
    return request('POST',
      `/api/v1/sessions/${sessionID}/tabs/${tabID}/interact`,
      { snapshot_id: snapshotID, rendering_profile: s.renderingProfile || 'box', event }
    );
  }

  function getResource(sessionID, tabID, resourceID) {
    const base = Settings.apiBase();
    return `${base}/api/v1/sessions/${sessionID}/tabs/${tabID}/resources/${resourceID}`;
  }

  function archivePage(sessionID, tabID) {
    return request('POST', `/api/v1/sessions/${sessionID}/tabs/${tabID}/archive`, {});
  }

  function listArchives() {
    return request('GET', '/api/v1/archives', undefined);
  }

  function deleteArchive(archiveID) {
    return request('DELETE', `/api/v1/archives/${archiveID}`, undefined);
  }

  async function openArchive(archiveID) {
    const base = Settings.apiBase();
    const headers = Settings.authHeaders();
    headers['Accept'] = 'application/minidom+json';
    headers['Accept-Encoding'] = 'gzip, br';
    const res = await fetch(`${base}/api/v1/archives/${archiveID}`, { headers });
    if (!res.ok) {
      let msg = res.statusText;
      try { const j = await res.json(); msg = j.error || msg; } catch(e) {}
      throw new Error(msg);
    }
    return res.json();
  }

  return {
    createSession,
    deleteSession,
    sleepSession,
    resumeSession,
    setAdBlock,
    createTab,
    closeTab,
    navigate,
    navigateAsync,
    subscribeTabEvents,
    getSnapshot,
    interact,
    getResource,
    archivePage,
    listArchives,
    deleteArchive,
    openArchive,
  };
})();
