'use strict';

// renderer.js — MiniDOM Text (JSON) → DOM
// Exports: window.MiniRenderer = { render(snap, container, onInteract) }

window.MiniRenderer = (function() {

  function render(snap, container, onInteract) {
    container.innerHTML = '';
    if (!snap || !snap.nodes || snap.nodes.length === 0) {
      container.textContent = '(empty page)';
      return;
    }

    // Index nodes by ID for fast lookup.
    const byId = {};
    for (const n of snap.nodes) byId[n.id] = n;

    // Find root node (DOCUMENT or first node with parent_id 0).
    const root = snap.nodes.find(n => n.type === 'DOCUMENT' || n.parent_id === 0);
    if (!root) return;

    const wrapper = document.createElement('div');
    wrapper.className = 'mn-document';

    function buildNode(node) {
      if (!node) return null;
      const el = createEl(node, onInteract);
      if (!el) return null;

      // Append children.
      if (node.children && node.children.length) {
        for (const childId of node.children) {
          const child = byId[childId];
          if (!child) continue;
          const childEl = buildNode(child);
          if (childEl) el.appendChild(childEl);
        }
      }
      return el;
    }

    // Render root's children into wrapper.
    if (root.children && root.children.length) {
      for (const childId of root.children) {
        const child = byId[childId];
        if (!child) continue;
        const el = buildNode(child);
        if (el) wrapper.appendChild(el);
      }
    } else {
      // Fallback: render all non-root nodes that have parent_id === root.id.
      for (const n of snap.nodes) {
        if (n.id === root.id) continue;
        if (n.parent_id === root.id) {
          const el = buildNode(n);
          if (el) wrapper.appendChild(el);
        }
      }
    }

    container.appendChild(wrapper);
  }

  function createEl(node, onInteract) {
    let el;

    switch (node.type) {
      case 'DOCUMENT':
      case 'SECTION':
        el = document.createElement('section');
        el.className = 'mn-section';
        break;

      case 'BLOCK':
        el = document.createElement('div');
        el.className = 'mn-block';
        break;

      case 'INLINE':
        el = document.createElement('span');
        el.className = 'mn-inline';
        break;

      case 'TEXT':
        el = document.createElement('span');
        el.className = 'mn-text';
        break;

      case 'HEADING': {
        // Infer heading level from font-size if available.
        const level = guessHeadingLevel(node);
        el = document.createElement('h' + level);
        el.className = 'mn-heading';
        el.dataset.level = String(level);
        break;
      }

      case 'LINK': {
        el = document.createElement('a');
        el.className = 'mn-link';
        el.href = '#';
        const href = node.interaction && node.interaction.href;
        if (href) el.title = href;
        el.addEventListener('click', e => {
          e.preventDefault();
          if (onInteract && node.interaction) {
            onInteract({ type: 'click', element_id: node.interaction.element_id });
          }
        });
        break;
      }

      case 'IMAGE': {
        el = document.createElement('img');
        el.className = 'mn-image';
        const src = node.attrs && node.attrs.src;
        if (src) el.src = src;
        el.alt = node.text || '';
        el.loading = 'lazy';
        break;
      }

      case 'BUTTON': {
        el = document.createElement('button');
        el.className = 'mn-button';
        el.type = 'button';
        if (node.interaction && !node.interaction.enabled) el.disabled = true;
        el.addEventListener('click', () => {
          if (onInteract && node.interaction) {
            onInteract({ type: 'click', element_id: node.interaction.element_id });
          }
        });
        break;
      }

      case 'INPUT': {
        const inputType = (node.interaction && node.interaction.input_type) || 'text';
        if (inputType === 'submit' || inputType === 'button') {
          el = document.createElement('button');
          el.className = 'mn-button';
          el.type = 'button';
          el.addEventListener('click', () => {
            if (onInteract && node.interaction) {
              onInteract({ type: 'click', element_id: node.interaction.element_id });
            }
          });
        } else if (inputType === 'hidden') {
          return null;
        } else {
          el = document.createElement('input');
          el.className = 'mn-input';
          el.type = inputType;
          el.placeholder = (node.interaction && node.interaction.placeholder) || '';
          el.value = (node.interaction && node.interaction.value) || '';
          if (node.interaction && node.interaction.readonly) el.readOnly = true;
          if (node.interaction && !node.interaction.enabled) el.disabled = true;
          el.dataset.elementId = String(node.interaction ? node.interaction.element_id : '');
          el.dataset.name = (node.interaction && node.interaction.name) || '';
          el.addEventListener('change', () => {
            if (onInteract && node.interaction) {
              onInteract({ type: 'change', element_id: node.interaction.element_id, value: el.value });
            }
          });
        }
        break;
      }

      case 'TEXTAREA': {
        el = document.createElement('textarea');
        el.className = 'mn-textarea';
        el.placeholder = (node.interaction && node.interaction.placeholder) || '';
        el.value = (node.interaction && node.interaction.value) || '';
        if (node.interaction && node.interaction.readonly) el.readOnly = true;
        el.dataset.elementId = String(node.interaction ? node.interaction.element_id : '');
        el.dataset.name = (node.interaction && node.interaction.name) || '';
        el.addEventListener('change', () => {
          if (onInteract && node.interaction) {
            onInteract({ type: 'change', element_id: node.interaction.element_id, value: el.value });
          }
        });
        break;
      }

      case 'SELECT': {
        el = document.createElement('select');
        el.className = 'mn-select';
        el.dataset.elementId = String(node.interaction ? node.interaction.element_id : '');
        el.dataset.name = (node.interaction && node.interaction.name) || '';
        el.addEventListener('change', () => {
          if (onInteract && node.interaction) {
            onInteract({ type: 'change', element_id: node.interaction.element_id, value: el.value });
          }
        });
        break;
      }

      case 'OPTION': {
        el = document.createElement('option');
        el.value = node.text || '';
        break;
      }

      case 'FORM': {
        el = document.createElement('div');
        el.className = 'mn-form';
        el.dataset.formId = String(node.interaction ? node.interaction.element_id : '');
        break;
      }

      case 'TABLE':
        el = document.createElement('table');
        el.className = 'mn-table';
        break;
      case 'TABLE_ROW':
        el = document.createElement('tr');
        el.className = 'mn-table-row';
        break;
      case 'TABLE_CELL':
        el = document.createElement('td');
        el.className = 'mn-table-cell';
        break;

      case 'LIST':
        el = document.createElement('ul');
        el.className = 'mn-list';
        break;
      case 'LIST_ITEM':
        el = document.createElement('li');
        el.className = 'mn-list-item';
        break;

      case 'CANVAS_FALLBACK':
        el = document.createElement('div');
        el.className = 'mn-canvas-fallback';
        el.textContent = '[canvas]';
        break;

      default:
        el = document.createElement('div');
        el.className = 'mn-unknown';
        break;
    }

    // Set text content if the node has text and no children bring it.
    if (node.text && el) {
      // Only set textContent directly if the element won't have meaningful children
      // rendered separately. Use a text node so children can still be appended.
      if (!['INPUT', 'TEXTAREA', 'SELECT', 'IMAGE'].includes(node.type)) {
        if (node.text.trim()) {
          // Prepend a text node; children will be appended after.
          el.appendChild(document.createTextNode(node.text));
        }
      }
    }

    return el;
  }

  function guessHeadingLevel(node) {
    // Try to infer from font-size.
    if (node.style && node.style.font_size) {
      const px = parseFloat(node.style.font_size);
      if (px >= 28) return 1;
      if (px >= 22) return 2;
      if (px >= 18) return 3;
      if (px >= 16) return 4;
      return 5;
    }
    return 2;
  }

  return { render };
})();
