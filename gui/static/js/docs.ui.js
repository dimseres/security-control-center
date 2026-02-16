(() => {
  const state = DocsPage.state;

  function bindUI() {
    const searchInput = document.getElementById('docs-search');
    const searchBtn = document.getElementById('btn-search');
    if (searchBtn) searchBtn.onclick = () => {
      state.filters.search = searchInput.value.trim();
      DocsPage.loadDocs();
    };
    if (searchInput) {
      searchInput.onkeypress = (e) => {
        if (e.key === 'Enter') {
          state.filters.search = searchInput.value.trim();
          DocsPage.loadDocs();
        }
      };
    }
    const statusSelect = document.getElementById('filter-status');
    if (statusSelect) statusSelect.onchange = () => {
      state.filters.status = statusSelect.value;
      DocsPage.loadDocs();
    };
    const tagsSelect = document.getElementById('filter-tags');
    if (tagsSelect) tagsSelect.onchange = () => {
      state.filters.tags = Array.from(tagsSelect.selectedOptions).map(o => o.value);
      DocsPage.loadDocs();
    };
    document.querySelectorAll('#quick-filters .chip').forEach(chip => {
      chip.onclick = () => {
        chip.classList.toggle('active');
        state.filters.mine = !!document.querySelector('.chip[data-filter="mine"].active');
        state.filters.review = !!document.querySelector('.chip[data-filter="review"].active');
        state.filters.secret = !!document.querySelector('.chip[data-filter="secret"].active');
        DocsPage.loadDocs();
      };
    });

    const newBtn = document.getElementById('btn-new-doc');
    if (newBtn) newBtn.onclick = () => DocsPage.openCreateModal();
    const importBtn = document.getElementById('btn-import-doc');
    if (importBtn) importBtn.onclick = () => {
      const input = document.getElementById('import-file-input');
      if (input) input.click();
    };
    const templatesBtn = document.getElementById('btn-templates');
    if (templatesBtn) templatesBtn.onclick = () => DocsPage.openTemplates();

    const fileInput = document.getElementById('import-file-input');
    if (fileInput) {
      fileInput.onchange = async (e) => {
        const file = e.target.files[0];
        if (!file) return;
        await DocsPage.startUpload(file);
        fileInput.value = '';
      };
    }

    bindModalClose();
    DocsPage.bindCreateForm();
    DocsPage.bindImportForm();
    DocsPage.bindFolderForm();
    DocsPage.bindTemplateForm();
    DocsPage.bindTemplateManagement();
    DocsPage.bindApprovalForm();
    bindContextMenu();
    DocsPage.bindViewerControls();
    renderTagFilters();
    document.addEventListener('tags:changed', renderTagFilters);
  }

  function bindModalClose() {
    document.querySelectorAll('[data-close]').forEach(btn => {
      btn.onclick = () => DocsPage.closeModal(btn.getAttribute('data-close'));
    });
  }

  function bindContextMenu() {
    hideContextMenu();
    document.addEventListener('click', hideContextMenu);
    window.addEventListener('resize', hideContextMenu);
    document.addEventListener('scroll', hideContextMenu, true);
    document.addEventListener('contextmenu', (e) => {
      if (!e.target.closest('#docs-table')) hideContextMenu();
    });
    const docsMain = document.querySelector('#docs-list-panel .docs-main');
    if (docsMain) {
      docsMain.addEventListener('contextmenu', (e) => {
        const inRow = e.target.closest('#docs-table tbody tr');
        if (inRow) return;
        e.preventDefault();
        e.stopPropagation();
        showContextMenu(e.clientX, e.clientY, { type: 'empty' });
      });
    }
    const table = document.getElementById('docs-table');
    if (table) {
      table.addEventListener('contextmenu', (e) => {
        const docRow = e.target.closest('tr[data-type="doc"]');
        const folderRow = e.target.closest('tr[data-type="folder"]');
        if (docRow && docRow.dataset.id) {
          e.preventDefault();
          e.stopPropagation();
          showContextMenu(e.clientX, e.clientY, { type: 'doc', docId: parseInt(docRow.dataset.id, 10) });
          return;
        }
        if (folderRow && folderRow.dataset.folderId) {
          e.preventDefault();
          e.stopPropagation();
          showContextMenu(e.clientX, e.clientY, { type: 'folder', folderId: parseInt(folderRow.dataset.folderId, 10) });
          return;
        }
        if (e.target.closest('tbody')) {
          e.preventDefault();
          showContextMenu(e.clientX, e.clientY, { type: 'empty' });
        }
      });
    }
  }

  function buildContextActions(ctx) {
    if (!ctx) return [];
    if (ctx.type === 'doc' && ctx.docId) {
      const docId = parseInt(ctx.docId, 10);
      const doc = DocsPage.state.docs.find(d => d.id === docId);
      return [
        { label: BerkutI18n.t('docs.menu.open'), handler: () => DocsPage.openDocTab(docId, 'view') },
        { label: BerkutI18n.t('docs.menu.edit'), handler: () => DocsPage.openDocTab(docId, 'edit') },
        { label: BerkutI18n.t('docs.menu.versions'), handler: () => DocsPage.openVersions(docId) },
        { label: BerkutI18n.t('docs.menu.approval'), handler: () => DocsPage.openApprovalModal(docId) },
        { label: `${BerkutI18n.t('docs.menu.export')} PDF`, handler: () => DocsPage.exportDoc(docId, 'pdf') },
        { label: `${BerkutI18n.t('docs.menu.export')} DOCX`, handler: () => DocsPage.exportDoc(docId, 'docx') },
        { label: `${BerkutI18n.t('docs.menu.export')} MD`, handler: () => DocsPage.exportDoc(docId, 'md') },
        { label: `${BerkutI18n.t('docs.menu.export')} JSON`, handler: () => DocsPage.exportDoc(docId, 'json') },
        { label: BerkutI18n.t('docs.menu.exportApprove'), handler: () => DocsPage.approveExport(docId) },
        { label: BerkutI18n.t('docs.menu.delete'), danger: true, handler: () => DocsPage.deleteDoc(docId) },
        {
          label: BerkutI18n.t('docs.menu.newDoc'),
          handler: () => {
            DocsPage.state.selectedFolder = doc?.folder_id || null;
            DocsPage.openCreateModal();
          }
        },
        {
          label: BerkutI18n.t('docs.menu.newFolder'),
          handler: () => DocsPage.openFolderModal(null)
        },
      ];
    }
    if (ctx.type === 'folder' && ctx.folderId) {
      const folderId = parseInt(ctx.folderId, 10);
      return [
        { label: BerkutI18n.t('docs.menu.openFolder'), handler: () => DocsPage.selectFolder(folderId) },
        { label: BerkutI18n.t('docs.menu.editFolder'), handler: () => DocsPage.openFolderModal(DocsPage.state.folderMap[folderId]) },
        { label: BerkutI18n.t('docs.menu.deleteFolder'), danger: true, handler: () => DocsPage.deleteFolder(folderId) },
        {
          label: BerkutI18n.t('docs.menu.newDoc'),
          handler: () => {
            DocsPage.state.selectedFolder = folderId;
            DocsPage.openCreateModal();
          }
        },
        {
          label: BerkutI18n.t('docs.menu.newFolder'),
          handler: () => DocsPage.openFolderModal(null)
        },
      ];
    }
    if (ctx.type === 'empty') {
      return [
        { label: BerkutI18n.t('docs.menu.newDoc'), handler: () => DocsPage.openCreateModal() },
        { label: BerkutI18n.t('docs.menu.newFolder'), handler: () => DocsPage.openFolderModal(null) },
      ];
    }
    return [];
  }

  function showContextMenu(x, y, ctx) {
    const menu = document.getElementById('doc-context-menu');
    if (!menu) return;
    const actions = buildContextActions(ctx);
    if (!actions.length) return;
    menu.innerHTML = '';
    actions.forEach(act => {
      const btn = document.createElement('button');
      btn.type = 'button';
      if (act.danger) btn.classList.add('danger');
      btn.textContent = act.label || '';
      btn.onclick = () => {
        hideContextMenu();
        act.handler();
      };
      menu.appendChild(btn);
    });
    const padding = 10;
    const maxX = window.innerWidth - menu.offsetWidth - padding;
    const maxY = window.innerHeight - menu.offsetHeight - padding;
    menu.style.left = `${Math.max(padding, Math.min(x, maxX))}px`;
    menu.style.top = `${Math.max(padding, Math.min(y, maxY))}px`;
    menu.hidden = false;
    menu.style.display = 'flex';
  }

  function hideContextMenu() {
    const menu = document.getElementById('doc-context-menu');
    if (menu) {
      menu.hidden = true;
      menu.style.display = 'none';
    }
  }

  function renderTagFilters() {
    const tagsSelect = document.getElementById('filter-tags');
    if (!tagsSelect) return;
    const available = DocUI.availableTags ? DocUI.availableTags() : [];
    const availableSet = new Set(available.map(tag => (tag.code || tag)));
    state.filters.tags = (state.filters.tags || []).filter(code => availableSet.has(code));
    const selected = new Set(state.filters.tags || []);
    tagsSelect.innerHTML = '';
    available.forEach(tag => {
      const code = tag.code || tag;
      const opt = document.createElement('option');
      opt.value = code;
      opt.textContent = DocUI.tagLabel ? DocUI.tagLabel(code) : (tag.label || code);
      opt.dataset.label = opt.textContent;
      if (selected.has(code)) opt.selected = true;
      tagsSelect.appendChild(opt);
    });
    DocsPage.enhanceMultiSelects([tagsSelect.id]);
    if (DocUI.bindTagHint) {
      DocUI.bindTagHint(tagsSelect, document.querySelector('[data-tag-hint="filter-tags"]'));
    }
  }

  DocsPage.bindUI = bindUI;
  DocsPage.bindModalClose = bindModalClose;
  DocsPage.bindContextMenu = bindContextMenu;
  DocsPage.buildContextActions = buildContextActions;
  DocsPage.showContextMenu = showContextMenu;
  DocsPage.hideContextMenu = hideContextMenu;
  DocsPage.renderTagFilters = renderTagFilters;
})();
