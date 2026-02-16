(() => {
  const state = DocsPage.state;
  const selectedValues = (selector) => Array.from(document.querySelector(selector)?.selectedOptions || []).map(o => o.value);
  const isEditableFormat = (format) => {
    const fmt = String(format || '').toLowerCase();
    if (fmt === 'md' || fmt === 'txt') return true;
    if (fmt !== 'docx') return false;
    return !!(window.DocsOnlyOffice && typeof window.DocsOnlyOffice.open === 'function');
  };

  function openCreateModal() {
    DocUI.populateClassificationSelect(document.getElementById('create-classification'));
    DocUI.renderTagCheckboxes('#create-tags', { name: 'tags' });
    DocsPage.loadUsersIntoSelects();
    DocsPage.syncFolderClassification('create');
    const folderSel = document.getElementById('create-folder');
    if (folderSel) folderSel.value = state.selectedFolder || '';
    DocsPage.openModal('#doc-create-modal');
  }

  function bindCreateForm() {
    const form = document.getElementById('create-form');
    const alertBox = document.getElementById('create-alert');
    const folderSel = document.getElementById('create-folder');
    if (folderSel) folderSel.onchange = () => DocsPage.syncFolderClassification('create');
    if (!form) return;
    form.onsubmit = async (e) => {
      e.preventDefault();
      DocsPage.hideAlert(alertBox);
      const data = DocsPage.formDataToObj(new FormData(form));
      const folderLock = DocsPage.folderClassification(DocsPage.parseNullableInt(data.folder_id));
      const payload = {
        title: data.title,
        folder_id: DocsPage.parseNullableInt(data.folder_id),
        classification_level: (folderLock && folderLock.levelCode) || data.classification_level || 'PUBLIC',
        classification_tags: selectedValues('#create-tags'),
        inherit_acl: form.querySelector('input[name="inherit_acl"]').checked,
      };
      let doc;
      try {
        doc = await Api.post('/api/docs', payload);
      } catch (err) {
        DocsPage.showAlert(alertBox, err.message);
        return;
      }
      if (doc && doc.id) {
        try {
          if (payload.folder_id) {
            state.selectedFolder = payload.folder_id;
          }
          addDocToState(doc);
          await DocsPage.loadDocs();
          await updateACL(doc.id, data);
          form.reset();
          DocsPage.closeModal('#doc-create-modal');
          openEditor(doc.id);
        } catch (err) {
          console.error('post-create handling', err);
          DocsPage.closeModal('#doc-create-modal');
          await DocsPage.loadDocs();
        }
      }
    };
  }

  async function startUpload(file) {
    const alertBox = document.getElementById('import-alert');
    DocsPage.hideAlert(alertBox);
    const fd = new FormData();
    fd.append('file', file);
    try {
      const res = await Api.upload('/api/docs/upload', fd);
      state.uploadCtx = { upload_id: res.upload_id, name: file.name, size: file.size };
      document.getElementById('import-file-name').textContent = `${file.name} (${Math.round(file.size / 1024)} KB)`;
      const titleInput = document.querySelector('#import-form input[name="title"]');
      if (titleInput) {
        titleInput.value = file.name.replace(/\.[^/.]+$/, '');
      }
      DocUI.populateClassificationSelect(document.getElementById('import-classification'));
      DocUI.renderTagCheckboxes('#import-tags', { name: 'tags' });
      DocsPage.loadUsersIntoSelects();
      const folderSel = document.getElementById('import-folder');
      if (folderSel) folderSel.value = state.selectedFolder || '';
      DocsPage.syncFolderClassification('import');
      DocsPage.openModal('#doc-import-modal');
    } catch (err) {
      DocsPage.showAlert(alertBox, err.message || BerkutI18n.t('docs.importFailed'));
    }
  }

  function bindImportForm() {
    const form = document.getElementById('import-form');
    const alertBox = document.getElementById('import-alert');
    const folderSel = document.getElementById('import-folder');
    if (folderSel) folderSel.onchange = () => DocsPage.syncFolderClassification('import');
    if (!form) return;
    form.onsubmit = async (e) => {
      e.preventDefault();
      DocsPage.hideAlert(alertBox);
      if (!state.uploadCtx || !state.uploadCtx.upload_id) {
        DocsPage.showAlert(alertBox, BerkutI18n.t('docs.importNoFile'));
        return;
      }
      const data = DocsPage.formDataToObj(new FormData(form));
      const folderLock = DocsPage.folderClassification(DocsPage.parseNullableInt(data.folder_id));
      const payload = {
        upload_id: state.uploadCtx.upload_id,
        title: data.title,
        reg_number: data.reg_number,
        folder_id: DocsPage.parseNullableInt(data.folder_id),
        classification_level: (folderLock && folderLock.levelCode) || data.classification_level,
        classification_tags: selectedValues('#import-tags'),
        acl_roles: DocsPage.toArray(data.acl_roles),
        acl_users: DocsPage.toArray(data.acl_users).map(v => parseInt(v, 10)).filter(Boolean),
        inherit_acl: form.querySelector('input[name="inherit_acl"]').checked,
        owner: DocsPage.parseNullableInt(data.owner),
      };
      try {
        const res = await Api.post('/api/docs/import/commit', payload);
        DocsPage.closeModal('#doc-import-modal');
        state.uploadCtx = null;
        if (res && res.id) {
          await updateACL(res.id, data);
        }
        if (payload.folder_id) {
          state.selectedFolder = payload.folder_id;
        }
        try {
          await DocsPage.loadDocs();
        } catch (err) {
          console.error('load docs after import', err);
        }
        if (res && res.id) openEditor(res.id);
      } catch (err) {
        DocsPage.showAlert(alertBox, err.message || BerkutI18n.t('docs.importFailed'));
      }
    };
  }

  async function updateACL(docId, data) {
    const rolesSel = Array.isArray(data.acl_roles) ? data.acl_roles : (data.acl_roles ? [data.acl_roles] : []);
    const rawUsers = Array.isArray(data.acl_users) ? data.acl_users : (data.acl_users ? [data.acl_users] : []);
    let usersSel = rawUsers.filter(Boolean).map(Number);
    if (!rolesSel.length && !usersSel.length && !data.owner && state.currentUser) {
      rawUsers.push(state.currentUser.id);
      usersSel = rawUsers.filter(Boolean).map(Number);
    }
    const seen = new Set();
    const acl = [];
    const pushRule = (subjectType, subjectId, permission) => {
      if (!subjectType || !subjectId || !permission) return;
      const key = `${subjectType}|${subjectId}|${permission}`;
      if (seen.has(key)) return;
      seen.add(key);
      acl.push({ subject_type: subjectType, subject_id: subjectId, permission });
    };
    rolesSel.forEach(r => {
      ['view', 'edit'].forEach(p => pushRule('role', r, p));
    });
    usersSel.forEach(uid => {
      const u = UserDirectory.get(uid);
      if (u) {
        ['view', 'edit'].forEach(p => pushRule('user', u.username, p));
      }
    });
    if (data.owner) {
      const u = UserDirectory.get(parseInt(data.owner, 10));
      if (u) {
        ['view', 'edit', 'manage'].forEach(p => pushRule('user', u.username, p));
      }
    }
    try {
      await Api.put(`/api/docs/${docId}/acl`, { acl });
    } catch (err) {
      console.warn('ACL update failed', err);
    }
  }

  function addDocToState(doc) {
    if (!doc) return;
    if (!state.docs) state.docs = [];
    const idx = state.docs.findIndex(d => d.id === doc.id);
    if (idx >= 0) {
      state.docs[idx] = doc;
    } else {
      if (!state.selectedFolder || state.selectedFolder === doc.folder_id || (state.selectedFolder === null && !doc.folder_id)) {
        state.docs.unshift(doc);
      }
    }
  }

  async function openEditor(docId) {
    try {
      const doc = (state.docs || []).find(d => d.id === docId);
      const mode = isEditableFormat(doc?.format) ? 'edit' : 'view';
      if (DocsPage.openDocTab) {
        DocsPage.openDocTab(docId, mode);
      } else {
        if (DocsPage.updateDocsPath) {
          DocsPage.updateDocsPath(docId, mode);
        }
        await DocEditor.open(docId, { mode });
      }
    } catch (err) {
      console.error('open editor', err);
    }
  }

  async function deleteDoc(docId) {
    const ok = DocsPage.confirmAction
      ? await DocsPage.confirmAction({
        title: BerkutI18n.t('docs.menu.delete'),
        message: BerkutI18n.t('docs.deleteConfirm'),
        confirmText: BerkutI18n.t('common.delete'),
        cancelText: BerkutI18n.t('common.cancel'),
      })
      : confirm(BerkutI18n.t('docs.deleteConfirm'));
    if (!ok) return;
    await Api.del(`/api/docs/${docId}`);
    await DocsPage.loadDocs();
  }

  async function openVersions(docId) {
    const alertBox = document.getElementById('versions-alert');
    DocsPage.hideAlert(alertBox);
    try {
      const res = await Api.get(`/api/docs/${docId}/versions`);
      renderVersions(res.versions || [], docId);
      DocsPage.openModal('#versions-modal');
    } catch (err) {
      DocsPage.showAlert(alertBox, err.message);
    }
  }

  function renderVersions(list, docId) {
    const tbody = document.querySelector('#versions-table tbody');
    if (!tbody) return;
    tbody.innerHTML = '';
    list.slice(0, 10).forEach(v => {
      const tr = document.createElement('tr');
      tr.innerHTML = `
        <td>${v.version}</td>
        <td>${DocsPage.escapeHtml(v.author_username || '')}</td>
        <td>${DocsPage.escapeHtml(v.reason || '')}</td>
        <td>${DocsPage.formatDate(v.created_at)}</td>
        <td>
          <button class="btn ghost" data-view="${v.version}">${BerkutI18n.t('docs.view')}</button>
          <button class="btn ghost" data-restore="${v.version}">${BerkutI18n.t('docs.restore')}</button>
        </td>
      `;
      tr.querySelector('[data-view]').onclick = () => viewVersion(docId, v.version);
      tr.querySelector('[data-restore]').onclick = () => restoreVersion(docId, v.version);
      tbody.appendChild(tr);
    });
  }

  async function viewVersion(docId, ver) {
    const viewer = document.getElementById('version-viewer');
    const label = document.getElementById('version-label');
    const contentEl = document.getElementById('version-content');
    if (!viewer || !label || !contentEl) return;
    const res = await Api.get(`/api/docs/${docId}/versions/${ver}/content`);
    label.textContent = `${BerkutI18n.t('docs.version')} ${res.version} (${res.format})`;
    contentEl.textContent = res.content || '';
    viewer.hidden = false;
    const closeBtn = document.getElementById('close-version-view');
    if (closeBtn) closeBtn.onclick = () => { viewer.hidden = true; };
  }

  async function restoreVersion(docId, ver) {
    if (!confirm(BerkutI18n.t('docs.restoreConfirm'))) return;
    await Api.post(`/api/docs/${docId}/versions/${ver}/restore`, {});
    await DocsPage.loadDocs();
    DocsPage.closeModal('#versions-modal');
  }

  DocsPage.openCreateModal = openCreateModal;
  DocsPage.bindCreateForm = bindCreateForm;
  DocsPage.startUpload = startUpload;
  DocsPage.bindImportForm = bindImportForm;
  DocsPage.updateACL = updateACL;
  DocsPage.addDocToState = addDocToState;
  DocsPage.openEditor = openEditor;
  DocsPage.deleteDoc = deleteDoc;
  DocsPage.openVersions = openVersions;
  DocsPage.renderVersions = renderVersions;
  DocsPage.viewVersion = viewVersion;
  DocsPage.restoreVersion = restoreVersion;
})();
