package chromedp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/chromedp/chromedp"
	"github.com/google/uuid"
	"github.com/user/miniweb/internal/minidom"
)

// jsFavicon extracts the page's best favicon URL.
const jsFavicon = `(function() {
  var icons = document.querySelectorAll('link[rel~="icon"], link[rel~="shortcut"]');
  var best = '';
  var bestSize = 0;
  icons.forEach(function(l) {
    var href = l.href || l.getAttribute('href') || '';
    if (!href) return;
    var sizes = l.getAttribute('sizes') || '';
    var sz = 0;
    var m = sizes.match(/(\d+)/);
    if (m) sz = parseInt(m[1], 10);
    if (sz > bestSize || !best) { best = href; bestSize = sz; }
  });
  if (!best) {
    // fallback: try /favicon.ico relative to origin
    best = window.location.origin + '/favicon.ico';
  }
  return best;
})()`

// jsFlowExtractor linearizes page content into a flat reading-order list.
// It emits only semantic content nodes: document root, headings, paragraphs,
// links, images, list items, and blockquotes — stripping layout/chrome noise.
const jsFlowExtractor = `(function() {
try {
  var interactionCounter = 1;
  var nodeCounter = 1;
  var nodes = [];
  var siblingCounters = {};` + jsStableIDFn + `
  function getLayout(el) {
    try {
      var r = el.getBoundingClientRect();
      return {x: Math.round(r.left), y: Math.round(r.top + window.scrollY),
              w: Math.round(r.width), h: Math.round(r.height)};
    } catch(e) { return null; }
  }

  function isVisible(el) {
    try {
      var cs = window.getComputedStyle(el);
      return cs.display !== 'none' && cs.visibility !== 'hidden' && cs.opacity !== '0';
    } catch(e) { return false; }
  }

  function collectText(el) {
    return (el.textContent || '').replace(/\s+/g, ' ').trim().slice(0, 512);
  }

  var rootSID = stableID('DOCUMENT', '', 1);
  var root = {id: nodeCounter++, stable_id: rootSID, type: 'DOCUMENT', parent_id: 0};
  nodes.push(root);

  function makeSID(type) {
    var key = rootSID + ':' + type;
    siblingCounters[key] = (siblingCounters[key] || 0) + 1;
    return stableID(type, rootSID, siblingCounters[key]);
  }

  function walk(el, depth) {
    if (!el || el.nodeType !== 1) return;
    if (!isVisible(el)) return;
    if (depth > 20) return;

    var tag = (el.tagName || '').toLowerCase();
    var type = null;
    var node = null;

    if (/^h[1-6]$/.test(tag)) {
      type = 'HEADING';
      var text = collectText(el);
      if (!text) return;
      node = {id: nodeCounter++, stable_id: makeSID(type), type: type, parent_id: root.id,
              text: text, layout: getLayout(el)};
      var px = parseFloat(window.getComputedStyle(el).fontSize) || 16;
      node.style = {font_size: px + 'px', font_weight: '700'};

    } else if (tag === 'p' || tag === 'blockquote') {
      var text = collectText(el);
      if (!text) return;
      node = {id: nodeCounter++, stable_id: makeSID('BLOCK'), type: 'BLOCK', parent_id: root.id,
              text: text, layout: getLayout(el)};

    } else if (tag === 'img') {
      var src = el.src || el.getAttribute('src') || el.currentSrc || '';
      if (!src) return;
      var attrs = {src: src};
      var w = el.naturalWidth || el.width;
      var h = el.naturalHeight || el.height;
      if (w) attrs.width = String(w);
      if (h) attrs.height = String(h);
      node = {id: nodeCounter++, stable_id: makeSID('IMAGE'), type: 'IMAGE', parent_id: root.id,
              text: el.alt || '', layout: getLayout(el), attrs: attrs};

    } else if (tag === 'a') {
      var text = collectText(el);
      if (!text) return;
      var href = el.href || el.getAttribute('href') || '';
      node = {id: nodeCounter++, stable_id: makeSID('LINK'), type: 'LINK', parent_id: root.id,
              text: text, layout: getLayout(el),
              interaction: {element_id: interactionCounter++, kind: 'link',
                            enabled: true, href: href, action_hint: 'click'}};

    } else if (tag === 'li') {
      var text = collectText(el);
      if (!text) return;
      node = {id: nodeCounter++, stable_id: makeSID('LIST_ITEM'), type: 'LIST_ITEM', parent_id: root.id,
              text: text, layout: getLayout(el)};

    } else if (tag === 'input' || tag === 'textarea' || tag === 'select' || tag === 'button') {
      // Keep interactive elements in flow mode.
      var itype = el.type || '';
      if (itype === 'hidden') return;
      var ntype = tag === 'input' ? 'INPUT'
               : tag === 'textarea' ? 'TEXTAREA'
               : tag === 'select' ? 'SELECT' : 'BUTTON';
      var kind = ntype.toLowerCase();
      var hint = (tag === 'button' || itype === 'submit') ? 'click' : 'type';
      node = {id: nodeCounter++, stable_id: makeSID(ntype), type: ntype, parent_id: root.id,
              text: collectText(el), layout: getLayout(el),
              interaction: {element_id: interactionCounter++, kind: kind,
                            enabled: !el.disabled, readonly: !!el.readOnly,
                            value: el.value || '', placeholder: el.placeholder || '',
                            action_hint: hint, input_type: itype, name: el.name || ''}};
    }

    if (node) {
      nodes.push(node);
    } else {
      // Recurse into containers.
      el.childNodes.forEach(function(c) { walk(c, depth + 1); });
      return;
    }

    // Don't recurse into content nodes (we already grabbed their text).
    if (type !== 'HEADING' && tag !== 'p' && tag !== 'blockquote' && tag !== 'li') {
      el.childNodes.forEach(function(c) { walk(c, depth + 1); });
    }
  }

  document.body && document.body.childNodes.forEach(function(c) { walk(c, 0); });

  return JSON.stringify({
    title: document.title || '',
    url: window.location.href || '',
    nodes: nodes
  });
} catch(e) {
  return JSON.stringify({title:'', url: window.location.href || '', nodes:[], error: String(e)});
}
})()`

