'use strict';

// mbpf.js — MBPF binary decoder
// Exports: window.MBPFDecoder = { decode(ArrayBuffer) }

window.MBPFDecoder = (function() {

  const SECTION_STRING_TABLE  = 1;
  const SECTION_PAGE_META     = 2;
  const SECTION_NODE_TREE     = 4;
  const SECTION_LAYOUT_TABLE  = 5;
  const SECTION_INTERACTION   = 6;
  const SECTION_RESOURCE_TABLE = 7;

  const NODE_TYPES = [
    '', 'DOCUMENT', 'SECTION', 'BLOCK', 'INLINE', 'TEXT',
    'LINK', 'IMAGE', 'BUTTON', 'INPUT', 'TEXTAREA', 'SELECT',
    'OPTION', 'FORM', 'TABLE', 'TABLE_ROW', 'TABLE_CELL',
    'LIST', 'LIST_ITEM', 'HEADING', 'CANVAS_FALLBACK', 'UNKNOWN'
  ];

  const KIND_NAMES = ['', 'link', 'button', 'input', 'textarea', 'select', 'form', 'custom'];

  class Reader {
    constructor(buf) {
      this.view = new DataView(buf);
      this.pos = 0;
    }

    readByte() {
      return this.view.getUint8(this.pos++);
    }

    readVarint() {
      let v = 0n, shift = 0n;
      for (;;) {
        const b = this.readByte();
        v |= BigInt(b & 0x7F) << shift;
        if ((b & 0x80) === 0) return Number(v);
        shift += 7n;
        if (shift >= 64n) throw new Error('varint overflow');
      }
    }

    readString() {
      const len = this.readVarint();
      const bytes = new Uint8Array(this.view.buffer, this.pos, len);
      this.pos += len;
      return new TextDecoder().decode(bytes);
    }

    readBytes(n) {
      const bytes = new Uint8Array(this.view.buffer, this.pos, n);
      this.pos += n;
      return bytes.slice();
    }
  }

  function decode(arrayBuffer) {
    const r = new Reader(arrayBuffer);

    // Check magic "MBPF"
    const magic = String.fromCharCode(
      r.readByte(), r.readByte(), r.readByte(), r.readByte()
    );
    if (magic !== 'MBPF') throw new Error('Invalid MBPF magic');

    r.readVarint(); // version
    r.readVarint(); // flags
    r.readVarint(); // page_id
    const snapshotID = r.readVarint();
    r.readVarint(); // profile_id
    const sectionCount = r.readVarint();

    const sections = {};
    for (let i = 0; i < sectionCount; i++) {
      const typeID = r.readVarint();
      const length = r.readVarint();
      const start = r.pos;
      sections[typeID] = r.view.buffer.slice(start, start + length);
      r.pos += length;
    }

    const strings = sections[SECTION_STRING_TABLE]
      ? decodeStringTable(sections[SECTION_STRING_TABLE]) : [];

    const str = idx => (idx < strings.length ? strings[idx] : '');

    const layouts = sections[SECTION_LAYOUT_TABLE]
      ? decodeLayoutTable(sections[SECTION_LAYOUT_TABLE]) : [];

    const interactions = sections[SECTION_INTERACTION]
      ? decodeInteractionTable(sections[SECTION_INTERACTION], str) : [];

    const resources = sections[SECTION_RESOURCE_TABLE]
      ? decodeResourceTable(sections[SECTION_RESOURCE_TABLE], str) : [];

    const nodes = sections[SECTION_NODE_TREE]
      ? decodeNodeTree(sections[SECTION_NODE_TREE], str, layouts, interactions) : [];

    let url = '', title = '', faviconURL = '';
    if (sections[SECTION_PAGE_META]) {
      const pm = decodePageMeta(sections[SECTION_PAGE_META], str);
      url = pm.url; title = pm.title; faviconURL = pm.favicon_url;
    }

    return {
      format: 'minidom-text', // expose same shape as JSON decoder
      version: 1,
      snapshot_id: snapshotID,
      url,
      title,
      favicon_url: faviconURL,
      nodes,
      resources,
    };
  }

  function decodePageMeta(buf, str) {
    const r = new Reader(buf);
    return {
      url:        str(r.readVarint()),
      title:      str(r.readVarint()),
      favicon_url: str(r.readVarint()),
    };
  }

  function decodeStringTable(buf) {
    const r = new Reader(buf);
    const count = r.readVarint();
    const strs = [];
    for (let i = 0; i < count; i++) {
      strs.push(r.readString());
    }
    return strs;
  }

  function decodeLayoutTable(buf) {
    const r = new Reader(buf);
    const count = r.readVarint();
    const layouts = [];
    for (let i = 0; i < count; i++) {
      layouts.push({
        x: r.readVarint() / 10,
        y: r.readVarint() / 10,
        w: r.readVarint() / 10,
        h: r.readVarint() / 10,
      });
    }
    return layouts;
  }

  function decodeInteractionTable(buf, str) {
    const r = new Reader(buf);
    const count = r.readVarint();
    const items = [];
    for (let i = 0; i < count; i++) {
      const elementID = r.readVarint();
      const kindID    = r.readVarint();
      const flags     = r.readVarint();
      const hrefIdx   = r.readVarint();
      const valueIdx  = r.readVarint();
      const placeholderIdx = r.readVarint();
      const formID    = r.readVarint();
      const inputTypeIdx = r.readVarint();
      const nameIdx   = r.readVarint();
      items.push({
        element_id:  elementID,
        kind:        KIND_NAMES[kindID] || 'custom',
        enabled:     (flags & 1) !== 0,
        readonly:    (flags & 2) !== 0,
        href:        str(hrefIdx),
        value:       str(valueIdx),
        placeholder: str(placeholderIdx),
        form_id:     formID,
        input_type:  str(inputTypeIdx),
        name:        str(nameIdx),
      });
    }
    return items;
  }

  function decodeResourceTable(buf, str) {
    const r = new Reader(buf);
    const count = r.readVarint();
    const items = [];
    for (let i = 0; i < count; i++) {
      const resourceID = str(r.readVarint());
      const url        = str(r.readVarint());
      const mimeType   = str(r.readVarint());
      const width      = r.readVarint();
      const height     = r.readVarint();
      const hasInline  = r.readVarint();
      let inlineData   = null;
      if (hasInline) {
        const len = r.readVarint();
        inlineData = r.readBytes(len);
      }
      items.push({ resource_id: resourceID, url, mime_type: mimeType, width, height, inline_data: inlineData });
    }
    return items;
  }

  function decodeNodeTree(buf, str, layouts, interactions) {
    const r = new Reader(buf);
    const count = r.readVarint();
    const nodes = [];
    const idxByID = {};

    for (let i = 0; i < count; i++) {
      const id        = r.readVarint();
      const typeID    = r.readVarint();
      const flags     = r.readVarint();
      const parentID  = r.readVarint();
      const textIdx   = r.readVarint();
      const stableIdx = r.readVarint();

      const node = {
        id,
        stable_id: str(stableIdx),
        type:      NODE_TYPES[typeID] || 'UNKNOWN',
        parent_id: parentID,
        text:      str(textIdx),
        children:  [],
      };

      if (flags & 1) {
        const layoutIdx = r.readVarint();
        if (layoutIdx < layouts.length) node.layout = layouts[layoutIdx];
      }
      if (flags & 4) {
        node.resource_id = str(r.readVarint());
      }
      if (flags & 2) {
        const interIdx = r.readVarint();
        if (interIdx < interactions.length) node.interaction = interactions[interIdx];
      }

      nodes.push(node);
      idxByID[id] = i;
    }

    // Rebuild children arrays.
    for (const n of nodes) {
      if (n.parent_id && idxByID[n.parent_id] !== undefined) {
        nodes[idxByID[n.parent_id]].children.push(n.id);
      }
    }

    return nodes;
  }

  return { decode };
})();
