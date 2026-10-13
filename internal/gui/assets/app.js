// Hatrack Desktop GUI App Frontend Logic

// hatrack opens this page with #t=<token>; every API call must send it back
const token = new URLSearchParams(location.hash.slice(1)).get('t') || '';
history.replaceState(null, '', location.pathname);

function api(path, body) {
  const opts = { headers: { 'X-Hatrack-Token': token } };
  if (body !== undefined) {
    opts.method = 'POST';
    opts.headers['Content-Type'] = 'application/json';
    opts.body = JSON.stringify(body);
  }
  return fetch(path, opts);
}

let expired = false;
let profilesState = [];
let activeProfile = null;

document.addEventListener('DOMContentLoaded', () => {
  initEventListeners();
  fetchProfiles();
  // Auto refresh every 5 seconds
  setInterval(fetchProfiles, 5000);
});

function initEventListeners() {
  document.getElementById('btnRefresh').addEventListener('click', () => {
    fetchProfiles(true);
  });

  document.getElementById('btnSaveCurrent').addEventListener('click', handleSaveCurrent);

  document.getElementById('btnAddAccount').addEventListener('click', () => {
    openModal('modalAdd');
  });

  document.getElementById('btnConfirmAdd').addEventListener('click', handleAddAccount);

  document.getElementById('btnConfirmRename').addEventListener('click', handleRenameProfile);

  document.getElementById('btnConfirmDelete').addEventListener('click', handleDeleteProfile);

  document.getElementById('searchInput').addEventListener('input', (e) => {
    renderProfiles(e.target.value);
  });

  // Close modals on clicking backdrop or close buttons
  document.querySelectorAll('[data-close]').forEach(btn => {
    btn.addEventListener('click', (e) => {
      const modalId = e.target.getAttribute('data-close');
      closeModal(modalId);
    });
  });
}

async function fetchProfiles(showToastOnSuccess = false) {
  try {
    const res = await api('/api/profiles');
    if (res.status === 403) {
      if (!expired) showToast('This window has expired. Open the dashboard again from the menu bar.', 'error');
      expired = true;
      return;
    }
    const data = await res.json();
    if (data.status === 'ok') {
      profilesState = data.profiles || [];
      activeProfile = profilesState.find(p => p.Active) || null;
      renderActiveHero();
      renderProfiles(document.getElementById('searchInput').value);
      if (showToastOnSuccess) {
        showToast('Accounts refreshed', 'success');
      }
    } else {
      showToast(data.error || 'Failed to fetch accounts', 'error');
    }
  } catch (err) {
    console.error(err);
  }
}

function renderActiveHero() {
  const heroEl = document.getElementById('activeHero');
  if (!activeProfile) {
    heroEl.classList.add('hidden');
    return;
  }
  heroEl.classList.remove('hidden');
  document.getElementById('activeProfileName').textContent = activeProfile.Name;
  document.getElementById('activeProfileEmail').textContent = activeProfile.Email;
  document.getElementById('activeProfileOrg').textContent = activeProfile.Org || 'Personal Org';
}

function renderProfiles(filterText = '') {
  const grid = document.getElementById('profilesGrid');
  const countEl = document.getElementById('accountCount');
  
  const query = filterText.trim().toLowerCase();
  const filtered = profilesState.filter(p => {
    return p.Name.toLowerCase().includes(query) ||
           (p.Email && p.Email.toLowerCase().includes(query)) ||
           (p.Org && p.Org.toLowerCase().includes(query));
  });

  countEl.textContent = filtered.length;

  if (filtered.length === 0) {
    grid.innerHTML = `
      <div class="loading-state">
        <p style="font-size:1.1rem; margin-bottom:8px;">No matching profiles found</p>
        <p style="font-size:0.85rem; color:var(--text-dim);">Click "+ Add Account" to sign in a new Claude account.</p>
      </div>
    `;
    return;
  }

  grid.innerHTML = filtered.map(p => `
    <div class="profile-card ${p.Active ? 'is-active' : ''}">
      <div class="card-top">
        <div class="profile-avatar">
          ${p.Active ? '⚡' : '👤'}
        </div>
        <div class="card-badges">
          ${p.Active ? '<span class="active-tag">Active</span>' : ''}
        </div>
      </div>

      <div class="card-info">
        <div class="profile-name">${escapeHtml(p.Name)}</div>
        <div class="profile-email">${escapeHtml(p.Email || 'No email')}</div>
        <div class="profile-org">${escapeHtml(p.Org || 'Personal')}</div>
      </div>

      <div class="card-bottom">
        ${p.Active ? `
          <button class="btn btn-secondary btn-switch" disabled>
            <span>✓ Active Account</span>
          </button>
        ` : `
          <button class="btn btn-primary btn-switch" onclick="switchAccount('${escapeHtml(p.Name)}')">
            <span>⚡ Switch to Account</span>
          </button>
        `}
        <button class="btn-action-icon" title="Rename Alias" onclick="openRenameModal('${escapeHtml(p.Name)}')">
          ✏️
        </button>
        <button class="btn-action-icon danger" title="Delete Profile" onclick="openDeleteModal('${escapeHtml(p.Name)}')">
          🗑️
        </button>
      </div>
    </div>
  `).join('');
}