// jsStableID is a shared JS snippet that provides the djb2-based stableID function.
// stableID(nodeType, parentStableID, siblingIndex) → 8-char hex string.
const jsStableIDFn = `
  function djb2(s) {
    var h = 5381;
    for (var i = 0; i < s.length; i++) {
      h = (((h << 5) + h) ^ s.charCodeAt(i)) >>> 0;
    }
    return ('00000000' + h.toString(16)).slice(-8);
  }
  function stableID(type, parentSID, sibIdx) {
    return djb2(type + ':' + parentSID + ':' + sibIdx);
  }
`

// jsExtractor is injected into the page and walks the live DOM post-render.
// It returns a JSON object with {title, url, nodes} where each node has
// id, type, parentId, text, attrs, layout, style, and interaction fields.
const jsExtractor = `(function() {
try {
  var interactionCounter = 1;
  var nodeCounter = 1;
  var nodes = [];
  var nodeMap = new Map(); // element -> nodeId
  var stableMap = new Map(); // element -> stableID` + jsStableIDFn + `
  function classifyTag(el) {
    var tag = el.tagName ? el.tagName.toLowerCase() : '';
    if (/^h[1-6]$/.test(tag)) return 'HEADING';
    switch (tag) {
      case 'a':        return 'LINK';
      case 'img':      return 'IMAGE';
      case 'button':   return 'BUTTON';
      case 'input':    return 'INPUT';
      case 'textarea': return 'TEXTAREA';
      case 'select':   return 'SELECT';
      case 'option':   return 'OPTION';
      case 'form':     return 'FORM';
      case 'table':    return 'TABLE';
      case 'tr':       return 'TABLE_ROW';
      case 'td': case 'th': return 'TABLE_CELL';
      case 'ul': case 'ol': return 'LIST';
      case 'li':       return 'LIST_ITEM';
      case 'canvas':   return 'CANVAS_FALLBACK';
      case 'span': case 'em': case 'strong': case 'b': case 'i':
      case 'code': case 'small': case 'label': return 'INLINE';
      case 'p': case 'div': case 'section': case 'article':
      case 'main': case 'header': case 'footer': case 'nav':
      case 'aside': case 'figure': case 'figcaption':
      case 'blockquote': case 'pre': return 'BLOCK';
      case 'body': case 'html': return 'SECTION';
      default: return null; // skip unknown non-semantic tags
    }
  }

  function isVisible(el) {
    try {
      var cs = window.getComputedStyle(el);
      if (cs.display === 'none' || cs.visibility === 'hidden' || cs.opacity === '0') return false;
      var r = el.getBoundingClientRect();
      // include elements with zero size only if they contain text or are interactive
      return true;
    } catch(e) { return false; }
  }

  function getLayout(el) {
    try {
      var r = el.getBoundingClientRect();
      var scrollX = window.scrollX || 0;
      var scrollY = window.scrollY || 0;
      return {x: Math.round(r.left + scrollX), y: Math.round(r.top + scrollY),
              w: Math.round(r.width), h: Math.round(r.height)};
    } catch(e) { return null; }
  }

  function getStyle(el) {
    try {
      var cs = window.getComputedStyle(el);
      return {
        color: cs.color || '',
        bg_color: cs.backgroundColor || '',
        font_size: cs.fontSize || '',
        font_weight: cs.fontWeight || '',
        display: cs.display || ''
      };
    } catch(e) { return null; }
  }

  function getInteraction(el, nodeType) {
    var tag = el.tagName ? el.tagName.toLowerCase() : '';
    var isMeta = {kind:'', hint:''};

    if (nodeType === 'LINK') {
      isMeta = {kind:'link', hint:'click'};
    } else if (nodeType === 'BUTTON' || (tag === 'input' && el.type === 'submit')) {
      isMeta = {kind:'button', hint:'click'};
    } else if (nodeType === 'INPUT') {
      isMeta = {kind:'input', hint:'type'};
    } else if (nodeType === 'TEXTAREA') {
      isMeta = {kind:'textarea', hint:'type'};
    } else if (nodeType === 'SELECT') {
      isMeta = {kind:'select', hint:'change'};
    } else if (nodeType === 'FORM') {
      isMeta = {kind:'form', hint:'submit'};
    } else {
      return null;
    }

    var formId = 0;
    if (el.form) {
      formId = nodeMap.get(el.form) || 0;
    }

    var meta = {
      element_id: interactionCounter++,
      kind: isMeta.kind,
      enabled: !el.disabled,
      readonly: !!el.readOnly,
      action_hint: isMeta.hint,
      form_id: formId,
      input_type: el.type || '',
      name: el.name || ''
    };
    if (nodeType === 'LINK') meta.href = el.href || el.getAttribute('href') || '';
    if (nodeType === 'INPUT' || nodeType === 'TEXTAREA' || nodeType === 'SELECT') {
      meta.value = el.value || '';
      meta.placeholder = el.placeholder || '';
    }
    return meta;
  }

  function getText(el, nodeType) {
    if (nodeType === 'IMAGE') return el.alt || '';
    // For block/inline containers, only grab direct text content to avoid duplication
    var text = '';
    el.childNodes.forEach(function(child) {
      if (child.nodeType === 3) { // TEXT_NODE
        var t = child.textContent.trim();
        if (t) text += (text ? ' ' : '') + t;
      }
    });
    return text;
  }

  function getAttrs(el, nodeType) {
    var attrs = {};
    if (nodeType === 'IMAGE') {
      var src = el.src || el.getAttribute('src') || el.currentSrc || '';
      if (src) attrs.src = src;
      var w = el.naturalWidth || el.width;
      var h = el.naturalHeight || el.height;
      if (w) attrs.width = String(w);
      if (h) attrs.height = String(h);
    }
    if (nodeType === 'INPUT') {
      attrs.type = el.type || 'text';
    }
    if (nodeType === 'FORM') {
      attrs.action = el.action || '';
      attrs.method = el.method || 'get';
    }
    return Object.keys(attrs).length ? attrs : null;
  }

  // siblingCounters tracks how many children of each nodeType a parent has seen.
  // Key: parentStableID + ":" + nodeType → count
  var siblingCounters = {};

  function processElement(el, parentId, parentSID) {
    if (el.nodeType !== 1) return; // only Element nodes
    if (!isVisible(el)) return;

    var nodeType = classifyTag(el);
    if (!nodeType) return;

    var scKey = parentSID + ':' + nodeType;
    siblingCounters[scKey] = (siblingCounters[scKey] || 0) + 1;
    var sid = stableID(nodeType, parentSID, siblingCounters[scKey]);

    var id = nodeCounter++;
    nodeMap.set(el, id);
    stableMap.set(el, sid);

    var node = {
      id: id,
      stable_id: sid,
      type: nodeType,
      parent_id: parentId,
      text: getText(el, nodeType),
      layout: getLayout(el),
      style: getStyle(el)
    };

    var interaction = getInteraction(el, nodeType);
    if (interaction) node.interaction = interaction;

    var attrs = getAttrs(el, nodeType);
    if (attrs) node.attrs = attrs;

    nodes.push(node);

    // Recurse into children (skip image/input/textarea internals)
    if (nodeType !== 'IMAGE' && nodeType !== 'INPUT' && nodeType !== 'TEXTAREA') {
      el.childNodes.forEach(function(child) {
        if (child.nodeType === 1) processElement(child, id, sid);
      });
    }
  }

  // Start from document root
  var rootSID = stableID('DOCUMENT', '', 1);
  var root = {id: nodeCounter++, stable_id: rootSID, type: 'DOCUMENT', parent_id: 0, children: []};
  nodes.push(root);
  nodeMap.set(document.documentElement, root.id);
  stableMap.set(document.documentElement, rootSID);

  document.body && document.body.childNodes.forEach(function(child) {
    if (child.nodeType === 1) processElement(child, root.id, rootSID);
  });

  return JSON.stringify({
    title: document.title || '',
    url: window.location.href || '',
    nodes: nodes
  });
} catch(e) {
  return JSON.stringify({title:'', url: window.location.href || '', nodes:[], error: String(e)});
}
})()`

