(() => {
  const state = TasksPage.state;
  const {
    t,
    hasPermission,
    showAlert,
    hideAlert,
    openModal,
    closeModal,
    resolveErrorMessage,
    showError,
    formatDateTime,
    toInputDate,
    toISODate,
    escapeHtml,
    confirmAction,
    applyTagStyle
  } = TasksPage;

  let links = [];
  let controlLinks = [];
  let blocks = [];
  let blocking = [];
  let blockTitles = {};
  let taskFiles = [];
  let comments = [];
  let editingCommentId = null;
  const taskTitleCache = {};
  const taskTitlePending = new Set();
  let pendingBlocker = null;
  const relationSelections = new Map();
  let activeRelationPicker = null;
  let relationDropdownOpen = false;
  let relationDropdownLoading = false;
  let linkSelection = null;
  let linkDropdownOpen = false;
  let linkDropdownLoading = false;
  const linkOptions = { doc: [], incident: [], control: [] };
  const linkOptionsLoaded = { doc: false, incident: false, control: false };
  let pendingCommentFiles = [];
  let suppressAutoSave = false;
  let assigneesSnapshot = [];
  const blockFieldIds = [
    'task-modal-tags-field',
    'task-modal-checklist-field',
    'task-modal-relations-parent-field',
    'task-modal-relations-child-field',
    'task-modal-external-link-field',
    'task-modal-files-field',
    'task-modal-size-field',
    'task-modal-business-field',
    'task-modal-links-field',
    'task-modal-controls-field'
  ];
  const singleInstanceBlocks = new Set(['tags', 'relations_child', 'business_customer', 'size']);
  const blockOrderStorageKey = 'tasks.blockOrder';
  const inlineFieldConfig = {
    external_link: {
      inputId: 'task-modal-external-link',
      viewId: 'task-modal-external-link-view',
      editorId: 'task-modal-external-link-editor',
      getValue: () => (state.card.original?.external_link || '').trim(),
      parse: raw => (raw || '').trim(),
      format: value => value
    },
    business_customer: {
      inputId: 'task-modal-business',
      viewId: 'task-modal-business-view',
      editorId: 'task-modal-business-editor',
      getValue: () => (state.card.original?.business_customer || '').trim(),
      parse: raw => (raw || '').trim(),
      format: value => value
    },
    size_estimate: {
      inputId: 'task-modal-size',
      viewId: 'task-modal-size-view',
      editorId: 'task-modal-size-editor',
      getValue: () => state.card.original?.size_estimate,
      parse: raw => {
        const cleaned = (raw || '').trim();
        if (!cleaned) return null;
        const parsed = parseInt(cleaned, 10);
        return Number.isNaN(parsed) ? null : parsed;
      },
      format: value => (typeof value === 'number' ? `${value}` : '')
    }
  };

  function initSidebar() {
    const modal = document.getElementById('task-modal');
    if (!modal || modal.dataset.bound === '1') return;
    modal.dataset.bound = '1';

    modal.addEventListener('click', (e) => {
      const menuBtn = e.target.closest('.task-block-menu');
      if (!menuBtn) return;
      const block = menuBtn.dataset.blockMenu;
      if (block) openBlockMenu(menuBtn, block);
    });

    document.addEventListener('click', (e) => {
      if (e.target.closest('#task-block-context-menu')) return;
      if (e.target.closest('.task-block-menu') || e.target.closest('[data-block-add]') || e.target.closest('#task-modal-add-block')) return;
      hideBlockContextMenu();
    });
    document.addEventListener('keydown', (e) => {
      if (e.key === 'Escape') hideBlockContextMenu();
    });
    document.getElementById('task-modal-close')?.addEventListener('click', closeTaskModal);
    document.getElementById('task-modal-copy-link')?.addEventListener('click', () => copyTaskLink(state.card.taskId));
    document.getElementById('task-modal-add-block')?.addEventListener('click', (e) => {
      e.preventDefault();
      e.stopPropagation();
      openBlockAddMenu(e.currentTarget);
    });
    document.getElementById('task-modal-location-link')?.addEventListener('click', () => focusTaskLocation());

    const titleDisplay = document.getElementById('task-modal-title-display');
    const titleSave = document.getElementById('task-modal-title-save');
    const titleCancel = document.getElementById('task-modal-title-cancel');
    const titleInput = document.getElementById('task-modal-title-input');
    titleDisplay?.addEventListener('dblclick', openTitleEditor);
    titleDisplay?.addEventListener('click', (e) => {
      if (e.detail === 2) openTitleEditor();
    });
    modal.addEventListener('dblclick', (e) => {
      if (e.target.closest('#task-modal-title-display')) openTitleEditor();
    });
    titleSave?.addEventListener('click', saveTitleEditor);
    titleCancel?.addEventListener('click', cancelTitleEditor);
    titleInput?.addEventListener('keydown', (e) => {
      if (e.key === 'Enter') {
        e.preventDefault();
        saveTitleEditor();
      }
      if (e.key === 'Escape') {
        e.preventDefault();
        cancelTitleEditor();
      }
    });

    const descView = document.getElementById('task-modal-description-view');
    const descSave = document.getElementById('task-modal-description-save');
    const descCancel = document.getElementById('task-modal-description-cancel');
    descView?.addEventListener('click', () => openTextEditor('description'));
    descSave?.addEventListener('click', () => saveTextEditor('description'));
    descCancel?.addEventListener('click', () => cancelTextEditor('description'));

    const resView = document.getElementById('task-modal-result-view');
    const resSave = document.getElementById('task-modal-result-save');
    const resCancel = document.getElementById('task-modal-result-cancel');
    resView?.addEventListener('click', () => openTextEditor('result'));
    resSave?.addEventListener('click', () => saveTextEditor('result'));
    resCancel?.addEventListener('click', () => cancelTextEditor('result'));

    document.getElementById('task-modal-link-add')?.addEventListener('click', addLink);
    document.getElementById('task-modal-relation-parent')?.addEventListener('click', () => addRelation('task_parent', 'task-modal-relation-parent-input'));
    document.getElementById('task-modal-relation-child')?.addEventListener('click', () => addRelation('task_child', 'task-modal-relation-child-input'));
    document.getElementById('task-modal-checklist-add')?.addEventListener('click', addChecklistItem);

    const tagInput = document.getElementById('task-modal-tag-input');
    tagInput?.addEventListener('keydown', (e) => {
      if (e.key === 'Enter') {
        e.preventDefault();
        addTag(tagInput.value);
        tagInput.value = '';
      }
    });

    bindRelationPicker('task-modal-relation-parent-input', 'task-modal-relation-parent-dropdown');
    bindRelationPicker('task-modal-relation-child-input', 'task-modal-relation-child-dropdown');
    bindLinkPicker();

    const assigneesSel = document.getElementById('task-modal-assignees');
    const assigneesSearch = document.getElementById('task-modal-assignees-search');
    const assigneesWrap = document.getElementById('task-modal-assignees-wrap');
    assigneesSel?.addEventListener('change', () => {
      renderAssigneesHint(assigneesSel);
      syncAssigneesList(assigneesSel);
      autoSaveAssignees();
    });
    assigneesSel?.addEventListener('selectionrefresh', () => {
      renderAssigneesHint(assigneesSel);
      syncAssigneesList(assigneesSel);
      autoSaveAssignees();
    });
    assigneesSearch?.addEventListener('input', () => {
      if (assigneesSel) filterAssigneesOptions(assigneesSel, assigneesSearch.value);
      toggleAssigneesList(true);
    });
    assigneesSearch?.addEventListener('focus', () => toggleAssigneesList(true));
    assigneesSearch?.addEventListener('click', () => toggleAssigneesList(true));
    document.addEventListener('click', (e) => {
      if (assigneesWrap && !e.target.closest('#task-modal-assignees-wrap')) {
        toggleAssigneesList(false);
      }
      if (!e.target.closest('.task-relation-picker') && !e.target.closest('.task-relation-dropdown')) {
        toggleRelationDropdown(false);
      }
      if (!e.target.closest('#task-modal-link-search') && !e.target.closest('#task-modal-link-dropdown')) {
        toggleLinkDropdown(false);
      }
    });

    document.getElementById('task-modal-block-btn')?.addEventListener('click', openBlockModal);
    document.getElementById('task-block-close')?.addEventListener('click', () => closeModal('task-block-modal'));
    document.getElementById('task-block-cancel')?.addEventListener('click', () => closeModal('task-block-modal'));
    document.getElementById('task-block-save')?.addEventListener('click', saveBlock);
    document.getElementById('task-block-add-link')?.addEventListener('click', openBlockerModal);
    document.getElementById('task-blocker-close')?.addEventListener('click', () => closeModal('task-blocker-modal'));

    const filesInput = document.getElementById('task-files-input');
    const filesUpload = document.getElementById('task-files-upload');
    filesUpload?.addEventListener('click', () => filesInput?.click());
    filesInput?.addEventListener('change', () => {
      if (filesInput?.files?.length) uploadTaskFiles(Array.from(filesInput.files));
      if (filesInput) filesInput.value = '';
    });

    const commentInput = document.getElementById('task-comment-input');
    const commentAttach = document.getElementById('task-comment-attach');
    const commentFile = document.getElementById('task-comment-file');
    const commentSubmit = document.getElementById('task-comment-submit');
    commentAttach?.addEventListener('click', () => commentFile?.click());
    commentFile?.addEventListener('change', () => {
      if (commentFile?.files?.length) addCommentFiles(Array.from(commentFile.files));
      if (commentFile) commentFile.value = '';
    });
    commentSubmit?.addEventListener('click', submitComment);
    commentInput?.addEventListener('keydown', (e) => {
      if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') {
        e.preventDefault();
        submitComment();
      }
    });
    commentInput?.addEventListener('paste', handleCommentPaste);
    commentInput?.addEventListener('dragover', (e) => {
      e.preventDefault();
    });
    commentInput?.addEventListener('drop', handleCommentDrop);

    modal.addEventListener('click', (e) => {
      const addBtn = e.target.closest('[data-block-add]');
      if (!addBtn) return;
      e.preventDefault();
      e.stopPropagation();
      addFunctionalBlock(addBtn.dataset.blockAdd);
    });
    modal.addEventListener('click', (e) => {
      const view = e.target.closest('[data-inline-view]');
      if (view) {
        e.preventDefault();
        e.stopPropagation();
        openInlineEditor(view.dataset.inlineView);
        return;
      }
      const saveBtn = e.target.closest('[data-inline-save]');
      if (saveBtn) {
        e.preventDefault();
        e.stopPropagation();
        saveInlineEditor(saveBtn.dataset.inlineSave);
        return;
      }
      const cancelBtn = e.target.closest('[data-inline-cancel]');
      if (cancelBtn) {
        e.preventDefault();
        e.stopPropagation();
        cancelInlineEditor(cancelBtn.dataset.inlineCancel);
      }
    });

    const prioritySel = document.getElementById('task-modal-priority');
    prioritySel?.addEventListener('change', () => {
      if (prioritySel.disabled || suppressAutoSave) return;
      updateTask({ priority: prioritySel.value }).catch(() => {});
    });

    const dueInput = document.getElementById('task-modal-due');
    dueInput?.addEventListener('change', () => {
      if (dueInput.disabled || suppressAutoSave) return;
      const value = dueInput.value ? toISODate(dueInput.value) : '';
      updateTask({ due_date: value }).catch(() => {});
    });

    bindBlockDrag();
    bindInlineEditors();
  }

  async function openTask(taskId) {
    if (!taskId) return;
    hideAlert('task-modal-alert');
    try {
      await TasksPage.ensureUserDirectory();
      const task = await Api.get(`/api/tasks/${taskId}`);
      if (TasksPage.updateTaskHash) TasksPage.updateTaskHash(taskId);
      setTaskState(task);
      renderTaskModal(task);
      resetCommentForm();
      openModal('task-modal');
      await Promise.all([loadLinks(taskId), loadControlLinks(taskId), loadBlocks(taskId), loadFiles(taskId), loadComments(taskId)]);
    } catch (err) {
      showError(err, 'common.error');
    }
  }

  function setTaskState(task) {
    state.card.taskId = task?.id || null;
    state.card.original = task || null;
    state.card.tags = Array.isArray(task?.tags) ? task.tags.slice() : [];
    state.card.checklist = Array.isArray(task?.checklist)
      ? task.checklist.map(item => ({
        text: item.text || '',
        done: !!item.done,
        done_by: item.done_by || null,
        done_at: item.done_at || null
      }))
      : [];
    state.card.externalLink = task?.external_link || '';
    state.card.businessCustomer = task?.business_customer || '';
    state.card.forcedBlocks = getForcedBlocks(task?.id || null);
  }

  function getForcedBlocks(taskId) {
    if (!taskId) return new Set();
    try {
      const raw = localStorage.getItem(`tasks.taskBlocks.${taskId}`);
      const parsed = raw ? JSON.parse(raw) : [];
      if (!Array.isArray(parsed)) return new Set();
      return new Set(parsed);
    } catch (_) {
      return new Set();
    }
  }

  function saveForcedBlocks(taskId, forced) {
    if (!taskId) return;
    try {
      localStorage.setItem(`tasks.taskBlocks.${taskId}`, JSON.stringify(Array.from(forced)));
    } catch (_) {
      // ignore
    }
  }

  function resetForcedBlockMarkers() {
    blockFieldIds.forEach(id => {
      const field = document.getElementById(id);
      if (field) delete field.dataset.forced;
    });
  }

  function applyForcedBlocks() {
    const forced = state.card.forcedBlocks || new Set();
    blockFieldIds.forEach(id => {
      const field = document.getElementById(id);
      if (!field) return;
      if (forced.has(field.dataset.block)) {
        field.dataset.forced = '1';
      }
    });
  }

  function getBlockOrder() {
    try {
      const raw = localStorage.getItem(blockOrderStorageKey);
      const parsed = raw ? JSON.parse(raw) : [];
      return Array.isArray(parsed) ? parsed : [];
    } catch (_) {
      return [];
    }
  }

  function saveBlockOrder(order) {
    try {
      localStorage.setItem(blockOrderStorageKey, JSON.stringify(order));
    } catch (_) {
      // ignore
    }
  }

  function applyBlockOrder() {
    const container = document.querySelector('#task-modal .task-functional-blocks');
    if (!container) return;
    const order = getBlockOrder();
    if (!order.length) return;
    const blocks = Array.from(container.querySelectorAll('.task-block'));
    const map = new Map(blocks.map(el => [el.dataset.block, el]));
    const next = [];
    order.forEach(key => {
      if (map.has(key)) {
        next.push(map.get(key));
        map.delete(key);
      }
    });
    blocks.forEach(el => {
      if (map.has(el.dataset.block)) next.push(el);
    });
    next.forEach(el => container.appendChild(el));
  }

  function persistBlockOrder(container) {
    if (!container) return;
    const order = Array.from(container.querySelectorAll('.task-block'))
      .map(el => el.dataset.block)
      .filter(Boolean);
    if (order.length) saveBlockOrder(order);
  }

  function bindBlockDrag() {
    const container = document.querySelector('#task-modal .task-functional-blocks');
    if (!container || container.dataset.dragBound === '1') return;
    container.dataset.dragBound = '1';
    let dragging = null;
    container.addEventListener('dragstart', (e) => {
      const block = e.target.closest('.task-block');
      if (!block || !block.draggable) return;
      dragging = block;
      block.classList.add('dragging');
      if (e.dataTransfer) e.dataTransfer.effectAllowed = 'move';
    });
    container.addEventListener('dragover', (e) => {
      if (!dragging) return;
      const target = e.target.closest('.task-block');
      if (!target || target === dragging) return;
      e.preventDefault();
      const rect = target.getBoundingClientRect();
      const after = e.clientY - rect.top > rect.height / 2;
      container.insertBefore(dragging, after ? target.nextSibling : target);
    });
    container.addEventListener('drop', (e) => {
      if (!dragging) return;
      e.preventDefault();
      persistBlockOrder(container);
    });
    container.addEventListener('dragend', () => {
      if (dragging) dragging.classList.remove('dragging');
      dragging = null;
      persistBlockOrder(container);
    });
  }

  function setBlockDragState(enabled) {
    const container = document.querySelector('#task-modal .task-functional-blocks');
    if (!container) return;
    container.querySelectorAll('.task-block').forEach(block => {
      block.draggable = !!enabled;
    });
  }

  function bindInlineEditors() {
    Object.keys(inlineFieldConfig).forEach((fieldKey) => {
      const cfg = inlineFieldConfig[fieldKey];
      const input = document.getElementById(cfg.inputId);
      if (!input || input.dataset.inlineBound === '1') return;
      input.dataset.inlineBound = '1';
      input.addEventListener('keydown', (e) => {
        if (e.key === 'Enter') {
          e.preventDefault();
          saveInlineEditor(fieldKey);
        }
        if (e.key === 'Escape') {
          e.preventDefault();
          cancelInlineEditor(fieldKey);
        }
      });
    });
  }

  function renderTaskModal(task) {
    if (!task) return;
    const prevSuppress = suppressAutoSave;
    suppressAutoSave = true;
    const canEdit = hasPermission('tasks.edit') && !task.closed_at && !task.is_archived;
    const canAssign = hasPermission('tasks.assign');
    const titleDisplay = document.getElementById('task-modal-title-display');
    const titleEdit = document.getElementById('task-modal-title-edit');
    const titleInput = document.getElementById('task-modal-title-input');
    const titleValue = task.title || '';
    if (titleDisplay) {
      titleDisplay.textContent = titleValue || t('tasks.fields.title');
      titleDisplay.classList.toggle('muted', !titleValue);
      titleDisplay.title = titleValue || '';
      titleDisplay.hidden = false;
    }
    if (titleInput) {
      titleInput.value = titleValue;
      titleInput.disabled = !canEdit;
    }
    if (titleEdit) titleEdit.hidden = true;

    renderPriorityOptions(task.priority);

    const sizeInput = document.getElementById('task-modal-size');
    if (sizeInput) {
      sizeInput.value = typeof task.size_estimate === 'number' ? `${task.size_estimate}` : '';
      sizeInput.disabled = !canEdit;
    }

    const dueInput = document.getElementById('task-modal-due');
    if (dueInput) {
      dueInput.value = task.due_date ? toInputDate(task.due_date) : '';
      dueInput.lang = 'ru';
      dueInput.disabled = !canEdit;
    }

    const externalInput = document.getElementById('task-modal-external-link');
    if (externalInput) {
      externalInput.value = task.external_link || '';
      externalInput.disabled = !canEdit;
    }

    const businessInput = document.getElementById('task-modal-business');
    if (businessInput) {
      businessInput.value = task.business_customer || '';
      businessInput.disabled = !canEdit;
    }

    renderAssignees(task, canEdit && canAssign);
    renderTextView('description', task.description, t('tasks.fields.descriptionHint'));
    renderTextView('result', task.result, t('tasks.fields.resultHint'));
    resetForcedBlockMarkers();
    applyForcedBlocks();
    renderTags();
    renderChecklist();
    renderLinks();
    renderRelations();
    relationSelections.clear();
    ['task-modal-relation-parent-input', 'task-modal-relation-child-input'].forEach((id) => {
      const relationInput = document.getElementById(id);
      if (relationInput) relationInput.value = '';
    });
    toggleRelationDropdown(false);
    linkSelection = null;
    const linkInput = document.getElementById('task-modal-link-search');
    if (linkInput) linkInput.value = '';
    toggleLinkDropdown(false);
    renderInlineField('external_link');
    renderInlineField('business_customer');
    renderInlineField('size_estimate');
    renderExternalLink();
    renderBusinessCustomer();
    renderSizeField();
    renderFiles();
    renderBlocks();
    renderComments();
    applyBlockOrder();
    setBlockDragState(canEdit);
    setCommentFormState(hasPermission('tasks.comment') && !task.closed_at && !task.is_archived);
    document.querySelectorAll('#task-modal [data-block-add]').forEach((btn) => {
      btn.disabled = !canEdit;
    });
    const addBlockBtn = document.getElementById('task-modal-add-block');
    if (addBlockBtn) addBlockBtn.disabled = !canEdit;
    ['task-modal-relation-parent-input', 'task-modal-relation-child-input'].forEach((id) => {
      const input = document.getElementById(id);
      if (input) input.disabled = !canEdit;
    });
    const relationParent = document.getElementById('task-modal-relation-parent');
    const relationChild = document.getElementById('task-modal-relation-child');
    if (relationParent) relationParent.disabled = !canEdit;
    if (relationChild) relationChild.disabled = !canEdit;
    if (linkInput) linkInput.disabled = !canEdit;
    const linkType = document.getElementById('task-modal-link-type');
    const linkAdd = document.getElementById('task-modal-link-add');
    if (linkType) linkType.disabled = !canEdit;
    if (linkAdd) linkAdd.disabled = !canEdit;

    suppressAutoSave = prevSuppress;
  }

  function openTitleEditor() {
    if (!hasPermission('tasks.edit') || state.card.original?.closed_at || state.card.original?.is_archived) {
      return;
    }
    const titleEdit = document.getElementById('task-modal-title-edit');
    const titleInput = document.getElementById('task-modal-title-input');
    const titleDisplay = document.getElementById('task-modal-title-display');
    if (titleDisplay) titleDisplay.hidden = true;
    if (titleEdit) titleEdit.hidden = false;
    if (titleInput) {
      titleInput.value = state.card.original?.title || '';
      titleInput.focus();
      titleInput.select();
    }
  }

  function cancelTitleEditor() {
    const titleEdit = document.getElementById('task-modal-title-edit');
    const titleDisplay = document.getElementById('task-modal-title-display');
    if (titleEdit) titleEdit.hidden = true;
    if (titleDisplay) titleDisplay.hidden = false;
  }

  async function saveTitleEditor() {
    if (!state.card.taskId) return;
    const titleInput = document.getElementById('task-modal-title-input');
    if (!titleInput) return;
    const value = (titleInput.value || '').trim();
    await updateTask({ title: value }, () => {
      const titleEdit = document.getElementById('task-modal-title-edit');
      const titleDisplay = document.getElementById('task-modal-title-display');
      if (titleEdit) titleEdit.hidden = true;
      if (titleDisplay) titleDisplay.hidden = false;
    });
  }

  function autoSaveAssignees() {
    const select = document.getElementById('task-modal-assignees');
    if (!select || select.disabled || suppressAutoSave) return;
    const assignees = Array.from(select.selectedOptions || []).map(o => o.value);
    if (assignees.length === assigneesSnapshot.length && assignees.every((val, idx) => val === assigneesSnapshot[idx])) {
      return;
    }
    assigneesSnapshot = assignees.slice();
    updateTask({ assigned_to: assignees }).catch(() => {});
  }

  function addFunctionalBlock(block) {
    if (!block) return;
    if (!hasPermission('tasks.edit') || state.card.original?.closed_at || state.card.original?.is_archived) {
      return;
    }
    focusBlock(block);
  }

  function renderPriorityOptions(selected) {
    const select = document.getElementById('task-modal-priority');
    if (!select) return;
    const options = ['low', 'medium', 'high', 'critical'];
    select.innerHTML = '';
    options.forEach(val => {
      const opt = document.createElement('option');
      opt.value = val;
      opt.textContent = t(`tasks.priority.${val}`);
      if (val === (selected || 'medium').toLowerCase()) opt.selected = true;
      select.appendChild(opt);
    });
    select.disabled = !(hasPermission('tasks.edit') && !state.card.original?.closed_at && !state.card.original?.is_archived);
  }

  function renderAssignees(task, editable) {
    const select = document.getElementById('task-modal-assignees');
    if (!select) return;
    const search = document.getElementById('task-modal-assignees-search');
    const dir = (typeof window !== 'undefined' && window.UserDirectory)
      ? window.UserDirectory
      : (typeof UserDirectory !== 'undefined' ? UserDirectory : null);
    const users = dir?.all ? dir.all() : [];
    select.innerHTML = '';
    users.forEach(u => {
      const opt = document.createElement('option');
      opt.value = u.username || `${u.id}`;
      opt.textContent = u.full_name || u.username;
      opt.dataset.label = opt.textContent;
      select.appendChild(opt);
    });
    const assignedIds = Array.isArray(task?.assigned_to) ? task.assigned_to : [];
    assignedIds.forEach(id => {
      const user = dir?.get ? dir.get(id) : users.find(u => u.id === id);
      const token = user?.username || `${id}`;
      const option = Array.from(select.options).find(o => o.value === token);
      if (option) option.selected = true;
    });
    select.disabled = !editable;
    if (search) search.disabled = !editable;
    assigneesSnapshot = Array.from(select.selectedOptions || []).map(o => o.value);
    renderAssigneesHint(select);
    renderAssigneesList(select);
    applyAssigneesFilter(select);
    toggleAssigneesList(false);
  }

  function filterAssigneesOptions(select, query) {
    const term = (query || '').trim().toLowerCase();
    const list = document.getElementById('task-modal-assignees-list');
    if (!list) return;
    Array.from(list.querySelectorAll('label')).forEach(row => {
      const label = (row.dataset.label || row.textContent || '').toLowerCase();
      row.hidden = !!term && !label.includes(term);
    });
  }

  function applyAssigneesFilter(select) {
    const search = document.getElementById('task-modal-assignees-search');
    if (!search) return;
    filterAssigneesOptions(select, search.value);
  }

  function renderAssigneesList(select) {
    const list = document.getElementById('task-modal-assignees-list');
    if (!list || !select) return;
    list.innerHTML = '';
    Array.from(select.options).forEach(opt => {
      const row = document.createElement('label');
      row.dataset.value = opt.value;
      row.dataset.label = opt.dataset.label || opt.textContent || '';
      const box = document.createElement('input');
      box.type = 'checkbox';
      box.checked = opt.selected;
      box.addEventListener('change', () => {
        opt.selected = box.checked;
        select.dispatchEvent(new Event('change', { bubbles: true }));
      });
      const text = document.createElement('span');
      text.textContent = opt.dataset.label || opt.textContent || '';
      row.appendChild(box);
      row.appendChild(text);
      list.appendChild(row);
    });
  }

  function syncAssigneesList(select) {
    const list = document.getElementById('task-modal-assignees-list');
    if (!list || !select) return;
    const map = new Map(Array.from(select.options).map(opt => [opt.value, opt.selected]));
    Array.from(list.querySelectorAll('label')).forEach(row => {
      const value = row.dataset.value || '';
      const input = row.querySelector('input[type="checkbox"]');
      if (input && map.has(value)) {
        input.checked = map.get(value);
      }
    });
  }

  function toggleAssigneesList(visible) {
    const list = document.getElementById('task-modal-assignees-list');
    if (!list) return;
    list.hidden = !visible;
  }

  function bindRelationPicker(inputId, dropdownId) {
    const input = document.getElementById(inputId);
    const dropdown = document.getElementById(dropdownId);
    if (!input || !dropdown || input.dataset.relationBound === '1') return;
    input.dataset.relationBound = '1';
    input.addEventListener('focus', () => toggleRelationDropdown(true, inputId, dropdownId));
    input.addEventListener('click', () => toggleRelationDropdown(true, inputId, dropdownId));
    input.addEventListener('keydown', (e) => {
      if (e.key === 'Enter') {
        e.preventDefault();
      }
    });
    input.addEventListener('input', () => {
      relationSelections.delete(inputId);
      filterRelationDropdown(input.value, inputId, dropdownId);
    });
  }

  function bindLinkPicker() {
    const input = document.getElementById('task-modal-link-search');
    const dropdown = document.getElementById('task-modal-link-dropdown');
    const typeSelect = document.getElementById('task-modal-link-type');
    if (!input || !dropdown || input.dataset.linkBound === '1') return;
    input.dataset.linkBound = '1';
    input.addEventListener('focus', () => toggleLinkDropdown(true));
    input.addEventListener('click', () => toggleLinkDropdown(true));
    input.addEventListener('input', () => {
      linkSelection = null;
      filterLinkDropdown(input.value);
    });
    typeSelect?.addEventListener('change', () => {
      linkSelection = null;
      input.value = '';
      toggleLinkDropdown(false);
    });
  }

  function toggleLinkDropdown(visible) {
    const dropdown = document.getElementById('task-modal-link-dropdown');
    const input = document.getElementById('task-modal-link-search');
    if (!dropdown || !input) return;
    if (!visible) {
      dropdown.hidden = true;
      linkDropdownOpen = false;
      return;
    }
    linkDropdownOpen = true;
    dropdown.hidden = false;
    renderLinkDropdown(input.value);
  }

  function filterLinkDropdown(term) {
    if (!linkDropdownOpen) {
      toggleLinkDropdown(true);
      return;
    }
    renderLinkDropdown(term);
  }

  async function renderLinkDropdown(term = '') {
    const dropdown = document.getElementById('task-modal-link-dropdown');
    const type = document.getElementById('task-modal-link-type')?.value || '';
    if (!dropdown || linkDropdownLoading) return;
    linkDropdownLoading = true;
    dropdown.innerHTML = `<div class="muted">${t('common.loading') || 'Loading'}</div>`;
    if (!type) {
      dropdown.innerHTML = `<div class="muted">${t('tasks.links.selectPlaceholder') || t('tasks.empty.noSelection')}</div>`;
      linkDropdownLoading = false;
      return;
    }
    try {
      await ensureLinkOptions(type);
      const normalized = term.toLowerCase().trim();
      const items = (linkOptions[type] || [])
        .filter(item => linkOptionLabel(type, item).toLowerCase().includes(normalized));
      if (!items.length) {
        dropdown.innerHTML = `<div class="muted">${t('tasks.empty.noSelection')}</div>`;
        linkDropdownLoading = false;
        return;
      }
      const list = document.createElement('div');
      list.className = 'task-link-options';
      items.forEach((item) => {
        const row = document.createElement('button');
        row.type = 'button';
        row.className = 'task-link-option';
        const label = linkOptionLabel(type, item);
        row.textContent = label || `#${linkOptionValue(type, item)}`;
        row.addEventListener('click', () => {
          linkSelection = { id: linkOptionValue(type, item), label };
          const input = document.getElementById('task-modal-link-search');
          if (input) input.value = label || `${linkSelection.id}`;
          toggleLinkDropdown(false);
        });
        list.appendChild(row);
      });
      dropdown.innerHTML = '';
      dropdown.appendChild(list);
    } catch (err) {
      dropdown.innerHTML = `<div class="muted">${t('common.error') || 'Error'}</div>`;
    } finally {
      linkDropdownLoading = false;
    }
  }

  async function ensureLinkOptions(type) {
    if (linkOptionsLoaded[type]) return;
    try {
      if (type === 'doc') {
        const res = await Api.get('/api/docs/list?limit=200').catch(() => ({ items: [] }));
        linkOptions.doc = res.items || [];
      }
      if (type === 'incident') {
        const res = await Api.get('/api/incidents?limit=200').catch(() => ({ items: [] }));
        linkOptions.incident = res.items || [];
      }
      if (type === 'control') {
        const res = await Api.get('/api/controls?limit=200').catch(() => ({ items: [] }));
        linkOptions.control = res.items || [];
      }
    } finally {
      linkOptionsLoaded[type] = true;
    }
  }

  function linkOptionLabel(type, item) {
    if (!item) return '';
    if (type === 'incident') {
      return `#${item.id} ${item.reg_no || ''} ${item.title || ''}`.trim();
    }
    if (type === 'control') {
      const code = item.code ? `${item.code} - ` : '';
      return `${code}${item.title || ''}`.trim();
    }
    const reg = item.reg_no ? `(${item.reg_no})` : '';
    return `${reg} ${item.title || ''}`.trim();
  }

  function linkOptionValue(type, item) {
    if (!item) return '';
    if (type === 'control') return item.control_id || item.id;
    return item.id;
  }

  function hideAllRelationDropdowns() {
    document.querySelectorAll('.task-relation-dropdown').forEach((dropdown) => {
      dropdown.hidden = true;
    });
    relationDropdownOpen = false;
    activeRelationPicker = null;
  }

  function toggleRelationDropdown(visible, inputId, dropdownId) {
    if (!visible) {
      hideAllRelationDropdowns();
      return;
    }
    const dropdown = document.getElementById(dropdownId);
    const input = document.getElementById(inputId);
    if (!dropdown || !input) return;
    hideAllRelationDropdowns();
    relationDropdownOpen = true;
    activeRelationPicker = { inputId, dropdownId };
    dropdown.hidden = false;
    renderRelationDropdown(input.value, inputId, dropdownId);
  }

  function filterRelationDropdown(term, inputId, dropdownId) {
    if (!relationDropdownOpen || activeRelationPicker?.inputId !== inputId) {
      toggleRelationDropdown(true, inputId, dropdownId);
      return;
    }
    renderRelationDropdown(term, inputId, dropdownId);
  }

  async function renderRelationDropdown(term = '', inputId, dropdownId) {
    const dropdown = document.getElementById(dropdownId);
    const input = document.getElementById(inputId);
    if (!dropdown || !input || relationDropdownLoading) return;
    relationDropdownLoading = true;
    dropdown.innerHTML = `<div class="muted">${t('common.loading') || 'Loading'}</div>`;
    try {
      const spaces = await ensureSpaces();
      const termLower = (term || '').trim().toLowerCase();
      const boardByID = new Map();
      const boardRequests = spaces.map((space) => fetchBoards(space.id).catch(() => []));
      const boardResults = await Promise.all(boardRequests);
      boardResults.forEach((boards, idx) => {
        const space = spaces[idx];
        (boards || []).forEach((board) => {
          boardByID.set(board.id, {
            name: board.name || `#${board.id}`,
            spaceName: space?.name || `#${space?.id || ''}`,
          });
        });
      });
      const taskRequests = spaces.map((space) => Api.get(`/api/tasks?space_id=${space.id}&limit=500`).catch(() => ({ items: [] })));
      const taskResults = await Promise.all(taskRequests);
      const items = [];
      taskResults.forEach((result) => {
        const tasks = result?.items || [];
        tasks.forEach((task) => {
          if (task.is_archived || task.id === state.card.taskId) return;
          const title = task.title || `#${task.id}`;
          const boardInfo = boardByID.get(task.board_id) || { name: `#${task.board_id}`, spaceName: '' };
          const searchText = `${title} ${task.id} ${boardInfo.name} ${boardInfo.spaceName}`.toLowerCase();
          if (termLower && !searchText.includes(termLower)) return;
          items.push({ id: task.id, title, boardName: boardInfo.name, spaceName: boardInfo.spaceName });
        });
      });
      if (!items.length) {
        dropdown.innerHTML = `<div class="muted">${t('tasks.empty.noSelection')}</div>`;
        return;
      }
      items.sort((a, b) => a.title.localeCompare(b.title));
      const list = document.createElement('div');
      list.className = 'task-relation-options';
      items.forEach((item) => {
        const row = document.createElement('button');
        row.type = 'button';
        row.className = 'task-relation-option';
        const titleEl = document.createElement('span');
        titleEl.className = 'task-relation-option-title';
        titleEl.textContent = item.title;
        const metaEl = document.createElement('span');
        metaEl.className = 'task-relation-option-meta';
        metaEl.textContent = item.spaceName ? `${item.spaceName} / ${item.boardName}` : item.boardName;
        row.appendChild(titleEl);
        row.appendChild(metaEl);
        row.addEventListener('click', () => {
          relationSelections.set(inputId, { id: item.id, title: item.title });
          input.value = item.title;
          toggleRelationDropdown(false);
        });
        list.appendChild(row);
      });
      dropdown.innerHTML = '';
      dropdown.appendChild(list);
    } catch (err) {
      const message = resolveErrorMessage(err, 'common.error');
      dropdown.innerHTML = `<div class="muted">${escapeHtml(message)}</div>`;
    } finally {
      relationDropdownLoading = false;
    }
  }

  function renderAssigneesHint(select) {
    const hint = document.getElementById('task-modal-assignees-hint');
    if (!select || !hint) return;
    hint.innerHTML = '';
    const selected = Array.from(select.selectedOptions || []);
    if (!selected.length) {
      hint.textContent = t('tasks.empty.noSelection');
      return;
    }
    selected.forEach(opt => {
      const tag = document.createElement('span');
      tag.className = 'tag';
      tag.textContent = opt.dataset.label || opt.textContent || '';
      const remove = document.createElement('button');
      remove.type = 'button';
      remove.className = 'tag-remove';
      remove.setAttribute('aria-label', t('common.delete') || 'Remove');
      remove.textContent = 'x';
      remove.addEventListener('click', (e) => {
        e.stopPropagation();
        opt.selected = false;
        select.dispatchEvent(new Event('change', { bubbles: true }));
      });
      tag.appendChild(remove);
      hint.appendChild(tag);
    });
  }

  function renderTextView(field, value, placeholder) {
    const view = document.getElementById(`task-modal-${field}-view`);
    const editor = document.getElementById(`task-modal-${field}-editor`);
    const input = document.getElementById(`task-modal-${field}`);
    if (view) {
      view.textContent = value ? value : placeholder;
      view.classList.toggle('muted', !value);
      view.hidden = false;
    }
    if (editor) editor.hidden = true;
    if (input) input.value = value || '';
  }

  function renderInlineField(fieldKey) {
    const cfg = inlineFieldConfig[fieldKey];
    if (!cfg) return;
    const view = document.getElementById(cfg.viewId);
    const editor = document.getElementById(cfg.editorId);
    const input = document.getElementById(cfg.inputId);
    const value = cfg.getValue();
    const displayValue = cfg.format(value) || (input?.value || '');
    if (view) {
      view.innerHTML = '';
      view.classList.toggle('muted', !displayValue);
      view.hidden = false;
      if (fieldKey === 'external_link' && displayValue) {
        const link = document.createElement('a');
        link.className = 'task-inline-link';
        link.href = displayValue;
        link.target = '_blank';
        link.rel = 'noopener noreferrer';
        link.textContent = displayValue;
        view.appendChild(link);
        view.removeAttribute('data-inline-view');
      } else {
        view.textContent = displayValue || t('tasks.fields.addValue');
        view.dataset.inlineView = fieldKey;
      }
    }
    if (input) input.value = displayValue || '';
    if (editor) editor.hidden = true;
  }

  function openInlineEditor(fieldKey) {
    const cfg = inlineFieldConfig[fieldKey];
    if (!cfg) return;
    if (!hasPermission('tasks.edit') || state.card.original?.closed_at || state.card.original?.is_archived) {
      return;
    }
    const view = document.getElementById(cfg.viewId);
    const editor = document.getElementById(cfg.editorId);
    const input = document.getElementById(cfg.inputId);
    if (view) view.hidden = true;
    if (editor) editor.hidden = false;
    if (input) {
      input.focus();
      input.select();
    }
  }

  function cancelInlineEditor(fieldKey) {
    const cfg = inlineFieldConfig[fieldKey];
    if (!cfg) return;
    const view = document.getElementById(cfg.viewId);
    const editor = document.getElementById(cfg.editorId);
    if (editor) editor.hidden = true;
    if (view) view.hidden = false;
    renderInlineField(fieldKey);
  }

  async function saveInlineEditor(fieldKey) {
    if (!state.card.taskId) return;
    const cfg = inlineFieldConfig[fieldKey];
    if (!cfg) return;
    const input = document.getElementById(cfg.inputId);
    if (!input) return;
    const parsed = cfg.parse ? cfg.parse(input.value) : input.value;
    const payload = {};
    payload[fieldKey] = parsed;
    await updateTask(payload, () => {
      const editor = document.getElementById(cfg.editorId);
      const view = document.getElementById(cfg.viewId);
      if (editor) editor.hidden = true;
      if (view) view.hidden = false;
    });
  }

  function openTextEditor(field) {
    const view = document.getElementById(`task-modal-${field}-view`);
    const editor = document.getElementById(`task-modal-${field}-editor`);
    if (editor) editor.hidden = false;
    if (view) view.hidden = true;
  }

  function cancelTextEditor(field) {
    const view = document.getElementById(`task-modal-${field}-view`);
    const editor = document.getElementById(`task-modal-${field}-editor`);
    if (editor) editor.hidden = true;
    if (view) view.hidden = false;
    if (field === 'description') renderTextView('description', state.card.original?.description || '', t('tasks.fields.descriptionHint'));
    if (field === 'result') renderTextView('result', state.card.original?.result || '', t('tasks.fields.resultHint'));
  }

  async function saveTextEditor(field) {
    if (!state.card.taskId) return;
    const input = document.getElementById(`task-modal-${field}`);
    if (!input) return;
    const payload = {};
    payload[field] = input.value || '';
    await updateTask(payload, () => {
      const view = document.getElementById(`task-modal-${field}-view`);
      const editor = document.getElementById(`task-modal-${field}-editor`);
      if (editor) editor.hidden = true;
      if (view) view.hidden = false;
    });
  }

  function renderTags() {
    const field = document.getElementById('task-modal-tags-field');
    const list = document.getElementById('task-modal-tags-list');
    if (!list || !field) return;
    const tags = state.card.tags || [];
    field.hidden = tags.length === 0 && field.dataset.forced !== '1';
    list.innerHTML = '';
    tags.forEach(tag => {
      const chip = document.createElement('span');
      chip.className = 'task-tag';
      chip.textContent = tag;
      if (applyTagStyle) applyTagStyle(chip, tag);
      const btn = document.createElement('button');
      btn.type = 'button';
      btn.className = 'task-tag-remove';
      btn.textContent = 'x';
      btn.addEventListener('click', () => {
        state.card.tags = (state.card.tags || []).filter(tg => tg !== tag);
        renderTags();
        updateTask({ tags: state.card.tags || [] })
          .then(() => refreshTagSuggestions())
          .catch(() => {});
      });
      chip.appendChild(btn);
      list.appendChild(chip);
    });
    renderTagSuggestions();
  }

  function addTag(raw) {
    const value = (raw || '').trim();
    if (!value) return;
    const tags = state.card.tags || [];
    if (tags.includes(value)) return;
    tags.push(value);
    state.card.tags = tags;
    renderTags();
    updateTask({ tags: state.card.tags || [] })
      .then(() => refreshTagSuggestions())
      .catch(() => {});
    const field = document.getElementById('task-modal-tags-field');
    if (field) field.hidden = false;
  }

  function normalizeTaskTag(tag) {
    if (!tag) return '';
    if (typeof tag === 'string') return tag;
    if (typeof tag === 'number') return `${tag}`;
    return tag.name || tag.code || tag.label || '';
  }

  function renderTagSuggestions() {
    const list = document.getElementById('task-modal-tags-suggestions');
    if (!list) return;
    const tags = (state.tags || [])
      .map(normalizeTaskTag)
      .filter(Boolean);
    list.innerHTML = '';
    tags
      .sort((a, b) => a.localeCompare(b))
      .forEach(tag => {
        const option = document.createElement('option');
        option.value = tag;
        list.appendChild(option);
      });
  }

  async function refreshTagSuggestions() {
    try {
      const res = await Api.get('/api/tasks/tags');
      state.tags = res.items || [];
    } catch (_) {
      // ignore
    }
    renderTagSuggestions();
  }

  function forceBlockVisible(blockId) {
    const field = document.getElementById(blockId);
    if (!field) return;
    field.dataset.forced = '1';
    field.hidden = false;
    if (state.card.taskId && field.dataset.block) {
      state.card.forcedBlocks = state.card.forcedBlocks || new Set();
      state.card.forcedBlocks.add(field.dataset.block);
      saveForcedBlocks(state.card.taskId, state.card.forcedBlocks);
    }
  }

  function clearBlockForced(blockId) {
    const field = document.getElementById(blockId);
    if (!field) return;
    delete field.dataset.forced;
    if (state.card.taskId && field.dataset.block && state.card.forcedBlocks) {
      state.card.forcedBlocks.delete(field.dataset.block);
      saveForcedBlocks(state.card.taskId, state.card.forcedBlocks);
    }
  }

  function openBlockAddMenu(anchor) {
    if (!anchor) return;
    if (!hasPermission('tasks.edit') || state.card.original?.closed_at || state.card.original?.is_archived) {
      return;
    }
    const actions = [
      { key: 'tags', label: t('tasks.fields.tags'), handler: () => focusBlock('tags') },
      { key: 'checklist', label: t('tasks.sections.checklist'), handler: () => focusBlock('checklist') },
      {
        key: 'relations_parent',
        label: t('tasks.links.addParent'),
        handler: () => {
          focusBlock('relations_parent');
          document.getElementById('task-modal-relation-parent')?.focus();
        }
      },
      {
        key: 'relations_child',
        label: t('tasks.links.addChild'),
        handler: () => {
          focusBlock('relations_child');
          document.getElementById('task-modal-relation-child')?.focus();
        }
      },
      { key: 'external_link', label: t('tasks.blocks.externalLink'), handler: () => focusBlock('external_link') },
      { key: 'files', label: t('tasks.blocks.files'), handler: () => focusBlock('files') },
      { key: 'size', label: t('tasks.fields.size'), handler: () => focusBlock('size') },
      { key: 'business_customer', label: t('tasks.blocks.businessCustomer'), handler: () => focusBlock('business_customer') },
      { key: 'links', label: t('tasks.sections.links'), handler: () => focusBlock('links') }
    ].filter((action) => shouldShowAddBlockAction(action.key));
    showBlockContextMenu(anchor, actions);
  }

  function shouldShowAddBlockAction(blockKey) {
    if (!blockKey) return true;
    if (!singleInstanceBlocks.has(blockKey)) return true;
    return !isSingleBlockAlreadyAdded(blockKey);
  }

  function isSingleBlockAlreadyAdded(blockKey) {
    switch (blockKey) {
      case 'tags': {
        const field = document.getElementById('task-modal-tags-field');
        return (state.card.tags || []).length > 0 || field?.dataset.forced === '1';
      }
      case 'relations_child': {
        const field = document.getElementById('task-modal-relations-child-field');
        const children = links.filter(link => link.target_type === 'task_child');
        return children.length > 0 || field?.dataset.forced === '1';
      }
      case 'business_customer': {
        const field = document.getElementById('task-modal-business-field');
        const value = (state.card.original?.business_customer || '').trim();
        return !!value || field?.dataset.forced === '1';
      }
      case 'size': {
        const field = document.getElementById('task-modal-size-field');
        const size = state.card.original?.size_estimate;
        return typeof size === 'number' || field?.dataset.forced === '1';
      }
      default:
        return false;
    }
  }

  function openBlockMenu(anchor, block) {
    if (!hasPermission('tasks.edit') || state.card.original?.closed_at || state.card.original?.is_archived) {
      return;
    }
    const actions = [
      { label: t('common.edit'), handler: () => focusBlock(block) },
      { label: t('common.delete'), danger: true, handler: () => deleteBlock(block) }
    ];
    showBlockContextMenu(anchor, actions);
  }

  function focusBlock(block) {
    switch (block) {
      case 'tags':
        forceBlockVisible('task-modal-tags-field');
        document.getElementById('task-modal-tag-input')?.focus();
        break;
      case 'checklist':
        forceBlockVisible('task-modal-checklist-field');
        document.getElementById('task-modal-checklist-input')?.focus();
        break;
      case 'relations_parent':
        forceBlockVisible('task-modal-relations-parent-field');
        document.getElementById('task-modal-relation-parent-input')?.focus();
        break;
      case 'relations_child':
        forceBlockVisible('task-modal-relations-child-field');
        document.getElementById('task-modal-relation-child-input')?.focus();
        break;
      case 'external_link':
        forceBlockVisible('task-modal-external-link-field');
        openInlineEditor('external_link');
        break;
      case 'files':
        forceBlockVisible('task-modal-files-field');
        document.getElementById('task-files-upload')?.focus();
        break;
      case 'size':
        forceBlockVisible('task-modal-size-field');
        openInlineEditor('size_estimate');
        break;
      case 'business_customer':
        forceBlockVisible('task-modal-business-field');
        openInlineEditor('business_customer');
        break;
      case 'links':
        forceBlockVisible('task-modal-links-field');
        document.getElementById('task-modal-link-search')?.focus();
        break;
      default:
        break;
    }
  }

  async function deleteBlock(block) {
    if (!state.card.taskId) return;
    const ok = await confirmAction({ message: t('tasks.blocks.deleteConfirm') });
    if (!ok) return;
    switch (block) {
      case 'tags':
        state.card.tags = [];
        clearBlockForced('task-modal-tags-field');
        await updateTask({ tags: [] });
        break;
      case 'checklist':
        state.card.checklist = [];
        clearBlockForced('task-modal-checklist-field');
        await updateTask({ checklist: [] });
        break;
      case 'relations_parent':
      case 'relations_child':
        await deleteRelationLinks();
        clearBlockForced('task-modal-relations-parent-field');
        clearBlockForced('task-modal-relations-child-field');
        await loadLinks(state.card.taskId);
        break;
      case 'external_link':
        clearBlockForced('task-modal-external-link-field');
        await updateTask({ external_link: '' });
        break;
      case 'files':
        await deleteAllFiles();
        clearBlockForced('task-modal-files-field');
        await loadFiles(state.card.taskId);
        break;
      case 'size':
        clearBlockForced('task-modal-size-field');
        await updateTask({ size_estimate: null });
        break;
      case 'business_customer':
        clearBlockForced('task-modal-business-field');
        await updateTask({ business_customer: '' });
        break;
      case 'links':
        await deleteEntityLinks();
        clearBlockForced('task-modal-links-field');
        await loadLinks(state.card.taskId);
        break;
      default:
        break;
    }
  }

  function showBlockContextMenu(anchor, actions) {
    const menu = document.getElementById('task-block-context-menu');
    if (!menu || !anchor) return;
    menu.innerHTML = '';
    actions.forEach(act => {
      const btn = document.createElement('button');
      btn.type = 'button';
      if (act.danger) btn.classList.add('danger');
      btn.textContent = act.label || '';
      btn.onclick = () => {
        menu.hidden = true;
        menu.innerHTML = '';
        act.handler();
      };
      menu.appendChild(btn);
    });
    const rect = anchor.getBoundingClientRect();
    menu.style.left = `${rect.left}px`;
    menu.style.top = `${rect.bottom + 6}px`;
    menu.hidden = false;
  }

  function hideBlockContextMenu() {
    const menu = document.getElementById('task-block-context-menu');
    if (!menu) return;
    menu.hidden = true;
    menu.innerHTML = '';
  }

  function renderChecklist() {
    const field = document.getElementById('task-modal-checklist-field');
    const list = document.getElementById('task-modal-checklist-list');
    if (!list || !field) return;
    const items = state.card.checklist || [];
    field.hidden = items.length === 0 && field.dataset.forced !== '1';
    list.innerHTML = '';
    if (!items.length) return;
    const dir = (typeof window !== 'undefined' && window.UserDirectory)
      ? window.UserDirectory
      : (typeof UserDirectory !== 'undefined' ? UserDirectory : null);
    items.forEach((item, idx) => {
      const row = document.createElement('label');
      row.className = 'task-checklist-item';
      const checkbox = document.createElement('input');
      checkbox.type = 'checkbox';
      checkbox.checked = !!item.done;
      const text = document.createElement('span');
      if (item.done) text.classList.add('done');
      text.textContent = item.text || '';
      const meta = document.createElement('span');
      meta.className = 'task-checklist-meta';
      if (item.done && item.done_by && item.done_at) {
        const name = dir?.name ? dir.name(item.done_by) : `#${item.done_by}`;
        const when = formatDateTime(item.done_at);
        meta.textContent = `${name} • ${when}`;
      }
      const remove = document.createElement('button');
      remove.type = 'button';
      remove.className = 'btn ghost btn-xs';
      remove.textContent = t('tasks.actions.remove');
      checkbox.addEventListener('change', (e) => {
        items[idx].done = !!e.target.checked;
        if (items[idx].done) {
          items[idx].done_by = state.currentUser?.id || null;
          items[idx].done_at = new Date().toISOString();
        } else {
          items[idx].done_by = null;
          items[idx].done_at = null;
        }
        renderChecklist();
        updateTask({ checklist: items }).catch(() => {});
      });
      remove.addEventListener('click', () => {
        items.splice(idx, 1);
        renderChecklist();
        updateTask({ checklist: items }).catch(() => {});
      });
      row.appendChild(checkbox);
      row.appendChild(text);
      if (meta.textContent) row.appendChild(meta);
      row.appendChild(remove);
      list.appendChild(row);
    });
  }

  function addChecklistItem() {
    const input = document.getElementById('task-modal-checklist-input');
    if (!input) return;
    const text = input.value.trim();
    if (!text) return;
    const items = state.card.checklist || [];
    items.push({ text, done: false });
    state.card.checklist = items;
    input.value = '';
    renderChecklist();
    updateTask({ checklist: items }).catch(() => {});
    const field = document.getElementById('task-modal-checklist-field');
    if (field) field.hidden = false;
  }

  async function loadLinks(taskId) {
    try {
      const res = await Api.get(`/api/tasks/${taskId}/links`);
      links = res.items || [];
    } catch (err) {
      links = [];
    }
    renderLinks();
    renderRelations();
  }

  async function loadControlLinks(taskId) {
    try {
      const res = await Api.get(`/api/tasks/${taskId}/control-links`);
      controlLinks = res.items || [];
    } catch (err) {
      controlLinks = [];
    }
    renderControlLinks();
  }

  async function loadFiles(taskId) {
    try {
      const res = await Api.get(`/api/tasks/${taskId}/files`);
      taskFiles = res.items || [];
    } catch (_) {
      taskFiles = [];
    }
    renderFiles();
  }

  function renderLinks() {
    const field = document.getElementById('task-modal-links-field');
    const list = document.getElementById('task-modal-links-list');
    if (!list || !field) return;
    const filtered = links.filter(link => !['task_parent', 'task_child'].includes(link.target_type));
    field.hidden = filtered.length === 0 && field.dataset.forced !== '1';
    list.innerHTML = '';
    if (!filtered.length) return;
    filtered.forEach(link => {
      const row = document.createElement('div');
      row.className = 'task-link';
      const label = `${t(`tasks.links.${link.target_type}`) || link.target_type} #${link.target_id}`;
      const title = link.target_title ? `${label} - ${link.target_title}` : label;
      row.innerHTML = `<span>${escapeHtml(title)}</span>`;
      if (hasPermission('tasks.edit')) {
        const btn = document.createElement('button');
        btn.type = 'button';
        btn.className = 'btn ghost btn-xs';
        btn.textContent = t('tasks.actions.remove');
        btn.addEventListener('click', async () => {
          try {
            await Api.del(`/api/tasks/${state.card.taskId}/links/${link.id}`);
            await loadLinks(state.card.taskId);
          } catch (err) {
            showError(err, 'common.error');
          }
        });
        row.appendChild(btn);
      }
      list.appendChild(row);
    });
  }

  function renderRelations() {
    const parentField = document.getElementById('task-modal-relations-parent-field');
    const childField = document.getElementById('task-modal-relations-child-field');
    const parentList = document.getElementById('task-modal-relations-parent');
    const childList = document.getElementById('task-modal-relations-child');
    if (!parentField || !childField || !parentList || !childList) return;
    const parents = links.filter(link => link.target_type === 'task_parent');
    const children = links.filter(link => link.target_type === 'task_child');
    parentField.hidden = parents.length === 0 && parentField.dataset.forced !== '1';
    childField.hidden = children.length === 0 && childField.dataset.forced !== '1';
    parentList.innerHTML = '';
    childList.innerHTML = '';
    const renderItem = (link, targetList) => {
      const row = document.createElement('div');
      row.className = 'task-link';
      const title = resolveTaskTitle(link.target_id);
      row.innerHTML = `<span>${escapeHtml(title)}</span>`;
      if (hasPermission('tasks.edit')) {
        const btn = document.createElement('button');
        btn.type = 'button';
        btn.className = 'btn ghost btn-xs';
        btn.textContent = t('tasks.actions.remove');
        btn.addEventListener('click', async () => {
          try {
            await Api.del(`/api/tasks/${state.card.taskId}/links/${link.id}`);
            await loadLinks(state.card.taskId);
          } catch (err) {
            showError(err, 'common.error');
          }
        });
        row.appendChild(btn);
      }
      targetList.appendChild(row);
    };
    parents.forEach(link => renderItem(link, parentList));
    children.forEach(link => renderItem(link, childList));
  }

  function resolveTaskTitle(id) {
    if (taskTitleCache[id]) return taskTitleCache[id];
    const fromState = state.taskMap.get(parseInt(id, 10));
    if (fromState?.title) {
      taskTitleCache[id] = fromState.title;
      return fromState.title;
    }
    if (!taskTitlePending.has(id)) {
      taskTitlePending.add(id);
      Api.get(`/api/tasks/${id}`)
        .then(task => {
          if (task?.title) {
            taskTitleCache[id] = task.title;
            renderRelations();
          }
        })
        .catch(() => {})
        .finally(() => taskTitlePending.delete(id));
    }
    return `#${id}`;
  }

  async function addRelation(type, inputId) {
    if (!state.card.taskId || !hasPermission('tasks.edit')) return;
    const targetType = type || '';
    const selection = inputId ? relationSelections.get(inputId) : null;
    const targetID = selection?.id ? `${selection.id}` : '';
    if (!targetType || !targetID) {
      showAlert('task-modal-alert', t('tasks.links.selectRequired'));
      return;
    }
    try {
      await Api.post(`/api/tasks/${state.card.taskId}/links`, { target_type: targetType, target_id: targetID });
      if (inputId) relationSelections.delete(inputId);
      const input = inputId ? document.getElementById(inputId) : null;
      if (input) input.value = '';
      await loadLinks(state.card.taskId);
      const parentField = document.getElementById('task-modal-relations-parent-field');
      const childField = document.getElementById('task-modal-relations-child-field');
      if (parentField) parentField.hidden = false;
      if (childField) childField.hidden = false;
    } catch (err) {
      showAlert('task-modal-alert', resolveErrorMessage(err, 'common.error'));
    }
  }

  async function deleteRelationLinks() {
    const targets = links.filter(link => link.target_type === 'task_parent' || link.target_type === 'task_child');
    for (const link of targets) {
      try {
        await Api.del(`/api/tasks/${state.card.taskId}/links/${link.id}`);
      } catch (_) {
        // handled below
      }
    }
  }

  async function deleteEntityLinks() {
    const targets = links.filter(link => !['task_parent', 'task_child'].includes(link.target_type));
    for (const link of targets) {
      try {
        await Api.del(`/api/tasks/${state.card.taskId}/links/${link.id}`);
      } catch (_) {
        // handled below
      }
    }
  }

  function renderExternalLink() {
    const field = document.getElementById('task-modal-external-link-field');
    if (!field) return;
    const value = (state.card.original?.external_link || '').trim();
    field.hidden = !value && field.dataset.forced !== '1';
  }

  function renderBusinessCustomer() {
    const field = document.getElementById('task-modal-business-field');
    if (!field) return;
    const value = (state.card.original?.business_customer || '').trim();
    field.hidden = !value && field.dataset.forced !== '1';
  }

  function renderSizeField() {
    const field = document.getElementById('task-modal-size-field');
    if (!field) return;
    const value = state.card.original?.size_estimate;
    const hasValue = typeof value === 'number';
    field.hidden = !hasValue && field.dataset.forced !== '1';
  }

  function renderControlLinks() {
    const field = document.getElementById('task-modal-controls-field');
    const list = document.getElementById('task-modal-controls-list');
    if (!list || !field) return;
    field.hidden = controlLinks.length === 0;
    list.innerHTML = '';
    if (!controlLinks.length) return;
    controlLinks.forEach(link => {
      const row = document.createElement('div');
      row.className = 'task-link';
      const title = `${link.code} - ${link.title}`;
      row.innerHTML = `<a href="/controls?control=${encodeURIComponent(link.control_id)}">${escapeHtml(title)}</a>`;
      list.appendChild(row);
    });
  }

  function renderFiles() {
    const field = document.getElementById('task-modal-files-field');
    const list = document.getElementById('task-modal-files-list');
    if (!field || !list) return;
    field.hidden = taskFiles.length === 0 && field.dataset.forced !== '1';
    list.innerHTML = '';
    if (!taskFiles.length) return;
    taskFiles.forEach(file => {
      const row = document.createElement('div');
      row.className = 'task-file-row';
      const link = document.createElement('a');
      link.href = file.url || '#';
      link.target = '_blank';
      link.rel = 'noreferrer';
      link.textContent = `${file.name || file.id || ''}${typeof file.size === 'number' ? ` (${formatFileSize(file.size)})` : ''}`;
      row.appendChild(link);
      if (hasPermission('tasks.edit')) {
        const remove = document.createElement('button');
        remove.type = 'button';
        remove.className = 'btn ghost btn-xs';
        remove.textContent = t('tasks.actions.remove');
        remove.addEventListener('click', async () => {
          try {
            await Api.del(`/api/tasks/${state.card.taskId}/files/${file.id}`);
            await loadFiles(state.card.taskId);
          } catch (err) {
            showError(err, 'common.error');
          }
        });
        row.appendChild(remove);
      }
      list.appendChild(row);
    });
  }

  async function uploadTaskFiles(files) {
    if (!state.card.taskId || !files.length || !hasPermission('tasks.edit')) return;
    const form = new FormData();
    files.forEach(file => form.append('files', file, file.name));
    try {
      await Api.upload(`/api/tasks/${state.card.taskId}/files`, form);
      await loadFiles(state.card.taskId);
      forceBlockVisible('task-modal-files-field');
    } catch (err) {
      showAlert('task-modal-alert', resolveErrorMessage(err, 'common.error'));
    }
  }

  async function deleteAllFiles() {
    for (const file of taskFiles) {
      try {
        await Api.del(`/api/tasks/${state.card.taskId}/files/${file.id}`);
      } catch (_) {
        // continue
      }
    }
  }

  async function addLink() {
    if (!state.card.taskId || !hasPermission('tasks.edit')) return;
    const type = document.getElementById('task-modal-link-type')?.value || '';
    const id = linkSelection?.id ? `${linkSelection.id}` : '';
    if (!type || !id) {
      showAlert('task-modal-alert', t('tasks.links.selectRequired'));
      return;
    }
    try {
      await Api.post(`/api/tasks/${state.card.taskId}/links`, { target_type: type, target_id: id });
      linkSelection = null;
      const linkInput = document.getElementById('task-modal-link-search');
      if (linkInput) linkInput.value = '';
      await loadLinks(state.card.taskId);
      const field = document.getElementById('task-modal-links-field');
      if (field) field.hidden = false;
    } catch (err) {
      showAlert('task-modal-alert', resolveErrorMessage(err, 'common.error'));
    }
  }

  async function loadBlocks(taskId) {
    try {
      const res = await Api.get(`/api/tasks/${taskId}/blocks`);
      blocks = res.items || [];
      blocking = res.blocking || [];
      blockTitles = res.titles || {};
    } catch (err) {
      blocks = [];
      blocking = [];
      blockTitles = {};
    }
    renderBlocks();
  }

  function renderBlocks() {
    const list = document.getElementById('task-modal-blocks-list');
    const blockingList = document.getElementById('task-modal-blocking-list');
    if (!list || !blockingList) return;
    list.innerHTML = '';
    blockingList.innerHTML = '';
    if (!blocks.length) {
      const empty = document.createElement('div');
      empty.className = 'muted';
      empty.textContent = t('tasks.blocks.empty');
      list.appendChild(empty);
    }
    blocks.forEach(block => {
      const row = document.createElement('div');
      row.className = 'task-block-row';
      let title = '';
      if (block.block_type === 'task' && block.blocker_task_id) {
        const ref = blockTitles[block.blocker_task_id] || `#${block.blocker_task_id}`;
        title = `${t('tasks.blocks.blockedByCard')} «${ref}»`;
      } else {
        title = block.reason || t('tasks.blocks.reasonMissing');
      }
      const meta = [];
      if (!block.is_active) meta.push(t('tasks.blocks.resolve'));
      row.innerHTML = `
        <div class="task-block-info">
          <div>${escapeHtml(title)}</div>
          ${meta.length ? `<div class="muted">${escapeHtml(meta.join(' - '))}</div>` : ''}
        </div>
      `;
      if (block.blocker_task_id) {
        const link = document.createElement('button');
        link.type = 'button';
        link.className = 'task-block-link';
        link.textContent = t('tasks.blocks.open');
        link.addEventListener('click', (e) => {
          e.preventDefault();
          e.stopPropagation();
          openTask(block.blocker_task_id);
        });
        row.querySelector('.task-block-info')?.appendChild(link);
      }
      if (block.is_active && hasPermission('tasks.block.resolve')) {
        const btn = document.createElement('button');
        btn.type = 'button';
        btn.className = 'btn ghost btn-xs';
        btn.textContent = t('tasks.blocks.resolve');
        btn.addEventListener('click', async () => {
          try {
            await Api.post(`/api/tasks/${state.card.taskId}/blocks/${block.id}/resolve`, {});
            await loadBlocks(state.card.taskId);
          } catch (err) {
            showError(err, 'common.error');
          }
        });
        row.appendChild(btn);
      }
      list.appendChild(row);
    });

    if (!blocking.length) {
      const empty = document.createElement('div');
      empty.className = 'muted';
      empty.textContent = t('tasks.blocks.emptyBlocking');
      blockingList.appendChild(empty);
      return;
    }
    blocking.forEach(blockedId => {
      const row = document.createElement('div');
      row.className = 'task-block-row';
      const ref = blockTitles[blockedId] || `#${blockedId}`;
      row.innerHTML = `
        <div class="task-block-info">
          <div>${escapeHtml(`${t('tasks.blocks.blocksCard')} «${ref}»`)}</div>
        </div>
      `;
      const link = document.createElement('button');
      link.type = 'button';
      link.className = 'task-block-link';
      link.textContent = t('tasks.blocks.open');
      link.addEventListener('click', (e) => {
        e.preventDefault();
        e.stopPropagation();
        openTask(blockedId);
      });
      row.querySelector('.task-block-info')?.appendChild(link);
      blockingList.appendChild(row);
    });
  }

  function resetCommentForm() {
    const input = document.getElementById('task-comment-input');
    if (input) input.value = '';
    pendingCommentFiles = [];
    renderCommentAttachments();
  }

  function setCommentFormState(enabled) {
    const input = document.getElementById('task-comment-input');
    const attach = document.getElementById('task-comment-attach');
    const submit = document.getElementById('task-comment-submit');
    if (input) input.disabled = !enabled;
    if (attach) attach.disabled = !enabled;
    if (submit) submit.disabled = !enabled;
  }

  async function loadComments(taskId) {
    try {
      const res = await Api.get(`/api/tasks/${taskId}/comments`);
      comments = res.items || [];
    } catch (err) {
      comments = [];
    }
    renderComments();
  }

  function renderComments() {
    const list = document.getElementById('task-modal-comments-list');
    if (!list) return;
    list.innerHTML = '';
    if (!comments.length) {
      const empty = document.createElement('div');
      empty.className = 'muted';
      empty.textContent = t('tasks.comments.empty') || t('tasks.empty.noSelection') || '-';
      list.appendChild(empty);
      return;
    }
    const dir = (typeof window !== 'undefined' && window.UserDirectory)
      ? window.UserDirectory
      : (typeof UserDirectory !== 'undefined' ? UserDirectory : null);
    comments.forEach(comment => {
      const row = document.createElement('div');
      row.className = 'task-comment';
      const author = dir?.get ? dir.get(comment.author_id) : null;
      const authorName = author?.full_name || author?.username || `${comment.author_id}`;
      const created = comment.created_at ? formatDateTime(comment.created_at) : '';
      const meta = document.createElement('div');
      meta.className = 'task-comment-meta';
      meta.innerHTML = `<span class="task-comment-author">${escapeHtml(authorName)}</span><span class="task-comment-time">${escapeHtml(created)}</span>`;
      const canManage = canManageComment(comment);
      if (canManage) {
        const actions = document.createElement('div');
        actions.className = 'task-comment-actions-inline';
        const editBtn = document.createElement('button');
        editBtn.type = 'button';
        editBtn.className = 'btn ghost btn-xs';
        editBtn.textContent = t('common.edit');
        editBtn.addEventListener('click', () => {
          editingCommentId = comment.id;
          renderComments();
        });
        const delBtn = document.createElement('button');
        delBtn.type = 'button';
        delBtn.className = 'btn ghost btn-xs';
        delBtn.textContent = t('common.delete');
        delBtn.addEventListener('click', () => deleteComment(comment));
        actions.appendChild(editBtn);
        actions.appendChild(delBtn);
        meta.appendChild(actions);
      }
      row.appendChild(meta);

      if (editingCommentId === comment.id) {
        const editor = document.createElement('div');
        editor.className = 'task-comment-editor';
        const textarea = document.createElement('textarea');
        textarea.className = 'textarea';
        textarea.rows = 3;
        textarea.value = comment.content || '';
        const actions = document.createElement('div');
        actions.className = 'task-comment-editor-actions';
        const save = document.createElement('button');
        save.type = 'button';
        save.className = 'btn primary btn-sm';
        save.textContent = t('common.save');
        save.addEventListener('click', async () => {
          const next = textarea.value.trim();
          try {
            const res = await Api.put(`/api/tasks/${state.card.taskId}/comments/${comment.id}`, { content: next });
            comments = comments.map(c => (c.id === comment.id ? res : c));
            editingCommentId = null;
            renderComments();
          } catch (err) {
            showError(err, 'common.error');
          }
        });
        const cancel = document.createElement('button');
        cancel.type = 'button';
        cancel.className = 'btn ghost btn-sm';
        cancel.textContent = t('common.cancel');
        cancel.addEventListener('click', () => {
          editingCommentId = null;
          renderComments();
        });
        actions.appendChild(save);
        actions.appendChild(cancel);
        editor.appendChild(textarea);
        editor.appendChild(actions);
        row.appendChild(editor);
      } else {
        const body = document.createElement('div');
        body.className = 'task-comment-body';
        body.textContent = comment.content || '';
        row.appendChild(body);
      }

      if (Array.isArray(comment.attachments) && comment.attachments.length) {
        const attList = document.createElement('div');
        attList.className = 'task-comment-files';
        comment.attachments.forEach(att => {
          const fileRow = document.createElement('div');
          fileRow.className = 'task-comment-file-row';
          const link = document.createElement('a');
          link.href = att.url || '#';
          link.target = '_blank';
          link.rel = 'noreferrer';
          const size = typeof att.size === 'number' ? ` (${formatFileSize(att.size)})` : '';
          link.textContent = `${att.name || att.id || ''}${size}`;
          fileRow.appendChild(link);
          if (canManage) {
            const remove = document.createElement('button');
            remove.type = 'button';
            remove.className = 'btn ghost btn-xs';
            remove.textContent = t('common.delete');
            remove.addEventListener('click', () => deleteCommentFile(comment, att));
            fileRow.appendChild(remove);
          }
          attList.appendChild(fileRow);
        });
        row.appendChild(attList);
      }
      list.appendChild(row);
    });
  }

  function canManageComment(comment) {
    if (!comment || !state.currentUser) return false;
    const roles = Array.isArray(state.currentUser.roles) ? state.currentUser.roles : [];
    const isAdmin = roles.includes('admin') || roles.includes('superadmin');
    return comment.author_id === state.currentUser.id || isAdmin || hasPermission('tasks.manage');
  }

  async function deleteComment(comment) {
    if (!comment || !state.card.taskId) return;
    const ok = await confirmAction({ message: t('tasks.comments.deleteConfirm') });
    if (!ok) return;
    try {
      await Api.del(`/api/tasks/${state.card.taskId}/comments/${comment.id}`);
      comments = comments.filter(c => c.id !== comment.id);
      renderComments();
    } catch (err) {
      showError(err, 'common.error');
    }
  }

  async function deleteCommentFile(comment, att) {
    if (!comment || !att || !state.card.taskId) return;
    try {
      const res = await Api.del(`/api/tasks/${state.card.taskId}/comments/${comment.id}/files/${att.id}`);
      if (res.status === 'deleted') {
        comments = comments.filter(c => c.id !== comment.id);
      } else {
        comments = comments.map(c => (c.id === comment.id ? res : c));
      }
      renderComments();
    } catch (err) {
      showError(err, 'common.error');
    }
  }

  function handleCommentPaste(e) {
    const items = e?.clipboardData?.items;
    if (!items || !items.length) return;
    const files = [];
    Array.from(items).forEach(item => {
      if (item.kind === 'file') {
        const file = item.getAsFile();
        if (file) files.push(file);
      }
    });
    if (files.length) {
      e.preventDefault();
      addCommentFiles(files);
    }
  }

  function handleCommentDrop(e) {
    if (!e?.dataTransfer?.files?.length) return;
    e.preventDefault();
    addCommentFiles(Array.from(e.dataTransfer.files));
  }

  function addCommentFiles(files) {
    if (!files || !files.length) return;
    pendingCommentFiles = pendingCommentFiles.concat(files);
    renderCommentAttachments();
  }

  function renderCommentAttachments() {
    const list = document.getElementById('task-comment-attachments');
    if (!list) return;
    list.innerHTML = '';
    pendingCommentFiles.forEach((file, idx) => {
      const row = document.createElement('div');
      row.className = 'task-comment-file';
      const name = document.createElement('span');
      name.textContent = `${file.name} (${formatFileSize(file.size)})`;
      const remove = document.createElement('button');
      remove.type = 'button';
      remove.className = 'btn ghost btn-xs';
      remove.textContent = 'x';
      remove.addEventListener('click', () => {
        pendingCommentFiles.splice(idx, 1);
        renderCommentAttachments();
      });
      row.appendChild(name);
      row.appendChild(remove);
      list.appendChild(row);
    });
  }

  async function submitComment() {
    if (!state.card.taskId) return;
    hideAlert('task-modal-alert');
    const input = document.getElementById('task-comment-input');
    const content = (input?.value || '').trim();
    if (!content && pendingCommentFiles.length === 0) {
      showAlert('task-modal-alert', t('tasks.commentRequired'));
      return;
    }
    const form = new FormData();
    form.append('content', content);
    pendingCommentFiles.forEach(file => form.append('files', file, file.name));
    try {
      const res = await Api.upload(`/api/tasks/${state.card.taskId}/comments`, form);
      comments.push(res);
      if (input) input.value = '';
      pendingCommentFiles = [];
      renderCommentAttachments();
      renderComments();
    } catch (err) {
      showAlert('task-modal-alert', resolveErrorMessage(err, 'common.error'));
    }
  }

  function formatFileSize(bytes) {
    if (!bytes && bytes !== 0) return '';
    if (bytes < 1024) return `${bytes} B`;
    const kb = bytes / 1024;
    if (kb < 1024) return `${kb.toFixed(1)} KB`;
    const mb = kb / 1024;
    if (mb < 1024) return `${mb.toFixed(1)} MB`;
    const gb = mb / 1024;
    return `${gb.toFixed(1)} GB`;
  }

  function openBlockModal() {
    if (!state.card.taskId) return;
    pendingBlocker = null;
    hideAlert('task-block-modal-alert');
    const reason = document.getElementById('task-block-reason-input');
    if (reason) reason.value = '';
    openModal('task-block-modal');
  }

  async function saveBlock() {
    hideAlert('task-block-modal-alert');
    const reason = (document.getElementById('task-block-reason-input')?.value || '').trim();
    if (!pendingBlocker && !reason) {
      showAlert('task-block-modal-alert', t('tasks.blocks.reasonRequired'));
      return;
    }
    try {
      if (pendingBlocker) {
        await Api.post(`/api/tasks/${state.card.taskId}/blocks/task`, { blocker_task_id: pendingBlocker.id });
      } else {
        await Api.post(`/api/tasks/${state.card.taskId}/blocks/text`, { reason });
      }
      closeModal('task-block-modal');
      await loadBlocks(state.card.taskId);
    } catch (err) {
      showAlert('task-block-modal-alert', resolveErrorMessage(err, 'common.error'));
    }
  }

  async function openBlockerModal() {
    const tree = document.getElementById('task-blocker-tree');
    if (!tree) return;
    tree.innerHTML = '';
    openModal('task-blocker-modal');
    try {
      const spaces = await ensureSpaces();
      for (const space of spaces) {
        const spaceNode = document.createElement('details');
        const spaceSummary = document.createElement('summary');
        spaceSummary.textContent = space.name || `#${space.id}`;
        spaceNode.appendChild(spaceSummary);
        const boards = await fetchBoards(space.id);
        for (const board of boards) {
          const boardNode = document.createElement('details');
          const boardSummary = document.createElement('summary');
          boardSummary.textContent = board.name || `#${board.id}`;
          boardNode.appendChild(boardSummary);
          const columns = await fetchColumns(board.id);
          const tasks = await fetchTasks(board.id);
          columns.forEach(col => {
            const colNode = document.createElement('details');
            const colSummary = document.createElement('summary');
            colSummary.textContent = col.name || `#${col.id}`;
            colNode.appendChild(colSummary);
            const colTasks = tasks.filter(tk => tk.column_id === col.id && !tk.is_archived);
            colTasks.forEach(task => {
              const row = document.createElement('div');
              row.className = 'blocker-task-row';
              const title = document.createElement('span');
              title.textContent = task.title || `#${task.id}`;
              const select = document.createElement('button');
              select.type = 'button';
              select.className = 'btn ghost btn-xs';
              select.textContent = t('tasks.blocks.select');
              select.addEventListener('click', () => {
                pendingBlocker = { id: task.id, title: task.title || `#${task.id}` };
                closeModal('task-blocker-modal');
                showAlert('task-block-modal-alert', `${t('tasks.blocks.blockedByTask')}: ${pendingBlocker.title}`);
              });
              const link = document.createElement('button');
              link.type = 'button';
              link.className = 'btn ghost btn-xs';
              link.innerHTML = '&#128279;';
              link.title = t('tasks.blocks.copyLink');
              link.addEventListener('click', () => copyTaskLink(task.id));
              row.appendChild(title);
              row.appendChild(select);
              row.appendChild(link);
              colNode.appendChild(row);
            });
            boardNode.appendChild(colNode);
          });
          spaceNode.appendChild(boardNode);
        }
        tree.appendChild(spaceNode);
      }
    } catch (err) {
      tree.textContent = resolveErrorMessage(err, 'common.error');
    }
  }

  async function ensureSpaces() {
    if (state.spaces && state.spaces.length) return state.spaces;
    const res = await Api.get('/api/tasks/spaces?include_inactive=1');
    state.spaces = res.items || [];
    return state.spaces;
  }

  async function fetchBoards(spaceId) {
    const res = await Api.get(`/api/tasks/boards?space_id=${spaceId}`);
    return res.items || [];
  }

  async function fetchColumns(boardId) {
    const res = await Api.get(`/api/tasks/boards/${boardId}/columns`);
    return res.items || [];
  }

  async function fetchTasks(boardId) {
    const res = await Api.get(`/api/tasks?board_id=${boardId}`);
    return res.items || [];
  }

  function copyTaskLink(taskId) {
    if (!taskId) return;
    const boardID = state.card.original?.board_id || 0;
    const scopedSpaceID = resolveSpaceIDForBoard(boardID) || state.spaceId || 0;
    const path = scopedSpaceID
      ? `/tasks/space/${scopedSpaceID}/task/${taskId}`
      : `/tasks/task/${taskId}`;
    const link = `${window.location.origin}${path}`;
    if (navigator.clipboard?.writeText) {
      navigator.clipboard.writeText(link);
      return;
    }
    const area = document.createElement('textarea');
    area.value = link;
    document.body.appendChild(area);
    area.select();
    document.execCommand('copy');
    document.body.removeChild(area);
  }

  function resolveSpaceIDForBoard(boardID) {
    if (!boardID) return 0;
    const spaces = Array.isArray(state.spaces) ? state.spaces : [];
    for (const space of spaces) {
      const boards = state.boardsBySpace?.[space.id] || [];
      if (boards.some((board) => board.id === boardID)) {
        return space.id;
      }
    }
    return 0;
  }

  async function saveTask() {
    if (!state.card.taskId) return;
    hideAlert('task-modal-alert');
    const title = document.getElementById('task-modal-title-input')?.value || '';
    const sizeRaw = document.getElementById('task-modal-size')?.value || '';
    const priority = document.getElementById('task-modal-priority')?.value || '';
    const dueRaw = document.getElementById('task-modal-due')?.value || '';
    const assignees = Array.from(document.getElementById('task-modal-assignees')?.selectedOptions || []).map(o => o.value);
    const externalInput = document.getElementById('task-modal-external-link');
    const externalField = document.getElementById('task-modal-external-link-field');
    const businessInput = document.getElementById('task-modal-business');
    const businessField = document.getElementById('task-modal-business-field');
    const payload = {
      title,
      priority,
      due_date: dueRaw ? toISODate(dueRaw) : '',
      assigned_to: assignees,
      tags: state.card.tags || [],
      checklist: state.card.checklist || []
    };
    if (sizeRaw !== '') payload.size_estimate = parseInt(sizeRaw, 10);
    if (externalField && (externalField.dataset.forced === '1' || (externalInput?.value || '').trim() !== '')) {
      payload.external_link = (externalInput?.value || '').trim();
    }
    if (businessField && (businessField.dataset.forced === '1' || (businessInput?.value || '').trim() !== '')) {
      payload.business_customer = (businessInput?.value || '').trim();
    }
    try {
      await updateTask(payload);
    } catch (_) {
      // handled in updateTask
    }
  }

  async function updateTask(payload, onSuccess) {
    if (!state.card.taskId) return;
    try {
      const updated = await Api.put(`/api/tasks/${state.card.taskId}`, payload);
      const merged = { ...(state.card.original || {}), ...(payload || {}), ...(updated || {}) };
      setTaskState(merged);
      renderTaskModal(merged);
      await refreshBoards(updated.board_id);
      if (onSuccess) onSuccess();
      hideAlert('task-modal-alert');
      if (payload && Object.prototype.hasOwnProperty.call(payload, 'tags')) {
        refreshTagSuggestions();
      }
    } catch (err) {
      showAlert('task-modal-alert', resolveErrorMessage(err, 'common.error'));
      throw err;
    }
  }

  async function closeTask() {
    if (!state.card.taskId) return;
    const ok = await confirmAction({ message: t('tasks.actions.closeTask') });
    if (!ok) return;
    try {
      const updated = await Api.post(`/api/tasks/${state.card.taskId}/close`, {});
      setTaskState(updated);
      renderTaskModal(updated);
      await refreshBoards(updated.board_id);
    } catch (err) {
      showAlert('task-modal-alert', resolveErrorMessage(err, 'common.error'));
    }
  }

  async function archiveTask() {
    if (!state.card.taskId) return;
    const ok = await confirmAction({ message: t('tasks.actions.archive') });
    if (!ok) return;
    try {
      const updated = await Api.post(`/api/tasks/${state.card.taskId}/archive`, {});
      setTaskState(updated);
      closeTaskModal();
      await refreshBoards(updated.board_id);
    } catch (err) {
      showAlert('task-modal-alert', resolveErrorMessage(err, 'common.error'));
    }
  }

  function closeTaskModal() {
    closeModal('task-modal');
    if (TasksPage.updateSpaceHash) {
      TasksPage.updateSpaceHash(state.spaceId);
    }
  }

  function focusTaskLocation() {
    if (!state.card.taskId) return;
    closeTaskModal();
    setTimeout(() => {
      const card = document.querySelector(`.task-card[data-task-id="${state.card.taskId}"]`);
      if (!card) return;
      const column = card.closest('.tasks-column') || card.closest('.tasks-subcolumn') || card;
      column.scrollIntoView({ behavior: 'smooth', block: 'center', inline: 'center' });
      card.scrollIntoView({ behavior: 'smooth', block: 'center', inline: 'center' });
      card.classList.add('task-card-focus');
      setTimeout(() => card.classList.remove('task-card-focus'), 1600);
    }, 0);
  }

  async function refreshBoards(boardId) {
    if (TasksPage.loadTasks) await TasksPage.loadTasks(boardId);
    if (TasksPage.renderBoards) TasksPage.renderBoards(state.spaceId);
  }

  function openPlusMenu(anchor) {
    if (!anchor) return;
    openBlockAddMenu(anchor);
  }

  function openTaskMenu(anchor) {
    if (!anchor) return;
    const actions = [];
    if (hasPermission('tasks.create')) {
      actions.push({ label: t('tasks.actions.clone'), handler: () => cloneTask(state.card.taskId) });
    }
    if (hasPermission('tasks.move') && TasksPage.openTaskMoveModal) {
      actions.push({ label: t('tasks.actions.moveTask'), handler: () => TasksPage.openTaskMoveModal(state.card.taskId) });
    }
    showContextMenu(anchor, actions);
  }

  function showContextMenu(anchor, actions) {
    const menu = document.getElementById('tasks-context-menu');
    if (!menu) return;
    menu.innerHTML = '';
    actions.forEach(act => {
      const btn = document.createElement('button');
      btn.type = 'button';
      if (act.danger) btn.classList.add('danger');
      btn.textContent = act.label || '';
      btn.onclick = () => {
        menu.hidden = true;
        menu.innerHTML = '';
        act.handler();
      };
      menu.appendChild(btn);
    });
    const rect = anchor.getBoundingClientRect();
    menu.style.left = `${rect.left}px`;
    menu.style.top = `${rect.bottom + 6}px`;
    menu.hidden = false;
  }

  async function cloneTask(taskId) {
    if (!taskId) return;
    try {
      const cloned = await Api.post(`/api/tasks/${taskId}/clone`, {});
      await refreshBoards(cloned.board_id || state.card.original?.board_id);
    } catch (err) {
      showError(err, 'common.error');
    }
  }

  TasksPage.initSidebar = initSidebar;
  TasksPage.openTask = openTask;
  TasksPage.closeTaskModal = closeTaskModal;
})();