async function switchAccount(name) {
  try {
    showToast(`Switching to ${name}...`, 'info');
    const res = await api('/api/use', { name });
    const data = await res.json();
    if (data.status === 'ok') {
      showToast(`Switched active account to ${name}`, 'success');
      await fetchProfiles();
    } else {
      showToast(`Failed: ${data.error}`, 'error');
    }
  } catch (err) {
    showToast(`Error: ${err.message}`, 'error');
  }
}

async function handleSaveCurrent() {
  try {
    showToast('Saving current Claude session...', 'info');
    const res = await api('/api/save', {});
    const data = await res.json();
    if (data.status === 'ok') {
      showToast(`Saved current session as profile "${data.name}"`, 'success');
      await fetchProfiles();
    } else {
      showToast(`Failed: ${data.error}`, 'error');
    }
  } catch (err) {
    showToast(`Error: ${err.message}`, 'error');
  }
}

async function handleAddAccount() {
  const alias = document.getElementById('newProfileAlias').value.trim();
  const statusContainer = document.getElementById('addStatusContainer');
  const statusText = document.getElementById('addStatusText');
  const btnConfirm = document.getElementById('btnConfirmAdd');

  statusContainer.classList.remove('hidden');
  statusText.textContent = 'Opening Terminal...';
  btnConfirm.disabled = true;

  try {
    const res = await api('/api/add', { name: alias });
    const data = await res.json();
    if (data.status === 'ok') {
      showToast('Finish signing in in the Terminal window. The new account shows up here when it is done.', 'success');
      closeModal('modalAdd');
      document.getElementById('newProfileAlias').value = '';
      await fetchProfiles();
    } else {
      showToast(`Add failed: ${data.error}`, 'error');
    }
  } catch (err) {
    showToast(`Error: ${err.message}`, 'error');
  } finally {
    statusContainer.classList.add('hidden');
    btnConfirm.disabled = false;
  }
}

function openRenameModal(oldName) {
  document.getElementById('renameOldName').value = oldName;
  document.getElementById('renameNewName').value = oldName;
  openModal('modalRename');
}

async function handleRenameProfile() {
  const oldName = document.getElementById('renameOldName').value;
  const newName = document.getElementById('renameNewName').value.trim();

  if (!newName) {
    showToast('Profile name cannot be empty', 'error');
    return;
  }

  try {
    const res = await api('/api/rename', { oldName, newName });
    const data = await res.json();
    if (data.status === 'ok') {
      showToast(`Profile renamed to ${newName}`, 'success');
      closeModal('modalRename');
      await fetchProfiles();
    } else {
      showToast(`Rename failed: ${data.error}`, 'error');
    }
  } catch (err) {
    showToast(`Error: ${err.message}`, 'error');
  }
}

function openDeleteModal(name) {
  document.getElementById('deleteTargetName').value = name;
  document.getElementById('deleteTargetLabel').textContent = name;
  openModal('modalDelete');
}

async function handleDeleteProfile() {
  const name = document.getElementById('deleteTargetName').value;
  try {
    const res = await api('/api/remove', { name });
    const data = await res.json();
    if (data.status === 'ok') {
      showToast(`Removed profile ${name}`, 'success');
      closeModal('modalDelete');
      await fetchProfiles();
    } else {
      showToast(`Delete failed: ${data.error}`, 'error');
    }
  } catch (err) {
    showToast(`Error: ${err.message}`, 'error');
  }
}

function openModal(id) {
  document.getElementById(id).classList.remove('hidden');
}

function closeModal(id) {
  document.getElementById(id).classList.add('hidden');
}

function showToast(message, type = 'info') {
  const container = document.getElementById('toastContainer');
  const toast = document.createElement('div');
  toast.className = `toast ${type}`;
  toast.innerHTML = `
    <span>${type === 'success' ? '✅' : type === 'error' ? '❌' : 'ℹ️'}</span>
    <span>${escapeHtml(message)}</span>
  `;
  container.appendChild(toast);
  setTimeout(() => {
    toast.style.opacity = '0';
    toast.style.transform = 'translateX(50px)';
    setTimeout(() => toast.remove(), 300);
  }, 4000);
}

function escapeHtml(str) {
  if (!str) return '';
  return str.replace(/[&<>"']/g, m => ({
    '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#039;'
  })[m]);
}