// rawNode is the raw JSON structure returned by the JS extractor.
type rawNode struct {
	ID          int                `json:"id"`
	StableID    string             `json:"stable_id"`
	Type        string             `json:"type"`
	ParentID    int                `json:"parent_id"`
	Text        string             `json:"text"`
	Layout      *minidom.LayoutBox `json:"layout"`
	Style       *minidom.StyleSubset `json:"style"`
	Interaction *minidom.InteractionMeta `json:"interaction"`
	Attrs       map[string]string  `json:"attrs"`
}

type extractResult struct {
	Title string    `json:"title"`
	URL   string    `json:"url"`
	Nodes []rawNode `json:"nodes"`
	Error string    `json:"error"`
}

// ExtractPage navigates to url using the provided chromedp context and
// returns a MiniDOM PageSnapshot by injecting the JS extractor.
func ExtractPage(ctx context.Context, url string) (*minidom.PageSnapshot, error) {
	if err := chromedp.Run(ctx,
		chromedp.Navigate(url),
		chromedp.WaitReady("body", chromedp.ByQuery),
	); err != nil {
		return nil, fmt.Errorf("navigate %s: %w", url, err)
	}
	return extractCurrent(ctx)
}

// ExtractCurrent runs the JS extractor on whatever page is currently loaded.
func ExtractCurrent(ctx context.Context) (*minidom.PageSnapshot, error) {
	return extractCurrent(ctx)
}

