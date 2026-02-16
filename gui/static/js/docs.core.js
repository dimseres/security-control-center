var UserDirectory = (() => {
  let cache = {};
  let loading;

  async function load(force = false) {
    if (loading && !force) return loading;
    loading = (async () => {
      try {
        const res = await Api.get('/api/accounts/users');
        cache = {};
        (res.users || []).forEach(u => { cache[u.id] = u; });
      } catch (err) {
        try {
          const me = await Api.get('/api/auth/me');
          const u = me.user;
          cache[u.id] = { id: u.id, username: u.username, full_name: u.username };
        } catch (_) {
          // ignored
        }
      }
      return cache;
    })();
    return loading;
  }

  function name(id) {
    const u = cache[id];
    return u ? (u.full_name || u.username) : `#${id}`;
  }

  function get(id) {
    return cache[id];
  }

  function all() {
    return Object.values(cache);
  }

  return { load, name, all, get };
})();

if (typeof window !== 'undefined') {
  window.UserDirectory = UserDirectory;
}

const DocUI = (() => {
  const allLevelCodes = ["PUBLIC", "INTERNAL", "CONFIDENTIAL", "RESTRICTED", "SECRET", "TOP_SECRET", "SPECIAL_IMPORTANCE"];
  const TAG_FALLBACK = [
    { code: 'COMMERCIAL_SECRET', label: 'Коммерческая тайна' },
    { code: 'PERSONAL_DATA', label: 'ПДн' },
    { code: 'CRITICAL_INFRASTRUCTURE', label: 'КИИ' },
    { code: 'FEDERAL_LAW_152', label: 'ФЗ 152' },
    { code: 'FEDERAL_LAW_149', label: 'ФЗ 149' },
    { code: 'FEDERAL_LAW_187', label: 'ФЗ 187' },
    { code: 'FEDERAL_LAW_63', label: 'ФЗ 63' },
    { code: 'PCI_DSS', label: 'PCI DSS' },
  ];

  function levelName(levelValue) {
    const code = typeof levelValue === 'number' ? allLevelCodes[levelValue] : (levelValue || "").toUpperCase();
    if (typeof ClassificationDirectory !== 'undefined' && ClassificationDirectory.label) {
      return ClassificationDirectory.label(code);
    }
    const key = `docs.classification.${(code || '').toLowerCase()}`;
    return BerkutI18n.t(key) || code || '-';
  }

  function statusLabel(status) {
    return BerkutI18n.t(`docs.status.${status}`) || status;
  }

  function populateClassificationSelect(select) {
    if (!select) return;
    select.innerHTML = '';
    const codes = (typeof ClassificationDirectory !== 'undefined' && ClassificationDirectory.codes)
      ? ClassificationDirectory.codes()
      : allLevelCodes;
    codes.forEach((code, idx) => {
      const opt = document.createElement('option');
      opt.value = code;
      opt.textContent = levelName(code);
      select.appendChild(opt);
    });
  }

  function levelCodeByIndex(idx) {
    if (typeof idx === 'string') {
      const up = idx.toUpperCase();
      if (allLevelCodes.includes(up)) return up;
    }
    return allLevelCodes[idx] || allLevelCodes[0];
  }

  function tagsText(tags) {
    if (!tags || !tags.length) return '-';
    return tags.map(t => tagLabel(t)).join(', ');
  }

  function tagLabel(code) {
    if (typeof TagDirectory !== 'undefined' && TagDirectory.label) {
      const label = TagDirectory.label(code);
      if (label && label !== `docs.tag.${(code || '').toLowerCase()}`) return label;
    }
    const fallback = TAG_FALLBACK.find(t => (t.code || '').toUpperCase() === (code || '').toUpperCase());
    return fallback?.label || code;
  }

  function availableTags() {
    if (typeof TagDirectory !== 'undefined' && TagDirectory.all) {
      return TagDirectory.all();
    }
    return TAG_FALLBACK;
  }

  function resolveTagHint(selectEl, opts = {}) {
    if (!selectEl) return null;
    if (opts.hint) return opts.hint;
    if (opts.hintId) return document.getElementById(opts.hintId);
    if (selectEl.id) {
      const byData = document.querySelector(`[data-tag-hint="${selectEl.id}"]`);
      if (byData) return byData;
      const byId = document.getElementById(`${selectEl.id}-hint`);
      if (byId) return byId;
    }
    return null;
  }

  function renderTagHint(selectEl, hintEl) {
    if (!selectEl || !hintEl) return;
    hintEl.innerHTML = '';
    const selected = Array.from(selectEl.selectedOptions || []);
    if (!selected.length) {
      const emptyText = BerkutI18n.t('docs.stageEmptySelection') || '';
      hintEl.textContent = emptyText;
      return;
    }
    selected.forEach(opt => {
      const tag = document.createElement('span');
      tag.className = 'tag';
      tag.textContent = opt.dataset.label || opt.textContent || '';
      const remove = document.createElement('button');
      remove.type = 'button';
      remove.className = 'tag-remove';
      remove.setAttribute('aria-label', BerkutI18n.t('common.delete') || 'Remove');
      remove.textContent = 'x';
      remove.addEventListener('click', (e) => {
        e.stopPropagation();
        opt.selected = false;
        selectEl.dispatchEvent(new Event('change', { bubbles: true }));
      });
      tag.appendChild(remove);
      hintEl.appendChild(tag);
    });
  }

  function bindTagHint(selectEl, hintEl) {
    if (!selectEl || !hintEl) return;
    if (selectEl.dataset.tagHintBound === '1') {
      renderTagHint(selectEl, hintEl);
      return;
    }
    selectEl.dataset.tagHintBound = '1';
    selectEl.addEventListener('change', () => renderTagHint(selectEl, hintEl));
    selectEl.addEventListener('selectionrefresh', () => renderTagHint(selectEl, hintEl));
    renderTagHint(selectEl, hintEl);
  }

  function renderTagCheckboxes(container, opts = {}) {
    const target = typeof container === 'string'
      ? document.querySelector(container.startsWith('#') ? container : `#${container}`)
      : container;
    if (!target) return;
    const selected = new Set((opts.selected || []).map(v => (v || '').toUpperCase()));
    let selectEl = target;
    if (target.tagName !== 'SELECT') {
      target.innerHTML = '';
      selectEl = document.createElement('select');
      selectEl.multiple = true;
      selectEl.className = 'select';
      if (target.id) selectEl.id = target.id;
      target.appendChild(selectEl);
    }
    if (opts.name) selectEl.name = opts.name;
    selectEl.innerHTML = '';
    availableTags().forEach(tag => {
      const code = tag.code || tag;
      const opt = document.createElement('option');
      opt.value = code;
      opt.textContent = tagLabel(code);
      opt.dataset.label = opt.textContent;
      opt.selected = selected.has((code || '').toUpperCase());
      selectEl.appendChild(opt);
    });
    if (typeof DocsPage !== 'undefined' && DocsPage.enhanceMultiSelects) {
      DocsPage.enhanceMultiSelects([selectEl.id]);
    }
    const hintEl = resolveTagHint(selectEl, opts);
    bindTagHint(selectEl, hintEl);
  }

  return {
    levelName,
    populateClassificationSelect,
    levelCodeByIndex,
    statusLabel,
    tagsText,
    levelCodes: allLevelCodes,
    tagLabel,
    availableTags,
    renderTagCheckboxes,
    renderTagHint,
    bindTagHint
  };
})();

