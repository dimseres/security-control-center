const DocEditor = (() => {
  let currentDocId = null;
  let currentFormat = 'md';
  let meta = null;
  let currentMode = 'view';
  let initialContent = '';
  let dirty = false;
  let callbacks = {};
  const els = {};
  const EDITABLE_FORMATS = new Set(['md', 'txt']);
  let onlyOfficeActive = false;
  let onlyOfficeOpenAttempts = 0;
  let modeSwitchInFlight = false;
  let pendingDocxMode = null;
  let saveInFlight = false;
  let currentDocVersion = 0;
  let lastSecurityEventAt = 0;

  function isEditableFormat(format) {
    return EDITABLE_FORMATS.has((format || '').toLowerCase());
  }

  function isBinaryFormat(format) {
    const next = (format || '').toLowerCase();
    return next === 'docx' || next === 'pdf';
  }

  function canEditFormat(format) {
    const next = (format || '').toLowerCase();
    if (isEditableFormat(next)) return true;
    if (next !== 'docx') return false;
    return !!(window.DocsOnlyOffice && typeof window.DocsOnlyOffice.open === 'function');
  }

  function localizeError(err, fallbackKey) {
    const raw = String((err && err.message) || '').trim();
    if (raw && typeof BerkutI18n !== 'undefined' && typeof BerkutI18n.t === 'function') {
      const translated = BerkutI18n.t(raw);
      if (translated && translated !== raw) return translated;
    }
    if (fallbackKey && typeof BerkutI18n !== 'undefined' && typeof BerkutI18n.t === 'function') {
      return BerkutI18n.t(fallbackKey);
    }
    return raw || fallbackKey || 'Error';
  }

  function init(opts = {}) {
    callbacks = opts;
    els.panel = document.getElementById('doc-editor');
    if (!els.panel) return;
    els.title = document.getElementById('editor-title');
    els.content = document.getElementById('editor-content');
    els.viewer = document.getElementById('editor-viewer');
    els.pdfFrame = document.getElementById('editor-pdf-frame');
    els.onlyOfficeWrap = document.getElementById('editor-onlyoffice-wrap');
    els.onlyOfficeLoading = document.getElementById('editor-onlyoffice-loading');
    els.onlyOfficeHost = document.getElementById('editor-onlyoffice-host');
    els.nonMdBlock = document.getElementById('editor-nonmd');
    els.nonMdHint = document.getElementById('editor-nonmd-hint');
    els.onlyOfficeOpenBtn = document.getElementById('editor-open-onlyoffice');
    els.downloadBtn = document.getElementById('editor-download');
    els.convertBtn = document.getElementById('editor-convert');
    els.reason = document.getElementById('editor-reason');
    els.alert = document.getElementById('editor-alert');
    els.mdView = document.getElementById('editor-md-view');
    els.editToggle = document.getElementById('editor-edit-toggle');
    els.classification = document.getElementById('editor-classification');
    els.tags = document.getElementById('editor-tags');
    els.status = document.getElementById('editor-status');
    els.owner = document.getElementById('editor-owner');
    els.reg = document.getElementById('editor-reg');
    els.folder = document.getElementById('editor-folder');
    els.toolbar = document.getElementById('editor-toolbar');
    els.saveBtn = document.getElementById('editor-save');
    els.closeBtn = document.getElementById('editor-close');
    els.linkType = document.getElementById('link-type');
    els.linkId = document.getElementById('link-id');
    els.addLink = document.getElementById('add-link');
    els.links = document.getElementById('editor-links');
    els.aclRoles = document.getElementById('editor-acl-roles');
    els.aclUsers = document.getElementById('editor-acl-users');
    els.aclSave = document.getElementById('editor-acl-save');
    els.aclRefresh = document.getElementById('editor-acl-refresh');
    DocUI.populateClassificationSelect(els.classification);
    renderEditorTags([]);
    bindToolbar();
    bindButtons();
    bindDirtyTracking();
    document.addEventListener('tags:changed', () => {
      const current = Array.from(document.querySelectorAll('#editor-tags option:checked')).map(opt => opt.value);
      if (current.length) {
        renderEditorTags(current);
      } else if (meta && meta.classification_tags) {
        renderEditorTags(meta.classification_tags);
      } else {
        renderEditorTags([]);
      }
    });
  }

  function bindToolbar() {
    if (!els.toolbar) return;
    els.toolbar.querySelectorAll('button[data-action]').forEach(btn => {
      btn.onclick = () => applyFormatting(btn.dataset.action);
    });
  }

  function bindButtons() {
    if (els.saveBtn) els.saveBtn.onclick = () => save();
    if (els.closeBtn) els.closeBtn.onclick = () => {
      if (callbacks.onClose) {
        callbacks.onClose(currentDocId);
      } else {
        close();
      }
    };
    if (els.editToggle) {
      els.editToggle.onclick = () => {
        if (currentMode === 'view') {
          setMode('edit');
        } else {
          setMode('view');
        }
      };
    }
    if (els.convertBtn) els.convertBtn.onclick = () => convertToMarkdown();
    if (els.downloadBtn) els.downloadBtn.onclick = () => download();
    if (els.onlyOfficeOpenBtn) els.onlyOfficeOpenBtn.onclick = () => openOnlyOffice();
    if (els.addLink) els.addLink.onclick = () => addLink();
    if (els.addLinkInline) els.addLinkInline.onclick = () => addLink();
    if (els.aclSave) els.aclSave.onclick = () => saveAcl();
    if (els.aclRefresh) els.aclRefresh.onclick = () => loadAcl();
  }

  async function open(docId, opts = {}) {
    if (!els.panel) return;
    teardownOnlyOffice();
    onlyOfficeOpenAttempts = 0;
    currentDocId = docId;
    currentMode = opts.mode === 'edit' ? 'edit' : 'view';
    setDirty(false);
    showAlert('');
    if (els.reason) els.reason.value = '';
    try {
      meta = await Api.get(`/api/docs/${docId}`);
      bindSecurityGuards();
      let cont;
      try {
        cont = await Api.get(`/api/docs/${docId}/content?audit=0`);
      } catch (err) {
        const msg = (err && err.message ? err.message : '').toLowerCase();
        if (msg.includes('not found')) {
          cont = { format: (meta && meta.format) || 'md', content: '' };
        } else {
          throw err;
        }
      }
      currentFormat = cont.format || 'md';
      currentDocVersion = Number(cont.version || 0);
      renderMeta(meta);
      await renderContent(cont);
      await loadLinks();
      await loadAclOptions();
      await loadAcl();
      els.panel.hidden = false;
      return meta;
    } catch (err) {
      console.error('[editor] open failed', { docId, err: err.message });
      showAlert(err.message || 'Failed to load document');
      return null;
    }
  }

  function renderMeta(doc) {
    els.title.textContent = `${doc.title || ''} (${doc.reg_number || ''})`;
    const code = DocUI.levelCodeByIndex(doc.classification_level);
    if (els.classification) els.classification.value = code;
    const tags = (doc.classification_tags || []).map(t => t.toUpperCase());
    renderEditorTags(tags);
    if (els.status) els.status.textContent = DocUI.statusLabel(doc.status) || '-';
    if (els.owner) els.owner.textContent = (UserDirectory ? UserDirectory.name(doc.created_by) : (doc.created_by || '')) || '-';
    if (els.reg) els.reg.textContent = doc.reg_number || '-';
    if (els.folder) {
      if (doc.folder_id) {
        const folder = (window.DocsPage && DocsPage.state && DocsPage.state.folders || []).find(f => f.id === doc.folder_id);
        els.folder.textContent = folder ? folder.name : `#${doc.folder_id}`;
      } else {
        els.folder.textContent = '-';
      }
    }
  }

  function renderEditorTags(selected = []) {
    DocUI.renderTagCheckboxes('#editor-tags', { className: 'editor-tag', selected });
  }

  async function renderContent(res) {
    const format = (res.format || '').toLowerCase();
    currentFormat = format || 'md';
    if (els.onlyOfficeWrap) els.onlyOfficeWrap.hidden = true;
    onlyOfficeActive = false;
    if (format === 'md' || format === 'txt') {
      initialContent = res.content || '';
      setDirty(false);
      if (currentMode === 'view') {
        els.content.hidden = true;
        els.viewer.hidden = true;
        if (els.mdView) {
          els.mdView.hidden = false;
          renderMarkdownView(initialContent);
        }
      } else {
        els.content.hidden = false;
        if (els.mdView) els.mdView.hidden = true;
        els.viewer.hidden = true;
        els.content.value = initialContent;
        els.content.focus();
      }
    } else if (format === 'pdf') {
      currentFormat = format;
      if (els.mdView) els.mdView.hidden = true;
      els.content.hidden = true;
      els.viewer.hidden = false;
      els.nonMdBlock.hidden = true;
      els.pdfFrame.hidden = false;
      els.pdfFrame.src = `/api/docs/${currentDocId}/content?raw=1`;
    } else if (format === 'docx') {
      currentFormat = format;
      if (els.content) els.content.hidden = true;
      if (els.viewer) els.viewer.hidden = true;
      if (els.pdfFrame) els.pdfFrame.hidden = true;
      if (els.nonMdBlock) els.nonMdBlock.hidden = true;
      if (els.mdView) {
        els.mdView.hidden = false;
        els.mdView.innerHTML = '';
      }
      await switchDocxModeWithLoader();
    } else {
      if (els.mdView) els.mdView.hidden = true;
      els.content.hidden = true;
      els.viewer.hidden = false;
      els.pdfFrame.hidden = true;
      els.nonMdBlock.hidden = false;
      els.nonMdHint.textContent = BerkutI18n.t('docs.nonEditable');
    }
    applyMode();
  }

  function showBinaryFallback(hintKey) {
    teardownOnlyOffice();
    if (els.mdView) els.mdView.hidden = true;
    if (els.viewer) els.viewer.hidden = false;
    if (els.nonMdBlock) els.nonMdBlock.hidden = false;
    if (els.nonMdHint) els.nonMdHint.textContent = BerkutI18n.t(hintKey || 'docs.nonEditable');
    if (els.pdfFrame) els.pdfFrame.hidden = true;
    if (els.onlyOfficeOpenBtn) els.onlyOfficeOpenBtn.hidden = true;
    if (els.convertBtn) els.convertBtn.hidden = true;
  }

  async function openOnlyOffice(mode) {
    if (currentFormat !== 'docx' || !currentDocId) return false;
    if (!window.DocsOnlyOffice || typeof window.DocsOnlyOffice.open !== 'function') {
      showBinaryFallback('docs.onlyoffice.unavailable');
      return false;
    }
    try {
      if (els.viewer) els.viewer.hidden = true;
      if (els.mdView) els.mdView.hidden = true;
      if (els.content) els.content.hidden = true;
      if (els.onlyOfficeWrap) els.onlyOfficeWrap.hidden = false;
      if (els.onlyOfficeLoading) els.onlyOfficeLoading.hidden = false;
      const requestedMode = mode === 'edit' ? 'edit' : 'view';
      await window.DocsOnlyOffice.open(currentDocId, 'editor-onlyoffice-host', requestedMode);
      if (els.onlyOfficeLoading) els.onlyOfficeLoading.hidden = true;
      onlyOfficeActive = true;
      onlyOfficeOpenAttempts = 0;
      return true;
    } catch (err) {
      if (els.onlyOfficeLoading) els.onlyOfficeLoading.hidden = true;
      if (onlyOfficeOpenAttempts < 1) {
        onlyOfficeOpenAttempts += 1;
        await new Promise((resolve) => setTimeout(resolve, 600));
        const retryMode = mode === 'edit' ? 'edit' : 'view';
        return openOnlyOffice(retryMode);
      }
      showAlert(localizeError(err, 'docs.onlyoffice.unavailable'));
      showBinaryFallback('docs.onlyoffice.unavailable');
      return false;
    }
  }

  function teardownOnlyOffice() {
    onlyOfficeActive = false;
    if (els.onlyOfficeLoading) els.onlyOfficeLoading.hidden = true;
    if (window.DocsOnlyOffice && typeof window.DocsOnlyOffice.destroy === 'function') {
      window.DocsOnlyOffice.destroy();
    }
    if (els.onlyOfficeWrap) els.onlyOfficeWrap.hidden = true;
  }

  async function switchDocxToEditWithLoader() {
    return switchDocxModeWithLoader();
  }

  async function switchDocxModeWithLoader() {
    const requestedMode = currentMode === 'edit' ? 'edit' : 'view';
    if (modeSwitchInFlight) {
      pendingDocxMode = requestedMode;
      return false;
    }
    pendingDocxMode = null;
    modeSwitchInFlight = true;
    teardownOnlyOffice();
    if (els.viewer) els.viewer.hidden = true;
    if (els.mdView) els.mdView.hidden = true;
    if (els.content) els.content.hidden = true;
    if (els.onlyOfficeWrap) els.onlyOfficeWrap.hidden = false;
    if (els.onlyOfficeLoading) els.onlyOfficeLoading.hidden = false;
    try {
      await new Promise((resolve) => setTimeout(resolve, 500));
      return await openOnlyOffice(requestedMode);
    } finally {
      modeSwitchInFlight = false;
      if (pendingDocxMode && pendingDocxMode !== requestedMode) {
        const nextMode = pendingDocxMode;
        pendingDocxMode = null;
        currentMode = nextMode;
        setTimeout(() => {
          void switchDocxModeWithLoader();
        }, 0);
      }
    }
  }

  async function save() {
    if (!currentDocId) return;
    if (saveInFlight) return;
    const reason = (els.reason && els.reason.value || '').trim();
    const contentEditable = currentFormat === 'docx' || currentFormat === 'md' || currentFormat === 'txt';
    if (contentEditable && !reason) {
      showAlert(BerkutI18n.t('editor.reasonRequired'));
      return;
    }
    if (currentFormat === 'docx') {
      if (!window.DocsOnlyOffice || typeof window.DocsOnlyOffice.forceSave !== 'function') {
        showAlert(BerkutI18n.t('docs.onlyoffice.unavailable'));
        return;
      }
      if (window.DocsOnlyOffice.isReady && !window.DocsOnlyOffice.isReady()) {
        showAlert(BerkutI18n.t('docs.onlyoffice.loading'));
        return;
      }
      try {
        saveInFlight = true;
        if (els.saveBtn) els.saveBtn.disabled = true;
        const baseVersion = await fetchCurrentDocVersion();
        await window.DocsOnlyOffice.forceSave(currentDocId, reason);
        const nextVersion = await waitForDocxVersionUpdate(baseVersion, 12000);
        if (!nextVersion || nextVersion <= baseVersion) {
          throw new Error('docs.onlyoffice.forceSaveNoVersion');
        }
        currentDocVersion = nextVersion;
        showAlert(BerkutI18n.t('docs.onlyoffice.status.saved'), true);
        if (callbacks.onSave) callbacks.onSave(currentDocId);
      } catch (err) {
        showAlert(localizeError(err, 'docs.onlyoffice.forceSaveFailed'));
      } finally {
        saveInFlight = false;
        if (els.saveBtn) els.saveBtn.disabled = false;
      }
      return;
    }
    if (currentFormat !== 'md' && currentFormat !== 'txt') {
      try {
        const changed = await maybeUpdateClassification();
        if (changed) {
          showAlert(BerkutI18n.t('editor.saved'), true);
          if (callbacks.onSave) callbacks.onSave(currentDocId);
        } else {
          showAlert(BerkutI18n.t('editor.readonly'));
        }
      } catch (err) {
        showAlert(err.message || 'save failed');
      }
      return;
    }
    try {
      saveInFlight = true;
      if (els.saveBtn) els.saveBtn.disabled = true;
      const format = currentFormat === 'txt' ? 'txt' : 'md';
      await Api.put(`/api/docs/${currentDocId}/content`, { content: els.content.value, format, reason });
      await maybeUpdateClassification();
      initialContent = els.content.value;
      setDirty(false);
      showAlert(BerkutI18n.t('editor.saved'), true);
      if (callbacks.onSave) callbacks.onSave(currentDocId);
    } catch (err) {
      showAlert(err.message || 'save failed');
    } finally {
      saveInFlight = false;
      if (els.saveBtn) els.saveBtn.disabled = false;
    }
  }

  async function fetchCurrentDocVersion() {
    if (!currentDocId) return 0;
    try {
      const latest = await Api.get(`/api/docs/${currentDocId}/content?audit=0`);
      const ver = Number((latest && latest.version) || 0);
      if (ver > 0) {
        currentDocVersion = ver;
      }
    } catch (_) {}
    return currentDocVersion || 0;
  }

  async function waitForDocxVersionUpdate(baseVersion, timeoutMs) {
    const started = Date.now();
    while (Date.now() - started < timeoutMs) {
      const ver = await fetchCurrentDocVersion();
      if (ver > baseVersion) return ver;
      await new Promise((resolve) => setTimeout(resolve, 600));
    }
    return currentDocVersion || 0;
  }

  async function maybeUpdateClassification() {
    if (!meta) return;
    const nextLevel = els.classification ? els.classification.value : null;
    const nextTags = Array.from(document.querySelectorAll('#editor-tags option:checked')).map(opt => opt.value);
    const currentLevelCode = DocUI.levelCodeByIndex(meta.classification_level);
    const changed = nextLevel !== currentLevelCode || JSON.stringify(nextTags.sort()) !== JSON.stringify((meta.classification_tags || []).map(t => t.toUpperCase()).sort());
    if (!changed) return false;
    try {
      await Api.post(`/api/docs/${currentDocId}/classification`, {
        classification_level: nextLevel,
        classification_tags: nextTags,
        inherit_classification: meta.inherit_classification,
      });
      meta.classification_level = nextLevel;
      meta.classification_tags = nextTags;
      return true;
    } catch (err) {
      showAlert(err.message || 'classification not updated');
      return false;
    }
  }

  async function loadLinks() {
    if (!els.links) return;
    els.links.innerHTML = '';
    try {
      const res = await Api.get(`/api/docs/${currentDocId}/links`);
      (res.links || []).forEach(l => {
        const row = document.createElement('div');
        row.className = 'link-item';
        row.innerHTML = `<span>${escapeHtml(l.target_type)} #${escapeHtml(l.target_id)}</span><button class="btn ghost" data-id="${l.id}">×</button>`;
        row.querySelector('button').onclick = () => removeLink(l.id);
        els.links.appendChild(row);
      });
    } catch (err) {
      console.warn('load links', err);
    }
  }

  async function addLink() {
    if (!currentDocId) return;
    const type = els.linkType.value;
    const id = els.linkId.value.trim();
    if (!type || !id) return;
    await Api.post(`/api/docs/${currentDocId}/links`, { target_type: type, target_id: id });
    els.linkId.value = '';
    await loadLinks();
  }

  async function removeLink(id) {
    await Api.del(`/api/docs/${currentDocId}/links/${id}`);
    await loadLinks();
  }

  function enhanceSelectWithTicks(sel) {
    if (!sel || sel.dataset.enhanced) return;
    sel.dataset.enhanced = '1';
    Array.from(sel.options).forEach(opt => {
      opt.dataset.label = opt.textContent;
      if (opt.selected) opt.textContent = `${opt.dataset.label} ✓`;
    });
    const refresh = () => {
      Array.from(sel.options).forEach(opt => {
        const base = opt.dataset.label || opt.textContent.replace(/ ✓$/, '');
        opt.dataset.label = base;
        opt.textContent = opt.selected ? `${base} ✓` : base;
      });
    };
    sel.addEventListener('mousedown', (e) => {
      const opt = e.target.closest('option');
      if (!opt) return;
      e.preventDefault();
      opt.selected = !opt.selected;
      refresh();
    });
    sel.addEventListener('change', refresh);
    sel.addEventListener('dblclick', (e) => {
      const opt = e.target.closest('option');
      if (!opt) return;
      opt.selected = !opt.selected;
      refresh();
    });
  }

  async function loadAclOptions() {
    if (!els.aclRoles || !els.aclUsers) return;
    await UserDirectory.load();
    const roleOptions = ['superadmin', 'admin', 'security_officer', 'doc_admin', 'doc_editor', 'doc_reviewer', 'doc_viewer', 'auditor', 'manager', 'analyst'];
    els.aclRoles.innerHTML = '';
    roleOptions.forEach(r => {
      const opt = document.createElement('option');
      opt.value = r;
      opt.textContent = r;
      els.aclRoles.appendChild(opt);
    });
    els.aclUsers.innerHTML = '';
    UserDirectory.all().forEach(u => {
      const opt = document.createElement('option');
      opt.value = u.id;
      opt.textContent = u.full_name || u.username;
      els.aclUsers.appendChild(opt);
    });
    enhanceSelectWithTicks(els.aclRoles);
    enhanceSelectWithTicks(els.aclUsers);
  }

  async function loadAcl() {
    if (!currentDocId || !els.aclRoles || !els.aclUsers) return;
    try {
      const res = await Api.get(`/api/docs/${currentDocId}/acl`);
      const acl = res.acl || [];
      const roleSelected = new Set(acl.filter(a => a.subject_type === 'role').map(a => a.subject_id));
      const userSelected = new Set(acl.filter(a => a.subject_type === 'user').map(a => a.subject_id));
      Array.from(els.aclRoles.options).forEach(o => { o.selected = roleSelected.has(o.value); });
      Array.from(els.aclUsers.options).forEach(o => {
        const u = UserDirectory.get(parseInt(o.value, 10));
        o.selected = u && userSelected.has(u.username);
      });
      enhanceSelectWithTicks(els.aclRoles);
      enhanceSelectWithTicks(els.aclUsers);
    } catch (err) {
      console.warn('load acl', err);
    }
  }

  async function saveAcl() {
    if (!currentDocId || !els.aclRoles || !els.aclUsers) return;
    const rolesSel = Array.from(els.aclRoles.selectedOptions).map(o => o.value);
    const usersSel = Array.from(els.aclUsers.selectedOptions).map(o => parseInt(o.value, 10)).filter(Boolean);
    const acl = [];
    rolesSel.forEach(r => {
      ['view', 'edit', 'manage'].forEach(p => acl.push({ subject_type: 'role', subject_id: r, permission: p }));
    });
    usersSel.forEach(uid => {
      const u = UserDirectory.get(uid);
      if (u) {
        ['view', 'edit', 'manage'].forEach(p => acl.push({ subject_type: 'user', subject_id: u.username, permission: p }));
      }
    });
    try {
      await Api.put(`/api/docs/${currentDocId}/acl`, { acl });
    } catch (err) {
      showAlert(err.message || 'ACL save failed');
    }
  }

  async function convertToMarkdown() {
    if (!currentDocId) return;
    try {
      const res = await Api.post(`/api/docs/${currentDocId}/convert`, {});
      currentFormat = 'md';
      els.content.value = res.content || '';
      els.content.hidden = false;
      els.viewer.hidden = true;
      showAlert(BerkutI18n.t('docs.converted'), true);
    } catch (err) {
      showAlert(err.message || 'convert failed');
    }
  }

  function download() {
    if (!currentDocId) return;
    const fmt = currentFormat || 'pdf';
    window.open(`/api/docs/${currentDocId}/export?format=${fmt}`, '_blank');
  }

  function applyFormatting(action) {
    if (!els.content || els.content.hidden) return;
    const textarea = els.content;
    const start = textarea.selectionStart;
    const end = textarea.selectionEnd;
    const selected = textarea.value.substring(start, end);
    let replacement = selected;
    switch (action) {
      case 'bold':
        replacement = `**${selected || BerkutI18n.t('editor.placeholder')}**`;
        break;
      case 'italic':
        replacement = `*${selected || BerkutI18n.t('editor.placeholder')}*`;
        break;
      case 'heading':
        replacement = `## ${selected || BerkutI18n.t('editor.placeholder')}`;
        break;
      case 'list':
        replacement = selected.split('\n').map(line => line ? `- ${line}` : '- ').join('\n');
        break;
      case 'quote':
        replacement = selected.split('\n').map(line => `> ${line || ''}`).join('\n');
        break;
      case 'code':
        replacement = `\`\`\`\n${selected || BerkutI18n.t('editor.placeholder')}\n\`\`\``;
        break;
      case 'link':
        replacement = `[${selected || BerkutI18n.t('editor.placeholder')}]()`;
        break;
      case 'table':
        replacement = `| Col1 | Col2 |\n| --- | --- |\n| ${selected || 'text'} |  |`;
        break;
    }
    textarea.setRangeText(replacement, start, end, 'end');
    textarea.focus();
  }

  function close(opts = {}) {
    teardownOnlyOffice();
    if (els.panel) {
      els.panel.hidden = true;
    }
    currentDocId = null;
    currentFormat = 'md';
    initialContent = '';
    if (els.content) els.content.value = '';
    if (els.mdView) els.mdView.innerHTML = '';
    if (els.pdfFrame) els.pdfFrame.src = 'about:blank';
    setDirty(false);
    if (!opts.silent && callbacks.onClose) callbacks.onClose();
  }

  function bindDirtyTracking() {
    if (!els.content) return;
    els.content.addEventListener('input', () => {
      if (currentMode !== 'edit') return;
      setDirty(els.content.value !== initialContent);
    });
  }

  function setDirty(next) {
    dirty = !!next;
  }

  function isDirty() {
    return dirty;
  }

  function setMode(mode) {
    const requestedMode = mode === 'edit' ? 'edit' : 'view';
    currentMode = requestedMode;
    applyMode();
    if (callbacks.onModeChange && currentDocId) {
      callbacks.onModeChange(currentDocId, currentMode);
    }
    if (currentFormat === 'docx') {
      // For DOCX, always prefer full tab reopen flow (same as context-menu Edit)
      // to avoid reusing stale OnlyOffice runtime state inside the same session.
      if (callbacks.onModeChange) {
        teardownOnlyOffice();
        return;
      }
      void switchDocxModeWithLoader();
      return;
    }
    if (currentFormat === 'md' || currentFormat === 'txt') {
      if (currentMode === 'view') {
        if (els.mdView) {
          els.mdView.hidden = false;
          renderMarkdownView(els.content?.value || initialContent);
        }
        if (els.content) els.content.hidden = true;
        if (els.viewer) els.viewer.hidden = true;
      } else {
        if (els.mdView) els.mdView.hidden = true;
        if (els.content) {
          els.content.hidden = false;
          if (!els.content.value) els.content.value = initialContent;
          els.content.focus();
        }
      }
    }
  }

  function isProtectedDoc() {
    if (!meta) return false;
    const level = Number(meta.classification_level || 0);
    const tags = Array.isArray(meta.classification_tags) ? meta.classification_tags : [];
    return level >= 2 || tags.length > 0;
  }

  function canProtectClipboardAndPrint() {
    const cfg = (window.__APP_CONFIG__ && window.__APP_CONFIG__.docs && window.__APP_CONFIG__.docs.dlp) || null;
    if (cfg && cfg.protect_clipboard_and_print === false) return false;
    return true;
  }

  function bindSecurityGuards() {
    if (!els.panel || els.panel.dataset.securityBound === '1') return;
    els.panel.dataset.securityBound = '1';
    const blockAndLog = (type, details, eventObj) => {
      if (!isProtectedDoc() || !canProtectClipboardAndPrint()) return;
      if (eventObj && eventObj.preventDefault) eventObj.preventDefault();
      if (eventObj && eventObj.stopPropagation) eventObj.stopPropagation();
      flashPrivacyShield();
      logSecurityEvent(type, details);
    };
    els.panel.addEventListener('copy', (e) => blockAndLog('copy_blocked', 'copy', e), true);
    els.panel.addEventListener('cut', (e) => blockAndLog('copy_blocked', 'cut', e), true);
    els.panel.addEventListener('contextmenu', (e) => {
      if (!isProtectedDoc() || !canProtectClipboardAndPrint()) return;
      const target = e.target;
      if (target && (target.closest('#editor-content') || target.closest('#editor-md-view'))) {
        blockAndLog('copy_blocked', 'context_menu', e);
      }
    }, true);
    els.panel.addEventListener('keydown', (e) => {
      if (!isProtectedDoc() || !canProtectClipboardAndPrint()) return;
      if ((e.ctrlKey || e.metaKey) && String(e.key || '').toLowerCase() === 'c') {
        blockAndLog('copy_blocked', 'ctrl_c', e);
        return;
      }
      if (String(e.key || '').toLowerCase() === 'printscreen') {
        blockAndLog('screenshot_attempt', 'print_screen_key', e);
      }
    }, true);
  }

  function flashPrivacyShield() {
    if (!els.panel) return;
    els.panel.classList.add('docs-privacy-shield');
    setTimeout(() => els.panel && els.panel.classList.remove('docs-privacy-shield'), 800);
  }

  async function logSecurityEvent(eventType, details) {
    if (!currentDocId) return;
    const now = Date.now();
    if (now-lastSecurityEventAt < 1500) return;
    lastSecurityEventAt = now;
    try {
      await Api.post(`/api/docs/${currentDocId}/security-events`, {
        event_type: eventType,
        details: details || ''
      });
    } catch (_) {
      // swallow security telemetry errors on client side
    }
  }

  function applyMode() {
    if (!els.panel) return;
    const editable = canEditFormat(currentFormat);
    const binary = isBinaryFormat(currentFormat);
    const viewOnly = currentMode !== 'edit';
    els.panel.classList.toggle('view-only', viewOnly);
    els.panel.classList.toggle('binary-mode', binary);
    if (els.editToggle) {
      els.editToggle.hidden = false;
      els.editToggle.textContent = viewOnly ? (BerkutI18n.t('editor.edit') || 'Edit') : (BerkutI18n.t('editor.view') || 'View');
    }
    if (els.reason) els.reason.hidden = viewOnly;
    if (els.saveBtn) els.saveBtn.hidden = viewOnly;
    if (els.toolbar) els.toolbar.hidden = !isEditableFormat(currentFormat) || viewOnly;
    if (els.convertBtn) els.convertBtn.hidden = binary || viewOnly;
    if (els.onlyOfficeOpenBtn) els.onlyOfficeOpenBtn.hidden = true;
    if (els.downloadBtn) els.downloadBtn.hidden = true;
    const disableInputs = [
      els.classification,
      els.tags,
      els.linkType,
      els.linkId,
      els.aclRoles,
      els.aclUsers
    ];
    disableInputs.forEach(el => {
      if (!el) return;
      el.disabled = viewOnly;
    });
  }

  function renderMarkdownView(content) {
    if (!els.mdView) return;
    if (typeof DocsPage !== 'undefined' && DocsPage.renderMarkdown) {
      const rendered = DocsPage.renderMarkdown(content || '');
      els.mdView.innerHTML = rendered.html || '';
    } else {
      els.mdView.textContent = content || '';
    }
  }

  function mount(container) {
    if (!els.panel || !container) return;
    container.appendChild(els.panel);
  }

  function showAlert(msg, success = false) {
    if (!els.alert) return;
    if (!msg) {
      els.alert.hidden = true;
      els.alert.classList.remove('success');
      return;
    }
    els.alert.textContent = msg;
    els.alert.hidden = false;
    els.alert.classList.toggle('success', !!success);
  }

  function escapeHtml(str) {
    return (str || '').toString().replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', '\'': '&#39;' }[c]));
  }

  return { init, open, close, mount, isDirty, setMode };
})();

if (typeof window !== 'undefined') {
  window.DocEditor = DocEditor;
}