// ExtractCurrentFlow runs the flow-mode extractor (linearized reading order).
func ExtractCurrentFlow(ctx context.Context) (*minidom.PageSnapshot, error) {
	return extractCurrentWithJS(ctx, jsFlowExtractor)
}

func extractCurrent(ctx context.Context) (*minidom.PageSnapshot, error) {
	return extractCurrentWithJS(ctx, jsExtractor)
}

// extractCurrentWithJS runs the given JS extractor string on the current page.
func extractCurrentWithJS(ctx context.Context, jsCode string) (*minidom.PageSnapshot, error) {
	var raw string
	if err := chromedp.Run(ctx, chromedp.Evaluate(jsCode, &raw)); err != nil {
		return nil, fmt.Errorf("js extractor: %w", err)
	}

	var result extractResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil, fmt.Errorf("parse extractor output: %w", err)
	}

	nodes := make([]minidom.Node, 0, len(result.Nodes))
	for _, rn := range result.Nodes {
		n := minidom.Node{
			ID:          rn.ID,
			StableID:    rn.StableID,
			Type:        minidom.NodeType(rn.Type),
			ParentID:    rn.ParentID,
			Text:        rn.Text,
			Layout:      rn.Layout,
			Style:       rn.Style,
			Interaction: rn.Interaction,
			Attrs:       rn.Attrs,
		}
		nodes = append(nodes, n)
	}

	// Build children lists from parent IDs.
	idxByID := make(map[int]int, len(nodes))
	for i, n := range nodes {
		idxByID[n.ID] = i
	}
	for _, n := range nodes {
		if n.ParentID != 0 {
			if pi, ok := idxByID[n.ParentID]; ok {
				nodes[pi].Children = append(nodes[pi].Children, n.ID)
			}
		}
	}

	// Convert IMAGE node src attrs into ResourceRef entries so the client never
	// fetches external URLs directly — all image requests go through /resources/.
	var resources []minidom.ResourceRef
	for i := range nodes {
		if nodes[i].Type != minidom.NodeImage {
			continue
		}
		src, ok := nodes[i].Attrs["src"]
		if !ok || src == "" {
			continue
		}
		// Inline data URIs don't need proxying; pass them through as-is.
		if strings.HasPrefix(src, "data:") {
			continue
		}
		// Only proxy http/https URLs.
		if !strings.HasPrefix(src, "http://") && !strings.HasPrefix(src, "https://") {
			continue
		}

		resID := "res_" + strings.ReplaceAll(uuid.New().String(), "-", "")
		w, h := 0, 0
		if nodes[i].Attrs["width"] != "" {
			fmt.Sscanf(nodes[i].Attrs["width"], "%d", &w)
		}
		if nodes[i].Attrs["height"] != "" {
			fmt.Sscanf(nodes[i].Attrs["height"], "%d", &h)
		}
		resources = append(resources, minidom.ResourceRef{
			ResourceID: resID,
			URL:        src,
			Width:      w,
			Height:     h,
		})

		// Replace src with the resource ID; remove the raw external URL.
		nodes[i].ResourceID = resID
		delete(nodes[i].Attrs, "src")
		if len(nodes[i].Attrs) == 0 {
			nodes[i].Attrs = nil
		}
	}

	// Extract favicon URL (best-effort; ignore errors).
	var faviconURL string
	_ = chromedp.Run(ctx, chromedp.Evaluate(jsFavicon, &faviconURL))

	return &minidom.PageSnapshot{
		URL:        result.URL,
		Title:      result.Title,
		FaviconURL: faviconURL,
		Nodes:      nodes,
		Resources:  resources,
	}, nil
}