const DocsPage = (() => {
  const state = {
    folders: [],
    folderMap: {},
    docs: [],
    selectedFolder: null,
    filters: { status: '', tags: [], mine: false, review: false, secret: false, search: '' },
    currentUser: null,
    uploadCtx: null,
    usersLoaded: false,
    templates: [],
    contextDoc: null,
    converterStatus: null,
    viewerDoc: null,
    viewerContent: '',
    viewerFormat: 'md',
    tabs: [],
    activeTabId: 'list',
  };

  function hasPermission(perm) {
    if (!perm) return true;
    const perms = Array.isArray(state.currentUser?.permissions) ? state.currentUser.permissions : [];
    if (!perms.length) return true;
    return perms.includes(perm);
  }

  async function init() {
    const page = document.getElementById('docs-page');
    if (!page) return;
    const initialRoute = parseDocsRoute();
    const dir = (typeof window !== 'undefined' && window.UserDirectory)
      ? window.UserDirectory
      : (typeof UserDirectory !== 'undefined' ? UserDirectory : null);
    if (dir && dir.load) {
      await dir.load();
    }
    if (DocsPage.loadCurrentUser) {
      state.currentUser = await DocsPage.loadCurrentUser();
    }
    if (DocsPage.bindUI) DocsPage.bindUI();
    if (DocsPage.bindTabs) DocsPage.bindTabs();
    DocEditor.init({
      onSave: async () => { if (DocsPage.loadDocs) await DocsPage.loadDocs(); },
      onModeChange: (docId, mode) => {
        // Re-open tab in requested mode so DOCX mode switch uses the same path
        // as context-menu Edit and always boots a clean OnlyOffice session.
        if (DocsPage.openDocTab) {
          DocsPage.openDocTab(docId, mode);
        } else if (DocsPage.updateActiveDocMode) {
          DocsPage.updateActiveDocMode(docId, mode);
        } else if (DocsPage.updateDocsPath) {
          DocsPage.updateDocsPath(docId, mode);
        }
      },
      onClose: () => {
        if (DocsPage.requestCloseTab && state.activeTabId) {
          const tab = (state.tabs || []).find(t => t.id === state.activeTabId);
          if (tab && tab.type === 'doc') {
            DocsPage.requestCloseTab(state.activeTabId);
            return;
          }
        }
        state.contextDoc = null;
      }
    });
    // Ensure editor and context menu are hidden on initial load in case markup state is stale
    DocEditor.close({ silent: true });
    if (DocsPage.hideContextMenu) DocsPage.hideContextMenu();
    const canViewDocs = hasPermission('docs.view');
    if (canViewDocs) {
      if (DocsPage.loadFolders) await DocsPage.loadFolders();
      if (DocsPage.loadUsersIntoSelects) await DocsPage.loadUsersIntoSelects();
      if (DocsPage.loadDocs) await DocsPage.loadDocs();
      if (DocsPage.renderConverterBanner) DocsPage.renderConverterBanner();
      if (window.__pendingDocOpen && DocsPage.openDocTab) {
        DocsPage.openDocTab(window.__pendingDocOpen, 'view');
        window.__pendingDocOpen = null;
      }
    }
    if (initialRoute?.docId) {
      if (DocsPage.openDocTab) {
        DocsPage.openDocTab(initialRoute.docId, initialRoute.mode === 'edit' ? 'edit' : 'view');
      }
    }
  }

  function parseDocsRoute() {
    const parts = window.location.pathname.split('/').filter(Boolean);
    if (parts[0] !== 'docs') return null;
    const id = parseInt(parts[1] || '', 10);
    if (Number.isFinite(id)) {
      return { docId: id, mode: parts[2] === 'edit' ? 'edit' : 'view' };
    }
    return { tab: 'list' };
  }

  function updateDocsPath(docId, mode = 'view') {
    let next = '/docs';
    if (docId) {
      next = mode === 'edit' ? `/docs/${docId}/edit` : `/docs/${docId}`;
    }
    if (window.location.pathname !== next) {
      window.history.replaceState({}, '', next);
    }
  }

  function confirmAction(opts = {}) {
    const modal = document.getElementById('docs-confirm-modal');
    const title = opts.title || BerkutI18n.t('common.confirm');
    const message = opts.message || '';
    const confirmText = opts.confirmText || BerkutI18n.t('common.confirm');
    const cancelText = opts.cancelText || BerkutI18n.t('common.cancel');
    if (!modal) {
      return Promise.resolve(window.confirm(message || title));
    }
    const titleEl = document.getElementById('docs-confirm-title');
    const msgEl = document.getElementById('docs-confirm-message');
    const yesBtn = document.getElementById('docs-confirm-yes');
    const noBtn = document.getElementById('docs-confirm-no');
    const closeBtn = document.getElementById('docs-confirm-close');
    if (titleEl) titleEl.textContent = title;
    if (msgEl) msgEl.textContent = message;
    if (yesBtn) yesBtn.textContent = confirmText;
    if (noBtn) noBtn.textContent = cancelText;
    modal.hidden = false;
    return new Promise(resolve => {
      const cleanup = (result) => {
        modal.hidden = true;
        if (yesBtn) yesBtn.onclick = null;
        if (noBtn) noBtn.onclick = null;
        if (closeBtn) closeBtn.onclick = null;
        resolve(result);
      };
      if (yesBtn) yesBtn.onclick = () => cleanup(true);
      if (noBtn) noBtn.onclick = () => cleanup(false);
      if (closeBtn) closeBtn.onclick = () => cleanup(false);
      if (yesBtn) yesBtn.focus();
    });
  }

  function enhanceMultiSelects(ids = []) {
    ids.forEach(id => {
      const sel = document.getElementById(id);
      if (!sel) return;
      // ensure true multi-select even if markup missed attributes
      sel.multiple = true;
      sel.setAttribute('multiple', 'multiple');
      if (!sel.size || sel.size < 2) sel.size = 6;
      const mark = '';
      const markRegex = /$/;
      let suppressNotify = false;
      const refresh = () => {
        Array.from(sel.options).forEach(opt => {
          const base = opt.dataset.label || opt.textContent.replace(markRegex, '');
          opt.dataset.label = base;
          opt.textContent = opt.selected ? `${base}${mark}` : base;
        });
        if (!suppressNotify) {
          suppressNotify = true;
          sel.dispatchEvent(new Event('selectionrefresh', { bubbles: false }));
          suppressNotify = false;
        }
      };
      const toggle = (opt) => {
        opt.selected = !opt.selected;
        refresh();
        const evt = new Event('change', { bubbles: true });
        sel.dispatchEvent(evt);
      };
      if (!sel.dataset.enhanced) {
        sel.dataset.enhanced = '1';
        sel.addEventListener('mousedown', (e) => {
          const opt = e.target.closest('option');
          if (!opt) return;
          e.preventDefault();
          toggle(opt);
        });
        sel.addEventListener('change', refresh);
        sel.addEventListener('dblclick', (e) => {
          const opt = e.target.closest('option');
          if (!opt) return;
          toggle(opt);
        });
      }
      refresh();
    });
  }

  function openModal(sel) {
    const el = document.querySelector(sel);
    if (el) el.hidden = false;
  }

  function closeModal(sel) {
    const el = document.querySelector(sel);
    if (el) el.hidden = true;
  }

  function hideAlert(el) {
    if (el) el.hidden = true;
  }

  function showAlert(el, msg) {
    if (!el) return;
    el.textContent = msg;
    el.hidden = false;
  }

  function formatDate(d) {
    if (!d) return '-';
    try {
      if (typeof AppTime !== 'undefined' && AppTime.formatDateTime) {
        return AppTime.formatDateTime(d);
      }
      const dt = new Date(d);
      const pad = (num) => `${num}`.padStart(2, '0');
      return `${pad(dt.getDate())}.${pad(dt.getMonth() + 1)}.${dt.getFullYear()} ${pad(dt.getHours())}:${pad(dt.getMinutes())}`;
    } catch (err) {
      return d;
    }
  }

  function escapeHtml(str) {
    return (str || '').toString().replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', '\'': '&#39;' }[c]));
  }

  function formDataToObj(fd) {
    const obj = {};
    fd.forEach((v, k) => {
      if (obj[k]) {
        if (!Array.isArray(obj[k])) obj[k] = [obj[k]];
        obj[k].push(v);
      } else {
        obj[k] = v;
      }
    });
    return obj;
  }

  function collectChecked(nodes) {
    return Array.from(nodes || []).map(n => n.value);
  }

  function toArray(val) {
    if (!val) return [];
    return Array.isArray(val) ? val : [val];
  }

  function parseNullableInt(val) {
    if (val === null || val === undefined || val === '') return null;
    const n = parseInt(val, 10);
    return isNaN(n) ? null : n;
  }

  function renderTemplate(content, vars) {
    return (content || '').replace(/\{\{(\w+)\}\}/g, (_, key) => vars[key] || '');
  }

  return {
    init,
    state,
    enhanceMultiSelects,
    openModal,
    closeModal,
    hideAlert,
    showAlert,
    formatDate,
    escapeHtml,
    formDataToObj,
    collectChecked,
    toArray,
    parseNullableInt,
    renderTemplate,
    updateDocsPath,
    confirmAction,
    hasPermission
  };
})();

if (typeof window !== 'undefined') {
  window.DocsPage = DocsPage;
}
