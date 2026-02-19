(() => {
  const {
    t,
    splitISODateTime,
    toISODateTime,
    populateUserSelect,
    getSelectedValues
  } = IncidentsPage;

  const OUTCOMES = ['closed', 'approved', 'rejected', 'blocked', 'deferred', 'monitor'];

  function uid(prefix = 'dec') {
    return `${prefix}-${Math.random().toString(36).slice(2, 8)}-${Date.now()}`;
  }

  function normalizeOutcome(raw) {
    const val = (raw || '').toString().trim().toLowerCase();
    if (OUTCOMES.includes(val)) return val;
    const aliases = {
      'разрешено': 'approved',
      'approved': 'approved',
      'closed': 'closed',
      'закрыт': 'closed',
      'отклонено': 'rejected',
      'rejected': 'rejected',
      'блокировка': 'blocked',
      'заблокировано': 'blocked',
      'blocked': 'blocked',
      'отложено': 'deferred',
      'deferred': 'deferred',
      'под наблюдением': 'monitor',
      'monitor': 'monitor',
      'monitoring': 'monitor',
    };
    if (aliases[val]) return aliases[val];
    return 'approved';
  }

  function toISODateValue(value) {
    if (!value) return '';
    if (typeof AppTime !== 'undefined' && AppTime.toISODate) {
      const parsed = AppTime.toISODate(value);
      if (parsed) return parsed;
    }
    return value;
  }

  function createDecisionItem(raw = {}) {
    return {
      id: raw.id || uid('decision-item'),
      outcome: normalizeOutcome(raw.outcome || raw.title || raw.decision || raw.selected || ''),
      options: Array.isArray(raw.options) ? raw.options.map(opt => (opt || '').toString()) : [],
      rationale: raw.rationale || raw.reason || '',
      risks: raw.risks || raw.compromise || '',
      approver: raw.approver || raw.owner || '',
      at: raw.at || raw.date || ''
    };
  }

  function normalizeBlock(block) {
    const normalized = { ...block };
    normalized.type = 'decisions';
    normalized.id = normalized.id || uid('decision');
    if (!Array.isArray(normalized.items) || !normalized.items.length) {
      normalized.items = [createDecisionItem()];
    } else {
      normalized.items = normalized.items.map(createDecisionItem);
    }
    return normalized;
  }

  function createBlockTemplate() {
    return {
      id: uid('decisions'),
      type: 'decisions',
      items: [createDecisionItem()]
    };
  }

  function render(block, target, ctx) {
    const items = Array.isArray(block.items) ? block.items : [];
    const wrapper = document.createElement('div');
    wrapper.className = 'decision-cards';
    items.forEach(item => {
      wrapper.appendChild(renderDecisionCard(block, item, ctx));
    });
    const add = document.createElement('button');
    add.type = 'button';
    add.className = 'btn ghost stage-row-add';
    add.textContent = t('incidents.stage.blocks.decisions.add');
    add.addEventListener('click', () => {
      block.items.push(createDecisionItem());
      notifyChange(ctx, true);
    });
    target.appendChild(wrapper);
    target.appendChild(add);
  }

  function renderDecisionCard(block, item, ctx) {
    const card = document.createElement('div');
    card.className = 'decision-card';
    card.dataset.outcome = normalizeOutcome(item.outcome);
    const header = document.createElement('div');
    header.className = 'decision-card-header';
    const titleLabel = document.createElement('label');
    titleLabel.className = 'decision-field';
    titleLabel.innerHTML = `<span class="stage-field-label">${t('incidents.stage.blocks.decisions.outcome')}</span>`;
    titleLabel.appendChild(renderOutcomeSelect(item, ctx));
    header.appendChild(titleLabel);
    const remove = document.createElement('button');
    remove.type = 'button';
    remove.className = 'btn ghost icon-btn decision-remove';
    remove.textContent = '-';
    remove.title = t('common.remove') || 'Remove';
    remove.addEventListener('click', () => {
      block.items = block.items.filter(it => it.id !== item.id);
      if (!block.items.length) {
        block.items.push(createDecisionItem());
      }
      notifyChange(ctx, true);
    });
    header.appendChild(remove);
    card.appendChild(header);

    const row = document.createElement('div');
    row.className = 'decision-row';
    row.appendChild(renderApproverField(item, ctx));
    row.appendChild(renderDateTimeField(item, ctx));
    card.appendChild(row);

    card.appendChild(renderOptions(item, ctx));
    card.appendChild(renderTextArea(item, 'rationale', 'incidents.stage.blocks.decisions.rationale', 'incidents.stage.blocks.decisions.rationalePlaceholder', ctx));
    card.appendChild(renderTextArea(item, 'risks', 'incidents.stage.blocks.decisions.risks', 'incidents.stage.blocks.decisions.risksPlaceholder', ctx));

    return card;
  }

  function renderOutcomeSelect(item, ctx) {
    const select = document.createElement('select');
    select.className = 'select';
    OUTCOMES.forEach(outcome => {
      const opt = document.createElement('option');
      opt.value = outcome;
      opt.textContent = t(`incidents.stage.blocks.decisions.outcome.${outcome}`);
      select.appendChild(opt);
    });
    select.value = normalizeOutcome(item.outcome);
    select.addEventListener('change', () => {
      item.outcome = normalizeOutcome(select.value);
      const card = select.closest('.decision-card');
      if (card) card.dataset.outcome = item.outcome;
      notifyChange(ctx);
    });
    return select;
  }

  function renderApproverField(item, ctx) {
    const field = document.createElement('label');
    field.className = 'decision-field';
    const caption = document.createElement('span');
    caption.className = 'stage-field-label';
    caption.textContent = t('incidents.stage.blocks.decisions.by');
    field.appendChild(caption);
    const select = document.createElement('select');
    select.className = 'select';
    if (populateUserSelect) {
      populateUserSelect(select, item.approver ? [item.approver] : []);
    }
    if (IncidentsPage.enforceSingleSelect) {
      IncidentsPage.enforceSingleSelect(select);
    }
    const sync = () => {
      if (typeof getSelectedValues === 'function') {
        item.approver = getSelectedValues(select)[0] || '';
      } else {
        item.approver = select.value || '';
      }
      notifyChange(ctx);
    };
    select.addEventListener('change', sync);
    select.addEventListener('selectionrefresh', sync);
    field.appendChild(select);
    return field;
  }

  function renderDateTimeField(item, ctx) {
    const field = document.createElement('label');
    field.className = 'decision-field';
    const caption = document.createElement('span');
    caption.className = 'stage-field-label';
    caption.textContent = t('incidents.stage.blocks.decisions.date');
    field.appendChild(caption);
    field.appendChild(renderDateTimeInput(item.at || '', (val) => {
      item.at = val;
      notifyChange(ctx);
    }));
    return field;
  }

  function renderDateTimeInput(value, onChange) {
    const wrap = document.createElement('div');
    wrap.className = 'datetime-field';
    const { date, time } = typeof splitISODateTime === 'function' ? splitISODateTime(value) : { date: '', time: '' };
    const dateInput = document.createElement('input');
    dateInput.type = 'date';
    dateInput.className = 'input date-input';
    dateInput.lang = (typeof BerkutI18n !== 'undefined' && BerkutI18n.currentLang && BerkutI18n.currentLang() === 'en') ? 'en' : 'ru';
    dateInput.value = toISODateValue(date);
    const timeInput = document.createElement('input');
    timeInput.type = 'time';
    timeInput.step = '60';
    timeInput.className = 'input time-input';
    timeInput.value = time;
    const sync = () => {
      const combined = typeof toISODateTime === 'function' ? toISODateTime(dateInput.value, timeInput.value) : `${dateInput.value}T${timeInput.value}`;
      onChange(combined);
    };
    const normalizeTime = () => {
      const raw = (timeInput.value || '').trim();
      if (!raw) {
        sync();
        return;
      }
      const parts = raw.split(':').map(p => p.trim());
      const hour = parseInt(parts[0] || '0', 10);
      const minute = parseInt(parts[1] || '0', 10);
      if ([hour, minute].some(Number.isNaN)) return;
      if (hour < 0 || hour > 23 || minute < 0 || minute > 59) return;
      const pad = (num) => `${num}`.padStart(2, '0');
      timeInput.value = `${pad(hour)}:${pad(minute)}`;
      sync();
    };
    dateInput.addEventListener('change', sync);
    dateInput.addEventListener('input', sync);
    timeInput.addEventListener('change', sync);
    timeInput.addEventListener('input', sync);
    timeInput.addEventListener('blur', normalizeTime);
    wrap.appendChild(dateInput);
    wrap.appendChild(timeInput);
    return wrap;
  }

  function renderTextArea(item, key, labelKey, placeholderKey, ctx) {
    const field = document.createElement('label');
    field.className = 'decision-field wide';
    const caption = document.createElement('span');
    caption.className = 'stage-field-label';
    caption.textContent = t(labelKey);
    field.appendChild(caption);
    const input = document.createElement('textarea');
    input.className = 'textarea';
    input.value = item[key] || '';
    input.placeholder = t(placeholderKey);
    input.addEventListener('input', (e) => {
      item[key] = e.target.value;
      notifyChange(ctx);
    });
    field.appendChild(input);
    return field;
  }

  function renderOptions(item, ctx) {
    const wrap = document.createElement('div');
    wrap.className = 'decision-options';
    const header = document.createElement('div');
    header.className = 'decision-options-header';
    const label = document.createElement('span');
    label.className = 'stage-field-label';
    label.textContent = t('incidents.stage.blocks.decisions.options');
    header.appendChild(label);
    wrap.appendChild(header);
    const options = Array.isArray(item.options) ? item.options : [];
    const list = document.createElement('div');
    list.className = 'decision-options-list';
    options.forEach((opt, idx) => {
      const row = document.createElement('div');
      row.className = 'decision-option-row';
      const input = document.createElement('input');
      input.className = 'input';
      input.value = opt || '';
      input.placeholder = t('incidents.stage.blocks.decisions.optionPlaceholder');
      input.addEventListener('input', (e) => {
        item.options[idx] = e.target.value;
        notifyChange(ctx);
      });
      row.appendChild(input);
      const remove = document.createElement('button');
      remove.type = 'button';
      remove.className = 'btn ghost icon-btn decision-option-remove';
      remove.textContent = '-';
      remove.title = t('common.remove') || 'Remove';
      remove.addEventListener('click', () => {
        item.options.splice(idx, 1);
        notifyChange(ctx, true);
      });
      row.appendChild(remove);
      list.appendChild(row);
    });
    const add = document.createElement('button');
    add.type = 'button';
    add.className = 'btn ghost decision-option-add';
    add.textContent = t('incidents.stage.blocks.decisions.addOption');
    add.addEventListener('click', () => {
      item.options.push('');
      notifyChange(ctx, true);
    });
    wrap.appendChild(list);
    wrap.appendChild(add);
    return wrap;
  }

  function notifyChange(ctx, rerender = false) {
    if (typeof ctx?.onChange === 'function') ctx.onChange();
    if (rerender && typeof ctx?.rerender === 'function') ctx.rerender();
  }

  IncidentsPage.DecisionBlock = {
    createBlockTemplate,
    normalizeBlock,
    render
  };
})();
